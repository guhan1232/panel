// Package setting 系统设置：面板信息、修改管理员密码。
package setting

import (
	"context"
	"crypto/sha256"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
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
	"cnb.cool/hanyuxin/panel/internal/auth"
"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/modules/cert"
	"cnb.cool/hanyuxin/panel/internal/selfcert"
)

type Module struct {
	db   *gorm.DB
	cfg  *config.Config
	cert *cert.Module
}

func New(db *gorm.DB, cfg *config.Config, certMod *cert.Module) *Module {
	return &Module{db: db, cfg: cfg, cert: certMod}
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/setting")
	g.GET("/info", m.info)
	g.POST("/password", m.changePassword)
	g.POST("/profile", m.changeUsername)
	g.GET("/panel-backup", m.panelBackup)
	g.POST("/panel-restore", m.panelRestore)
	g.POST("/panel-restore/rollback", m.panelRestoreRollback)
	g.GET("/upgrade/check", m.upgradeCheck)
	g.POST("/upgrade/apply", m.upgradeApply)
	// 移动 APP（docs/APP.md §7 F3）：对外地址 / APK 分发 / 自更新元数据
	g.GET("/app-config", m.appConfigGet)
	g.POST("/app-config", m.appConfigSet)
	// HTTPS 证书（自签兜底 10 年 / 中间 CA 交叉签发跟随 CA 有效期）
	g.GET("/tls/status", m.tlsStatus)
	g.POST("/tls/apply", m.tlsApply)
	// 自定义面板名称（admin 可改；brand 为匿名可读的公开端点，见 main.go）
	g.GET("/name", m.panelNameGet)
	g.POST("/name", m.panelNameSet)
}

// RegisterTokenAdmin 开放 API Token 管理（与 MCP Token 相互独立，鉴权规则见 internal/auth/apitoken.go）。
func (m *Module) RegisterTokenAdmin(admin gin.IRouter) {
	admin.GET("/setting/api-token", m.apiTokenGet)
	admin.POST("/setting/api-token", m.apiTokenGen)
	admin.PUT("/setting/api-token", m.apiTokenPermUpdate)
	admin.DELETE("/setting/api-token", m.apiTokenClear)
}

// ---- 开放 API Token 管理 ----

func (m *Module) apiTokenGet(c *gin.Context) {
	rec := auth.APITokenRecOf(m.db)
	if rec == nil {
		core.OK(c, gin.H{"enabled": false, "token": "", "mode": "full", "scopes": []string{}, "readonly": false, "scopes_known": auth.ScopesKnown})
		return
	}
	scopes := rec.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	core.OK(c, gin.H{"enabled": true, "token": rec.Token, "mode": rec.Mode, "scopes": scopes, "readonly": rec.ReadOnly, "scopes_known": auth.ScopesKnown})
}

func (m *Module) apiTokenGen(c *gin.Context) {
	var p auth.PermInput
	_ = c.ShouldBindJSON(&p)
	rec, err := auth.BuildPerm(&p)
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		core.Err(c, 500, "生成 Token 失败")
		return
	}
	rec.Token = hex.EncodeToString(b)
	if err := auth.SaveTokenRecord(m.db, rec); err != nil {
		core.Err(c, 500, "保存 Token 失败")
		return
	}
	slog.Info("开放 API Token 已生成", "by", c.GetString("user"), "mode", rec.Mode, "readonly", rec.ReadOnly)
	core.OK(c, gin.H{"enabled": true, "token": rec.Token, "mode": rec.Mode, "scopes": rec.Scopes, "readonly": rec.ReadOnly, "scopes_known": auth.ScopesKnown})
}

