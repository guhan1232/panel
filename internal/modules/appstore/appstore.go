// Package appstore 应用商店：两种部署形态。
//   - compose：Docker Compose 容器化部署（模板生成 compose 文件）
//   - native：Linux 原生二进制部署（下载/解压 + systemd 托管），自动适配 amd64/arm64
package appstore

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

// Param 模板参数定义。
type Param struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Def      string `json:"def"`
	Required bool   `json:"required"`
}

// NativeSpec 原生二进制应用的安装描述。
type NativeSpec struct {
	URLs      map[string]string                       // arch(amd64/arm64) → 下载地址（tar.gz 或裸二进制）
	Archive   bool                                    // 是否为 tar.gz 压缩包
	InArchive string                                  // 压缩包内二进制相对路径，{arch} 会被替换
	Binary    string                                  // 目标二进制文件名
	Args      func(p map[string]string) []string      // ExecStart 参数
	Env       func(p map[string]string) map[string]string
	Files     func(name string, p map[string]string) map[string]string // 附加配置文件（相对安装目录）
	Dirs      []string                                // 需预创建的子目录（如 www/data）
}

// Template 应用模板。
type Template struct {
	Key    string      `json:"key"`
	Name   string      `json:"name"`
	Desc   string      `json:"desc"`
	Kind   string      `json:"kind"` // compose | native
	Params []Param     `json:"params"`
	Gen    func(p map[string]string) (string, error) // compose 用
	Native *NativeSpec                               // native 用
}

// AppInst 已安装实例。
type AppInst struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:40" json:"name"`
	AppKey    string    `gorm:"size:32" json:"app_key"`
	Kind      string    `gorm:"size:16" json:"kind"` // compose | native（历史空值按 compose）
	Params    string    `gorm:"type:text" json:"params"`
	CreatedAt time.Time `json:"created_at"`
}

// ---- 工具 ----

// yamlStr YAML 双引号安全包裹。
func yamlStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// unitQuote systemd 单元文件值转义。
func unitQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func portParam(p map[string]string, def string) (string, error) {
	port := p["port"]
	if port == "" {
		port = def
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("端口非法: %s", port)
	}
	return port, nil
}

func portOf(p map[string]string, key, def string) (string, error) {
	return portParam(map[string]string{"port": p[key]}, def)
}

func required(p map[string]string, key, label string) (string, error) {
	v := p[key]
	if v == "" {
		return "", fmt.Errorf("缺少参数: %s", label)
	}
	return v, nil
}

// ---- 模板库 ----

