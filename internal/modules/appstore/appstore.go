// Package appstore 应用商店：基于 Docker Compose 的应用一键部署（模板化）。
// 实例部署在 <dataDir>/apps/<name>/docker-compose.yml，支持启停/重启/日志/卸载。
package appstore

import (
	"encoding/json"
	"fmt"
	"log/slog"
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

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

// Param 模板参数定义。
type Param struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Def      string `json:"def"`
	Required bool   `json:"required"`
}

// Template 应用模板。
type Template struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Desc   string  `json:"desc"`
	Params []Param `json:"params"`
	Gen    func(p map[string]string) (string, error)
}

// AppInst 已安装实例。
type AppInst struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:40" json:"name"`
	AppKey    string    `gorm:"size:32" json:"app_key"`
	Params    string    `gorm:"type:text" json:"params"`
	CreatedAt time.Time `json:"created_at"`
}

// yamlStr YAML 双引号字符串安全包裹。
func yamlStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
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

func required(p map[string]string, key, label string) (string, error) {
	v := p[key]
	if v == "" {
		return "", fmt.Errorf("缺少参数: %s", label)
	}
	return v, nil
}

var templates = []Template{
	{
		Key: "nginx", Name: "Nginx", Desc: "高性能 Web 服务器/反向代理",
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
		Key: "mysql", Name: "MySQL 8", Desc: "关系型数据库",
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
		Key: "redis", Name: "Redis 7", Desc: "内存数据库/缓存",
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
		Key: "wordpress", Name: "WordPress", Desc: "博客/CMS（内置数据库）",
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
}

type Module struct {
	db      *gorm.DB
	dataDir string
}

func New(db *gorm.DB, dataDir string) *Module {
	return &Module{db: db, dataDir: dataDir}
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

func (m *Module) dir(name string) string { return filepath.Join(m.dataDir, "apps", name) }

func (m *Module) composeFile(name string) string {
	return filepath.Join(m.dir(name), "docker-compose.yml")
}

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
	// 模板的 Gen 为函数字段，JSON 不支持，输出 DTO
	out := make([]gin.H, 0, len(templates))
	for _, t := range templates {
		out = append(out, gin.H{"key": t.Key, "name": t.Name, "desc": t.Desc, "params": t.Params})
	}
	core.OK(c, out)
}

func (m *Module) list(c *gin.Context) {
	var rows []AppInst
	m.db.Order("id").Find(&rows)
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out, _, err := execx.Run("appstore", 15*time.Second,
			"docker", composeArgs(m.composeFile(r.Name), "ps")...)
		status := strings.TrimSpace(out)
		if err != nil {
			status = "未知（docker 不可用或未运行）"
		}
		if status == "" {
			status = "无运行容器"
		}
		items = append(items, gin.H{"app": r, "status": status})
	}
	core.OK(c, items)
}

func (m *Module) install(c *gin.Context) {
	var req struct {
		AppKey string            `binding:"required"`
		Name   string            `binding:"required"`
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
	var tpl *Template
	for i := range templates {
		if templates[i].Key == req.AppKey {
			tpl = &templates[i]
			break
		}
	}
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
	content, err := tpl.Gen(orParams(req.Params))
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	dir := m.dir(req.Name)
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Join(dir, "www"), 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.WriteFile(m.composeFile(req.Name), []byte(content), 0o644); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	out, code, err := execx.Run("appstore", 15*time.Minute,
		"docker", composeArgs(m.composeFile(req.Name), "up", "-d")...)
	if err != nil {
		_ = os.RemoveAll(dir)
		core.Err(c, 500, fmt.Sprintf("部署失败(exit %d): %s", code, tailOf(out)))
		return
	}
	row := AppInst{Name: req.Name, AppKey: req.AppKey}
	if data, jerr := json.Marshal(req.Params); jerr == nil {
		row.Params = string(data)
	}
	if err := m.db.Create(&row).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("应用已部署", "app", req.AppKey, "name", req.Name)
	core.OK(c, row)
}

func (m *Module) action(c *gin.Context, timeout time.Duration, args ...string) (string, int, error) {
	id, _ := strconv.Atoi(c.Param("id"))
	var r AppInst
	if err := m.db.First(&r, id).Error; err != nil {
		core.Err(c, 404, "实例不存在")
		return "", 0, fmt.Errorf("not found")
	}
	return execx.Run("appstore", timeout, "docker", composeArgs(m.composeFile(r.Name), args...)...)
}

func (m *Module) start(c *gin.Context) {
	if out, code, err := m.action(c, 15*time.Minute, "up", "-d"); err != nil {
		core.Err(c, 500, fmt.Sprintf("启动失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) stop(c *gin.Context) {
	if out, code, err := m.action(c, 5*time.Minute, "stop"); err != nil {
		core.Err(c, 500, fmt.Sprintf("停止失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) restart(c *gin.Context) {
	if out, code, err := m.action(c, 5*time.Minute, "restart"); err != nil {
		core.Err(c, 500, fmt.Sprintf("重启失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) logs(c *gin.Context) {
	out, _, err := m.action(c, 30*time.Second, "logs", "--tail=200")
	if err != nil && out == "" {
		core.Err(c, 404, "日志不可用")
		return
	}
	core.OK(c, gin.H{"content": out})
}

func (m *Module) uninstall(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var r AppInst
	if err := m.db.First(&r, id).Error; err != nil {
		core.Err(c, 404, "实例不存在")
		return
	}
	var req struct {
		RemoveData bool `json:"remove_data"`
	}
	_ = c.ShouldBindJSON(&req)
	// down 尽力而为；docker 不可用也允许删除记录
	_, _, _ = execx.Run("appstore", 10*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "down")...)
	m.db.Delete(&r)
	if req.RemoveData {
		_ = os.RemoveAll(m.dir(r.Name))
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