func (m *Module) apiTokenPermUpdate(c *gin.Context) {
	rec := auth.APITokenRecOf(m.db)
	if rec == nil {
		core.Err(c, 400, "尚未启用开放 API Token")
		return
	}
	var p auth.PermInput
	if err := c.ShouldBindJSON(&p); err != nil {
		core.BindErr(c, err)
		return
	}
	perm, err := auth.BuildPerm(&p)
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	rec.Mode, rec.Scopes, rec.ReadOnly = perm.Mode, perm.Scopes, perm.ReadOnly
	if err := auth.SaveTokenRecord(m.db, rec); err != nil {
		core.Err(c, 500, "保存权限失败")
		return
	}
	slog.Info("开放 API Token 权限已更新", "by", c.GetString("user"), "mode", rec.Mode, "readonly", rec.ReadOnly)
	core.OK(c, gin.H{"enabled": true, "token": rec.Token, "mode": rec.Mode, "scopes": rec.Scopes, "readonly": rec.ReadOnly, "scopes_known": auth.ScopesKnown})
}

func (m *Module) apiTokenClear(c *gin.Context) {
	if err := core.SetSetting(m.db, core.KApiToken, ""); err != nil {
		core.Err(c, 500, "禁用失败")
		return
	}
	slog.Info("开放 API Token 已禁用", "by", c.GetString("user"))
	core.OK(c, gin.H{"enabled": false})
}

// ---- HTTPS 证书管理 ----

// tlsStatus 当前 TLS 配置与证书有效期。
func (m *Module) tlsStatus(c *gin.Context) {
	out := gin.H{
		"tls":      m.cfg.Server.TLS.Enabled,
		"listen":   m.cfg.Server.Listen,
		"has_cert": m.cfg.Server.TLS.Cert != "",
		"has_ca":   false,
		"acme_ip":  gin.H{"supported": false, "reason": "Let's Encrypt 的 IP 证书目前仅提供 6 天短期证书且需占用 443 端口自动续期，与长期证书诉求冲突；建议使用中间 CA 交叉签发（有效期跟随 CA）或自签 10 年"},
	}
	// CA 是否已就位
	caDir := filepath.Join(m.cfg.DataDir, "certs", "ca")
	if _, err := os.Stat(filepath.Join(caDir, "ca.crt")); err == nil {
		if _, err2 := os.Stat(filepath.Join(caDir, "ca.key")); err2 == nil {
			out["has_ca"] = true
		}
	}
	// 当前证书有效期
	if m.cfg.Server.TLS.Cert != "" {
		out["cert_path"] = m.cfg.Server.TLS.Cert
		out["key_path"] = m.cfg.Server.TLS.Key
		if data, err := os.ReadFile(m.cfg.Server.TLS.Cert); err == nil {
			if subject, notAfter, err := selfcert.Inspect(data); err == nil {
				out["cert_subject"] = subject
				out["cert_not_after"] = notAfter.Format("2006-01-02")
			}
		}
	} else {
		// 未配置路径时检查默认自签位置
		def := filepath.Join(m.cfg.DataDir, "certs", "panel.crt")
		if data, err := os.ReadFile(def); err == nil {
			if subject, notAfter, err := selfcert.Inspect(data); err == nil {
				out["cert_subject"] = subject
				out["cert_not_after"] = notAfter.Format("2006-01-02")
			}
		}
	}
	core.OK(c, out)
}

