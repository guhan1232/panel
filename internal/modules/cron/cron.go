// Package cron 计划任务模块：内置调度引擎（robfig/cron）。
// 任务定义持久化 SQLite，支持动态增删启停、手动执行、终止运行实例、
// 执行历史、连续失败告警；不写入系统 crontab。
package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
	"cnb.cool/hanyuxin/panel/internal/notify"
	"cnb.cool/hanyuxin/panel/internal/objectstore"
)

var parser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

var allowedTypes = map[string]bool{
	"shell": true, "backup_dir": true, "visit_url": true,
	"backup_db": true, "log_rotate": true, "sys_update": true,
}

var dbIdentRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

var nameSanitizer = regexp.MustCompile(`[^A-Za-z0-9\x{4e00}-\x{9fa5}_-]+`)

// Task 任务定义。
type Task struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Name       string     `gorm:"size:128" json:"name"`
	Type       string     `gorm:"size:32" json:"type"` // shell | backup_dir | visit_url
	Spec       string     `gorm:"size:64" json:"spec"` // cron 表达式（5 段或带秒的 6 段，支持 @daily 等）
	Command    string     `gorm:"type:text" json:"command"`
	Params     string     `gorm:"type:text" json:"params"`
	NodeID     uint       `json:"node_id"`     // >0 时经 Agent 在对应节点执行（仅 shell 类）
	TimeoutSec int        `json:"timeout_sec"` // 0 = 不限
	Concurrent bool       `json:"concurrent"`
	NotifyFail int        `json:"notify_fail"`
	Enabled    bool       `json:"enabled"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastStatus string     `gorm:"size:16" json:"last_status"`
	FailStreak int        `json:"fail_streak"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// TaskLog 单次执行记录。
type TaskLog struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	TaskID    uint       `gorm:"index" json:"task_id"`
	Trigger   string     `gorm:"size:8" json:"trigger"` // cron | manual
	Status    string     `gorm:"size:16" json:"status"` // running|success|failed|timeout|stopped|skipped
	StartAt   time.Time  `json:"start_at"`
	EndAt     *time.Time `json:"end_at"`
	CostMS    int64      `json:"cost_ms"`
	ExitCode  int        `json:"exit_code"`
	Output    string     `gorm:"type:text" json:"output"`
	CreatedAt time.Time  `json:"created_at"`
}

type runHandle struct {
	cancel context.CancelFunc
	start  time.Time
}

// NodeExecFunc 远程节点执行函数（node.ExecOnNode），可为 nil。
type NodeExecFunc func(nodeID uint, script string, timeoutSec int) (string, int, error)

type Module struct {
	db       *gorm.DB
	cron     *cron.Cron
	mu       sync.Mutex
	entries  map[uint]cron.EntryID
	running  map[uint]*runHandle
	dataDir  string
	nodeExec NodeExecFunc
}

func New(db *gorm.DB, dataDir string, nodeExec NodeExecFunc) *Module {
	return &Module{
		db:       db,
		cron:     cron.New(cron.WithParser(parser)),
		entries:  map[uint]cron.EntryID{},
		running:  map[uint]*runHandle{},
		dataDir:  dataDir,
		nodeExec: nodeExec,
	}
}

// Start 启动调度器并恢复已启用任务；每小时清理 90 天前的执行历史。
func (m *Module) Start() error {
	if err := m.db.AutoMigrate(&Task{}, &TaskLog{}); err != nil {
		return err
	}
	m.cron.Start()
	var tasks []Task
	if err := m.db.Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return err
	}
	for i := range tasks {
		t := &tasks[i]
		if _, err := m.addEntry(t); err != nil {
			slog.Warn("恢复计划任务失败，已跳过", "task", t.Name, "err", err)
		}
	}
	_, _ = m.cron.AddFunc("0 3 * * * *", m.pruneLogs)
	return nil
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/tasks")
	g.GET("", m.list)
	g.POST("", m.create)
	g.PUT("/:id", m.update)
	g.DELETE("/:id", m.remove)
	g.POST("/:id/toggle", m.toggle)
	g.POST("/:id/run", m.runNow)
	g.DELETE("/:id/running", m.stopRunning)
	g.GET("/:id/logs", m.logs)
	api.GET("/task-logs/:logId", m.logDetail)
}

