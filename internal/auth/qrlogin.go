// qrlogin.go —— 扫码登录与 APP 更新检查（docs/APP.md §5）。
//
// 协议：Web 登录页调 create 拿 key 与 qr_text（含面板地址 + 入口码）展示二维码；
// APP 扫码后先 confirm（标记已扫）再由用户 approve（确认登录）；Web 轮询 state
// 得 confirmed + ticket，最后 exchange 兑换会话 Cookie。key 单次有效、120s 过期、
// 内存存储（面板重启即清空，Web 重新生成即可）；票据 60s 一次性。
package auth

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	qrTTL       = 120 * time.Second // 二维码有效期
	qrTicketTTL = 60 * time.Second  // 兑换票据有效期
)

// QR 状态机：pending → scanned → confirmed → used（expired 为查询时对失效态的呈现）。
const (
	qrPending   = "pending"
	qrScanned   = "scanned"
	qrConfirmed = "confirmed"
	qrUsed      = "used"
)

// qrRecord 一个扫码登录密钥的完整状态。
type qrRecord struct {
	Key       string
	State     string
	UID       uint // 发起 confirm/approve 的 APP 会话用户（approve 兜底写入）
	WebIP     string
	WebUA     string
	Ticket    string
	TicketAt  time.Time
	ExpiresAt time.Time
}

// qrStore 内存记录存储 + 匿名端点按 IP 限频（与登录防爆破隔离，避免互相牵连锁定）。
type qrStore struct {
	mu      sync.Mutex
	m       map[string]*qrRecord
	limiter map[string][]time.Time
	once    sync.Once
}

func newQRStore() *qrStore {
	return &qrStore{m: map[string]*qrRecord{}, limiter: map[string][]time.Time{}}
}

// qrStartJanitor 启动过期清扫（幂等，Register 时调用）。
func (s *Service) qrStartJanitor() {
	s.qr.once.Do(func() {
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for range t.C {
				s.qr.sweep()
			}
		}()
	})
}

func (q *qrStore) sweep() {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	for k, r := range q.m {
		if now.After(r.ExpiresAt) || (r.State == qrUsed) {
			delete(q.m, k)
		}
	}
	for ip, ts := range q.limiter {
		var keep []time.Time
		for _, t := range ts {
			if now.Sub(t) < 10*time.Minute {
				keep = append(keep, t)
			}
		}
		if len(keep) == 0 {
			delete(q.limiter, ip)
		} else {
			q.limiter[ip] = keep
		}
	}
}

// allow 滑动窗口限频：window 内最多 max 次。
func (q *qrStore) allow(key string, max int, window time.Duration) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	var keep []time.Time
	for _, t := range q.limiter[key] {
		if now.Sub(t) < window {
			keep = append(keep, t)
		}
	}
	if len(keep) >= max {
		q.limiter[key] = keep
		return false
	}
	q.limiter[key] = append(keep, now)
	return true
}

// get 取记录（过期的顺带删除并返回 nil）。
func (q *qrStore) get(key string) *qrRecord {
	q.mu.Lock()
	defer q.mu.Unlock()
	r, ok := q.m[key]
	if !ok {
		return nil
	}
	if time.Now().After(r.ExpiresAt) {
		delete(q.m, key)
		return nil
	}
	return r
}

func (q *qrStore) put(r *qrRecord) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.m[r.Key] = r
}

// mutate 在锁内修改记录（nil 时返回 false）。
func (q *qrStore) mutate(key string, fn func(r *qrRecord)) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	r, ok := q.m[key]
	if !ok || time.Now().After(r.ExpiresAt) {
		if ok {
			delete(q.m, key)
		}
		return false
	}
	fn(r)
	return true
}

// ---- 对外地址推导 ----

