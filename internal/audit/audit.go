// Package audit 操作审计：写操作留痕（操作人/中文操作说明/脱敏请求体/来源/状态）。
// 存储为按天分文件的 JSONL（data/audit/audit-YYYY-MM-DD.log），不写数据库；
// 仅保留 7 天，写入时自动清理过期文件；查询 API 直接读文件（供 APP 审计页与导出）。
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	maxBodyCapture  = 8 << 10 // 8KB
	auditRetention  = 7 * 24 * time.Hour
	terminalLogTTL  = 180 * 24 * time.Hour // 终端会话日志（DB）保留期不变
	auditDateLayout = "2006-01-02"
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
	{regexp.MustCompile(`^POST /api/apps/sources$`), "添加仓库应用源"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/start$`), "启动应用"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/stop$`), "停止应用"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/restart$`), "重启应用"},
	{regexp.MustCompile(`^POST /api/apps/[^/]+/uninstall$`), "卸载应用"},
	{regexp.MustCompile(`^POST /api/nodes/ssh-install$`), "SSH 接入节点（自动安装 Agent）"},
	{regexp.MustCompile(`^POST /api/mcp/token$`), "生成开放 API Token"},
	{regexp.MustCompile(`^PUT /api/mcp/token$`), "更新开放 API Token 权限"},
	{regexp.MustCompile(`^DELETE /api/mcp/token$`), "禁用开放 API Token"},
	{regexp.MustCompile(`^POST /api/mcp$`), "MCP 工具调用"},
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
	{regexp.MustCompile(`^PUT /api/auth/session-policy$`), "修改会话自动踢出策略"},
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
	{regexp.MustCompile(`^POST /api/scripts/upload`), "上传脚本"},
	{regexp.MustCompile(`^DELETE /api/scripts/`), "删除脚本"},
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

// auditEntry 审计记录（JSONL 文件行；字段名与旧 DB 模型保持一致，APP 审计页无感）。
type auditEntry struct {
	ID        int64  `json:"id"`
	Time      string `json:"time"`
	CreatedAt string `json:"created_at"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Action    string `json:"action"`
	IP        string `json:"ip"`
	UA        string `json:"ua"`
	Status    int    `json:"status"`
	CostMS    int64  `json:"cost_ms"`
	Body      string `json:"body,omitempty"`
}

// Module 审计模块。
type Module struct {
	db     *gorm.DB
	dataMu sync.Mutex
	seq    int64 // 当日行序号（id 生成用）
	clean  string
}

func New(db *gorm.DB) *Module { return &Module{db: db} }

// auditDir 审计文件目录（data/audit）。
func (m *Module) auditDir() string {
	d, err := filepath.Abs(filepath.Join("data", "audit"))
	if err != nil {
		return filepath.Join("data", "audit")
	}
	return d
}

// Start 启动定期清理：审计文件保留 7 天，终端会话日志（DB）保留期不变。
func (m *Module) Start() {
	m.cleanupAuditFiles()
	go func() {
		for {
			time.Sleep(time.Hour)
			m.cleanupAuditFiles()
			cut := time.Now().Add(-terminalLogTTL)
			m.db.Where("created_at < ?", cut).Delete(&core.TerminalLog{})
		}
	}()
}

// cleanupAuditFiles 删除 7 天前的审计日志文件（按文件名日期判定）。
func (m *Module) cleanupAuditFiles() {
	m.dataMu.Lock()
	defer m.dataMu.Unlock()
	cut := time.Now().Add(-auditRetention).Format(auditDateLayout)
	entries, err := os.ReadDir(m.auditDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "audit-"), ".log")
		if day < cut {
			_ = os.Remove(filepath.Join(m.auditDir(), name))
			slog.Info("过期审计日志已清理", "file", name)
		}
	}
}

