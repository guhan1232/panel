// NGINX 接管：服务启停、系统站点扫描导入、配置文件查看/编辑（宝塔式管理）。
package website

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

// nginxAction 服务控制：start | stop | restart | reload。
func nginxAction(c *gin.Context) {
	var req struct {
		Action string `binding:"required,oneof=start stop restart reload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	var out string
	var code int
	var err error
	switch req.Action {
	case "start":
		out, code, err = execx.Run("website", 60*time.Second, "systemctl", "start", "nginx")
	case "stop":
		out, code, err = execx.Run("website", 60*time.Second, "systemctl", "stop", "nginx")
	case "restart":
		out, code, err = execx.Run("website", 60*time.Second, "systemctl", "restart", "nginx")
	case "reload":
		out, code, err = execx.Run("website", 30*time.Second, "nginx", "-s", "reload")
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("%s 失败(exit %d): %s", req.Action, code, strings.TrimSpace(out)))
		return
	}
	slog.Info("nginx 服务操作", "action", req.Action, "by", c.GetString("user"))
	core.OK(c, gin.H{"ok": true, "action": req.Action})
}

// confDirs 扫描的配置目录（conf.d + sites-enabled）。
var confDirs = []string{"/etc/nginx/conf.d", "/etc/nginx/sites-enabled"}

// confDirExists 兼容不同发行版的 include 目录。
func confDirExists(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

// nginxConfigList 配置文件清单（conf.d 与 sites-enabled）。
func (m *Module) nginxConfigList(c *gin.Context) {
	items := []gin.H{}
	for _, dir := range confDirs {
		if !confDirExists(dir) {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
				continue
			}
			info, _ := e.Info()
			items = append(items, gin.H{
				"file":    e.Name(),
				"dir":     dir,
				"size":    info.Size(),
				"time":    info.ModTime().Format("2006-01-02 15:04:05"),
				"system":  strings.HasPrefix(e.Name(), "hanyuxin_"),
				"managed": m.managedConf(e.Name()),
			})
		}
	}
	core.OK(c, items)
}

// managedConf 判断 conf 是否由面板站点生成（hanyuxin_<name>.conf）。
func (m *Module) managedConf(fileName string) bool {
	if !strings.HasPrefix(fileName, "hanyuxin_") {
		return false
	}
	var cnt int64
	m.db.Model(&Site{}).Where("system = ?", false).Count(&cnt)
	_ = cnt
	rows := []Site{}
	m.db.Find(&rows)
	for _, r := range rows {
		if confPath(r.Name, true) == filepath.Join("/etc/nginx/conf.d", fileName) {
			return true
		}
	}
	return false
}

// nginxConfigRead 读取配置文件内容。
func nginxConfigRead(c *gin.Context) {
	file := c.Query("file")
	dir := c.Query("dir")
	if !isSafeConfName(file) || !isConfDirAllowed(dir) {
		core.Err(c, 400, "路径非法")
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		core.Err(c, 404, "文件不存在")
		return
	}
	core.OK(c, gin.H{"file": file, "dir": dir, "content": string(data)})
}

// nginxConfigSave 保存配置文件：nginx -t 校验（失败不写入）→ 写入 → reload。
func nginxConfigSave(c *gin.Context) {
	var req struct {
		File    string `binding:"required"`
		Dir     string `binding:"required"`
		Content string `binding:"required"`
		Reload  bool   `json:"reload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !isSafeConfName(req.File) || !isConfDirAllowed(req.Dir) {
		core.Err(c, 400, "路径非法")
		return
	}
	full := filepath.Join(req.Dir, req.File)
	// nginx -t 校验：写入临时文件并临时 include 校验不可行——直接备份原文件后写入，nginx -t 失败则回滚
	bak := full + ".bak-" + strconv.FormatInt(time.Now().Unix(), 10)
	orig, origErr := os.ReadFile(full)
	if origErr == nil {
		_ = os.WriteFile(bak, orig, 0o644)
	}
	if err := os.WriteFile(full, []byte(req.Content), 0o644); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if out, code, err := execx.Run("website", 30*time.Second, "nginx", "-t"); err != nil {
		// 回滚
		if origErr == nil {
			_ = os.WriteFile(full, orig, 0o644)
		} else {
			_ = os.Remove(full)
		}
		core.Err(c, 400, fmt.Sprintf("nginx -t 校验失败(exit %d)，已回滚: %s", code, strings.TrimSpace(out)))
		return
	}
	if req.Reload {
		if out, err := reloadNginx(); err != nil {
			core.Err(c, 400, out+" "+err.Error())
			return
		}
	}
	slog.Info("nginx 配置已保存", "file", req.File, "by", c.GetString("user"))
	core.OK(c, gin.H{"ok": true})
}

// isSafeConfName 配置文件名防穿越。
func isSafeConfName(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") || strings.HasPrefix(name, ".") {
		return false
	}
	return strings.HasSuffix(name, ".conf") && filepath.Base(name) == name
}

// isConfDirAllowed 只允许系统 nginx 配置目录。
func isConfDirAllowed(dir string) bool {
	for _, d := range confDirs {
		if dir == d {
			return true
		}
	}
	return false
}