func (m *Module) addEntry(t *Task) (cron.EntryID, error) {
	if _, err := parser.Parse(t.Spec); err != nil {
		return 0, fmt.Errorf("cron 表达式无效: %w", err)
	}
	id, err := m.cron.AddFunc(t.Spec, func() { m.run(t.ID, "cron") })
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	m.entries[t.ID] = id
	m.mu.Unlock()
	return id, nil
}

func (m *Module) removeEntry(id uint) {
	m.mu.Lock()
	entryID, ok := m.entries[id]
	if ok {
		delete(m.entries, id)
	}
	m.mu.Unlock()
	if ok {
		m.cron.Remove(entryID)
	}
}

// ---- 执行引擎 ----

func (m *Module) run(id uint, trigger string) {
	var t Task
	if err := m.db.First(&t, id).Error; err != nil {
		return
	}
	if trigger == "cron" && !t.Enabled {
		return
	}

	m.mu.Lock()
	if h, ok := m.running[id]; ok && !t.Concurrent {
		m.mu.Unlock()
		m.db.Create(&TaskLog{
			TaskID: id, Trigger: trigger, Status: "skipped", StartAt: time.Now(),
			Output: fmt.Sprintf("上一实例自 %s 起仍在运行，本次已跳过", h.start.Format("15:04:05")),
		})
		return
	}
	ctx := context.Background()
	var cancel context.CancelFunc = func() {}
	if t.TimeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(t.TimeoutSec)*time.Second)
	}
	h := &runHandle{cancel: cancel, start: time.Now()}
	m.running[id] = h
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.running, id)
		m.mu.Unlock()
		h.cancel()
	}()

	log := TaskLog{TaskID: id, Trigger: trigger, Status: "running", StartAt: time.Now()}
	m.db.Create(&log)

	exit, err, output := m.execute(ctx, &t)
	end := time.Now()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		log.Status = "timeout"
	case errors.Is(ctx.Err(), context.Canceled):
		log.Status = "stopped"
	case err != nil:
		log.Status = "failed"
	default:
		log.Status = "success"
	}
	log.EndAt = &end
	log.CostMS = end.Sub(log.StartAt).Milliseconds()
	log.ExitCode = exit
	log.Output = output
	m.db.Save(&log)

	updates := map[string]any{"last_run_at": end, "last_status": log.Status}
	if log.Status == "success" {
		updates["fail_streak"] = 0
	} else {
		updates["fail_streak"] = gorm.Expr("fail_streak + 1")
	}
	m.db.Model(&Task{}).Where("id = ?", id).Updates(updates)

	if log.Status != "success" {
		var nt Task
		m.db.First(&nt, id)
		if nt.NotifyFail > 0 && nt.FailStreak >= nt.NotifyFail {
			slog.Warn("计划任务连续失败，已触发告警通知", "task", nt.Name, "status", log.Status, "streak", nt.FailStreak)
			content := fmt.Sprintf("任务「%s」(ID %d) 已连续失败 %d 次\n最近状态: %s\n耗时: %dms\n触发方式: %s\n输出:\n%s",
				nt.Name, nt.ID, nt.FailStreak, log.Status, log.CostMS, trigger, log.Output)
			go notify.Send(m.db, "计划任务失败告警: "+nt.Name, content)
		}
	}
}

func (m *Module) execute(ctx context.Context, t *Task) (exit int, err error, output string) {
	switch t.Type {
	case "shell":
		if t.NodeID > 0 {
			if m.nodeExec == nil {
				output, exit, err = "", 1, fmt.Errorf("节点执行通道未就绪")
			} else {
				timeout := t.TimeoutSec
				if timeout <= 0 {
					timeout = 300
				}
				output, exit, err = m.nodeExec(t.NodeID, t.Command, timeout)
			}
		} else {
			output, exit, err = execx.RunShellContext(ctx, fmt.Sprintf("task:%d", t.ID), t.Command)
		}
	case "backup_dir":
		output, err = m.runBackupDir(ctx, t)
		if err != nil {
			exit = 1
		}
	case "visit_url":
		output, err = m.runVisitURL(ctx, t)
		if err != nil {
			exit = 1
		}
	case "backup_db":
		output, err = m.runBackupDB(ctx, t)
		if err != nil {
			exit = 1
		}
	case "log_rotate":
		output, err = m.runLogRotate(ctx, t)
		if err != nil {
			exit = 1
		}
	case "sys_update":
		output, err = m.runSysUpdate(ctx, t)
		if err != nil {
			exit = 1
		}
	default:
		err = fmt.Errorf("未知任务类型: %s", t.Type)
	}
	return exit, err, output
}

