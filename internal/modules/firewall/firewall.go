// Package firewall 防火墙管理：宝塔式端口规则（协议/端口段/来源/策略/方向/备注），
// 规则以面板数据库为准（导入/导出/同步），应用到 ufw 或 firewalld；支持 Web 一键安装与端口防扫描。
package firewall

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

var portSpecRe = regexp.MustCompile(`^\d+(-\d+)?(,\d+(-\d+)?)*$`)

// panelListen 面板监听地址（install/sync 时防自锁放行）。
var panelListen string

// FirewallRule 面板托管的防火墙规则（导入导出与同步的权威来源）。
type FirewallRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Protocol  string    `gorm:"size:8" json:"protocol"`  // tcp | udp
	Ports     string    `gorm:"size:128" json:"ports"`   // 80,88,90-99（入参原样）
	Source    string    `gorm:"size:64" json:"source"`   // 空=所有IP，或 IP/CIDR
	Action    string    `gorm:"size:8" json:"action"`    // accept | deny
	Direction string    `gorm:"size:8" json:"direction"` // in | out
	Remark    string    `gorm:"size:128" json:"remark"`
	Auto      bool      `json:"auto"` // 系统生成（防扫描预置等）
	Applied   string    `gorm:"type:text" json:"-"` // 已执行命令（删除/同步重放用）
	CreatedAt time.Time `json:"created_at"`
}

func (FirewallRule) TableName() string { return "firewall_rules" }

var dbh *gorm.DB

func Register(api gin.IRouter, listen string, gdb *gorm.DB) {
	panelListen = listen
	dbh = gdb
	if err := gdb.AutoMigrate(&FirewallRule{}); err != nil {
		slog.Warn("防火墙规则表迁移失败", "err", err)
	}
	g := api.Group("/firewall")
	g.GET("/status", status)
	g.GET("/rules", listRules)
	g.POST("/rule", addRule)
	g.DELETE("/rule/:id", delRule)
	g.POST("/rules/import", importRules)
	g.GET("/rules/export", exportRules)
	g.POST("/sync", syncRules)
	g.POST("/anti-scan", antiScan)
	g.POST("/install", install)
}

func detect() (string, error) {
	if out, _, err := execx.Run("firewall", 10*time.Second, "ufw", "--version"); err == nil && strings.Contains(out, "ufw") {
		return "ufw", nil
	}
	if _, _, err := execx.Run("firewall", 10*time.Second, "firewall-cmd", "--version"); err == nil {
		return "firewalld", nil
	}
	return "", fmt.Errorf("未检测到 ufw 或 firewalld（面板需部署在 Linux 上）")
}

// detectPM 探测包管理器（debian→apt，rhel→dnf/yum）。
func detectPM() (string, error) {
	if _, _, err := execx.Run("firewall", 5*time.Second, "apt-get", "--version"); err == nil {
		return "apt", nil
	}
	if _, _, err := execx.Run("firewall", 5*time.Second, "dnf", "--version"); err == nil {
		return "dnf", nil
	}
	if _, _, err := execx.Run("firewall", 5*time.Second, "yum", "--version"); err == nil {
		return "yum", nil
	}
	return "", fmt.Errorf("未识别的包管理器（仅支持 apt/dnf/yum 系发行版）")
}

// panelPort 从监听地址取端口（用于防自锁放行）。
func panelPort() string {
	p := panelListen
	if idx := strings.LastIndex(p, ":"); idx >= 0 {
		p = p[idx+1:]
	}
	if p == "" || p == "0" {
		return "27958"
	}
	return p
}

func status(c *gin.Context) {
	tool, err := detect()
	if err != nil {
		core.OK(c, gin.H{"tool": "none", "enabled": false, "message": err.Error()})
		return
	}
	enabled := false
	antiScan := core.GetSetting(dbh, KAntiScan) == "1"
	if tool == "ufw" {
		out, _, _ := execx.Run("firewall", 10*time.Second, "ufw", "status")
		enabled = strings.Contains(out, "Status: active")
	} else {
		out, _, err := execx.Run("firewall", 10*time.Second, "firewall-cmd", "--state")
		enabled = err == nil && strings.TrimSpace(out) == "running"
	}
	core.OK(c, gin.H{"tool": tool, "enabled": enabled, "anti_scan": antiScan})
}

