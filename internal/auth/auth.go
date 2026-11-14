// Package auth 登录认证、会话管理、安全入口与防爆破锁定。
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	CookieSession  = "wm_session"
	CookieEntrance = "wm_entrance"
)

// 会话客户端类型（docs/APP.md §4.2）：web 走 Cookie 24h；app 走 Bearer 30 天滑动续期。
const (
	SessionClientWeb = "web"
	SessionClientApp = "app"

	appSessionTTL    = 30 * 24 * time.Hour // APP 会话有效期
	appRenewBelow    = 15 * 24 * time.Hour // 剩余寿命低于此值时自动续期
)

// Role 面板角色：admin 全权限；operator 可操作全部功能（无用户管理）；viewer 只读。
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

var validRoles = map[Role]bool{RoleAdmin: true, RoleOperator: true, RoleViewer: true}

func ValidRole(r string) bool { return validRoles[Role(r)] }

type Service struct {
	db    *gorm.DB
	cfg   *config.Config
	mu    sync.Mutex
	fails map[string][]time.Time
	qr    *qrStore
}

func New(db *gorm.DB, cfg *config.Config) *Service {
	return &Service{db: db, cfg: cfg, fails: map[string][]time.Time{}, qr: newQRStore()}
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- 防爆破：15 分钟内失败 5 次锁定该 IP ----

func (s *Service) locked(ip string) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var keep []time.Time
	for _, t := range s.fails[ip] {
		if now.Sub(t) < 15*time.Minute {
			keep = append(keep, t)
		}
	}
	s.fails[ip] = keep
	if len(keep) >= 5 {
		return true, 15*time.Minute - now.Sub(keep[0])
	}
	return false, 0
}

func (s *Service) recordFail(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[ip] = append(s.fails[ip], time.Now())
}

func (s *Service) clearFails(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fails, ip)
}

// ---- 路由 ----

// Register 注册认证路由；/auth/login 在安全入口校验之后匿名可访问，其余需会话。
func (s *Service) Register(api gin.IRouter) {
	api.POST("/auth/login", s.login)
	// 扫码登录（docs/APP.md §5）：create/state/exchange 匿名（安全入口之后）；
	// confirm/approve 需 APP 会话；/app/check-update 供 APP 自更新检查。
	s.qrStartJanitor()
	api.POST("/auth/qr/create", s.qrCreate)
	api.GET("/auth/qr/state", s.qrState)
	api.POST("/auth/qr/exchange", s.qrExchange)
	api.GET("/app/check-update", s.appCheckUpdate)
	authed := api.Group("", s.Middleware())
	authed.POST("/auth/logout", s.logout)
	authed.GET("/auth/me", s.me)
	authed.POST("/setting/2fa/setup", s.twoFASetup)
	authed.POST("/setting/2fa/enable", s.twoFAEnable)
	authed.POST("/setting/2fa/disable", s.twoFADisable)
	authed.GET("/sessions", s.listSessions)
	authed.GET("/auth/sessions", s.listSessions) // 别名：与前端 Setting.vue 一致
	authed.DELETE("/sessions/:id", s.revokeSession)
	authed.DELETE("/auth/sessions/:id", s.revokeSession)
	authed.POST("/sessions/revoke-others", s.revokeOtherSessions)
	authed.POST("/auth/sessions/revoke-others", s.revokeOtherSessions) // 别名
	authed.GET("/auth/app-bind", s.appBindInfo)
	authed.GET("/auth/bind/status", s.bindStatus)
	authed.POST("/auth/bind/generate", s.bindGenerate)
	authed.POST("/auth/bind/unbind", s.bindUnbind)
	api.POST("/auth/bind/login", s.bindLogin)
	authed.POST("/auth/qr/confirm", s.qrConfirm)
	authed.POST("/auth/qr/approve", s.qrApprove)
}

