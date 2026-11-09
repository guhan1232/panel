// Package website 网站管理：基于 Nginx 的静态站/PHP 站/反向代理。
// 配置生成到 /etc/nginx/conf.d/hanyuxin_<name>.conf，写入前 nginx -t 校验，失败自动回滚。
package website

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

const confDir = "/etc/nginx/conf.d"

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,62}[a-z0-9]$`)

// Site 站点定义。
type Site struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"uniqueIndex;size:64" json:"name"` // 小写域名式标识
	Type       string    `gorm:"size:16" json:"type"`             // static | php | proxy
	Root       string    `gorm:"size:256" json:"root"`            // 静态/PHP 站根目录
	PHPSocket  string    `gorm:"size:128" json:"php_socket"`      // unix:/run/php/php-fpm.sock 或 127.0.0.1:9000
	Upstream   string    `gorm:"size:256" json:"upstream"`        // proxy_pass 目标
	Locations  string    `gorm:"type:text" json:"locations"`      // 附加反代规则 JSON: [{prefix,target}]
	Domains    string    `gorm:"size:512" json:"domains"`         // 空格分隔
	SSLCert    string    `gorm:"size:256" json:"ssl_cert"`
	SSLKey     string    `gorm:"size:256" json:"ssl_key"`
	ForceHTTPS bool      `json:"force_https"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type proxyLoc struct {
	Prefix string `json:"prefix"`
	Target string `json:"target"`
}

type Module struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Module { return &Module{db: db} }

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/sites")
	g.GET("", m.list)
	g.POST("", m.create)
	g.PUT("/:id", m.update)
	g.DELETE("/:id", m.remove)
	g.POST("/:id/toggle", m.toggle)
	g.GET("/:id/log", m.siteLog)
	g.GET("/nginx/status", nginxStatus)
	g.POST("/nginx/reload", nginxReload)
	g.POST("/nginx/install", nginxInstall)
}

func (m *Module) Start() error {
	return m.db.AutoMigrate(&Site{})
}

// ---- 配置生成 ----

func splitDomains(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == ';' || r == '\n' })
	out := fields[:0]
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func genConf(s *Site) string {
	domains := strings.Join(splitDomains(s.Domains), " ")
	logAcc := fmt.Sprintf("/var/log/nginx/hanyuxin_%s.access.log", s.Name)
	logErr := fmt.Sprintf("/var/log/nginx/hanyuxin_%s.error.log", s.Name)
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	if s.SSLCert != "" && s.SSLKey != "" {
		if s.ForceHTTPS {
			w("server {\n    listen 80;\n    server_name %s;\n    return 301 https://$host$request_uri;\n}\n\n", domains)
		}
		w("server {\n    listen 443 ssl;\n    server_name %s;\n", domains)
		w("    ssl_certificate %s;\n    ssl_certificate_key %s;\n", s.SSLCert, s.SSLKey)
	} else {
		w("server {\n    listen 80;\n    server_name %s;\n", domains)
	}
	w("    access_log %s;\n    error_log %s;\n", logAcc, logErr)

	switch s.Type {
	case "static":
		w("    root %s;\n    index index.html index.htm;\n", s.Root)
		w("    location / {\n        try_files $uri $uri/ /index.html;\n    }\n")
	case "php":
		w("    root %s;\n    index index.php index.html;\n", s.Root)
		w("    location / {\n        try_files $uri $uri/ /index.php?$query_string;\n    }\n")
		sock := s.PHPSocket
		if sock == "" {
			sock = "/run/php/php-fpm.sock"
		}
		if strings.HasPrefix(sock, "/") {
			sock = "unix:" + sock
		}
		w("    location ~ \\.php$ {\n        fastcgi_pass %s;\n        fastcgi_index index.php;\n        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n        include fastcgi_params;\n    }\n", sock)
	case "proxy":
		w("    location / {\n        proxy_pass %s;\n", s.Upstream)
		w("        proxy_set_header Host $host;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_set_header X-Forwarded-Proto $scheme;\n        proxy_http_version 1.1;\n        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection \"upgrade\";\n        proxy_read_timeout 300s;\n    }\n")
		var extra []proxyLoc
		_ = json.Unmarshal([]byte(s.Locations), &extra)
		for _, l := range extra {
			if l.Prefix == "" || l.Target == "" {
				continue
			}
			w("    location %s {\n        proxy_pass %s;\n        proxy_set_header Host $host;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_set_header X-Forwarded-Proto $scheme;\n        proxy_http_version 1.1;\n        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection \"upgrade\";\n    }\n", l.Prefix, l.Target)
		}
	}
	w("}\n")
	return b.String()
}

func confPath(name string, enabled bool) string {
	if enabled {
		return filepath.Join(confDir, "hanyuxin_"+name+".conf")
	}
	return filepath.Join(confDir, "hanyuxin_"+name+".conf.off")
}

// writeConfWithCheck 写入配置并 nginx -t 校验；失败恢复原内容。
func writeConfWithCheck(path, content string) error {
	if _, err := os.Stat(confDir); err != nil {
		return fmt.Errorf("未找到 %s，请先安装 nginx", confDir)
	}
	old, oldErr := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	out, code, err := execx.Run("website", 30*time.Second, "nginx", "-t")
	if err != nil {
		if oldErr == nil {
			_ = os.WriteFile(path, old, 0o644)
		} else {
			_ = os.Remove(path)
		}
		return fmt.Errorf("nginx 配置校验失败(exit %d)，已回滚: %s", code, strings.TrimSpace(out))
	}
	return nil
}

func reloadNginx() (string, error) {
	out, code, err := execx.Run("website", 30*time.Second, "nginx", "-s", "reload")
	if err != nil {
		return out, fmt.Errorf("reload 失败(exit %d): %s", code, strings.TrimSpace(out))
	}
	return "", nil
}

// ---- 路由处理 ----

func nginxStatus(c *gin.Context) {
	out, _, err := execx.Run("website", 15*time.Second, "nginx", "-v")
	installed := err == nil
	version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "nginx version:"))
	confOK := false
	if installed {
		if _, err := os.Stat(confDir); err == nil {
			confOK = true
		}
	}
	core.OK(c, gin.H{"installed": installed, "version": version, "conf_dir_ok": confOK, "conf_dir": confDir})
}

func nginxReload(c *gin.Context) {
	if out, err := reloadNginx(); err != nil {
		core.Err(c, 500, out+" "+err.Error())
		return
	}
	core.OK(c, nil)
}

func nginxInstall(c *gin.Context) {
	script := `if command -v apt-get >/dev/null 2>&1; then apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y nginx; elif command -v dnf >/dev/null 2>&1; then dnf install -y nginx && systemctl enable --now nginx; elif command -v yum >/dev/null 2>&1; then yum install -y nginx && systemctl enable --now nginx; else echo "未找到 apt-get/dnf/yum"; exit 1; fi`
	out, code, err := execx.RunShell("website:install", 10*time.Minute, script)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("安装失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, nil)
}

func (m *Module) list(c *gin.Context) {
	var sites []Site
	if err := m.db.Order("id").Find(&sites).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	items := make([]gin.H, 0, len(sites))
	for _, s := range sites {
		_, err := os.Stat(confPath(s.Name, true))
		items = append(items, gin.H{
			"site": s, "conf_exists": err == nil,
		})
	}
	core.OK(c, items)
}

func (m *Module) validate(s *Site) error {
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("站点标识非法（小写字母/数字/点/短横线，2-64 位）")
	}
	switch s.Type {
	case "static", "php":
		if s.Root == "" || !filepath.IsAbs(s.Root) {
			return fmt.Errorf("站点根目录必须为绝对路径")
		}
	case "proxy":
		if s.Upstream == "" {
			return fmt.Errorf("反代目标不能为空")
		}
		if !strings.HasPrefix(s.Upstream, "http://") && !strings.HasPrefix(s.Upstream, "https://") &&
			!strings.HasPrefix(s.Upstream, "unix:") {
			return fmt.Errorf("反代目标需以 http:// https:// 或 unix: 开头")
		}
	default:
		return fmt.Errorf("不支持的站点类型: %s", s.Type)
	}
	if len(splitDomains(s.Domains)) == 0 {
		return fmt.Errorf("至少填写一个域名")
	}
	if (s.SSLCert == "") != (s.SSLKey == "") {
		return fmt.Errorf("证书与私钥需同时提供")
	}
	return nil
}

func (m *Module) create(c *gin.Context) {
	var s Site
	if err := c.ShouldBindJSON(&s); err != nil {
		core.BindErr(c, err)
		return
	}
	s.ID = 0
	s.Enabled = true
	if err := m.validate(&s); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	if err := m.db.Create(&s).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := writeConfWithCheck(confPath(s.Name, true), genConf(&s)); err != nil {
		m.db.Delete(&s)
		core.Err(c, 400, err.Error())
		return
	}
	if out, err := reloadNginx(); err != nil {
		core.Err(c, 400, "站点已创建，但 "+out+" "+err.Error())
		return
	}
	core.OK(c, s)
}

func (m *Module) update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var s Site
	if err := m.db.First(&s, id).Error; err != nil {
		core.Err(c, 404, "站点不存在")
		return
	}
	var req Site
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	oldName := s.Name
	s.Type, s.Root, s.PHPSocket = req.Type, req.Root, req.PHPSocket
	s.Upstream, s.Locations, s.Domains = req.Upstream, req.Locations, req.Domains
	s.SSLCert, s.SSLKey, s.ForceHTTPS = req.SSLCert, req.SSLKey, req.ForceHTTPS
	if err := m.validate(&s); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	if err := m.db.Save(&s).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if oldName != s.Name {
		_ = os.Remove(confPath(oldName, true))
		_ = os.Remove(confPath(oldName, false))
	}
	if err := writeConfWithCheck(confPath(s.Name, true), genConf(&s)); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	if _, err := reloadNginx(); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	core.OK(c, s)
}

func (m *Module) remove(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var s Site
	if err := m.db.First(&s, id).Error; err != nil {
		core.Err(c, 404, "站点不存在")
		return
	}
	_ = os.Remove(confPath(s.Name, true))
	_ = os.Remove(confPath(s.Name, false))
	m.db.Delete(&s)
	if _, err := reloadNginx(); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) toggle(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var s Site
	if err := m.db.First(&s, id).Error; err != nil {
		core.Err(c, 404, "站点不存在")
		return
	}
	s.Enabled = !s.Enabled
	if err := m.db.Save(&s).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	target := confPath(s.Name, s.Enabled)
	// 切换：删除两个可能的旧文件后按新状态写
	_ = os.Remove(confPath(s.Name, true))
	_ = os.Remove(confPath(s.Name, false))
	if s.Enabled {
		if err := writeConfWithCheck(target, genConf(&s)); err != nil {
			s.Enabled = false
			m.db.Save(&s)
			core.Err(c, 400, err.Error())
			return
		}
	}
	if _, err := reloadNginx(); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	core.OK(c, gin.H{"enabled": s.Enabled})
}

func (m *Module) siteLog(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var s Site
	if err := m.db.First(&s, id).Error; err != nil {
		core.Err(c, 404, "站点不存在")
		return
	}
	lines := c.DefaultQuery("lines", "100")
	if n, err := strconv.Atoi(lines); err != nil || n < 1 || n > 1000 {
		lines = "100"
	}
	kind := c.DefaultQuery("type", "access")
	if kind != "access" && kind != "error" {
		kind = "access"
	}
	path := fmt.Sprintf("/var/log/nginx/hanyuxin_%s.%s.log", s.Name, kind)
	out, _, err := execx.Run("website", 15*time.Second, "tail", "-n", lines, path)
	if err != nil && out == "" {
		core.Err(c, 404, "日志文件不存在或不可读: "+path)
		return
	}
	core.OK(c, gin.H{"path": path, "content": out})
}