// ---- 端口规格展开：80,88,90-99 → ["80,88", "90:99"]（ufw 形式）----

func expandGroups(spec string) ([]string, error) {
	spec = strings.ReplaceAll(spec, "，", ",")
	tokens := strings.Split(spec, ",")
	var singles, ranges []string
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if strings.Contains(t, "-") {
			parts := strings.SplitN(t, "-", 2)
			lo, err1 := strconv.Atoi(parts[0])
			hi, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || lo < 1 || hi > 65535 || lo > hi {
				return nil, fmt.Errorf("端口范围 %s 无效", t)
			}
			ranges = append(ranges, fmt.Sprintf("%d:%d", lo, hi))
		} else {
			n, err := strconv.Atoi(t)
			if err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("端口 %s 无效", t)
			}
			singles = append(singles, t)
		}
	}
	if len(singles) == 0 && len(ranges) == 0 {
		return nil, fmt.Errorf("端口不能为空")
	}
	var groups []string
	if len(singles) > 0 {
		groups = append(groups, strings.Join(singles, ","))
	}
	groups = append(groups, ranges...)
	return groups, nil
}

// appliedCmd 已应用的命令（删除/同步重放）。
type appliedCmd struct {
	Args       []string `json:"args,omitempty"`        // ufw 参数（不含 ufw 本体）
	Shell      string   `json:"shell,omitempty"`       // firewalld shell
	RemoveArgs []string `json:"remove_args,omitempty"` // ufw 删除参数
	RemoveShell string  `json:"remove_shell,omitempty"`
}

func marshalApplied(cmds []appliedCmd) string {
	b, _ := json.Marshal(cmds)
	return string(b)
}

// ufwArgs 生成一条规则的 ufw 参数（不含 ufw 本体）。
func ufwArgs(r *FirewallRule, group string, withComment bool) []string {
	verb := "allow"
	if r.Action == "deny" {
		verb = "deny"
	}
	dir := ""
	if r.Direction == "out" {
		dir = "out"
	}
	var args []string
	if r.Source == "" {
		args = []string{verb, dir, group + "/" + r.Protocol}
	} else {
		args = []string{verb, dir, "from", r.Source, "to", "any", "port", group, "proto", r.Protocol}
	}
	// 压缩空元素
	clean := args[:0]
	for _, a := range args {
		if a != "" {
			clean = append(clean, a)
		}
	}
	args = clean
	if withComment && r.Remark != "" {
		args = append(args, "comment", r.Remark)
	}
	return args
}

// firewalldCmds 生成 firewalld 添加/删除命令。
func firewalldCmds(r *FirewallRule, group string) (shell, remove string) {
	if r.Source == "" && r.Action == "accept" && r.Direction == "in" {
		port := group
		if strings.Contains(group, ":") {
			port = group // firewalld 支持范围 90-99 用连字符
			port = strings.ReplaceAll(group, ":", "-")
		}
		shell = "firewall-cmd --permanent --add-port=" + port + "/" + r.Protocol
		remove = "firewall-cmd --permanent --remove-port=" + port + "/" + r.Protocol
		return
	}
	// rich rule
	var parts []string
	if r.Source != "" {
		family := "ipv4"
		if strings.Contains(r.Source, ":") {
			family = "ipv6"
		}
		parts = append(parts, fmt.Sprintf("family=\"%s\"", family), fmt.Sprintf("source address=\"%s\"", r.Source))
	}
	port := strings.ReplaceAll(group, ":", "-")
	parts = append(parts, fmt.Sprintf("port port=\"%s\" protocol=\"%s\"", port, r.Protocol))
	if r.Action == "deny" {
		parts = append(parts, "drop")
	} else {
		parts = append(parts, "accept")
	}
	rich := "rule " + strings.Join(parts, " ")
	_ = r.Direction // firewalld 出站需 direct 规则，这里统一入站富规则
	shell = "firewall-cmd --permanent --add-rich-rule='" + rich + "'"
	remove = "firewall-cmd --permanent --remove-rich-rule='" + rich + "'"
	return
}

