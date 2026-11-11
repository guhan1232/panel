// Package node 多节点管理：Agent 经 WebSocket 反连注册（token 鉴权），
// 指标入库（3 天保留），支持节点列表/历史查询/远程命令执行。
package node

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	metricsRetention = 3 * 24 * time.Hour
	readIdleTimeout  = 60 * time.Second // 依赖 agent 15s 心跳指标保活
)

// Node 节点注册信息。
type Node struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:64" json:"name"`
	Hostname  string    `gorm:"size:128" json:"hostname"`
	OS        string    `gorm:"size:64" json:"os"`
	Arch      string    `gorm:"size:32" json:"arch"`
	IP        string    `gorm:"size:64" json:"ip"`
	Version   string    `gorm:"size:32" json:"version"`
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
}

// NodeMetricHourly 节点小时聚合指标（长期保留，月级视图）。
type NodeMetricHourly struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	NodeID   uint      `gorm:"index" json:"node_id"`
	Hour     time.Time `gorm:"index" json:"hour"` // 整点
	AvgCPU   float64   `json:"avg_cpu"`
	MaxCPU   float64   `json:"max_cpu"`
	AvgMem   float64   `json:"avg_mem"`
	MaxMem   float64   `json:"max_mem"`
	MaxLoad1 float64   `json:"max_load1"`
	Samples  int       `json:"samples"`
}

// NodeMetric 节点采样指标。
type NodeMetric struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	NodeID   uint      `gorm:"index" json:"node_id"`
	Time     time.Time `gorm:"index" json:"time"`
	CPU      float64   `json:"cpu"`
	MemPct   float64   `json:"mem_pct"`
	MemUsed  uint64    `json:"mem_used"`
	MemTotal uint64    `json:"mem_total"`
	Recv     uint64    `json:"recv"`
	Sent     uint64    `json:"sent"`
	Load1    float64   `json:"load1"`
}

// ---- Agent 连接 Hub ----

type agentConn struct {
	nodeID uint
	name   string
	conn   *websocket.Conn
	wmu    sync.Mutex
}

func (a *agentConn) send(v any) error {
	a.wmu.Lock()
	defer a.wmu.Unlock()
	return a.conn.WriteJSON(v)
}

type execResult struct {
	Output string
	Exit   int
	Err    string
}

type Hub struct {
	mu          sync.Mutex
	conns       map[uint]*agentConn
	pending     map[string]chan execResult
	pendingFile map[string]chan fileResult
	terms       map[string]*websocket.Conn // termID → 浏览器 WS 桥
	termSeq     atomic.Uint64
	execSeq     atomic.Uint64
}

func (h *Hub) add(id uint, a *agentConn) { h.mu.Lock(); h.conns[id] = a; h.mu.Unlock() }
func (h *Hub) remove(id uint, a *agentConn) {
	h.mu.Lock()
	if h.conns[id] == a {
		delete(h.conns, id)
	}
	h.mu.Unlock()
}
func (h *Hub) get(id uint) *agentConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conns[id]
}

// ---- 模块 ----

type Module struct {
	db       *gorm.DB
	hub      Hub
	upgrader websocket.Upgrader
}

func New(db *gorm.DB) *Module {
	return &Module{
		db:  db,
		hub: Hub{conns: map[uint]*agentConn{}, pending: map[string]chan execResult{}, pendingFile: map[string]chan fileResult{}, terms: map[string]*websocket.Conn{}},
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true }, // agent 无浏览器 Origin
		},
	}
}

func (m *Module) Start() error {
	if err := m.db.AutoMigrate(&Node{}, &NodeMetric{}, &NodeMetricHourly{}); err != nil {
		return err
	}
	// 首次生成节点接入 token
	if core.GetSetting(m.db, core.KNodeToken) == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		if err := core.SetSetting(m.db, core.KNodeToken, hex.EncodeToString(b)); err != nil {
			return err
		}
	}
	// 每小时清理 3 天前的指标
	go func() {
		for {
			m.prune()
			time.Sleep(time.Hour)
		}
	}()
	return nil
}