// publicHost 面板对外访问地址（qr_text 的 host）：设置项 app_public_url 优先，
// 其次 X-Forwarded-Proto/Host（域名反代场景），最后请求 Host + 配置 TLS 状态。
func (s *Service) publicHost(c *gin.Context) string {
	if u := strings.TrimSpace(core.GetSetting(s.db, core.KAppPublicURL)); u != "" {
		return strings.TrimRight(u, "/")
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		if c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + host
}

// qrV2Flag 二维码 v2 位：面板存在启用 2FA 的用户时为 1（APP 侧提示语参考，不作为流程依据）。
func (s *Service) qrV2Flag() int {
	var n int64
	s.db.Model(&core.User{}).Where("totp_secret <> ''").Count(&n)
	if n > 0 {
		return 1
	}
	return 0
}

func (s *Service) qrText(c *gin.Context, key string) string {
	return fmt.Sprintf("hanyuxin://login?v=1&host=%s&entrance=%s&key=%s&v2=%d",
		url.QueryEscape(s.publicHost(c)), url.QueryEscape(s.cfg.Server.Entrance), key, s.qrV2Flag())
}

// ---- 端点 ----

// qrCreate Web 登录页创建扫码记录（匿名可，入口校验之后；按 IP 限频 10 次/分钟）。
// appBindInfo 已登录 Web 端生成「绑定 APP」二维码信息（无 key：APP 扫码=添加服务器+账密登录，
// 与扫码登录码不同——绑定码不需要 Web 侧确认流程）。
func (s *Service) appBindInfo(c *gin.Context) {
	host := s.publicHost(c)
	qr := fmt.Sprintf("hanyuxin://login?v=1&host=%s&entrance=%s&v2=%d",
		url.QueryEscape(host), url.QueryEscape(s.cfg.Server.Entrance), s.qrV2Flag())
	core.OK(c, gin.H{
		"host":     host,
		"entrance": s.cfg.Server.Entrance,
		"v2":       s.qrV2Flag(),
		"qr_text":  qr,
	})
}

func (s *Service) qrCreate(c *gin.Context) {
	ip := c.ClientIP()
	if !s.qr.allow("create:"+ip, 10, time.Minute) {
		core.Err(c, 429, "请求过于频繁，请稍后再试")
		return
	}
	rec := &qrRecord{
		Key:       randomToken(16),
		State:     qrPending,
		WebIP:     ip,
		WebUA:     c.Request.UserAgent(),
		ExpiresAt: time.Now().Add(qrTTL),
	}
	var req struct {
		UA string `json:"ua"` // 可选：Web 显式传入自己的 UA（缺省取请求 UA）
	}
	if err := c.ShouldBindJSON(&req); err == nil && strings.TrimSpace(req.UA) != "" {
		rec.WebUA = strings.TrimSpace(req.UA)
	}
	s.qr.put(rec)
	core.OK(c, gin.H{"key": rec.Key, "qr_text": s.qrText(c, rec.Key), "expires_in": int(qrTTL.Seconds())})
}

// qrState Web 每 2s 轮询扫码状态（匿名可，入口校验之后；限频 120 次/分钟容纳多标签）。
func (s *Service) qrState(c *gin.Context) {
	if !s.qr.allow("state:"+c.ClientIP(), 120, time.Minute) {
		core.Err(c, 429, "请求过于频繁，请稍后再试")
		return
	}
	rec := s.qr.get(strings.TrimSpace(c.Query("key")))
	if rec == nil {
		core.OK(c, gin.H{"state": "expired"})
		return
	}
	state := rec.State
	if state == qrUsed {
		state = "expired" // 已完成兑换：对 Web 呈现过期，若仍在轮询则重新生成
	}
	out := gin.H{"state": state}
	if state == qrConfirmed {
		out["ticket"] = rec.Ticket
	}
	core.OK(c, out)
}

// qrConfirm APP 扫码后第一步「我扫了」（需 APP 会话）。
func (s *Service) qrConfirm(c *gin.Context) {
	var req struct {
		Key string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	key := strings.TrimSpace(req.Key)
	rec := s.qr.get(key)
	if rec == nil {
		core.Err(c, 404, "二维码已过期，请刷新后重试")
		return
	}
	if rec.State == qrUsed {
		core.Err(c, 400, "二维码已被使用")
		return
	}
	uid := c.GetUint("uid")
	ok := s.qr.mutate(key, func(r *qrRecord) {
		r.UID = uid
		if r.State == qrPending {
			r.State = qrScanned
		}
	})
	if !ok {
		core.Err(c, 404, "二维码已过期，请刷新后重试")
		return
	}
	slog.Info("扫码登录：APP 已扫码", "uid", uid, "ip", c.ClientIP())
	core.OK(c, gin.H{"ok": true})
}

// qrApprove APP 用户确认登录（需 APP 会话）：生成一次性票据，等 Web 来兑换。
func (s *Service) qrApprove(c *gin.Context) {
	var req struct {
		Key string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	key := strings.TrimSpace(req.Key)
	rec := s.qr.get(key)
	if rec == nil {
		core.Err(c, 404, "二维码已过期，请刷新后重试")
		return
	}
	if rec.State == qrUsed {
		core.Err(c, 400, "二维码已被使用")
		return
	}
	uid := c.GetUint("uid")
	ok := s.qr.mutate(key, func(r *qrRecord) {
		r.UID = uid // 未走 confirm 直接 approve 时兜底
		r.State = qrConfirmed
		r.Ticket = randomToken(32)
		r.TicketAt = time.Now()
	})
	if !ok {
		core.Err(c, 404, "二维码已过期，请刷新后重试")
		return
	}
	slog.Info("扫码登录：APP 已确认", "uid", uid, "ip", c.ClientIP(), "web_ip", rec.WebIP)
	core.OK(c, gin.H{"ok": true})
}

// qrExchange Web 拿 key + ticket 兑换会话 Cookie（匿名可，入口校验之后）。
// 会话按 create 时记录的 Web 侧 IP/UA 与 approve 用户的身份创建，client=web、24h。
func (s *Service) qrExchange(c *gin.Context) {
	if !s.qr.allow("exchange:"+c.ClientIP(), 20, time.Minute) {
		core.Err(c, 429, "请求过于频繁，请稍后再试")
		return
	}
	var req struct {
		Key    string `binding:"required"`
		Ticket string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	key := strings.TrimSpace(req.Key)
	rec := s.qr.get(key)
	if rec == nil || rec.State != qrConfirmed {
		core.Err(c, 400, "二维码状态无效或已过期")
		return
	}
	if rec.Ticket == "" || subtle.ConstantTimeCompare([]byte(rec.Ticket), []byte(strings.TrimSpace(req.Ticket))) != 1 {
		core.Err(c, 400, "兑换票据无效")
		return
	}
	if time.Since(rec.TicketAt) > qrTicketTTL {
		core.Err(c, 400, "兑换票据已过期，请重新扫码")
		return
	}
	var u core.User
	if err := s.db.First(&u, rec.UID).Error; err != nil {
		core.Err(c, 401, "扫码用户不存在")
		return
	}
	if u.Disabled {
		core.Err(c, 403, "账号已被禁用")
		return
	}
	token := randomToken(32)
	sess := core.Session{
		Token: token, UserID: u.ID, IP: rec.WebIP, UA: rec.WebUA,
		Client: SessionClientWeb, ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := s.db.Create(&sess).Error; err != nil {
		core.Err(c, 500, "创建会话失败")
		return
	}
	s.qr.mutate(key, func(r *qrRecord) { // 票据一次性：用后作废
		r.State = qrUsed
		r.Ticket = ""
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name: CookieSession, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 86400,
	})
	slog.Info("扫码登录成功", "username", u.Username, "web_ip", rec.WebIP, "approve_ip", c.ClientIP())
	core.OK(c, gin.H{"username": u.Username, "role": u.Role})
}

// ---- APP 自更新检查 ----

// appCheckUpdate GET /api/app/check-update?platform=android&v=<当前版本>（匿名可，入口校验之后）。
// 版本/下载地址/说明存设置项（管理员在设置页维护），无记录时 latest 为空 → 不提示更新。
func (s *Service) appCheckUpdate(c *gin.Context) {
	platform := strings.TrimSpace(c.Query("platform"))
	if platform == "" {
		platform = "android"
	}
	latest := strings.TrimSpace(core.GetSetting(s.db, core.KAppLatestVer))
	out := gin.H{
		"platform": platform,
		"current":  func() string { v := strings.TrimSpace(c.Query("v")); if v != "" { return v }; return core.Version }(),
		"latest":   latest,
		"apk_url":  core.GetSetting(s.db, core.KAppApkURL),
		"notes":    core.GetSetting(s.db, core.KAppReleaseNotes),
	}
	out["has_update"] = latest != "" && compareVersion(c.Query("v"), latest) < 0
	core.OK(c, out)
}

// compareVersion 点分数字版本比较（忽略 v 前缀与后缀），a<b 返回 -1，a>b 返回 1，相等 0。
func compareVersion(a, b string) int {
	parse := func(s string) []int {
		s = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(s)), "v")
		if i := strings.IndexAny(s, "-+"); i >= 0 {
			s = s[:i]
		}
		parts := strings.Split(s, ".")
		out := make([]int, 0, len(parts))
		for _, p := range parts {
			n, _ := strconv.Atoi(strings.TrimSpace(p))
			out = append(out, n)
		}
		return out
	}
	pa, pb := parse(a), parse(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		av, bv := 0, 0
		if i < len(pa) {
			av = pa[i]
		}
		if i < len(pb) {
			bv = pb[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
