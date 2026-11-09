// Package alert 告警策略引擎：指标阈值 + 持续时间 + 冷却时间，
// 触发后通过通知模块推送并记录事件历史。
package alert

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/notify"
)

var allowedMetrics = map[string]string{
	"cpu": "CPU 使用率(%)", "mem": "内存使用率(%)",
	"disk": "磁盘使用率(%)", "load1": "1分钟负载",
}

// AlertRule 告警规则。
type AlertRule struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"size:128" json:"name"`
	Metric      string     `gorm:"size:16" json:"metric"` // cpu | mem | disk | load1
	Op          string     `gorm:"size:4" json:"op"`      // > | <
	Threshold   float64    `json:"threshold"`
	NodeID      uint       `json:"node_id"`      // >0 时按对应节点的最新指标判定
	DurationMin int        `json:"duration_min"` // 持续 N 分钟触发
	CooldownMin int        `json:"cooldown_min"` // 触发后冷却分钟
	Enabled     bool       `json:"enabled"`
	LastFiredAt *time.Time `json:"last_fired_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AlertEvent 触发历史。
type AlertEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RuleID    uint      `gorm:"index" json:"rule_id"`
	RuleName  string    `json:"rule_name"`
	Metric    string    `json:"metric"`
	Value     string    `json:"value"`
	Message   string    `gorm:"type:text" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// MetricsProvider 指标来源（由 dashboard 模块实现）。
type MetricsProvider interface {
	Current() (cpu, mem, disk, load1 float64)
}

type Module struct {
	db         *gorm.DB
	mp         MetricsProvider
	mu         sync.Mutex
	since      map[uint]time.Time
	nodeLatest func(nodeID uint) (cpu, mem, load1 float64, ok bool)
}

func New(db *gorm.DB, mp MetricsProvider, nodeLatest func(nodeID uint) (cpu, mem, load1 float64, ok bool)) *Module {
	return &Module{
		nodeLatest: nodeLatest, db: db, mp: mp, since: map[uint]time.Time{}}
}

func (m *Module) Start() error {
	if err := m.db.AutoMigrate(&AlertRule{}, &AlertEvent{}); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			m.evaluate()
		}
	}()
	return nil
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/alerts")
	g.GET("/rules", m.listRules)
	g.POST("/rules", m.createRule)
	g.PUT("/rules/:id", m.updateRule)
	g.DELETE("/rules/:id", m.removeRule)
	g.POST("/rules/:id/toggle", m.toggleRule)
	g.GET("/events", m.listEvents)
}

func metricLabel(k string) string { return allowedMetrics[k] }

func (m *Module) evaluate() {
	var rules []AlertRule
	m.db.Where("enabled = ?", true).Find(&rules)
	if len(rules) == 0 {
		return
	}
	cpu, mem, disk, load1 := m.mp.Current()
	now := time.Now()

	for _, r := range rules {
		var val float64
		if r.NodeID > 0 {
			// 节点维度：取该节点最新上报指标（node 模块回调）
			if r.Metric == "disk" {
				continue // 节点指标暂无磁盘数据
			}
			nc, nm, nl, ok := m.nodeLatest(r.NodeID)
			if !ok {
				continue // 节点离线或无数据
			}
			switch r.Metric {
			case "cpu":
				val = nc
			case "mem":
				val = nm
			case "load1":
				val = nl
			default:
				continue
			}
		} else {
			switch r.Metric {
			case "cpu":
				val = cpu
			case "mem":
				val = mem
			case "disk":
				val = disk
			case "load1":
				val = load1
			default:
				continue
			}
		}
		cond := (r.Op == ">" && val > r.Threshold) || (r.Op == "<" && val < r.Threshold)

		m.mu.Lock()
		if !cond {
			delete(m.since, r.ID)
			m.mu.Unlock()
			continue
		}
		if m.since[r.ID].IsZero() {
			m.since[r.ID] = now
		}
		sustained := now.Sub(m.since[r.ID])
		m.mu.Unlock()

		if r.DurationMin > 0 && sustained < time.Duration(r.DurationMin)*time.Minute {
			continue
		}
		if r.LastFiredAt != nil && now.Sub(*r.LastFiredAt) < time.Duration(maxOf(r.CooldownMin, 5))*time.Minute {
			continue
		}

		valStr := strconv.FormatFloat(val, 'f', 2, 64)
		scope := ""
		if r.NodeID > 0 {
			var nodeName string
			m.db.Table("nodes").Select("name").Where("id = ?", r.NodeID).Scan(&nodeName)
			scope = "[节点 " + nodeName + "] "
		}
		msg := fmt.Sprintf("规则「%s」触发：%s %s当前 %s %s 阈值 %s，已持续 %.0f 分钟",
			r.Name, metricLabel(r.Metric), scope, valStr, r.Op,
			strconv.FormatFloat(r.Threshold, 'f', 2, 64), sustained.Minutes())
		slog.Warn("告警触发", "rule", r.Name, "metric", r.Metric, "value", valStr)

		m.db.Create(&AlertEvent{RuleID: r.ID, RuleName: r.Name, Metric: r.Metric, Value: valStr, Message: msg})
		fired := now
		m.db.Model(&AlertRule{}).Where("id = ?", r.ID).Update("last_fired_at", &fired)
		m.mu.Lock()
		delete(m.since, r.ID)
		m.mu.Unlock()

		notify.Send(m.db, "寒语馨面板告警: "+r.Name, msg)
	}
}

func maxOf(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- 路由 ----

func (m *Module) listRules(c *gin.Context) {
	var rules []AlertRule
	m.db.Order("id").Find(&rules)
	core.OK(c, rules)
}

func validate(r *AlertRule) error {
	if r.Name == "" {
		return fmt.Errorf("规则名称不能为空")
	}
	if _, ok := allowedMetrics[r.Metric]; !ok {
		return fmt.Errorf("不支持的指标: %s", r.Metric)
	}
	if r.Op != ">" && r.Op != "<" {
		return fmt.Errorf("比较符仅支持 > 或 <")
	}
	if r.DurationMin < 0 || r.CooldownMin < 0 {
		return fmt.Errorf("持续时间/冷却时间不能为负")
	}
	return nil
}

func (m *Module) createRule(c *gin.Context) {
	var r AlertRule
	if err := c.ShouldBindJSON(&r); err != nil {
		core.BindErr(c, err)
		return
	}
	r.ID = 0
	if err := validate(&r); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	// DurationMin=0 表示立即触发；CooldownMin=0 时运行期按最小 5 分钟去重
	if err := m.db.Create(&r).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, r)
}

func (m *Module) updateRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var dbR AlertRule
	if err := m.db.First(&dbR, id).Error; err != nil {
		core.Err(c, 404, "规则不存在")
		return
	}
	var req AlertRule
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := validate(&req); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	dbR.Name, dbR.Metric, dbR.Op, dbR.Threshold = req.Name, req.Metric, req.Op, req.Threshold
	dbR.DurationMin, dbR.CooldownMin, dbR.Enabled = req.DurationMin, req.CooldownMin, req.Enabled
	if err := m.db.Save(&dbR).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, dbR)
}

func (m *Module) removeRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	m.db.Delete(&AlertRule{}, id)
	m.db.Where("rule_id = ?", id).Delete(&AlertEvent{})
	core.OK(c, nil)
}

func (m *Module) toggleRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var r AlertRule
	if err := m.db.First(&r, id).Error; err != nil {
		core.Err(c, 404, "规则不存在")
		return
	}
	r.Enabled = !r.Enabled
	if err := m.db.Save(&r).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"enabled": r.Enabled})
}

func (m *Module) listEvents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	var total int64
	m.db.Model(&AlertEvent{}).Count(&total)
	var items []AlertEvent
	m.db.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	core.OK(c, gin.H{"total": total, "items": items})
}