// tlsApply 签发证书并启用 HTTPS。三种模式：
//   - ca（默认推荐）：用户中间 CA 交叉签发，有效期跟随 CA；无已存 CA 时回退自签 10 年。
//   - self：纯自签 10 年兜底（无 CA 时）。
//   - acme：公共免费 CA（Let's Encrypt）签发域名证书（HTTP-01，需 80 端口与域名解析）。
//
// extra_ips/extra_dns：显式指定的访问地址写入 SAN。成功后写 config.yaml 并自动重启面板。
func (m *Module) tlsApply(c *gin.Context) {
	var req struct {
		Mode     string   `binding:"required,oneof=self ca acme"`
		CACert   string   `json:"ca_cert"` // PEM 文本（mode=ca 时必填，除非已保存过 CA）
		CAKey    string   `json:"ca_key"`
		ExtraIPs []string `json:"extra_ips"`
		ExtraDNS []string `json:"extra_dns"`
		// acme 专用
		AcmeDomain string `json:"acme_domain"`
		AcmeEmail  string `json:"acme_email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	panelDir := filepath.Join(m.cfg.DataDir, "certs", "panel")
	var certPEM, keyPEM []byte

	if req.Mode == "acme" {
		// 公共免费 CA（Let's Encrypt）：域名必须已解析到本服务器（HTTP-01 占 80）
		dom := strings.TrimSpace(req.AcmeDomain)
		if dom == "" {
			core.Err(c, 400, "请填写要签发的域名")
			return
		}
		certPath, keyPath, err := m.acmeIssue(dom, strings.TrimSpace(req.AcmeEmail))
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		if certPEM, err = os.ReadFile(certPath); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		if keyPEM, err = os.ReadFile(keyPath); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		m.writePanelCertAndRestart(certPEM, keyPEM, panelDir, "acme")
		core.OK(c, gin.H{"ok": true, "restarting": true, "mode": "acme"})
		return
	}

	// 显式指定的 SAN（公网 IP / 域名）
	ips := selfcert.PublicIPs()
	for _, s := range req.ExtraIPs {
		if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil {
			ips = append(ips, ip)
		}
	}
	hostnames := []string{}
	for _, s := range req.ExtraDNS {
		if s = strings.TrimSpace(s); s != "" {
			hostnames = append(hostnames, s)
		}
	}

	if req.Mode == "ca" {
		caDir := filepath.Join(m.cfg.DataDir, "certs", "ca")
		caCertPath, caKeyPath := filepath.Join(caDir, "ca.crt"), filepath.Join(caDir, "ca.key")
		// 优先使用本次上传的 CA；否则复用已保存的
		if req.CACert != "" && req.CAKey != "" {
			if err := os.MkdirAll(caDir, 0o700); err != nil {
				core.Err(c, 500, err.Error())
				return
			}
			if err := os.WriteFile(caCertPath, []byte(req.CACert), 0o644); err != nil {
				core.Err(c, 500, err.Error())
				return
			}
			if err := os.WriteFile(caKeyPath, []byte(req.CAKey), 0o600); err != nil {
				core.Err(c, 500, err.Error())
				return
			}
		}
		caCertPEM, err := os.ReadFile(caCertPath)
		if err != nil {
			core.Err(c, 400, "请先上传中间 CA 证书与私钥")
			return
		}
		caKeyPEM, err := os.ReadFile(caKeyPath)
		if err != nil {
			core.Err(c, 400, "请先上传中间 CA 私钥")
			return
		}
		certPEM, keyPEM, err = selfcert.IssueFromCA(caCertPEM, caKeyPEM, ips, hostnames)
		if err != nil {
			core.Err(c, 400, err.Error())
			return
		}
	} else {
		// 自签兜底：10 年，SAN 含公网 IP + 显式指定的访问地址
		var serr error
		certPEM, keyPEM, serr = selfcert.IssueSelfSigned(hostnames, ips)
		if serr != nil {
			core.Err(c, 500, serr.Error())
			return
		}
	}

	m.writePanelCertAndRestart(certPEM, keyPEM, panelDir, req.Mode)
	core.OK(c, gin.H{"ok": true, "restarting": true, "mode": req.Mode})
}

// acmeIssue 用公共免费 CA（Let's Encrypt，HTTP-01）给域名签发证书，返回证书/私钥路径。
// 前提：域名已解析到本服务器公网 IP，且 80 端口可从外网访问（面板与 nginx 都不占用 80）。
func (m *Module) acmeIssue(domain, email string) (string, string, error) {
	if email == "" {
		email = "admin@" + domain
	}
	// nginx 占 80（面板反代 vhost）时临时让位：停 nginx → 签发 → 立即恢复（约 15 秒）
	nginxWasRunning := false
	if _, err := exec.Command("sh", "-c", "systemctl is-active --quiet nginx").CombinedOutput(); err == nil {
		nginxWasRunning = true
		_, _ = exec.Command("sh", "-c", "systemctl stop nginx").CombinedOutput()
		slog.Info("ACME 签发：临时停止 nginx 释放 80 端口")
	}
	dir, err := m.cert.IssuePublicCert(domain, email)
	if nginxWasRunning {
		_, _ = exec.Command("sh", "-c", "systemctl start nginx").CombinedOutput()
		slog.Info("ACME 签发完成，nginx 已恢复")
	}
	if err != nil {
		return "", "", fmt.Errorf("公共 CA 签发失败（确认域名已解析到本服务器且 80 端口外网可达）: %w", err)
	}
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem"), nil
}

// writePanelCertAndRestart 落盘证书 → 写 config.yaml → 异步重启面板。
func (m *Module) writePanelCertAndRestart(certPEM, keyPEM []byte, panelDir, mode string) {
	if err := os.MkdirAll(panelDir, 0o700); err != nil {
		slog.Error("创建证书目录失败", "err", err)
		return
	}
	certPath := filepath.Join(panelDir, "panel.crt")
	keyPath := filepath.Join(panelDir, "panel.key")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		slog.Error("写证书失败", "err", err)
		return
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		slog.Error("写私钥失败", "err", err)
		return
	}
	m.cfg.Server.TLS.Enabled = true
	m.cfg.Server.TLS.Cert = certPath
	m.cfg.Server.TLS.Key = keyPath
	if err := m.cfg.Save(m.cfg.Path); err != nil {
		slog.Error("写入配置失败", "err", err)
		return
	}
	slog.Info("HTTPS 证书已更新，准备重启面板", "mode", mode, "cert", certPath)
	go func() {
		time.Sleep(800 * time.Millisecond)
		cmd2 := exec.Command("sh", "-c", "systemctl restart hanyuxin || systemctl restart digong 2>/dev/null")
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			slog.Warn("systemctl 重启不可用，改用 exec 自重启", "out", strings.TrimSpace(string(out2)))
			exe, _ := os.Executable()
			if exePath, _ := filepath.Abs(exe); exePath != "" {
				_ = syscall.Exec(exePath, os.Args, os.Environ())
			}
		}
	}()
}

func (m *Module) info(c *gin.Context) {
	core.OK(c, gin.H{
		"panel_name":  m.panelName(),
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

// appConfigGet 移动 APP 配置读取（登录会话即可读；APP 设置页与更新检查共用）。
func (m *Module) appConfigGet(c *gin.Context) {
	core.OK(c, gin.H{
		"public_url":     core.GetSetting(m.db, core.KAppPublicURL),
		"apk_url":        core.GetSetting(m.db, core.KAppApkURL),
		"latest_version": core.GetSetting(m.db, core.KAppLatestVer),
		"release_notes":  core.GetSetting(m.db, core.KAppReleaseNotes),
		"sha256":         core.GetSetting(m.db, core.KAppApkSha256),
	})
}

// appConfigSet 移动 APP 配置写入（仅 admin）：对外地址、APK 下载地址、最新版本、更新说明。
func (m *Module) appConfigSet(c *gin.Context) {
	if c.GetString("role") != "admin" {
		core.Err(c, 403, "仅管理员可修改 APP 配置")
		return
	}
	var req struct {
		PublicURL    string `json:"public_url"`
		ApkURL       string `json:"apk_url"`
		LatestVer    string `json:"latest_version"`
		ReleaseNotes string `json:"release_notes"`
		Sha256       string `json:"sha256"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	sets := []struct{ key, val string }{
		{core.KAppPublicURL, strings.TrimSpace(req.PublicURL)},
		{core.KAppApkURL, strings.TrimSpace(req.ApkURL)},
		{core.KAppLatestVer, strings.TrimSpace(req.LatestVer)},
		{core.KAppReleaseNotes, req.ReleaseNotes},
	}
	for _, s := range sets {
		if err := core.SetSetting(m.db, s.key, s.val); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
	}
	slog.Info("APP 配置已更新", "ip", c.ClientIP())
	core.OK(c, gin.H{"ok": true})
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
	out := filepath.Join(tmpDir, "hanyuxin-panel-backup.zip")
	if err := m.packPanelData(c.Request.Context(), []string{"db", "config", "certs"}, out); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	name := "hanyuxin-panel-backup-" + time.Now().Format("20060102-150405") + ".zip"
	c.FileAttachment(out, name)
}

// packPanelData 统一打包面板数据到暂存目录再压缩（条目名干净：panel.db/config.yaml/certs-managed/…）。
// items: db | config | certs 子集；out 为输出 zip 绝对路径。
func (m *Module) packPanelData(ctx context.Context, items []string, out string) error {
	stage, err := os.MkdirTemp("", "hanyuxin-pack-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, it := range items {
		switch it {
		case "db":
			snap := filepath.Join(stage, "panel.db")
			if err := m.db.Exec("VACUUM INTO ?", snap).Error; err != nil {
				return fmt.Errorf("数据库快照失败: %w", err)
			}
		case "config":
			if m.cfg.Path == "" {
				continue
			}
			data, err := os.ReadFile(m.cfg.Path)
			if err != nil {
				continue
			}
			_ = os.WriteFile(filepath.Join(stage, "config.yaml"), data, 0o600)
		case "certs":
			src := filepath.Join(m.cfg.DataDir, "certs-managed")
			if _, err := os.Stat(src); err == nil {
				_ = copyDirLocal(src, filepath.Join(stage, "certs-managed"))
			}
		}
	}
	var tops []string
	entries, _ := os.ReadDir(stage)
	for _, e := range entries {
		tops = append(tops, filepath.Join(stage, e.Name()))
	}
	if err := archive.ZipAt(ctx, stage, tops, out); err != nil {
		return fmt.Errorf("打包失败: %w", err)
	}
	return nil
}

// copyDirLocal 递归拷贝目录（备份暂存用）。
func copyDirLocal(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// buildRestoreScript 生成脱离执行的恢复脚本（停服 → 替换 → 启动 → 清理）。
func buildRestoreScript(workDir, restoreDir, cfgPath, dataDir, exeName string, items []string) string {
	stop := "systemctl stop hanyuxin 2>/dev/null || pkill -f \"" + exeName + "\" 2>/dev/null || true"
	start := "systemctl start hanyuxin 2>/dev/null || (cd \"" + workDir + "\" && nohup ./" + exeName + " -c config.yaml >> panel.log 2>&1 &)"
	itemLines := ""
	for _, it := range items {
		switch it {
		case "db":
			itemLines += "cp -f \"" + restoreDir + "/check/panel.db\" \"" + workDir + "/data/panel.db\"\n"
		case "config":
			itemLines += "cp -f \"" + restoreDir + "/check/config.yaml\" \"" + cfgPath + "\"\n"
		case "certs":
			itemLines += "rm -rf \"" + dataDir + "/certs-managed\"\n" +
				"cp -r \"" + restoreDir + "/check/certs-managed\" \"" + dataDir + "/certs-managed\"\n"
		}
	}
	return "#!/bin/sh\n" +
		"sleep 2\n" +
		stop + "\n" +
		"sleep 1\n" +
		"cd \"" + workDir + "\"\n" +
		itemLines +
		start + "\n" +
		"rm -rf \"" + restoreDir + "\"\n"
}

// panelRestore 导入备份包并恢复：multipart 上传 zip（panel.db / config.yaml / certs-managed/）。
// items 指定恢复项（缺省全恢复）；恢复前自动留存当前数据为回滚点；由 setsid 脱离脚本执行替换与重启。
func (m *Module) panelRestore(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		core.BindErr(c, err)
		return
	}
	items := c.PostFormArray("items")
	if len(items) == 0 {
		items = []string{"db", "config", "certs"}
	}
	restoreDir := filepath.Join(m.cfg.DataDir, "restore-pending")
	if err := os.MkdirAll(restoreDir, 0o700); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	zipPath := filepath.Join(restoreDir, "restore.zip")
	if err := c.SaveUploadedFile(file, zipPath); err != nil {
		core.Err(c, 500, "保存上传失败: "+err.Error())
		return
	}
	// 解包校验（防路径穿越由 archive.UnZip 处理；必含所选恢复项）
	checkDir := filepath.Join(restoreDir, "check")
	_ = os.RemoveAll(checkDir)
	if err := archive.UnZip(zipPath, checkDir); err != nil {
		_ = os.RemoveAll(restoreDir)
		core.Err(c, 400, "备份包解析失败: "+err.Error())
		return
	}
	itemFiles := map[string]string{"db": "panel.db", "config": "config.yaml", "certs": "certs-managed"}
	for _, it := range items {
		if _, err := os.Stat(filepath.Join(checkDir, itemFiles[it])); err != nil {
			_ = os.RemoveAll(restoreDir)
			core.Err(c, 400, "备份包中缺少 "+itemFiles[it]+" 项，无法恢复")
			return
		}
	}
	defer os.RemoveAll(checkDir)

	// 回滚点：当前数据打包留存
	rollbackZip, err := m.makeRollback(c, items)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}

	exe, _ := os.Executable()
	workDir := filepath.Dir(exe)
	exeName := filepath.Base(exe)
	script := buildRestoreScript(workDir, restoreDir, m.cfg.Path, m.cfg.DataDir, exeName, items)
	if err := os.WriteFile(filepath.Join(restoreDir, "restore.sh"), []byte(script), 0o700); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("面板恢复任务已受理", "items", strings.Join(items, ","), "rollback", filepath.Base(rollbackZip))
	_, _ = exec.Command("sh", "-c", "setsid nohup sh "+filepath.Join(restoreDir, "restore.sh")+" >/dev/null 2>&1 &").CombinedOutput()
	core.OK(c, gin.H{"ok": true, "message": "恢复任务已执行：面板将在数秒内重启，恢复前数据已留存为回滚点", "rollback": filepath.Base(rollbackZip)})
}