func (m *Module) prune() {
	res := m.db.Where("time < ?", time.Now().Add(-metricsRetention)).Delete(&NodeMetric{})
	m.aggregateHourly()
	if res.Error == nil && res.RowsAffected > 0 {
		slog.Info("已清理过期节点指标", "rows", res.RowsAffected)
	}
}

// Register 注册节点 API。all=只读组, rw=operator+ 组, admin=admin 组。
func (m *Module) Register(all gin.IRouter, rw gin.IRouter, admin gin.IRouter) {
	all.GET("/nodes", m.list)
	all.GET("/nodes/:id/metrics", m.metrics)
	rw.POST("/nodes/:id/exec", m.exec)
	admin.GET("/nodes/token", m.getToken)
	admin.POST("/nodes/token/regenerate", m.regenToken)
	admin.POST("/nodes/ssh-install", m.sshInstall)
	admin.DELETE("/nodes/:id", m.remove)
	all.GET("/nodes/:id/metrics/hourly", m.metricsHourly)
}

// ---- Agent WS 接入（token 鉴权，不走面板会话/入口） ----

type agentMsg struct {
	Type     string `json:"type"` // hello | metric | exec_result
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	ID       string `json:"id"`
	Output   string `json:"output"`
	Exit     int    `json:"exit"`
	Err      string `json:"err"`
	Ok       bool   `json:"ok"`
	Sample   *struct {
		CPU      float64 `json:"cpu"`
		MemPct   float64 `json:"mem_pct"`
		MemUsed  uint64  `json:"mem_used"`
		MemTotal uint64  `json:"mem_total"`
		Recv     uint64  `json:"recv"`
		Sent     uint64  `json:"sent"`
		Load1    float64 `json:"load1"`
	} `json:"sample"`
	Data    string      `json:"data,omitempty"`    // file write base64 / term_in
	Force   bool        `json:"force,omitempty"`   // file delete 确认
	Entries []fileEntry `json:"entries,omitempty"` // file ls
	DataB64 bool        `json:"data_b64,omitempty"`
}

// fileEntry 节点文件条目。
type fileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// fileResult 节点文件操作结果。
type fileResult struct {
	Err     string      `json:"err"`
	Data    string      `json:"data"`
	DataB64 bool        `json:"data_b64"`
	Entries []fileEntry `json:"entries"`
}

func (m *Module) HandleAgent(c *gin.Context) {
	token := c.Query("token")
	name := c.Query("name")
	if token == "" || name == "" {
		c.String(http.StatusUnauthorized, "missing token/name")
		return
	}
	if token != core.GetSetting(m.db, core.KNodeToken) {
		c.String(http.StatusUnauthorized, "invalid token")
		return
	}
	ws, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	// 首条消息必须是 hello
	var hello agentMsg
	ws.SetReadDeadline(time.Now().Add(readIdleTimeout))
	if err := ws.ReadJSON(&hello); err != nil || hello.Type != "hello" {
		return
	}
	node, err := m.upsertNode(name, &hello, c.ClientIP())
	if err != nil {
		slog.Warn("节点注册失败", "name", name, "err", err)
		return
	}
	ac := &agentConn{nodeID: node.ID, name: name, conn: ws}
	m.hub.add(node.ID, ac)
	defer m.hub.remove(node.ID, ac)
	slog.Info("节点已接入", "name", name, "ip", c.ClientIP(), "version", hello.Version)

	for {
		ws.SetReadDeadline(time.Now().Add(readIdleTimeout))
		var msg agentMsg
		if err := ws.ReadJSON(&msg); err != nil {
			slog.Info("节点断开", "name", name, "err", err)
			return
		}
		switch msg.Type {
		case "metric":
			if msg.Sample == nil {
				continue
			}
			s := msg.Sample
			m.db.Create(&NodeMetric{
				NodeID: node.ID, Time: time.Now(),
				CPU: s.CPU, MemPct: s.MemPct, MemUsed: s.MemUsed, MemTotal: s.MemTotal,
				Recv: s.Recv, Sent: s.Sent, Load1: s.Load1,
			})
			m.db.Model(&Node{}).Where("id = ?", node.ID).Update("last_seen", time.Now())
		case "exec_result":
			m.hub.mu.Lock()
			if ch, ok := m.hub.pending[msg.ID]; ok {
				delete(m.hub.pending, msg.ID)
				ch <- execResult{Output: msg.Output, Exit: msg.Exit, Err: msg.Err}
			}
			m.hub.mu.Unlock()
		case "file_result":
			m.hub.mu.Lock()
			if ch, ok := m.hub.pendingFile[msg.ID]; ok {
				delete(m.hub.pendingFile, msg.ID)
				ch <- fileResult{Err: msg.Err, Data: msg.Data, DataB64: msg.DataB64, Entries: msg.Entries}
			}
			m.hub.mu.Unlock()
		case "term_out":
			m.hub.mu.Lock()
			bw := m.hub.terms[msg.ID]
			m.hub.mu.Unlock()
			if bw != nil {
				payload := msg.Output
				if msg.DataB64 {
					if dec, err := base64.StdEncoding.DecodeString(msg.Output); err == nil {
						payload = string(dec)
					}
				}
				_ = bw.WriteMessage(websocket.TextMessage, []byte(payload))
			}
		}
	}
}