type backupParams struct {
	Source string `json:"source"`
	Keep   int    `json:"keep"`
	Remote bool   `json:"remote"` // 同时上传到对象存储
}

func (m *Module) runBackupDir(ctx context.Context, t *Task) (string, error) {
	var p backupParams
	if err := json.Unmarshal([]byte(orDefault(t.Params)), &p); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if p.Source == "" {
		return "", errors.New("缺少 source 参数")
	}
	info, err := os.Stat(p.Source)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s 不是目录", p.Source)
	}
	outDir := filepath.Join(m.dataDir, "backups")
	_ = os.MkdirAll(outDir, 0o755)
	out := filepath.Join(outDir, sanitizeName(t.Name)+"-"+time.Now().Format("20060102-150405")+".tar.gz")
	if err := archive.TarGz(ctx, p.Source, out); err != nil {
		return "", err
	}
	msg := "备份完成: " + out
	if p.Remote {
		cfg := objectstore.Get(m.db)
		if cfg.Enabled {
			obj, uerr := objectstore.Upload(ctx, cfg, out, filepath.Base(out))
			if uerr != nil {
				return "", fmt.Errorf("上传对象存储失败: %w", uerr)
			}
			msg += "\n已上传: s3://" + cfg.Bucket + "/" + obj
		} else {
			msg += "\n对象存储未启用，仅保留本地副本"
		}
	}
	if p.Keep > 0 {
		pattern := filepath.Join(outDir, sanitizeName(t.Name)+"-*.tar.gz")
		files, _ := filepath.Glob(pattern)
		if len(files) > p.Keep {
			for _, f := range files[:len(files)-p.Keep] {
				if os.Remove(f) == nil {
					msg += "\n清理旧备份: " + f
				}
			}
		}
	}
	return msg, nil
}

type visitParams struct {
	URL string `json:"url"`
}

func (m *Module) runVisitURL(ctx context.Context, t *Task) (string, error) {
	var p visitParams
	if err := json.Unmarshal([]byte(orDefault(t.Params)), &p); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if p.URL == "" {
		return "", errors.New("缺少 url 参数")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode), fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return fmt.Sprintf("HTTP %d，健康检查通过", resp.StatusCode), nil
}

// shQuote shell 单引号安全包裹。
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

type backupDBParams struct {
	Engine   string `json:"engine"` // mysql | pg
	Database string `json:"database"`
	Keep     int    `json:"keep"`
}

func (m *Module) runBackupDB(ctx context.Context, t *Task) (string, error) {
	var p backupDBParams
	if err := json.Unmarshal([]byte(orDefault(t.Params)), &p); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if p.Engine != "mysql" && p.Engine != "pg" {
		return "", errors.New("engine 需为 mysql 或 pg")
	}
	if !dbIdentRe.MatchString(p.Database) {
		return "", fmt.Errorf("数据库名非法: %s", p.Database)
	}
	outDir := filepath.Join(m.dataDir, "backups")
	_ = os.MkdirAll(outDir, 0o755)
	out := filepath.Join(outDir, fmt.Sprintf("db-%s-%s-%s.sql.gz", p.Engine, p.Database, time.Now().Format("20060102-150405")))

	var script string
	if p.Engine == "mysql" {
		var cfg struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			User     string `json:"user"`
			Password string `json:"password"`
		}
		_ = json.Unmarshal([]byte(core.GetSetting(m.db, core.KMysqlCfg)), &cfg)
		if cfg.Host == "" {
			cfg.Host = "127.0.0.1"
		}
		if cfg.Port == 0 {
			cfg.Port = 3306
		}
		if cfg.User == "" {
			cfg.User = "root"
		}
		script = fmt.Sprintf("set -o pipefail\nexport MYSQL_PWD=%s\nmysqldump -h %s -P %d -u %s --single-transaction --routines --databases %s | gzip > %s\n",
			shQuote(cfg.Password), shQuote(cfg.Host), cfg.Port, shQuote(cfg.User), shQuote(p.Database), shQuote(out))
	} else {
		var cfg struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			User     string `json:"user"`
			Password string `json:"password"`
			SSLMode  string `json:"ssl_mode"`
		}
		_ = json.Unmarshal([]byte(core.GetSetting(m.db, core.KPgCfg)), &cfg)
		if cfg.Host == "" {
			cfg.Host = "127.0.0.1"
		}
		if cfg.Port == 0 {
			cfg.Port = 5432
		}
		if cfg.SSLMode == "" {
			cfg.SSLMode = "disable"
		}
		script = fmt.Sprintf("set -o pipefail\nexport PGPASSWORD=%s\nexport PGSSLMODE=%s\npg_dump -h %s -p %d -U %s -d %s | gzip > %s\n",
			shQuote(cfg.Password), shQuote(cfg.SSLMode), shQuote(cfg.Host), cfg.Port, shQuote(cfg.User), shQuote(p.Database), shQuote(out))
	}
	output, _, err := execx.RunShellContext(ctx, fmt.Sprintf("task:%d", t.ID), script)
	if err != nil {
		return output, err
	}
	msg := "备份完成: " + out
	if p.Keep > 0 {
		pattern := filepath.Join(outDir, fmt.Sprintf("db-%s-%s-*.sql.gz", p.Engine, p.Database))
		files, _ := filepath.Glob(pattern)
		if len(files) > p.Keep {
			for _, f := range files[:len(files)-p.Keep] {
				if os.Remove(f) == nil {
					msg += "\n清理旧备份: " + f
				}
			}
		}
	}
	return msg, nil
}

