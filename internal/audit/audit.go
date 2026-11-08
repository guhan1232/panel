// Package audit 操作审计：写操作留痕（操作人/中文操作说明/脱敏请求体/来源/状态）+ 定期清理。
package audit

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	maxBodyCapture = 8 << 10 // 8KB
	retention      = 180 * 24 * time.Hour
)

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var sensitiveKeys = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api[_-]?key|auth)`)

// sanitizeBody 脱敏请求体：密码/密钥字段打码，超长截断。
func sanitizeBody(raw []byte) string {
	s := string(raw)
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		masked := false
		for k := range m {
			if sensitiveKeys.MatchString(k) {
				m[k] = "******"
				masked = true
			}
		}
		if masked {
			if b, err := json.Marshal(m); err == nil {
				s = string(b)
			}
		}
	}
	return truncate(s, maxBodyCapture)
}

// ---- 中文操作描述 ----

var opRules = []struct {
	re   *regexp.Regexp
	desc string
}{
	{regexp.MustCompile(`^POST /api/auth/login$`), "用户登录"},
	{regexp.MustCompile(`^POST /api/auth/logout$`), "退出登录"},
	{regexp.MustCompile(`^POST /api/tasks$`), "创建计划任务"},
	{regexp.MustCompile(`^PUT /api/tasks/[^/]+$`), "修改计划任务"},
	{regexp.MustCompile(`^DELETE /api/tasks/[^/]+$`), "删除计划任务"},
	{regexp.MustCompile(`^POST /api/tasks/[^/]+/toggle$`), "启停计划任务"},
	{regexp.MustCompile(`^POST /api/tasks/[^/]+/run$`), "手动执行计划任务"},
	{regexp.MustCompile(`^POST /api/apps$`), "部署应用"},
	{regexp.MustCompile(`^POST /api/apps/docker/install$`), "安装 Docker"},
	{regexp.MustCompile(`^POST /api/apps/docker/mirrors$`), "配置 Docker 镜像源"},
	{regexp.MustCompile(`^POST /api/apps/source$`), "配置应用源"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/start$`), "启动应用"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/stop$`), "停止应用"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/restart$`), "重启应用"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/uninstall$`), "卸载应用"},
	{regexp.MustCompile(`^POST /api/runtimes/[^/]+/install$`), "安装运行时"},
	{regexp.MustCompile(`^POST /api/runtimes/[^/]+/set-default$`), "设置默认运行时版本"},
	{regexp.MustCompile(`^DELETE /api/runtimes/[^/]+$`), "卸载运行时版本"},
	{regexp.MustCompile(`^POST /api/runtimes/php/composer$`), "设置 Composer 源"},
	{regexp.MustCompile(`^POST /api/runtimes/php/composer/install$`), "安装 Composer"},
	{regexp.MustCompile(`^POST /api/runtimes/php/pecl$`), "设置 PECL 源"},
	{regexp.MustCompile(`^POST /api/runtimes/[^/]+/pypi$`), "设置 pypi 源"},
	{regexp.MustCompile(`^POST /api/runtimes/[^/]+/registry$`), "设置 npm registry"},
	{regexp.MustCompile(`^POST /api/sites$`), "创建网站"},
	{regexp.MustCompile(`^PUT /api/sites/[^/]+$`), "修改网站"},
	{regexp.MustCompile(`^DELETE /api/sites/[^/]+$`), "删除网站"},
	{regexp.MustCompile(`^POST /api/sites/[^/]+/toggle$`), "启停网站"},
	{regexp.MustCompile(`^POST /api/sites/nginx/reload$`), "重载 Nginx"},
	{regexp.MustCompile(`^POST /api/sites/nginx/install$`), "安装 Nginx"},
	{regexp.MustCompile(`^(POST|DELETE|PUT) /api/dbmgr/`), "数据库管理操作"},
	{regexp.MustCompile(`^POST /api/certs/upload$`), "上传证书"},
	{regexp.MustCompile(`^POST /api/certs/issue$`), "签发证书"},
	{regexp.MustCompile(`^DELETE /api/certs/[^/]+$`), "删除证书"},
	{regexp.MustCompile(`^POST /api/firewall/port$`), "防火墙端口操作"},
	{regexp.MustCompile(`^POST /api/firewall/switch$`), "防火墙开关"},
	{regexp.MustCompile(`^POST /api/proc/kill$`), "结束进程"},
	{regexp.MustCompile(`^POST /api/proc/service$`), "服务管理操作"},
	{regexp.MustCompile(`^POST /api/nodes/[^/]+/exec$`), "节点远程命令"},
	{regexp.MustCompile(`^POST /api/users$`), "创建用户"},
	{regexp.MustCompile(`^PUT /api/users/[^/]+$`), "修改用户"},
	{regexp.MustCompile(`^DELETE /api/users/[^/]+$`), "删除用户"},
	{regexp.MustCompile(`^POST /api/setting/password$`), "修改管理员密码"},
	{regexp.MustCompile(`^POST /api/notify/`), "通知配置"},
	{regexp.MustCompile(`^POST /api/s3/`), "对象存储配置"},
	{regexp.MustCompile(`^POST /api/files/mkdir$`), "创建目录"},
	{regexp.MustCompile(`^POST /api/files/rename$`), "重命名文件"},
	{regexp.MustCompile(`^POST /api/files/move$`), "移动文件"},
	{regexp.MustCompile(`^POST /api/files/delete$`), "删除文件"},
	{regexp.MustCompile(`^POST /api/files/save$`), "保存文件"},
	{regexp.MustCompile(`^POST /api/files/upload$`), "上传文件"},
	{regexp.MustCompile(`^POST /api/files/chmod$`), "修改权限"},
	{regexp.MustCompile(`^POST /api/files/compress$`), "压缩文件"},
	{regexp.MustCompile(`^POST /api/files/decompress$`), "解压文件"},
	{regexp.MustCompile(`^(POST|PUT|DELETE) /api/alerts`), "告警规则操作"},
	{regexp.MustCompile(`^POST /api/dockerstore/`), "容器操作"},
	{regexp.MustCompile(`^POST /api/runtimes/php/versions`), "PHP 版本操作"},
}

func describe(method, path string) string {
	key := method + " " + path
	for _, r := range opRules {
		if r.re.MatchString(key) {
			return r.desc
		}
	}
	return method + " " + path
}

// Module 审计模块。
type Module struct{ db *gorm.DB }

func New(db *gorm.DB) *Module { return &Module{db: db} }

// Start 启动定期清理（保留 180 天）。
func (m *Module) Start() {
	go func() {
		for {
			cut := time.Now().Add(-retention)
			m.db.Where("created_at < ?", cut).Delete(&core.AuditLog{})
			m.db.Where("created_at < ?", cut).Delete(&core.TerminalLog{})
			time.Sleep(time.Hour)
		}
	}()
}

// Register 审计查询 API（挂载于鉴权组）。
func (m *Module) Register(api gin.IRouter) {
	api.GET("/audit/list", m.list)
	api.GET("/terminal-logs", m.terminalLogs)
}

func (m *Module) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	qry := m.db.Model(&core.AuditLog{})
	if method := c.Query("method"); method != "" {
		qry = qry.Where("method = ?", method)
	}
	if q := c.Query("q"); q != "" {
		qry = qry.Where("path LIKE ?", "%"+q+"%")
	}
	if u := c.Query("username"); u != "" {
		qry = qry.Where("username = ?", u)
	}
	var total int64
	qry.Count(&total)
	var items []core.AuditLog
	qry.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	core.OK(c, gin.H{"total": total, "items": items})
}

func (m *Module) terminalLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	qry := m.db.Model(&core.TerminalLog{})
	if u := c.Query("user"); u != "" {
		qry = qry.Where("username = ?", u)
	}
	if q := c.Query("q"); q != "" {
		qry = qry.Where("line LIKE ?", "%"+q+"%")
	}
	var total int64
	qry.Count(&total)
	var items []core.TerminalLog
	qry.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	core.OK(c, gin.H{"total": total, "items": items})
}

// Middleware 写操作审计中间件：预读并回填请求体（供脱敏记录），
// 记录发生在响应后（届时鉴权已注入操作人/角色信息）。
func Middleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var bodyRaw []byte
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead &&
			c.Request.Body != nil && !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			b, _ := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyCapture+1))
			bodyRaw = b
			c.Request.Body = io.NopCloser(strings.NewReader(string(b)))
		}
		start := time.Now()
		c.Next()
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			return
		}
		entry := core.AuditLog{
			Username: c.GetString("user"),
			Role:     c.GetString("role"),
			Method:   c.Request.Method,
			Path:     truncate(c.Request.URL.Path, 256),
			Action:   truncate(describe(c.Request.Method, c.Request.URL.Path), 128),
			IP:       c.ClientIP(),
			UA:       truncate(c.Request.UserAgent(), 256),
			Status:   c.Writer.Status(),
			CostMS:   time.Since(start).Milliseconds(),
		}
		if bodyRaw != nil {
			entry.Body = sanitizeBody(bodyRaw)
		}
		if err := db.Create(&entry).Error; err != nil {
			slog.Warn("审计记录写入失败", "err", err)
		}
	}
}