// ---- 系统站点扫描导入 ----

type parsedServer struct {
	ServerName string
	Listen443  bool
	SSLCert    string
	SSLKey     string
	Root       string
	ProxyPass  string
	PHP        bool
	ForceHTTPS bool
}

var serverNameRe = regexp.MustCompile(`server_name\s+([^;]+);`)
var sslCertRe = regexp.MustCompile(`ssl_certificate\s+([^;]+);`)
var sslKeyRe = regexp.MustCompile(`ssl_certificate_key\s+([^;]+);`)
var rootRe = regexp.MustCompile(`(?m)^\s*root\s+([^;]+);`)
var proxyPassRe = regexp.MustCompile(`proxy_pass\s+(https?://[^;]+);`)
var listenRe = regexp.MustCompile(`listen\s+[^;]*443[^;]*;`)
var phpRe = regexp.MustCompile(`fastcgi_pass`)
var httpsRedirectRe = regexp.MustCompile(`return\s+301\s+https`)

// parseServers 拆分 conf 文本中的 server 块并提取关键信息。
func parseServers(content string) []parsedServer {
	var out []parsedServer
	depth := 0
	var buf strings.Builder
	inServer := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inServer && strings.HasPrefix(trimmed, "server") && strings.HasSuffix(trimmed, "{") {
			inServer = true
			depth = 1
			buf.Reset()
			continue
		}
		if inServer {
			buf.WriteString(line + "\n")
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				body := buf.String()
				m := serverNameRe.FindStringSubmatch(body)
				if m != nil {
					ps := parsedServer{ServerName: strings.TrimSpace(m[1])}
					if listenRe.MatchString(body) {
						ps.Listen443 = true
					}
					if mm := sslCertRe.FindStringSubmatch(body); mm != nil {
						ps.SSLCert = strings.TrimSpace(mm[1])
					}
					if mm := sslKeyRe.FindStringSubmatch(body); mm != nil {
						ps.SSLKey = strings.TrimSpace(mm[1])
					}
					if mm := rootRe.FindStringSubmatch(body); mm != nil {
						ps.Root = strings.TrimSpace(mm[1])
					}
					if mm := proxyPassRe.FindStringSubmatch(body); mm != nil {
						ps.ProxyPass = strings.TrimSpace(mm[1])
					}
					ps.PHP = phpRe.MatchString(body)
					ps.ForceHTTPS = httpsRedirectRe.MatchString(body)
					out = append(out, ps)
				}
				inServer = false
			}
		}
	}
	return out
}

// nginxImportScan 扫描系统 nginx 配置（conf.d + sites-enabled），解析 server 块导入为面板站点。
// 已导入（同名）跳过；面板自管 conf（hanyuxin_ 前缀）跳过；80 跳转段与 443 段合并为一个站点。
func (m *Module) nginxImportScan(c *gin.Context) {
	type importItem struct {
		File   string `json:"file"`
		Name   string `json:"name"`
		Domain string `json:"domain"`
		Type   string `json:"type"`
		Status string `json:"status"` // imported | exists | skipped
	}
	results := []importItem{}
	imported := 0
	exists := 0
	for _, dir := range confDirs {
		if !confDirExists(dir) {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") || strings.HasPrefix(e.Name(), "hanyuxin_") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			servers := parseServers(string(data))
			if len(servers) == 0 {
				continue
			}
			// 合并：同 server_name 首域取第一个 server（443 优先）
			main := servers[0]
			for _, sv := range servers {
				if sv.Listen443 {
					main = sv
					break
				}
			}
			name := strings.TrimSuffix(e.Name(), ".conf")
			name = strings.ReplaceAll(name, ".", "-")
			if !nameRe.MatchString(name) {
				name = "site-" + strconv.Itoa(imported+exists+1)
			}
			var cnt int64
			m.db.Model(&Site{}).Where("name = ?", name).Count(&cnt)
			if cnt > 0 {
				exists++
				results = append(results, importItem{File: e.Name(), Name: name, Domain: main.ServerName, Status: "exists"})
				continue
			}
			typ := "proxy"
			root := ""
			if main.Root != "" && main.ProxyPass == "" {
				typ = "static"
				root = main.Root
			}
			if main.PHP && main.Root != "" {
				typ = "php"
			}
			row := Site{
				Name: name, Type: typ, Root: root,
				Domains: main.ServerName,
				Upstream: main.ProxyPass,
				SSLCert: main.SSLCert, SSLKey: main.SSLKey,
				ForceHTTPS: main.ForceHTTPS,
				Enabled:    true,
				System:     false,
			}
			if err := m.db.Create(&row).Error; err != nil {
				results = append(results, importItem{File: e.Name(), Name: name, Domain: main.ServerName, Status: "error"})
				continue
			}
			imported++
			results = append(results, importItem{File: e.Name(), Name: name, Domain: main.ServerName, Type: typ, Status: "imported"})
		}
	}
	slog.Info("系统站点扫描完成", "imported", imported, "exists", exists)
	core.OK(c, gin.H{"imported": imported, "exists": exists, "items": results})
}

var _ = archive.ZipAt