var templates = []Template{
	{
		Key: "nginx", Name: "Nginx", Desc: "Web 服务器/反向代理（容器）", Kind: "compose",
		Params: []Param{{Key: "port", Label: "宿主机端口", Def: "80", Required: true}},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "80")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  nginx:
    image: nginx:stable
    ports:
      - %q:80
    volumes:
      - ./conf:/etc/nginx/conf.d:ro
      - ./www:/usr/share/nginx/html:ro
    restart: always
`, port), nil
		},
	},
	{
		Key: "mysql", Name: "MySQL 8", Desc: "关系型数据库（容器）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "3306", Required: true},
			{Key: "root_pass", Label: "root 密码", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "3306")
			if err != nil {
				return "", err
			}
			pass, err := required(p, "root_pass", "root 密码")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  mysql:
    image: mysql:8
    ports:
      - %q:3306
    environment:
      MYSQL_ROOT_PASSWORD: %s
    volumes:
      - ./data:/var/lib/mysql
    restart: always
`, port, yamlStr(pass)), nil
		},
	},
	{
		Key: "redis", Name: "Redis 7", Desc: "内存数据库/缓存（容器）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "6379", Required: true},
			{Key: "password", Label: "访问密码（留空=无密码）", Required: false},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "6379")
			if err != nil {
				return "", err
			}
			cmd := ""
			if pass := p["password"]; pass != "" {
				cmd = fmt.Sprintf("    command: [\"redis-server\", \"--requirepass\", %s]\n", yamlStr(pass))
			}
			return fmt.Sprintf(`services:
  redis:
    image: redis:7-alpine
    ports:
      - %q:6379
%s    volumes:
      - ./data:/data
    restart: always
`, port, cmd), nil
		},
	},
	{
		Key: "wordpress", Name: "WordPress", Desc: "博客/CMS（容器，内置数据库）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "8080", Required: true},
			{Key: "root_pass", Label: "数据库 root 密码", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "8080")
			if err != nil {
				return "", err
			}
			pass, err := required(p, "root_pass", "数据库 root 密码")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: %s
      MYSQL_DATABASE: wordpress
    volumes:
      - db_data:/var/lib/mysql
    restart: always
  app:
    image: wordpress:latest
    ports:
      - %q:80
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: root
      WORDPRESS_DB_PASSWORD: %s
    volumes:
      - wp_data:/var/www/html
    depends_on:
      - db
    restart: always
volumes:
  db_data:
  wp_data:
`, yamlStr(pass), port, yamlStr(pass)), nil
		},
	},
	{
		Key: "caddy", Name: "Caddy", Desc: "Web 服务器，自动 HTTPS（原生 v2.8.4）", Kind: "native",
		Params: []Param{{Key: "port", Label: "HTTP 端口", Def: "2015", Required: true}},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://github.com/caddyserver/caddy/releases/download/v2.8.4/caddy_2.8.4_linux_amd64.tar.gz",
				"arm64": "https://github.com/caddyserver/caddy/releases/download/v2.8.4/caddy_2.8.4_linux_arm64.tar.gz",
			},
			Archive: true, InArchive: "caddy", Binary: "caddy",
			Dirs: []string{"www"},
			Args: func(p map[string]string) []string { return []string{"run", "--config", "Caddyfile"} },
			Files: func(name string, p map[string]string) map[string]string {
				port, _ := portOf(p, "port", "2015")
				return map[string]string{
					"Caddyfile": fmt.Sprintf(`{
	admin off
}

:%s {
	root * ./www
	file_server
}
`, port),
				}
			},
		},
	},
	{
		Key: "minio", Name: "MinIO", Desc: "S3 兼容对象存储（原生，RELEASE 版）", Kind: "native",
		Params: []Param{
			{Key: "port", Label: "API 端口", Def: "9000", Required: true},
			{Key: "console_port", Label: "控制台端口", Def: "9001", Required: true},
			{Key: "access_key", Label: "Access Key", Def: "minioadmin", Required: true},
			{Key: "secret_key", Label: "Secret Key", Required: true},
		},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://dl.min.io/server/minio/release/linux-amd64/minio",
				"arm64": "https://dl.min.io/server/minio/release/linux-arm64/minio",
			},
			Archive: false, Binary: "minio",
			Dirs: []string{"data"},
			Env: func(p map[string]string) map[string]string {
				return map[string]string{
					"MINIO_ROOT_USER":     orStr(p, "access_key", "minioadmin"),
					"MINIO_ROOT_PASSWORD": p["secret_key"],
				}
			},
			Args: func(p map[string]string) []string {
				api, _ := portOf(p, "port", "9000")
				console, _ := portOf(p, "console_port", "9001")
				return []string{"server", "./data", "--address", ":" + api, "--console-address", ":" + console}
			},
		},
	},
	{
		Key: "node_exporter", Name: "node_exporter", Desc: "Prometheus 主机指标采集（原生 v1.8.2）", Kind: "native",
		Params: []Param{{Key: "port", Label: "监听端口", Def: "9100", Required: true}},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-amd64.tar.gz",
				"arm64": "https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-arm64.tar.gz",
			},
			Archive: true, InArchive: "node_exporter-1.8.2.linux-{arch}/node_exporter", Binary: "node_exporter",
			Args: func(p map[string]string) []string {
				port, _ := portOf(p, "port", "9100")
				return []string{"--web.listen-address", ":" + port}
			},
		},
	},
	{
		Key: "frps", Name: "FRP 服务端", Desc: "内网穿透服务端（原生 v0.61.1）", Kind: "native",
		Params: []Param{
			{Key: "port", Label: "绑定端口", Def: "7000", Required: true},
			{Key: "token", Label: "认证 Token", Required: true},
		},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://github.com/fatedier/frp/releases/download/v0.61.1/frp_0.61.1_linux_amd64.tar.gz",
				"arm64": "https://github.com/fatedier/frp/releases/download/v0.61.1/frp_0.61.1_linux_arm64.tar.gz",
			},
			Archive: true, InArchive: "frp_0.61.1_linux_{arch}/frps", Binary: "frps",
			Args: func(p map[string]string) []string { return []string{"-c", "frps.toml"} },
			Files: func(name string, p map[string]string) map[string]string {
				port, _ := portOf(p, "port", "7000")
				return map[string]string{
					"frps.toml": fmt.Sprintf("bindAddr = \"0.0.0.0\"\nbindPort = %s\nauth.token = %s\n", port, yamlStr(p["token"])),
				}
			},
		},
	},
}

func orStr(p map[string]string, key, def string) string {
	if p[key] == "" {
		return def
	}
	return p[key]
}

// ---- 模块 ----

type Module struct {
	db      *gorm.DB
	dataDir string
}

func New(db *gorm.DB, dataDir string) *Module {
	return &Module{db: db, dataDir: dataDir}
}

func (m *Module) Start() error {
	return m.db.AutoMigrate(&AppInst{})
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/apps")
	g.GET("/templates", listTemplates)
	g.GET("/docker/status", dockerStatus)
	g.POST("/docker/install", installDocker)
	g.GET("", m.list)
	g.POST("", m.install)
	g.POST("/:id/start", m.start)
	g.POST("/:id/stop", m.stop)
	g.POST("/:id/restart", m.restart)
	g.GET("/:id/logs", m.logs)
	g.POST("/:id/uninstall", m.uninstall)
}

// 目录

func (m *Module) dir(name string) string { return filepath.Join(m.dataDir, "apps", name) }

func (m *Module) nativeDir(name string) string {
	return filepath.Join(m.dataDir, "apps-native", name)
}

func (m *Module) composeFile(name string) string {
	return filepath.Join(m.dir(name), "docker-compose.yml")
}

func (m *Module) unitName(name string) string { return "webmblx-app-" + name }

func composeArgs(file string, args ...string) []string {
	return append([]string{"compose", "-f", file}, args...)
}

// ---- Docker 环境 ----

func dockerStatus(c *gin.Context) {
	out, _, err := execx.Run("appstore", 20*time.Second, "docker", "--version")
	installed := err == nil
	composeOK := false
	if installed {
		if _, _, err2 := execx.Run("appstore", 20*time.Second, "docker", "compose", "version"); err2 == nil {
			composeOK = true
		}
	}
	core.OK(c, gin.H{"installed": installed, "version": strings.TrimSpace(out), "compose": composeOK})
}

func installDocker(c *gin.Context) {
	script := `curl -fsSL https://get.docker.com | sh && systemctl enable --now docker && docker compose version`
	out, code, err := execx.RunShell("appstore:install-docker", 30*time.Minute, script)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("Docker 安装失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, nil)
}

// ---- 路由处理 ----

func listTemplates(c *gin.Context) {
	// 模板的 Gen/Native 为函数字段，JSON 不支持，输出 DTO
	out := make([]gin.H, 0, len(templates))
	for _, t := range templates {
		out = append(out, gin.H{"key": t.Key, "name": t.Name, "desc": t.Desc, "kind": t.Kind, "params": t.Params})
	}
	core.OK(c, out)
}

func findTemplate(key string) *Template {
	for i := range templates {
		if templates[i].Key == key {
			return &templates[i]
		}
	}
	return nil
}

func (m *Module) list(c *gin.Context) {
	var rows []AppInst
	m.db.Order("id").Find(&rows)
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		status := m.statusText(&r)
		items = append(items, gin.H{"app": r, "status": status})
	}
	core.OK(c, items)
}