// makeRollback 把当前面板数据打包为回滚点 zip（复用 packPanelData，条目名干净）。
func (m *Module) makeRollback(c *gin.Context, items []string) (string, error) {
	bakDir := filepath.Join(m.cfg.DataDir, "restore-rollback")
	if err := os.MkdirAll(bakDir, 0o700); err != nil {
		return "", err
	}
	rollbackZip := filepath.Join(bakDir, "restore-bak-"+time.Now().Format("20060102-150405")+".zip")
	if err := m.packPanelData(c.Request.Context(), items, rollbackZip); err != nil {
		return "", err
	}
	return rollbackZip, nil
}

// panelRestoreRollback 用最近一次回滚点恢复（同一恢复模板）。
func (m *Module) panelRestoreRollback(c *gin.Context) {
	bakDir := filepath.Join(m.cfg.DataDir, "restore-rollback")
	entries, err := os.ReadDir(bakDir)
	if err != nil || len(entries) == 0 {
		core.Err(c, 404, "没有可用的回滚点")
		return
	}
	latest := entries[0]
	for _, e := range entries {
		if e.Name() > latest.Name() {
			latest = e
		}
	}
	restoreDir := filepath.Join(m.cfg.DataDir, "restore-pending")
	_ = os.MkdirAll(restoreDir, 0o700)
	zipPath := filepath.Join(restoreDir, "restore.zip")
	in, err := os.Open(filepath.Join(bakDir, latest.Name()))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	out, err := os.Create(zipPath)
	if err != nil {
		in.Close()
		core.Err(c, 500, err.Error())
		return
	}
	_, _ = io.Copy(out, in)
	in.Close()
	out.Close()
	checkDir := filepath.Join(restoreDir, "check")
	_ = os.RemoveAll(checkDir)
	if err := archive.UnZip(zipPath, checkDir); err != nil {
		core.Err(c, 400, "回滚点解析失败: "+err.Error())
		return
	}
	// 回滚点包内 db 在顶层，config/certs 也在顶层
	items := []string{}
	itemFiles := map[string]string{"db": "panel.db", "config": "config.yaml", "certs": "certs-managed"}
	for _, it := range []string{"db", "config", "certs"} {
		if _, err := os.Stat(filepath.Join(checkDir, itemFiles[it])); err == nil {
			items = append(items, it)
		}
	}
	if len(items) == 0 {
		items = []string{"db", "config", "certs"}
	}
	exe, _ := os.Executable()
	workDir := filepath.Dir(exe)
	exeName := filepath.Base(exe)
	script := buildRestoreScript(workDir, restoreDir, m.cfg.Path, m.cfg.DataDir, exeName, items)
	_ = os.WriteFile(filepath.Join(restoreDir, "restore.sh"), []byte(script), 0o700)
	_, _ = exec.Command("sh", "-c", "setsid nohup sh "+filepath.Join(restoreDir, "restore.sh")+" >/dev/null 2>&1 &").CombinedOutput()
	core.OK(c, gin.H{"ok": true, "message": "回滚任务已执行（使用 " + latest.Name() + "），面板将重启"})
}