// applyRule 执行规则（ufw/firewalld），返回已应用命令记录。
func applyRule(r *FirewallRule, tool string) ([]appliedCmd, error) {
	groups, err := expandGroups(r.Ports)
	if err != nil {
		return nil, err
	}
	if net.ParseIP(r.Source) == nil && r.Source != "" && !strings.Contains(r.Source, "/") {
		return nil, fmt.Errorf("来源应为 IP 或 CIDR（如 10.0.0.1、192.168.1.0/24）")
	}
	var cmds []appliedCmd
	for _, g := range groups {
		if tool == "ufw" {
			args := ufwArgs(r, g, true)
			out, code, err := execx.Run("firewall", 30*time.Second, "ufw", args...)
			if err != nil && strings.Contains(out, "comment") {
				// 老版本 ufw 不支持 comment，去注释重试
				args = ufwArgs(r, g, false)
				out, code, err = execx.Run("firewall", 30*time.Second, "ufw", args...)
			}
			if err != nil {
				return cmds, fmt.Errorf("ufw 执行失败(exit %d): %s", code, out)
			}
			cmds = append(cmds, appliedCmd{Args: args, RemoveArgs: append([]string{"delete"}, args...)})
		} else {
			shell, remove := firewalldCmds(r, g)
			out, code, err := execx.RunShell("firewall", 30*time.Second, shell)
			if err != nil && (strings.Contains(out, "ALREADY_ENABLED") || strings.Contains(out, "already")) {
				err = nil
			}
			if err != nil {
				return cmds, fmt.Errorf("firewall-cmd 执行失败(exit %d): %s", code, out)
			}
			cmds = append(cmds, appliedCmd{Shell: shell, RemoveShell: remove})
		}
	}
	if tool == "firewalld" {
		_, _, _ = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--reload")
	}
	return cmds, nil
}

// removeRule 按已应用记录回滚。
func removeRule(r *FirewallRule) {
	var cmds []appliedCmd
	if r.Applied != "" {
		_ = json.Unmarshal([]byte(r.Applied), &cmds)
	}
	tool, _ := detect()
	for _, cmd := range cmds {
		if tool == "ufw" && len(cmd.RemoveArgs) > 0 {
			_, _, _ = execx.Run("firewall", 30*time.Second, "ufw", cmd.RemoveArgs...)
		} else if cmd.RemoveShell != "" {
			_, _, _ = execx.RunShell("firewall", 30*time.Second, cmd.RemoveShell)
		}
	}
	if tool == "firewalld" {
		_, _, _ = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--reload")
	}
}

// ensureBase 防自锁：保证 SSH 与面板端口放行（直接 ufw/firewalld，不入库）。
func ensureBase() {
	tool, err := detect()
	if err != nil {
		return
	}
	if tool == "ufw" {
		_, _, _ = execx.Run("firewall", 30*time.Second, "ufw", "allow", "22/tcp")
		_, _, _ = execx.Run("firewall", 30*time.Second, "ufw", "allow", panelPort()+"/tcp")
	} else {
		_, _, _ = execx.RunShell("firewall", 30*time.Second, "firewall-cmd --permanent --add-port=22/tcp --permanent --add-port="+panelPort()+"/tcp && firewall-cmd --reload")
	}
}

// seedIfEmpty 首次接管时把现有放行规则导入面板（避免升级后列表为空）。
func seedIfEmpty() {
	var count int64
	dbh.Model(&FirewallRule{}).Count(&count)
	if count > 0 {
		return
	}
	tool, err := detect()
	if err != nil {
		return
	}
	now := time.Now()
	if tool == "ufw" {
		out, _, _ := execx.Run("firewall", 15*time.Second, "ufw", "status")
		seen := map[string]bool{}
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) < 2 || (f[1] != "ALLOW" && f[1] != "LIMIT") || strings.Contains(line, "(v6)") {
				continue
			}
			spec := f[0]
			if spec == "Anywhere" || seen[spec] {
				continue
			}
			seen[spec] = true
			port, proto := spec, "tcp"
			if i := strings.Index(spec, "/"); i > 0 {
				port, proto = spec[:i], spec[i+1:]
			}
			if !portSpecRe.MatchString(port) {
				continue
			}
			action := "accept"
			if f[1] == "LIMIT" {
				action = "deny"
			}
			row := FirewallRule{Protocol: proto, Ports: port, Source: "", Action: action, Direction: "in", Remark: "接管现有规则", Applied: marshalApplied(nil), CreatedAt: now}
			dbh.Create(&row)
		}
	} else {
		out, _, _ := execx.Run("firewall", 15*time.Second, "firewall-cmd", "--list-ports", "--permanent")
		for _, p := range strings.Fields(out) {
			port, proto := p, "tcp"
			if i := strings.Index(p, "/"); i > 0 {
				port, proto = p[:i], p[i+1:]
			}
			row := FirewallRule{Protocol: proto, Ports: port, Action: "accept", Direction: "in", Remark: "接管现有规则", CreatedAt: now}
			dbh.Create(&row)
		}
	}
}