type logRotateParams struct {
	Pattern  string `json:"pattern"`   // 如 /var/log/nginx/*.log
	SizeMB   int    `json:"size_mb"`   // 超过该大小切割，默认 50
	KeepDays int    `json:"keep_days"` // 压缩包保留天数，默认 30
}

func (m *Module) runLogRotate(ctx context.Context, t *Task) (string, error) {
	var p logRotateParams
	if err := json.Unmarshal([]byte(orDefault(t.Params)), &p); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if p.Pattern == "" || strings.ContainsAny(p.Pattern, "'\"\n") || strings.Contains(p.Pattern, " ") {
		return "", errors.New("pattern 非法（不含空格与引号的 glob 路径）")
	}
	if p.SizeMB <= 0 {
		p.SizeMB = 50
	}
	if p.KeepDays <= 0 {
		p.KeepDays = 30
	}
	script := fmt.Sprintf(`set -e
SIZE=$((%d*1024*1024))
for f in %s; do
  [ -f "$f" ] || continue
  s=$(stat -c%%s "$f" 2>/dev/null || echo 0)
  if [ "$s" -ge "$SIZE" ]; then
    gzip -c "$f" > "$f.$(date +%%Y%%m%%d%%H%%M%%S).gz" && : > "$f" && echo "rotated: $f"
  fi
done
find %s -name "*.gz" -mtime +%d -delete 2>/dev/null || true
echo "日志切割任务完成"`, p.SizeMB, p.Pattern, shQuote(filepath.Dir(p.Pattern)), p.KeepDays)
	output, _, err := execx.RunShellContext(ctx, fmt.Sprintf("task:%d", t.ID), script)
	return output, err
}

var sysUpdateScript = `set -e
if command -v apt-get >/dev/null 2>&1; then
  apt-get update && DEBIAN_FRONTEND=noninteractive apt-get -y upgrade
elif command -v dnf >/dev/null 2>&1; then
  dnf -y upgrade
elif command -v yum >/dev/null 2>&1; then
  yum -y update
else
  echo "未找到 apt-get/dnf/yum"; exit 1
fi
echo "系统更新完成"`

func (m *Module) runSysUpdate(ctx context.Context, t *Task) (string, error) {
	output, _, err := execx.RunShellContext(ctx, fmt.Sprintf("task:%d", t.ID), sysUpdateScript)
	return output, err
}

// ---- 路由处理 ----

func (m *Module) list(c *gin.Context) {
	var tasks []Task
	if err := m.db.Order("id").Find(&tasks).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	now := time.Now()
	items := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		item := gin.H{}
		data, _ := json.Marshal(t)
		_ = json.Unmarshal(data, &item)
		if t.Enabled {
			if sched, err := parser.Parse(t.Spec); err == nil {
				item["next_run"] = sched.Next(now)
			}
		}
		items = append(items, item)
	}
	core.OK(c, items)
}

func (m *Module) create(c *gin.Context) {
	var t Task
	if err := c.ShouldBindJSON(&t); err != nil {
		core.BindErr(c, err)
		return
	}
	if !allowedTypes[t.Type] {
		core.Err(c, 400, "不支持的任务类型")
		return
	}
	if t.Name == "" {
		core.Err(c, 400, "任务名称不能为空")
		return
	}
	if _, err := parser.Parse(t.Spec); err != nil {
		core.Err(c, 400, "cron 表达式无效: "+err.Error())
		return
	}
	t.ID = 0
	t.Enabled = true // 新建任务默认启用，可在列表中随时停用
	t.LastStatus = ""
	t.FailStreak = 0
	if err := m.db.Create(&t).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if t.Enabled {
		if _, err := m.addEntry(&t); err != nil {
			core.Err(c, 400, err.Error())
			return
		}
	}
	core.OK(c, t)
}

