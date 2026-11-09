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
}

func New(db *gorm.DB, cfg *config.Config) *Service {
	return &Service{db: db, cfg: cfg, fails: map[string][]time.Time{}}
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
	authed := api.Group("", s.Middleware())
	authed.POST("/auth/logout", s.logout)
	authed.GET("/auth/me", s.me)
	authed.POST("/setting/2fa/setup", s.twoFASetup)
	authed.POST("/setting/2fa/enable", s.twoFAEnable)
	authed.POST("/setting/2fa/disable", s.twoFADisable)
	authed.GET("/sessions", s.listSessions)
	authed.DELETE("/sessions/:id", s.revokeSession)
}

func (s *Service) login(c *gin.Context) {
	var req struct {
		Username string `binding:"required"`
		Password string `binding:"required"`
		Code     string `json:"code"` // 两因素验证码（启用 TOTP 时必填）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
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
	sess := core.Session{Token: token, UserID: u.ID, IP: ip, UA: c.Request.UserAgent(), ExpiresAt: time.Now().Add(24 * time.Hour)}
	if err := s.db.Create(&sess).Error; err != nil {
		core.Err(c, 500, "创建会话失败")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: CookieSession, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 86400,
	})
	slog.Info("登录成功", "username", u.Username, "role", u.Role, "ip", ip)
	core.OK(c, gin.H{"username": u.Username, "role": u.Role})
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

// Middleware API 鉴权：校验会话、注入 user/uid/role/session，并拦截 viewer 的写操作。
func (s *Service) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, ok := s.currentSession(c)
		if !ok {
			core.Unauthorized(c)
			return
		}
		var u core.User
		if err := s.db.First(&u, sess.UserID).Error; err != nil {
			core.Unauthorized(c)
			return
		}
		if u.Disabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "账号已被禁用"})
			return
		}
		c.Set("user", u.Username)
		c.Set("uid", u.ID)
		c.Set("role", u.Role)
		c.Set("session", sess.Token)

		// viewer 只读：拦截写方法（退出登录豁免）
		if Role(u.Role) == RoleViewer {
			switch c.Request.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if c.Request.URL.Path != "/api/auth/logout" {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "viewer 为只读角色，无权执行此操作"})
					return
				}
			}
		}
		c.Next()
	}
}

// EntranceRequired API 入口防护：包括登录在内的所有 /api 请求必须先经过安全入口。
func (s *Service) EntranceRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		items = append(items, gin.H{
			"id":      r.Token,
			"ip":      r.IP,
			"ua":      r.UA,
			"current": r.Token == cur,
			"expires": r.ExpiresAt.Format("2006-01-02 15:04:05"),
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
