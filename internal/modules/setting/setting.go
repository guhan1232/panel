// Package setting 系统设置：面板信息、修改管理员密码。
package setting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/core"
)

type Module struct {
	db  *gorm.DB
	cfg *config.Config
}

func New(db *gorm.DB, cfg *config.Config) *Module {
	return &Module{db: db, cfg: cfg}
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/setting")
	g.GET("/info", m.info)
	g.POST("/password", m.changePassword)
	g.POST("/profile", m.changeUsername)
	g.GET("/panel-backup", m.panelBackup)
	g.GET("/upgrade/check", m.upgradeCheck)
	g.POST("/upgrade/apply", m.upgradeApply)
}

func (m *Module) info(c *gin.Context) {
	core.OK(c, gin.H{
		"version":     core.Version,
		"listen":      m.cfg.Server.Listen,
		"entrance_on": m.cfg.Server.Entrance != "",
		"tls":         m.cfg.Server.TLS.Enabled,
		"data_dir":    m.cfg.DataDir,
		"go_version":  runtime.Version(),
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	})
}

func (m *Module) changePassword(c *gin.Context) {
	var req struct {
		Old string `binding:"required"`
		New string `binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	// 密码强度：≥8 位且同时包含字母与数字
	hasLetter, hasDigit := false, false
	for _, ch := range req.New {
		switch {
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z'):
			hasLetter = true
		}
	}
	if !hasLetter || !hasDigit {
		core.Err(c, 400, "密码需包含字母和数字")
		return
	}
	uid := c.GetUint("uid")
	var u core.User
	if err := m.db.First(&u, uid).Error; err != nil {
		core.Err(c, 404, "用户不存在")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Old)) != nil {
		core.Err(c, 400, "原密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := m.db.Model(&u).Update("password", string(hash)).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	// 初始凭据文件仅保留到用户首次改密，避免明文残留
	core.RemoveInitialAdminFile(m.cfg.DataDir)
	// 注销除当前会话外的所有会话
	cur := c.GetString("session")
	m.db.Where("user_id = ? AND token <> ?", uid, cur).Delete(&core.Session{})
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "密码已修改，其它会话已注销"})
}

// panelBackup 面板数据一键备份：SQLite VACUUM INTO 快照 + config.yaml + certs-managed 打包下载。
func (m *Module) panelBackup(c *gin.Context) {
	tmpDir, err := os.MkdirTemp("", "hanyuxin-backup-*")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	snap := filepath.Join(tmpDir, "panel.db")
	if err := m.db.Exec("VACUUM INTO ?", snap).Error; err != nil {
		core.Err(c, 500, "数据库快照失败: "+err.Error())
		return
	}

	paths := []string{snap}
	if m.cfg.Path != "" {
		if _, err := os.Stat(m.cfg.Path); err == nil {
			paths = append(paths, m.cfg.Path)
		}
	}
	certsDir := filepath.Join(m.cfg.DataDir, "certs-managed")
	if _, err := os.Stat(certsDir); err == nil {
		paths = append(paths, certsDir)
	}

	out := filepath.Join(tmpDir, "hanyuxin-panel-backup.zip")
	if err := archive.ZipAt(c.Request.Context(), tmpDir, paths, out); err != nil {
		core.Err(c, 500, "打包失败: "+err.Error())
		return
	}
	name := "hanyuxin-panel-backup-" + time.Now().Format("20060102-150405") + ".zip"
	c.FileAttachment(out, name)
}

// ---- 面板在线自升级：清单 JSON {version, binary_url, sha256?} ----

type upgradeManifest struct {
	Version   string `json:"version"`
	BinaryURL string `json:"binary_url"`
	SHA256    string `json:"sha256"`
	Notes     string `json:"notes"`
}

func (m *Module) upgradeCheck(c *gin.Context) {
	url := core.GetSetting(m.db, core.KUpgradeURL)
	if url == "" {
		url = "https://cnb.cool/hanyuxin/panel/-/git/raw/main/upgrade.json"
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		core.OK(c, gin.H{"configured": true, "reachable": false, "err": err.Error(), "current": core.Version})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		core.OK(c, gin.H{"configured": true, "reachable": false, "err": fmt.Sprintf("清单地址 HTTP %d", resp.StatusCode), "current": core.Version})
		return
	}
	var mf upgradeManifest
	if err := json.NewDecoder(resp.Body).Decode(&mf); err != nil {
		core.OK(c, gin.H{"configured": true, "reachable": false, "err": "清单解析失败: " + err.Error(), "current": core.Version})
		return
	}
	core.OK(c, gin.H{
		"configured": true, "reachable": true,
		"current": core.Version, "remote": mf.Version,
		"binary_url": mf.BinaryURL, "sha256": mf.SHA256,
		"notes":      mf.Notes,
		"has_update": mf.Version != "" && mf.Version != core.Version,
	})
}

// upgradeApply 执行自升级：下载 → SHA256 校验（可选）→ 备份 → 原子替换 → 重启。
func (m *Module) upgradeApply(c *gin.Context) {
	var req struct {
		BinaryURL string `binding:"required"`
		SHA256    string `json:"sha256"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	tmp, err := os.CreateTemp("", "hanyuxin-upgrade-*")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(req.BinaryURL)
	if err != nil {
		core.Err(c, 500, "下载失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		core.Err(c, 500, fmt.Sprintf("下载失败: HTTP %d", resp.StatusCode))
		return
	}
	n, err := io.Copy(tmp, resp.Body)
	if err != nil {
		core.Err(c, 500, "保存失败: "+err.Error())
		return
	}
	if n < 1024 {
		core.Err(c, 500, "下载内容过小，疑似无效二进制")
		return
	}

	if req.SHA256 != "" {
		tmp.Seek(0, 0)
		h := sha256.New()
		if _, err := io.Copy(h, tmp); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, req.SHA256) {
			core.Err(c, 400, "SHA256 校验失败: got "+got)
			return
		}
	}

	exe, err := os.Executable()
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	exePath, _ := filepath.Abs(exe)
	// 备份当前二进制
	data, err := os.ReadFile(exePath)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	_ = os.WriteFile(exePath+".bak", data, 0o755)
	// 校验 + 赋权 + 原子替换
	tmp.Seek(0, 0)
	if _, err := tmp.Seek(0, 0); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	newBin, err := os.ReadFile(tmp.Name())
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.WriteFile(exePath+".new", newBin, 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.Rename(exePath+".new", exePath); err != nil {
		core.Err(c, 500, "替换二进制失败: "+err.Error())
		return
	}
	slog.Info("面板二进制已替换，准备重启", "from", core.Version, "size", n)

	// 响应先行，随后异步重启（systemd 环境由守护进程拉起；否则 exec 自身）
	core.OK(c, gin.H{"ok": true, "restarting": true})
	go func() {
		time.Sleep(800 * time.Millisecond)
		cmd2 := exec.Command("sh", "-c", "systemctl restart hanyuxin || systemctl restart digong 2>/dev/null")
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			slog.Warn("systemctl 重启不可用，改用 exec 自重启", "out", strings.TrimSpace(string(out2)))
			_ = syscall.Exec(exePath, os.Args, os.Environ())
		}
	}()
}

// changeUsername 当前登录用户修改自己的用户名（唯一性校验，3-64 位）。
func (m *Module) changeUsername(c *gin.Context) {
	var req struct {
		Username string `binding:"required,min=3,max=64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	name := strings.TrimSpace(req.Username)
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			core.Err(c, 400, "用户名仅允许字母、数字、下划线与短横线")
			return
		}
	}
	uid := c.GetUint("uid")
	var u core.User
	if err := m.db.First(&u, uid).Error; err != nil {
		core.Err(c, 404, "用户不存在")
		return
	}
	if name == u.Username {
		core.OK(c, gin.H{"username": name})
		return
	}
	var cnt int64
	m.db.Model(&core.User{}).Where("username = ?", name).Count(&cnt)
	if cnt > 0 {
		core.Err(c, 400, "用户名已被占用")
		return
	}
	if err := m.db.Model(&u).Update("username", name).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"username": name})
}