func (m *Module) statusText(r *AppInst) string {
	if r.Kind == "native" {
		out, _, err := execx.Run("appstore", 10*time.Second, "systemctl", "is-active", m.unitName(r.Name))
		state := strings.TrimSpace(out)
		if err != nil || state == "" {
			return "未知（systemd 不可用）"
		}
		enableOut, _, _ := execx.Run("appstore", 10*time.Second, "systemctl", "is-enabled", m.unitName(r.Name))
		return fmt.Sprintf("%s (%s)", state, strings.TrimSpace(enableOut))
	}
	out, _, err := execx.Run("appstore", 15*time.Second,
		"docker", composeArgs(m.composeFile(r.Name), "ps")...)
	if err != nil {
		return "未知（docker 不可用或未运行）"
	}
	status := strings.TrimSpace(out)
	if status == "" {
		status = "无运行容器"
	}
	return status
}

func (m *Module) install(c *gin.Context) {
	var req struct {
		AppKey string            `json:"app_key" binding:"required"`
		Name   string            `json:"name" binding:"required"`
		Params map[string]string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !nameRe.MatchString(req.Name) {
		core.Err(c, 400, "实例名非法（小写字母数字短横线，3-40 位）")
		return
	}
	tpl := findTemplate(req.AppKey)
	if tpl == nil {
		core.Err(c, 404, "模板不存在")
		return
	}
	var count int64
	m.db.Model(&AppInst{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		core.Err(c, 400, "同名实例已存在")
		return
	}
	p := orParams(req.Params)

	var status string
	if tpl.Kind == "native" {
		s, err := m.installNative(tpl, req.Name, p)
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		status = s
	} else {
		s, err := m.installCompose(tpl, req.Name, p)
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		status = s
	}

	row := AppInst{Name: req.Name, AppKey: req.AppKey, Kind: tpl.Kind}
	if data, jerr := json.Marshal(req.Params); jerr == nil {
		row.Params = string(data)
	}
	if err := m.db.Create(&row).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("应用已部署", "app", req.AppKey, "name", req.Name, "kind", tpl.Kind)
	core.OK(c, gin.H{"app": row, "status": status})
}

// ---- compose 安装 ----

func (m *Module) installCompose(tpl *Template, name string, p map[string]string) (string, error) {
	content, err := tpl.Gen(p)
	if err != nil {
		return "", err
	}
	dir := m.dir(name)
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "www"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(m.composeFile(name), []byte(content), 0o644); err != nil {
		return "", err
	}
	out, code, err := execx.Run("appstore", 15*time.Minute,
		"docker", composeArgs(m.composeFile(name), "up", "-d")...)
	if err != nil {
		_ = os.RemoveAll(dir)
		msg := tailOf(out)
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("部署失败(exit %d): %s", code, msg)
	}
	return "容器已启动", nil
}

// ---- native 安装 ----

func (m *Module) installNative(tpl *Template, name string, p map[string]string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("原生应用部署仅支持在 Linux 服务器上执行")
	}
	spec := tpl.Native
	arch := runtime.GOARCH
	rawURL, ok := spec.URLs[arch]
	if !ok {
		return "", fmt.Errorf("模板暂不支持架构 %s", arch)
	}
	dir := m.nativeDir(name)
	_ = os.MkdirAll(dir, 0o755)
	fail := func(err error) error {
		_ = os.RemoveAll(dir)
		return err
	}

	// 1. 下载
	local := filepath.Join(dir, "download.tmp")
	if _, code, err := execx.Run("appstore", 15*time.Minute, "curl", "-fSL", "--retry", "3", "--connect-timeout", "30", "-o", local, rawURL); err != nil {
		return "", fail(fmt.Errorf("下载失败(exit %d): %s", code, rawURL))
	}

	// 2. 解压或直接使用
	var dst string
	if spec.Archive {
		if err := archive.UnTarGz(local, dir); err != nil {
			return "", fail(fmt.Errorf("解压失败: %w", err))
		}
		_ = os.Remove(local)
		src := filepath.Join(dir, strings.ReplaceAll(spec.InArchive, "{arch}", arch))
		if _, err := os.Stat(src); err != nil {
			return "", fail(fmt.Errorf("压缩包内未找到 %s", spec.InArchive))
		}
		dst = filepath.Join(dir, spec.Binary)
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			return "", fail(err)
		}
	} else {
		dst = filepath.Join(dir, spec.Binary)
		if err := os.Rename(local, dst); err != nil {
			return "", fail(err)
		}
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", fail(err)
	}

	// 3. 附加目录与配置文件
	for _, d := range spec.Dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return "", fail(err)
		}
	}
	if spec.Files != nil {
		for rel, content := range spec.Files(name, p) {
			if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
				return "", fail(err)
			}
		}
	}

	// 4. systemd 单元
	unit := buildUnit(tpl, name, dir, dst, spec, p)
	unitPath := filepath.Join("/etc/systemd/system", m.unitName(name)+".service")
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return "", fail(fmt.Errorf("写入 systemd 单元失败: %w", err))
	}
	if _, _, err := execx.Run("appstore", 30*time.Second, "systemctl", "daemon-reload"); err != nil {
		return "", fail(fmt.Errorf("daemon-reload 失败: %w", err))
	}

	// 5. 启动
	if out, code, err := execx.Run("appstore", 60*time.Second, "systemctl", "enable", "--now", m.unitName(name)); err != nil {
		return "", fail(fmt.Errorf("服务启动失败(exit %d): %s", code, tailOf(out)))
	}
	time.Sleep(2 * time.Second)
	stateOut, _, _ := execx.Run("appstore", 10*time.Second, "systemctl", "is-active", m.unitName(name))
	state := strings.TrimSpace(stateOut)
	if state != "active" {
		logs, _, _ := execx.Run("appstore", 15*time.Second, "journalctl", "-u", m.unitName(name), "-n", "30", "--no-pager")
		return "", fail(fmt.Errorf("服务未进入 active 状态（当前 %s），最近日志:\n%s", state, tailOf(logs)))
	}
	return fmt.Sprintf("服务已启动（%s，目录 %s）", m.unitName(name), dir), nil
}