func (s *Service) login(c *gin.Context) {
	var req struct {
		Username   string `binding:"required"`
		Password   string `binding:"required"`
		Code       string `json:"code"`       // 两因素验证码（启用 TOTP 时必填）
		Client     string `json:"client"`     // 缺省 web；app = 移动 APP 登录（30 天会话 + 响应返回 token）
		DeviceName string `json:"device_name"` // APP 设备名（机型），仅 client=app 时落库
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	client := SessionClientWeb
	if req.Client == SessionClientApp {
		client = SessionClientApp
	}
	ip := c.ClientIP()
	if ok, wait := s.locked(ip); ok {
		core.Err(c, 429, fmt.Sprintf("登录失败次数过多，请 %d 分钟后再试", int(wait.Minutes())+1))
		return
	}
	var u core.User
	if err := s.db.Where("username = ?", req.Username).First(&u).Error; err != nil ||
		bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
		s.recordFail(ip)
		slog.Warn("登录失败", "username", req.Username, "ip", ip)
		core.Err(c, 401, "用户名或密码错误")
		return
	}
	if u.Disabled {
		slog.Warn("已禁用账号尝试登录", "username", req.Username, "ip", ip)
		core.Err(c, 403, "账号已被禁用，请联系管理员")
		return
	}
	if u.TOTPSecret != "" {
		if req.Code == "" {
			core.OK(c, gin.H{"need_2fa": true})
			return
		}
		if !totpVerify(u.TOTPSecret, req.Code) {
			s.recordFail(ip)
			core.Err(c, 401, "两因素验证码错误")
			return
		}
	}
	s.clearFails(ip)
	token := randomToken(32)
	ttl := 24 * time.Hour
	if client == SessionClientApp {
		ttl = appSessionTTL
	}
	device := strings.TrimSpace(req.DeviceName)
	if len(device) > 128 {
		device = device[:128]
	}
	sess := core.Session{Token: token, UserID: u.ID, IP: ip, UA: c.Request.UserAgent(),
		Client: client, DeviceName: device, ExpiresAt: time.Now().Add(ttl)}
	if err := s.db.Create(&sess).Error; err != nil {
		core.Err(c, 500, "创建会话失败")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: CookieSession, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds()),
	})
	slog.Info("登录成功", "username", u.Username, "role", u.Role, "ip", ip, "client", client)
	out := gin.H{"username": u.Username, "role": u.Role, "client": client}
	if client == SessionClientApp {
		// APP 会话：token 进响应体供 Bearer 携带（web 保持不下发 token 的既有行为）
		out["token"] = token
		out["expires_at"] = sess.ExpiresAt.Unix()
	}
	core.OK(c, out)
}

func (s *Service) logout(c *gin.Context) {
	if tok := c.GetString("session"); tok != "" {
		s.db.Where("token = ?", tok).Delete(&core.Session{})
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: CookieSession, Value: "", Path: "/", MaxAge: -1})
	core.OK(c, nil)
}

func (s *Service) me(c *gin.Context) {
	core.OK(c, gin.H{"username": c.GetString("user"), "role": c.GetString("role")})
}

// ---- 中间件 ----

// apiTokenFrom 提取开放 API Token（Authorization: Bearer xxx 或 X-Api-Token）。
// Token 记录与 scope 校验见 apitoken.go。
func apiTokenFrom(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return strings.TrimSpace(c.GetHeader("X-Api-Token"))
}

func (s *Service) currentSession(c *gin.Context) (*core.Session, bool) {
	tok, err := c.Cookie(CookieSession)
	if err != nil || tok == "" {
		return nil, false
	}
	var sess core.Session
	if err := s.db.Where("token = ?", tok).First(&sess).Error; err != nil {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.db.Delete(&sess)
		return nil, false
	}
	return &sess, true
}

// bearerSession Bearer 令牌按 APP 会话匹配（仅 client=app；web 会话只走 Cookie）。
// 命中后缓存进 gin context，同一请求的入口中间件与会话中间件不再重复查库。
func (s *Service) bearerSession(c *gin.Context, token string) (*core.Session, bool) {
	if v, ok := c.Get("bearer_session"); ok {
		// 缓存值可能是类型化 nil（未命中时写入），断言成功也必须判空
		if sess, ok := v.(*core.Session); ok && sess != nil {
			return sess, true
		}
		return nil, false
	}
	var sess core.Session
	if err := s.db.Where("token = ? AND client = ?", token, SessionClientApp).First(&sess).Error; err != nil {
		c.Set("bearer_session", (*core.Session)(nil))
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.db.Delete(&sess)
		c.Set("bearer_session", (*core.Session)(nil))
		return nil, false
	}
	c.Set("bearer_session", &sess)
	return &sess, true
}