func (m *Module) upsertNode(name string, hello *agentMsg, ip string) (*Node, error) {
	var n Node
	err := m.db.Where("name = ?", name).First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		n = Node{Name: name}
	} else if err != nil {
		return nil, err
	}
	n.Hostname, n.OS, n.Arch, n.Version, n.IP, n.LastSeen =
		hello.Hostname, hello.OS, hello.Arch, hello.Version, ip, time.Now()
	if err := m.db.Save(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// ---- API ----

func (m *Module) list(c *gin.Context) {
	var nodes []Node
	m.db.Order("name").Find(&nodes)
	items := make([]gin.H, 0, len(nodes))
	for _, n := range nodes {
		online := m.hub.get(n.ID) != nil
		var latest NodeMetric
		has := m.db.Where("node_id = ?", n.ID).Order("time DESC").First(&latest).Error == nil
		item := gin.H{"node": n, "online": online}
		if has {
			item["latest"] = latest
		}
		items = append(items, item)
	}
	core.OK(c, items)
}

func (m *Module) metrics(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "240"))
	if limit < 1 || limit > 2000 {
		limit = 240
	}
	var rows []NodeMetric
	m.db.Where("node_id = ?", id).Order("time DESC").Limit(limit).Find(&rows)
	// 反转为时间升序
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	core.OK(c, rows)
}

