// Package auth 登录认证、会话管理、安全入口与防爆破锁定。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
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
}

func (s *Service) login(c *gin.Context) {
	var req struct {
		Username string `binding:"required"`
		Password string `binding:"required"`
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
	slog.Info("登录成功", "username", u.Username, "ip", ip)
	core.OK(c, gin.H{"username": u.Username})
}

func (s *Service) logout(c *gin.Context) {
	if tok := c.GetString("session"); tok != "" {
		s.db.Where("token = ?", tok).Delete(&core.Session{})
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: CookieSession, Value: "", Path: "/", MaxAge: -1})
	core.OK(c, nil)
}

func (s *Service) me(c *gin.Context) {
	core.OK(c, gin.H{"username": c.GetString("user")})
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

// Middleware API 鉴权：校验会话并注入 user/uid/session。
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
		c.Set("user", u.Username)
		c.Set("uid", u.ID)
		c.Set("session", sess.Token)
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