// applySession 校验用户并注入上下文、APP 会话滑动续期、viewer 只读拦截。
func (s *Service) applySession(c *gin.Context, sess *core.Session) bool {
	var u core.User
	if err := s.db.First(&u, sess.UserID).Error; err != nil {
		core.Unauthorized(c)
		return false
	}
	if u.Disabled {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "账号已被禁用"})
		return false
	}
	c.Set("user", u.Username)
	c.Set("uid", u.ID)
	c.Set("role", u.Role)
	c.Set("session", sess.Token)

	// APP 会话滑动续期：剩余寿命 <15 天自动续至 30 天（仅跨过阈值才写库）
	if sess.Client == SessionClientApp && time.Until(sess.ExpiresAt) < appRenewBelow {
		if err := s.db.Model(&core.Session{}).Where("token = ?", sess.Token).
			Update("expires_at", time.Now().Add(appSessionTTL)).Error; err != nil {
			slog.Warn("APP 会话续期失败", "err", err)
		}
	}

	// viewer 只读：拦截写方法（退出登录与扫码确认/授权为认证动作，一并豁免）
	if Role(u.Role) == RoleViewer {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			switch c.Request.URL.Path {
			case "/api/auth/logout", "/api/auth/qr/confirm", "/api/auth/qr/approve":
			default:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "viewer 为只读角色，无权执行此操作"})
				return false
			}
		}
	}
	c.Next()
	return true
}

// Middleware API 鉴权：校验会话、注入 user/uid/role/session，并拦截 viewer 的写操作。
// Bearer 令牌两级匹配：先开放 API Token（等效 admin），未命中再按 APP 会话（docs/APP.md §4.2）。
func (s *Service) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开放 API Token：等效 admin 会话 + scope 权限校验（外部脚本 / MCP 客户端）
		if bearer := apiTokenFrom(c); bearer != "" {
			// 第一级：开放 API Token（等效 admin，按自身 scope 授权；不可调用 MCP 端点）
			if rec := s.APITokenRec(); rec != nil && s.apiTokenMatch(c, rec) {
				if !s.apiTokenDenied(c, rec) {
					c.Set("user", "api")
					c.Set("uid", uint(0))
					c.Set("role", string(RoleAdmin))
					c.Next()
					return
				}
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "API Token 权限不足（模块未授权、只读模式，或该端点不开放给 API Token）"})
				return
			}
			// 第二级：MCP Token——仅 MCP 端点与本机回环（MCP 工具执行）放行，权限独立于 API Token
			if mrec := s.MCPTokenRec(); mrec != nil && s.apiTokenMatch(c, mrec) {
				path := c.Request.URL.Path
				mcpEndpoint := path == "/api/mcp" || strings.HasPrefix(path, "/api/mcp/")
				if mcpEndpoint && !s.mcpTokenDenied(c, mrec) {
					c.Set("user", "mcp")
					c.Set("uid", uint(0))
					c.Set("role", string(RoleAdmin))
					c.Next()
					return
				}
				// 工具内部调用必须同时满足：本机回环 + MCP Server 专属标记（外部即使从本机发起也不带此标记）
				if !mcpEndpoint && isLoopback(c) && c.GetHeader("X-MCP-Loopback") == "1" && !s.mcpTokenDenied(c, mrec) {
					c.Set("user", "mcp")
					c.Set("uid", uint(0))
					c.Set("role", string(RoleAdmin))
					c.Next()
					return
				}
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "MCP Token 仅可用于 MCP 端点（/api/mcp）；开放 API 请使用 API Token"})
				return
			}
			// 第三级：APP 会话令牌（docs/APP.md §4.2）
			if sess, ok := s.bearerSession(c, bearer); ok {
				s.applySession(c, sess)
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "API Token 无效或未启用"})
			return
		}
		sess, ok := s.currentSession(c)
		if !ok {
			core.Unauthorized(c)
			return
		}
		s.applySession(c, sess)
	}
}

// EntranceRequired API 入口防护：包括登录在内的所有 /api 请求必须先经过安全入口；
// 持有效开放 API Token 或 APP 会话令牌的客户端豁免入口 Cookie。
// 出示了 Bearer 凭证但验证失败的请求返回 401（客户端据此回到登录页），未出示凭证的仍 404 隐身。
func (s *Service) EntranceRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.APITokenValid(c) {
			c.Next()
			return
		}
		if bearer := apiTokenFrom(c); bearer != "" {
			if _, ok := s.bearerSession(c, bearer); ok {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "凭证无效或已过期，请重新登录"})
			return
		}
		if ck, err := c.Cookie(CookieEntrance); err == nil && ck == s.cfg.Server.Entrance {
			c.Next()
			return
		}
		c.String(http.StatusNotFound, "404 page not found")
		c.Abort()
	}
}

// HandleEntrance 处理 /s/:code：入口串正确时写入口 Cookie 并跳转首页，否则 404。
func (s *Service) HandleEntrance(c *gin.Context) {
	if c.Param("code") != s.cfg.Server.Entrance {
		c.String(http.StatusNotFound, "404 page not found")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: CookieEntrance, Value: s.cfg.Server.Entrance, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 30 * 86400,
	})
	c.Redirect(http.StatusFound, "/")
}