func buildUnit(tpl *Template, name, dir, binary string, spec *NativeSpec, p map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=webmblx app: %s (%s)\nAfter=network.target\n\n[Service]\nType=simple\n", tpl.Name, name)
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", dir)
	fmt.Fprintf(&b, "ExecStart=%s", binary)
	if spec.Args != nil {
		for _, a := range spec.Args(p) {
			fmt.Fprintf(&b, " \"%s\"", unitQuote(a))
		}
	}
	b.WriteString("\n")
	if spec.Env != nil {
		for k, v := range spec.Env(p) {
			fmt.Fprintf(&b, "Environment=\"%s=%s\"\n", k, unitQuote(v))
		}
	}
	b.WriteString("Restart=always\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n")
	return b.String()
}

// ---- 运维操作（按 kind 分发）----

func (m *Module) loadInst(c *gin.Context) (*AppInst, bool) {
	id, _ := strconv.Atoi(c.Param("id"))
	var r AppInst
	if err := m.db.First(&r, id).Error; err != nil {
		core.Err(c, 404, "实例不存在")
		return nil, false
	}
	return &r, true
}

func (m *Module) start(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var code int
	var err error
	if r.Kind == "native" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "start", m.unitName(r.Name))
	} else {
		out, code, err = execx.Run("appstore", 15*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "up", "-d")...)
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("启动失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) stop(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var code int
	var err error
	if r.Kind == "native" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "stop", m.unitName(r.Name))
	} else {
		out, code, err = execx.Run("appstore", 5*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "stop")...)
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("停止失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) restart(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var code int
	var err error
	if r.Kind == "native" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "restart", m.unitName(r.Name))
	} else {
		out, code, err = execx.Run("appstore", 5*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "restart")...)
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("重启失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) logs(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var err error
	if r.Kind == "native" {
		out, _, err = execx.Run("appstore", 30*time.Second, "journalctl", "-u", m.unitName(r.Name), "-n", "200", "--no-pager")
	} else {
		out, _, err = execx.Run("appstore", 30*time.Second, "docker", composeArgs(m.composeFile(r.Name), "logs", "--tail=200")...)
	}
	if err != nil && out == "" {
		core.Err(c, 404, "日志不可用")
		return
	}
	core.OK(c, gin.H{"content": out})
}

func (m *Module) uninstall(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var req struct {
		RemoveData bool `json:"remove_data"`
	}
	_ = c.ShouldBindJSON(&req)

	if r.Kind == "native" {
		_, _, _ = execx.Run("appstore", 60*time.Second, "systemctl", "disable", "--now", m.unitName(r.Name))
		_ = os.Remove(filepath.Join("/etc/systemd/system", m.unitName(r.Name)+".service"))
		_, _, _ = execx.Run("appstore", 30*time.Second, "systemctl", "daemon-reload")
	} else {
		_, _, _ = execx.Run("appstore", 10*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "down")...)
	}
	m.db.Delete(&r)
	if req.RemoveData {
		if r.Kind == "native" {
			_ = os.RemoveAll(m.nativeDir(r.Name))
		} else {
			_ = os.RemoveAll(m.dir(r.Name))
		}
	}
	core.OK(c, nil)
}

// ---- 工具 ----

func orParams(p map[string]string) map[string]string {
	if p == nil {
		return map[string]string{}
	}
	return p
}

func tailOf(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1024 {
		return "..." + s[len(s)-1024:]
	}
	return s
}