// ---- 面板在线自升级：清单 JSON {version, binary_url, sha256?} ----

type upgradeArchEntry struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// upgradeManifest CNB Release 升级清单（panel-release 附件 upgrade-manifest.json）。
// 新格式按架构区分（arch.amd64/arm64）；旧扁平格式（binary_url/sha256 顶层）自动兼容为当前架构。
type upgradeManifest struct {
	Version   string                       `json:"version"`
	Notes     string                       `json:"notes"`
	Arch      map[string]upgradeArchEntry  `json:"arch"`
	BinaryURL string                       `json:"binary_url"` // 旧格式兼容
	SHA256    string                       `json:"sha256"`     // 旧格式兼容
}

// upgradeCheck 从 CNB Release 清单检查面板新版本（按运行架构返回对应下载地址与 SHA256）。
func (m *Module) upgradeCheck(c *gin.Context) {
	url := core.GetSetting(m.db, core.KUpgradeURL)
	if url == "" {
		url = "https://cnb.cool/hanyuxin/panel/-/releases/download/panel-release/upgrade-manifest.json"
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
	goarch := runtime.GOARCH
	url = ""
	sha := ""
	if mf.Arch != nil {
		if a, ok := mf.Arch[goarch]; ok {
			url, sha = a.URL, a.SHA256
		}
	}
	if url == "" {
		url, sha = mf.BinaryURL, mf.SHA256 // 旧扁平清单兼容
	}
	core.OK(c, gin.H{
		"configured": true, "reachable": true,
		"current": core.Version, "remote": mf.Version,
		"arch": goarch, "binary_url": url, "sha256": sha,
		"notes": mf.Notes,
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
	m.db.Model(&core.User{}).Where("username = ? AND id <> ?", name, uid).Count(&cnt)
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

// ---- 自定义面板名称 ----

// panelName 读取当前面板名（settings 表；空值由前端回退默认「寒语馨面板」）。
func (m *Module) panelName() string {
	return strings.TrimSpace(core.GetSetting(m.db, core.KPanelName))
}

// panelNameGet 登录会话读取面板名。
func (m *Module) panelNameGet(c *gin.Context) {
	core.OK(c, gin.H{"name": m.panelName()})
}

// panelNameSet 修改面板名（仅 admin，1-40 字符；前后端品牌处即时生效，无需重启）。
func (m *Module) panelNameSet(c *gin.Context) {
	if c.GetString("role") != "admin" {
		core.Err(c, 403, "仅管理员可修改面板名称")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		// 清空 = 恢复默认
		if err := core.SetSetting(m.db, core.KPanelName, ""); err != nil {
			core.Err(c, 500, "保存失败")
			return
		}
		core.OK(c, gin.H{"name": ""})
		return
	}
	if n := len([]rune(name)); n < 1 || n > 40 {
		core.Err(c, 400, "面板名称需 1-40 个字")
		return
	}
	if err := core.SetSetting(m.db, core.KPanelName, name); err != nil {
		core.Err(c, 500, "保存失败")
		return
	}
	core.OK(c, gin.H{"name": name})
}

// BrandPublic 匿名可读的面板品牌信息（登录页 / 布局壳品牌名用；仍受安全入口与审计中间件防护）。
func (m *Module) BrandPublic(c *gin.Context) {
	name := strings.TrimSpace(core.GetSetting(m.db, core.KPanelName))
	core.OK(c, gin.H{"name": name, "version": core.Version})
}