// SpaAllowed SPA 页面防护：有效会话或入口 Cookie 才可见面板界面，否则 404。
func (s *Service) SpaAllowed(c *gin.Context) bool {
	if _, ok := s.currentSession(c); ok {
		return true
	}
	ck, err := c.Cookie(CookieEntrance)
	return err == nil && ck == s.cfg.Server.Entrance
}

// ---- TOTP（RFC 6238，SHA-1 / 6 位 / 30s 窗口，容差 ±1）----

func totpCode(secret string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / 30)
	var buf [8]byte
	for i := 7; i >= 0; i-- {
		buf[i] = byte(counter)
		counter >>= 8
	}
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint64(sum[off])&0x7f)<<24 | uint64(sum[off+1])<<16 | uint64(sum[off+2])<<8 | uint64(sum[off+3])
	return fmt.Sprintf("%06d", code%1000000), nil
}

func totpVerify(secret, code string) bool {
	now := time.Now()
	for _, d := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		got, err := totpCode(secret, now.Add(d))
		if err == nil && subtle.ConstantTimeCompare([]byte(got), []byte(strings.TrimSpace(code))) == 1 {
			return true
		}
	}
	return false
}

// totpGenerate 随机 20 字节 base32 secret。
func totpGenerate() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// twoFASetup 生成待绑定的 TOTP secret 与 otpauth URL（不落库，enable 时才保存）。
func (s *Service) twoFASetup(c *gin.Context) {
	uid := c.GetUint("uid")
	var u core.User
	if err := s.db.First(&u, uid).Error; err != nil {
		core.Err(c, 404, "用户不存在")
		return
	}
	secret, err := totpGenerate()
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	url := fmt.Sprintf("otpauth://totp/hanyuxin:%s?secret=%s&issuer=hanyuxin", u.Username, secret)
	core.OK(c, gin.H{"secret": secret, "url": url})
}

// twoFAEnable 校验验证码后启用 2FA。
func (s *Service) twoFAEnable(c *gin.Context) {
	var req struct {
		Secret string `binding:"required"`
		Code   string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !totpVerify(req.Secret, req.Code) {
		core.Err(c, 400, "验证码错误，请核对时间与密钥")
		return
	}
	uid := c.GetUint("uid")
	if err := s.db.Model(&core.User{}).Where("id = ?", uid).Update("totp_secret", req.Secret).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"ok": true})
}

// twoFADisable 关闭 2FA（需密码确认）。
func (s *Service) twoFADisable(c *gin.Context) {
	var req struct {
		Password string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	uid := c.GetUint("uid")
	var u core.User
	if err := s.db.First(&u, uid).Error; err != nil {
		core.Err(c, 404, "用户不存在")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
		core.Err(c, 400, "密码错误")
		return
	}
	if err := s.db.Model(&u).Update("totp_secret", "").Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"ok": true})
}

// listSessions 当前用户的在线会话（其他设备登录）。
func (s *Service) listSessions(c *gin.Context) {
	uid := c.GetUint("uid")
	cur := c.GetString("session")
	var rows []core.Session
	s.db.Where("user_id = ? AND expires_at > ?", uid, time.Now()).Order("expires_at DESC").Find(&rows)
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		client := r.Client
		if client == "" {
			client = SessionClientWeb // 存量数据缺省视为 web
		}
		items = append(items, gin.H{
			"id":           r.Token,
			"ip":           r.IP,
			"ua":           r.UA,
			"client":       client,
			"device_name":  r.DeviceName,
			"current":      r.Token == cur,
			"expires":      r.ExpiresAt.Format("2006-01-02 15:04:05"),
		})
	}
	core.OK(c, items)
}

// revokeSession 踢出指定会话。
func (s *Service) revokeSession(c *gin.Context) {
	uid := c.GetUint("uid")
	id := c.Param("id")
	res := s.db.Where("token = ? AND user_id = ?", id, uid).Delete(&core.Session{})
	if res.Error != nil {
		core.Err(c, 500, res.Error.Error())
		return
	}
	core.OK(c, gin.H{"revoked": res.RowsAffected})
}

// revokeOtherSessions 一键退出除当前设备外的全部会话。
func (s *Service) revokeOtherSessions(c *gin.Context) {
	uid := c.GetUint("uid")
	cur := c.GetString("session")
	res := s.db.Where("user_id = ? AND token <> ? AND expires_at > ?", uid, cur, time.Now()).Delete(&core.Session{})
	if res.Error != nil {
		core.Err(c, 500, res.Error.Error())
		return
	}
	slog.Info("一键退出其他设备", "uid", uid, "revoked", res.RowsAffected, "ip", c.ClientIP())
	core.OK(c, gin.H{"revoked": res.RowsAffected})
}