// ---- 接口 ----

func listRules(c *gin.Context) {
	seedIfEmpty()
	var rows []FirewallRule
	dbh.Order("id DESC").Find(&rows)
	core.OK(c, rows)
}

func addRule(c *gin.Context) {
	var req struct {
		Protocol  string `binding:"required,oneof=tcp udp"`
		Ports     string `binding:"required"`
		Source    string `json:"source"`
		Action    string `binding:"required,oneof=accept deny"`
		Direction string `binding:"required,oneof=in out"`
		Remark    string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if _, err := expandGroups(req.Ports); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	tool, err := detect()
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	r := FirewallRule{Protocol: req.Protocol, Ports: strings.ReplaceAll(req.Ports, "，", ","), Source: strings.TrimSpace(req.Source), Action: req.Action, Direction: req.Direction, Remark: strings.TrimSpace(req.Remark)}
	cmds, err := applyRule(&r, tool)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	r.Applied = marshalApplied(cmds)
	if err := dbh.Create(&r).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("防火墙规则已添加", "ports", r.Ports, "action", r.Action, "source", r.Source)
	core.OK(c, r)
}

func delRule(c *gin.Context) {
	var r FirewallRule
	if err := dbh.First(&r, c.Param("id")).Error; err != nil {
		core.Err(c, 404, "规则不存在")
		return
	}
	removeRule(&r)
	dbh.Delete(&r)
	core.OK(c, nil)
}

// importRules 批量导入（JSON 数组，字段同 addRule）。
func importRules(c *gin.Context) {
	var req struct {
		Rules []FirewallRule `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	tool, err := detect()
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ok, fail := 0, 0
	var lastErr string
	for _, r := range req.Rules {
		r.ID = 0
		r.Protocol = strings.ToLower(strings.TrimSpace(r.Protocol))
		r.Action = strings.ToLower(strings.TrimSpace(r.Action))
		r.Direction = strings.ToLower(strings.TrimSpace(r.Direction))
		if r.Direction == "" {
			r.Direction = "in"
		}
		if r.Action == "" {
			r.Action = "accept"
		}
		if r.Protocol != "tcp" && r.Protocol != "udp" {
			fail++
			lastErr = "协议须为 tcp/udp: " + r.Protocol
			continue
		}
		if r.Action != "accept" && r.Action != "deny" {
			r.Action = "accept"
		}
		cmds, err := applyRule(&r, tool)
		if err != nil {
			fail++
			lastErr = err.Error()
			continue
		}
		r.Applied = marshalApplied(cmds)
		if err := dbh.Create(&r).Error; err != nil {
			fail++
			lastErr = err.Error()
			continue
		}
		ok++
	}
	msg := fmt.Sprintf("导入 %d 条", ok)
	if fail > 0 {
		msg += fmt.Sprintf("，失败 %d 条（%s）", fail, lastErr)
	}
	core.OK(c, gin.H{"ok": ok, "fail": fail, "message": msg})
}

// exportRules 导出全部规则（JSON 下载）。
func exportRules(c *gin.Context) {
	var rows []FirewallRule
	dbh.Order("id ASC").Find(&rows)
	for i := range rows {
		rows[i].Applied = ""
	}
	data, _ := json.MarshalIndent(rows, "", "  ")
	c.Header("Content-Disposition", "attachment; filename=hanyuxin-firewall-rules.json")
	c.Data(200, "application/json", data)
}

// syncRules 同步规则：防自锁放行 → 全量重放面板规则（ufw 同命令幂等）。
func syncRules(c *gin.Context) {
	tool, err := detect()
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ensureBase()
	var rows []FirewallRule
	dbh.Order("id ASC").Find(&rows)
	ok, fail := 0, 0
	var lastErr string
	for i := range rows {
		cmds, err := applyRule(&rows[i], tool)
		if err != nil {
			fail++
			lastErr = err.Error()
			continue
		}
		rows[i].Applied = marshalApplied(cmds)
		dbh.Model(&rows[i]).Update("applied", rows[i].Applied)
		ok++
	}
	msg := fmt.Sprintf("同步 %d 条规则", ok)
	if fail > 0 {
		msg += fmt.Sprintf("，失败 %d 条（%s）", fail, lastErr)
	}
	core.OK(c, gin.H{"ok": ok, "fail": fail, "message": msg})
}

// antiScan 端口防扫描：SSH 限速（ufw limit）+ 预置高危端口拒绝规则。
var antiScanPorts = []string{"21", "23", "135", "137", "138", "139", "445", "3389"}

const KAntiScan = "firewall_anti_scan"

func antiScan(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	tool, err := detect()
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	if req.Enabled {
		if tool == "ufw" {
			_, _, _ = execx.Run("firewall", 30*time.Second, "ufw", "limit", "22/tcp")
		}
		for _, p := range antiScanPorts {
			var count int64
			dbh.Model(&FirewallRule{}).Where("auto = ? AND ports = ?", true, p).Count(&count)
			if count > 0 {
				continue
			}
			r := FirewallRule{Protocol: "tcp", Ports: p, Action: "deny", Direction: "in", Remark: "防扫描（高危端口）", Auto: true}
			cmds, err := applyRule(&r, tool)
			if err != nil {
				continue
			}
			r.Applied = marshalApplied(cmds)
			dbh.Create(&r)
		}
		core.SetSetting(dbh, KAntiScan, "1")
		core.OK(c, gin.H{"enabled": true, "message": "防扫描已开启：SSH 限速 + " + strconv.Itoa(len(antiScanPorts)) + " 个高危端口拒绝"})
		return
	}
	// 关闭：移除自动规则
	var rows []FirewallRule
	dbh.Where("auto = ?", true).Find(&rows)
	for i := range rows {
		removeRule(&rows[i])
		dbh.Delete(&rows[i])
	}
	if tool == "ufw" {
		_, _, _ = execx.Run("firewall", 30*time.Second, "ufw", "limit", "22/tcp")
	}
	core.SetSetting(dbh, KAntiScan, "0")
	core.OK(c, gin.H{"enabled": false, "message": "防扫描已关闭（SSH 限速保留）"})
}

// install 一键安装防火墙（默认安装）。防自锁：启用前先放行 SSH 与面板端口。
func install(c *gin.Context) {
	if _, err := detect(); err == nil {
		core.OK(c, gin.H{"ok": true, "message": "防火墙已存在，无需安装"})
		return
	}
	pm, err := detectPM()
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	var tool, out string
	var code int
	if pm == "apt" {
		out, code, err = execx.Run("firewall", 600*time.Second, "sh", "-c", "DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ufw")
		tool = "ufw"
		if err != nil {
			core.Err(c, 500, fmt.Sprintf("安装 ufw 失败(exit %d): %s", code, out))
			return
		}
		out, _, _ = execx.Run("firewall", 30*time.Second, "ufw", "allow", "22/tcp")
		out, _, _ = execx.Run("firewall", 30*time.Second, "ufw", "allow", panelPort()+"/tcp")
		out, code, err = execx.Run("firewall", 30*time.Second, "ufw", "--force", "enable")
		if err != nil {
			core.Err(c, 500, fmt.Sprintf("启用 ufw 失败(exit %d): %s（已放行 22 与面板端口 %s）", code, out, panelPort()))
			return
		}
	} else {
		out, code, err = execx.Run("firewall", 600*time.Second, "sh", "-c", pm+" install -y firewalld && systemctl enable --now firewalld")
		tool = "firewalld"
		if err != nil {
			core.Err(c, 500, fmt.Sprintf("安装 firewalld 失败(exit %d): %s", code, out))
			return
		}
		out, _, _ = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--permanent", "--add-port="+panelPort()+"/tcp")
		_, _, _ = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--reload")
	}
	slog.Info("防火墙安装完成", "tool", tool, "panel_port", panelPort())
	core.OK(c, gin.H{"ok": true, "tool": tool, "message": "安装完成，已放行 SSH(22/tcp) 与面板端口 " + panelPort() + "/tcp"})
}
