// bind.go —— 面板与 APP 的一对一绑定模型（docs/APP.md）。
//   - 管理员在 Web 生成「绑定密钥」（64 hex，持久存储，可重新生成顶替）
//   - APP 扫绑定二维码（host+入口码+密钥）或手动输入密钥 → bind/login 免账号密码换取会话
//   - 一个面板同时只绑定一台设备：新设备 bind/login 自动顶替旧设备（吊销旧会话）
//   - 会话 30 天滑动续期（复用 client=app 续期逻辑），APP 打开即用无需账号密码
//   - Web 弹窗轮询 bind/status 感知绑定完成（自动回调，二维码不再刷新）
package auth

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
)

// 绑定信息（settings KV 持久化）。
type bindDevice struct {
	UID          uint      `json:"uid"`           // 生成密钥的管理员（APP 以其身份获得会话）
	DeviceName   string    `json:"device_name"`   // APP 上报的设备名
	DeviceModel  string    `json:"device_model"`
	BoundAt      time.Time `json:"bound_at"`
	SessionToken string    `json:"session_token"` // 绑定会话令牌（新设备顶替时吊销）
}

const (
	kBindKey    = "app_bind_key"    // 绑定密钥（64 hex，空=未生成/未启用）
	kBindDevice = "app_bind_device" // 绑定设备 JSON
)

// bindRate 绑定登录限频（IP 级滑动窗口，防密钥爆破；与扫码限频器同实现）。
var bindRate = newQRStore()

// bindGenerate POST /auth/bind/generate（仅 admin）：生成/重新生成绑定密钥。
// 重新生成 = 旧密钥作废 + 旧绑定设备解绑（吊销其会话）。
func (s *Service) bindGenerate(c *gin.Context) {
	if c.GetString("role") != "admin" {
		core.Err(c, 403, "仅管理员可生成绑定密钥")
		return
	}
	uid := c.GetUint("uid")
	key := randomToken(32)
	if raw := core.GetSetting(s.db, kBindDevice); raw != "" {
		var dev bindDevice
		if json.Unmarshal([]byte(raw), &dev) == nil && dev.SessionToken != "" {
			s.db.Table("sessions").Where("token = ?", dev.SessionToken).Delete(nil)
		}
	}
	if err := core.SetSetting(s.db, kBindKey, key); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	// 记录生成者身份：APP 凭密钥绑定后以该账号身份获得会话
	newDev, _ := json.Marshal(bindDevice{UID: uid, BoundAt: time.Now()})
	core.SetSetting(s.db, kBindDevice, string(newDev))
	slog.Info("绑定密钥已生成", "uid", uid, "ip", c.ClientIP())
	core.OK(c, gin.H{"key": key})
}

// bindStatus GET /auth/bind/status（登录态）：绑定状态（Web 弹窗轮询感知绑定完成）。
func (s *Service) bindStatus(c *gin.Context) {
	out := gin.H{"enabled": core.GetSetting(s.db, kBindKey) != "", "bound": false}
	if raw := core.GetSetting(s.db, kBindDevice); raw != "" {
		var dev bindDevice
		if json.Unmarshal([]byte(raw), &dev) == nil {
			out["bound"] = true
			out["device_name"] = dev.DeviceName
			out["device_model"] = dev.DeviceModel
			out["bound_at"] = dev.BoundAt.Format("2006-01-02 15:04")
			out["uid"] = dev.UID
		}
	}
	core.OK(c, out)
}

// bindUnbind POST /auth/bind/unbind（仅 admin）：解绑（吊销设备会话 + 清密钥）。
func (s *Service) bindUnbind(c *gin.Context) {
	if c.GetString("role") != "admin" {
		core.Err(c, 403, "仅管理员可解绑")
		return
	}
	if raw := core.GetSetting(s.db, kBindDevice); raw != "" {
		var dev bindDevice
		if json.Unmarshal([]byte(raw), &dev) == nil && dev.SessionToken != "" {
			s.db.Table("sessions").Where("token = ?", dev.SessionToken).Delete(nil)
		}
	}
	core.SetSetting(s.db, kBindDevice, "")
	core.SetSetting(s.db, kBindKey, "")
	slog.Info("APP 绑定已解绑", "ip", c.ClientIP())
	core.OK(c, gin.H{"ok": true})
}

// bindLogin POST /auth/bind/login（匿名，安全入口之后）：绑定密钥换取会话。
// 请求：{bind_key, device_name, device_model}
// 校验通过 → 以密钥生成者身份签发 client=app 的 30 天会话（新设备顶替旧设备）。
func (s *Service) bindLogin(c *gin.Context) {
	ip := c.ClientIP()
	if !bindRate.allow("bind:"+ip, 5, time.Minute) {
		core.Err(c, 429, "尝试次数过多，请 1 分钟后再试")
		return
	}
	var req struct {
		BindKey     string `json:"bind_key" binding:"required"`
		DeviceName  string `json:"device_name"`
		DeviceModel string `json:"device_model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	key := core.GetSetting(s.db, kBindKey)
	if key == "" {
		core.Err(c, 404, "面板尚未生成绑定密钥（请在 Web 端系统设置生成）")
		return
	}
	if subtle.ConstantTimeCompare([]byte(key), []byte(strings.TrimSpace(req.BindKey))) != 1 {
		slog.Warn("绑定密钥校验失败", "ip", ip)
		core.Err(c, 401, "绑定密钥错误")
		return
	}
	var dev bindDevice
	if raw := core.GetSetting(s.db, kBindDevice); raw != "" {
		_ = json.Unmarshal([]byte(raw), &dev)
	}
	// 绑定者身份（生成密钥的管理员）
	var u core.User
	if err := s.db.First(&u, dev.UID).Error; err != nil {
		core.Err(c, 401, "绑定密钥的生成账号已不存在，请在 Web 端重新生成密钥")
		return
	}
	if u.Disabled {
		core.Err(c, 403, "绑定密钥的生成账号已被禁用")
		return
	}
	// 一对一顶替：吊销旧绑定会话
	if dev.SessionToken != "" {
		s.db.Table("sessions").Where("token = ?", dev.SessionToken).Delete(nil)
	}
	token := randomToken(32)
	sess := core.Session{
		Token: token, UserID: u.ID, IP: ip, UA: c.Request.UserAgent(),
		Client:     SessionClientApp,
		DeviceName: strings.TrimSpace(req.DeviceName),
		ExpiresAt:  time.Now().Add(appSessionTTL),
	}
	if err := s.db.Create(&sess).Error; err != nil {
		core.Err(c, 500, "创建会话失败")
		return
	}
	// 更新绑定设备信息（一对一）
	newDev, _ := json.Marshal(bindDevice{
		UID: u.ID, DeviceName: strings.TrimSpace(req.DeviceName),
		DeviceModel: strings.TrimSpace(req.DeviceModel),
		BoundAt:     time.Now(), SessionToken: token,
	})
	core.SetSetting(s.db, kBindDevice, string(newDev))
	slog.Info("APP 绑定登录成功", "username", u.Username, "device", req.DeviceName, "ip", ip)
	core.OK(c, gin.H{
		"token":      token,
		"expires_at": sess.ExpiresAt.Unix(),
		"username":   u.Username,
		"role":       u.Role,
	})
}