func (m *Module) update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var t Task
	if err := m.db.First(&t, id).Error; err != nil {
		core.Err(c, 404, "任务不存在")
		return
	}
	var req Task
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !allowedTypes[req.Type] {
		core.Err(c, 400, "不支持的任务类型")
		return
	}
	if _, err := parser.Parse(req.Spec); err != nil {
		core.Err(c, 400, "cron 表达式无效: "+err.Error())
		return
	}
	oldEnabled := t.Enabled
	t.Name, t.Type, t.Spec = req.Name, req.Type, req.Spec
	t.Command, t.Params = req.Command, req.Params
	t.TimeoutSec, t.Concurrent, t.NotifyFail = req.TimeoutSec, req.Concurrent, req.NotifyFail
	if err := m.db.Save(&t).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	// 重新注册调度
	m.removeEntry(t.ID)
	if t.Enabled {
		if _, err := m.addEntry(&t); err != nil {
			core.Err(c, 400, err.Error())
			return
		}
	} else if oldEnabled {
		m.db.Model(&t).Update("enabled", false)
	}
	core.OK(c, t)
}

func (m *Module) remove(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	m.mu.Lock()
	_, running := m.running[uint(id)]
	m.mu.Unlock()
	if running {
		core.Err(c, 400, "任务正在运行，请先终止后再删除")
		return
	}
	m.removeEntry(uint(id))
	m.db.Delete(&Task{}, id)
	m.db.Where("task_id = ?", id).Delete(&TaskLog{})
	core.OK(c, nil)
}

func (m *Module) toggle(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var t Task
	if err := m.db.First(&t, id).Error; err != nil {
		core.Err(c, 404, "任务不存在")
		return
	}
	t.Enabled = !t.Enabled
	if err := m.db.Save(&t).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if t.Enabled {
		if _, err := m.addEntry(&t); err != nil {
			t.Enabled = false
			m.db.Save(&t)
			core.Err(c, 400, err.Error())
			return
		}
	} else {
		m.removeEntry(t.ID)
	}
	core.OK(c, gin.H{"enabled": t.Enabled})
}

func (m *Module) runNow(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var t Task
	if err := m.db.First(&t, id).Error; err != nil {
		core.Err(c, 404, "任务不存在")
		return
	}
	m.mu.Lock()
	_, running := m.running[uint(id)]
	m.mu.Unlock()
	if running && !t.Concurrent {
		core.Err(c, 400, "任务正在运行中")
		return
	}
	go m.run(uint(id), "manual")
	core.OK(c, nil)
}

func (m *Module) stopRunning(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	m.mu.Lock()
	h, ok := m.running[uint(id)]
	m.mu.Unlock()
	if !ok {
		core.Err(c, 400, "任务当前未在运行")
		return
	}
	h.cancel()
	core.OK(c, nil)
}

func (m *Module) logs(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	var total int64
	q := m.db.Model(&TaskLog{}).Where("task_id = ?", id)
	q.Count(&total)
	var items []TaskLog
	q.Select("id, task_id, trigger, status, start_at, end_at, cost_ms, exit_code, created_at").
		Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	core.OK(c, gin.H{"total": total, "items": items})
}

func (m *Module) logDetail(c *gin.Context) {
	logID, _ := strconv.Atoi(c.Param("logId"))
	var l TaskLog
	if err := m.db.First(&l, logID).Error; err != nil {
		core.Err(c, 404, "记录不存在")
		return
	}
	core.OK(c, l)
}

// ---- 工具 ----

func (m *Module) pruneLogs() {
	cut := time.Now().AddDate(0, 0, -90)
	res := m.db.Where("created_at < ?", cut).Delete(&TaskLog{})
	if res.Error == nil && res.RowsAffected > 0 {
		slog.Info("已清理过期任务执行历史", "rows", res.RowsAffected)
	}
}

func orDefault(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}

func sanitizeName(s string) string {
	out := nameSanitizer.ReplaceAllString(s, "_")
	if len(out) > 64 {
		out = out[:64]
	}
	if out == "" {
		out = "task"
	}
	return out
}