// append 写入一条审计记录（JSONL 追加；每小时首写顺带清理过期文件）。
func (m *Module) append(entry auditEntry) {
	day := time.Now().Format(auditDateLayout)
	m.dataMu.Lock()
	defer m.dataMu.Unlock()
	hour := time.Now().Format(auditDateLayout + "-15")
	if m.clean != hour {
		m.clean = hour
		go m.cleanupAuditFiles()
	}
	m.seq++
	entry.ID = time.Now().UnixNano() + m.seq
	if entry.Time == "" {
		entry.Time = time.Now().Format("2006-01-02 15:04:05")
	}
	if entry.CreatedAt == "" {
		entry.CreatedAt = entry.Time
	}
	if err := os.MkdirAll(m.auditDir(), 0o700); err != nil {
		slog.Warn("审计目录创建失败", "err", err)
		return
	}
	f, err := os.OpenFile(filepath.Join(m.auditDir(), "audit-"+day+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Warn("审计文件打开失败", "err", err)
		return
	}
	defer f.Close()
	raw, _ := json.Marshal(entry)
	if _, err := f.Write(append(raw, '\n')); err != nil {
		slog.Warn("审计记录写入失败", "err", err)
	}
}

// readAll 倒序读取全部保留期内的审计记录（新文件在前，文件内行倒序）。
func (m *Module) readAll() []auditEntry {
	m.dataMu.Lock()
	defer m.dataMu.Unlock()
	entries, err := os.ReadDir(m.auditDir())
	if err != nil {
		return nil
	}
	files := []string{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".log") {
			files = append(files, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files))) // 文件名日期降序
	out := []auditEntry{}
	for _, name := range files {
		f, err := os.Open(filepath.Join(m.auditDir(), name))
		if err != nil {
			continue
		}
		var lines []auditEntry
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), maxBodyCapture+4096)
		for sc.Scan() {
			var e auditEntry
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				lines = append(lines, e)
			}
		}
		f.Close()
		for i := len(lines) - 1; i >= 0; i-- {
			out = append(out, lines[i])
		}
	}
	return out
}

// Register 审计查询 API（挂载于鉴权组；Web 前台已移除展示，接口保留供 APP 审计页与导出使用）。
func (m *Module) Register(api gin.IRouter) {
	api.GET("/audit/list", m.list)
	api.GET("/audit/export", m.exportCSV)
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
	method := c.Query("method")
	q := c.Query("q")
	u := c.Query("username")
	all := m.readAll()
	items := []auditEntry{}
	for _, e := range all {
		if method != "" && e.Method != method {
			continue
		}
		if q != "" && !strings.Contains(e.Path, q) {
			continue
		}
		if u != "" && e.Username != u {
			continue
		}
		items = append(items, e)
	}
	total := int64(len(items))
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	core.OK(c, gin.H{"total": total, "items": items[start:end]})
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
// 记录发生在响应后（届时鉴权已注入操作人/角色信息），写入当日审计文件。
func Middleware(m *Module) gin.HandlerFunc {
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
		entry := auditEntry{
			Username: c.GetString("user"),
			Role:     c.GetString("role"),
			Method:   c.Request.Method,
			Path:     truncate(c.Request.URL.Path, 256),
			Action:   truncate(describe(c.Request.Method, c.Request.URL.Path), 128),
			IP:       c.ClientIP(),
			UA:       truncate(c.Request.UserAgent(), 256),
			Status:   c.Writer.Status(),
			CostMS:   time.Since(start).Milliseconds(),
			Time:     start.Format("2006-01-02 15:04:05"),
		}
		if bodyRaw != nil {
			entry.Body = sanitizeBody(bodyRaw)
		}
		m.append(entry)
	}
}

// exportCSV 审计日志全量导出（读文件，CSV 流式下载）。
func (m *Module) exportCSV(c *gin.Context) {
	method := c.Query("method")
	q := c.Query("q")
	u := c.Query("username")
	all := m.readAll()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=hanyuxin-audit.csv")
	// BOM 使 Excel 正确识别 UTF-8
	_, _ = c.Writer.WriteString("\ufeff")
	_, _ = c.Writer.WriteString("id,time,username,method,path,status,action\n")
	for _, it := range all {
		if method != "" && it.Method != method {
			continue
		}
		if q != "" && !strings.Contains(it.Path, q) {
			continue
		}
		if u != "" && it.Username != u {
			continue
		}
		action := strings.ReplaceAll(strings.ReplaceAll(it.Action, "\n", " "), "\r", "")
		action = strings.ReplaceAll(action, ",", "，")
		fmt.Fprintf(c.Writer, "%d,%s,%s,%s,%s,%d,%s\n",
			it.ID, it.CreatedAt, csvField(it.Username),
			it.Method, it.Path, it.Status, action)
	}
}

// csvField CSV 单元格转义（引号包裹 + 双写内引号）。
func csvField(v string) string {
	return "\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
}