func (m *Module) exec(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Script     string `binding:"required"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	ac := m.hub.get(uint(id))
	if ac == nil {
		core.Err(c, 400, "节点离线或不存在")
		return
	}
	if req.TimeoutSec <= 0 || req.TimeoutSec > 1800 {
		req.TimeoutSec = 120
	}
	execID := fmt.Sprintf("e%d", m.hub.execSeq.Add(1))
	ch := make(chan execResult, 1)
	m.hub.mu.Lock()
	m.hub.pending[execID] = ch
	m.hub.mu.Unlock()
	defer func() {
		m.hub.mu.Lock()
		delete(m.hub.pending, execID)
		m.hub.mu.Unlock()
	}()

	if err := ac.send(map[string]any{
		"type": "exec", "id": execID, "script": req.Script, "timeout_sec": req.TimeoutSec,
	}); err != nil {
		core.Err(c, 500, "下发失败: "+err.Error())
		return
	}
	select {
	case r := <-ch:
		if r.Err != "" {
			core.OK(c, gin.H{"output": r.Output, "exit": r.Exit, "err": r.Err})
			return
		}
		core.OK(c, gin.H{"output": r.Output, "exit": r.Exit})
	case <-time.After(time.Duration(req.TimeoutSec)*time.Second + 10*time.Second):
		core.Err(c, 500, "节点执行超时")
	}
}

func (m *Module) remove(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	m.db.Delete(&Node{}, id)
	m.db.Where("node_id = ?", id).Delete(&NodeMetric{})
	core.OK(c, nil)
}

func (m *Module) getToken(c *gin.Context) {
	t := core.GetSetting(m.db, core.KNodeToken)
	if t == "" {
		core.Err(c, 500, "token 未初始化")
		return
	}
	core.OK(c, gin.H{"token": t})
}

func (m *Module) regenToken(c *gin.Context) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	t := hex.EncodeToString(b)
	if err := core.SetSetting(m.db, core.KNodeToken, t); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("节点接入 token 已重置", "by", c.GetString("user"))
	core.OK(c, gin.H{"token": t})
}

// ExecOnNode 在指定在线节点执行脚本，供计划任务等模块复用 agent 通道。
func (m *Module) ExecOnNode(nodeID uint, script string, timeoutSec int) (string, int, error) {
	ac := m.hub.get(nodeID)
	if ac == nil {
		return "", -1, fmt.Errorf("节点离线或不存在")
	}
	if timeoutSec <= 0 || timeoutSec > 1800 {
		timeoutSec = 120
	}
	execID := fmt.Sprintf("e%d", m.hub.execSeq.Add(1))
	ch := make(chan execResult, 1)
	m.hub.mu.Lock()
	m.hub.pending[execID] = ch
	m.hub.mu.Unlock()
	defer func() {
		m.hub.mu.Lock()
		delete(m.hub.pending, execID)
		m.hub.mu.Unlock()
	}()
	if err := ac.send(map[string]any{
		"type": "exec", "id": execID, "script": script, "timeout_sec": timeoutSec,
	}); err != nil {
		return "", -1, err
	}
	select {
	case r := <-ch:
		if r.Err != "" {
			return r.Output, r.Exit, fmt.Errorf("%s", r.Err)
		}
		return r.Output, r.Exit, nil
	case <-time.After(time.Duration(timeoutSec)*time.Second + 10*time.Second):
		return "", -1, fmt.Errorf("节点执行超时")
	}
}

// aggregateHourly 将聚合水位之后的小时指标汇总入 NodeMetricHourly（幂等，按 node+hour 去重）。
func (m *Module) aggregateHourly() {
	var lastHour time.Time
	if err := m.db.Model(&NodeMetricHourly{}).Select("MAX(hour)").Scan(&lastHour).Error; err != nil {
		return
	}
	if lastHour.IsZero() {
		lastHour = time.Now().AddDate(0, 0, -1)
	}
	rows2, rerr := m.db.Raw(`SELECT node_id,`+
		" date_format(time, '%Y-%m-%d %H:00:00') AS hr,"+
		" AVG(cpu) AS avg_cpu, MAX(cpu) AS max_cpu, AVG(mem_pct) AS avg_mem, MAX(mem_pct) AS max_mem,"+
		" MAX(load1) AS max_load1, COUNT(*) AS samples "+
		"FROM node_metrics WHERE time > ? GROUP BY node_id, hr HAVING COUNT(*) >= 2", lastHour).Rows()
	if rerr == nil {
		defer rows2.Close()
		for rows2.Next() {
			var nodeID uint
			var hr time.Time
			var avgCPU, maxCPU, avgMem, maxMem, maxLoad float64
			var samples int
			if rows2.Scan(&nodeID, &hr, &avgCPU, &maxCPU, &avgMem, &maxMem, &maxLoad, &samples) == nil {
				var cnt int64
				m.db.Model(&NodeMetricHourly{}).Where("node_id = ? AND hour = ?", nodeID, hr).Count(&cnt)
				if cnt == 0 {
					m.db.Create(&NodeMetricHourly{NodeID: nodeID, Hour: hr, AvgCPU: avgCPU, MaxCPU: maxCPU, AvgMem: avgMem, MaxMem: maxMem, MaxLoad1: maxLoad, Samples: samples})
				}
			}
		}
	}
}

// metricsHourly 节点小时聚合查询（月级视图）：GET /nodes/:id/metrics/hourly?days=30
func (m *Module) metricsHourly(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days < 1 || days > 180 {
		days = 30
	}
	var rows []NodeMetricHourly
	m.db.Where("node_id = ? AND hour > ?", id, time.Now().AddDate(0, 0, -days)).Order("hour").Find(&rows)
	core.OK(c, rows)
}

// fileOp 向节点下发文件操作并等待结果（ls/read/write/delete）。
func (m *Module) fileOp(nodeID uint, op, path, data string, force bool) (*fileResult, error) {
	ac := m.hub.get(nodeID)
	if ac == nil {
		return nil, fmt.Errorf("节点离线或不存在")
	}
	id := fmt.Sprintf("f%d", m.hub.termSeq.Add(1))
	ch := make(chan fileResult, 1)
	m.hub.mu.Lock()
	m.hub.pendingFile[id] = ch
	m.hub.mu.Unlock()
	defer func() {
		m.hub.mu.Lock()
		delete(m.hub.pendingFile, id)
		m.hub.mu.Unlock()
	}()
	if err := ac.send(map[string]any{
		"type": "file", "id": id, "script": op, "path": path, "data": data, "force": force,
	}); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return &r, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("节点文件操作超时")
	}
}

// files 列目录：GET /nodes/:id/files?path=
func (m *Module) files(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	path := c.DefaultQuery("path", "/")
	r, err := m.fileOp(uint(id), "ls", path, "", false)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if r.Err != "" {
		core.Err(c, 400, r.Err)
		return
	}
	core.OK(c, gin.H{"path": path, "entries": r.Entries})
}

// fileRead 读取文件（base64）：GET /nodes/:id/file?path=
func (m *Module) fileRead(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	path := c.Query("path")
	if strings.Contains(path, "..") {
		core.Err(c, 400, "路径不允许包含 ..")
		return
	}
	r, err := m.fileOp(uint(id), "read", path, "", false)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if r.Err != "" {
		core.Err(c, 400, r.Err)
		return
	}
	core.OK(c, gin.H{"path": path, "data": r.Data})
}

// fileWrite 写文件：POST /nodes/:id/file {path, data(base64)}
func (m *Module) fileWrite(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Path string `binding:"required"`
		Data string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if strings.Contains(req.Path, "..") {
		core.Err(c, 400, "路径不允许包含 ..")
		return
	}
	r, err := m.fileOp(uint(id), "write", req.Path, req.Data, false)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if r.Err != "" {
		core.Err(c, 400, r.Err)
		return
	}
	core.OK(c, gin.H{"ok": true})
}

// fileDelete 删除：DELETE /nodes/:id/file?path=&force=1
func (m *Module) fileDelete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	path := c.Query("path")
	force := c.Query("force") == "1"
	if strings.Contains(path, "..") {
		core.Err(c, 400, "路径不允许包含 ..")
		return
	}
	r, err := m.fileOp(uint(id), "delete", path, "", force)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if r.Err != "" {
		core.Err(c, 400, r.Err)
		return
	}
	core.OK(c, gin.H{"ok": true})
}

// HandleNodeTerm 节点终端 WS 桥：浏览器 ↔ 面板 ↔ agent。
// 路由：GET /api/ws/nodeterm/:id（会话鉴权中间件之后）。
func (m *Module) HandleNodeTerm(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	ac := m.hub.get(uint(id))
	if ac == nil {
		c.String(http.StatusServiceUnavailable, "node offline")
		return
	}
	ws, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	termID := fmt.Sprintf("t%d", m.hub.termSeq.Add(1))
	if err := ac.send(map[string]any{"type": "term_open", "id": termID}); err != nil {
		return
	}
	defer func() {
		_ = ac.send(map[string]any{"type": "term_close", "id": termID})
		m.hub.mu.Lock()
		delete(m.hub.terms, termID)
		m.hub.mu.Unlock()
	}()
	m.hub.mu.Lock()
	m.hub.terms[termID] = ws
	m.hub.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			mt, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			_ = mt
			data := base64.StdEncoding.EncodeToString(msg)
			_ = ac.send(map[string]any{"type": "term_in", "id": termID, "data": data})
		}
	}()
	select {
	case <-done:
	case <-time.After(6 * time.Hour):
	}
}
