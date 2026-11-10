// Package firewall 防火墙管理：ufw (Debian/Ubuntu) 与 firewalld (RHEL) 统一抽象，支持 Web 一键安装。
package firewall

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

var portSpecRe = regexp.MustCompile(`^\d+(-\d+)?/(tcp|udp)$`)

// panelListen 面板监听地址（install 时防自锁放行）。
var panelListen string

func Register(api gin.IRouter, listen string) {
	panelListen = listen
	g := api.Group("/firewall")
	g.GET("/status", status)
	g.GET("/ports", ports)
	g.POST("/port", changePort)
	g.POST("/switch", switchOn)
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

// install 一键安装防火墙（默认安装）。防自锁：启用前先放行 SSH(22/tcp) 与面板端口。
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
	keep := func(step string, o string, e error, cd int) {
		if e != nil {
			slog.Warn("防火墙安装步骤失败", "step", step, "exit", cd, "out", strings.TrimSpace(o))
		}
	}
	if pm == "apt" {
		out, code, err = execx.Run("firewall", 600*time.Second, "sh", "-c", "DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ufw")
		tool = "ufw"
		if err != nil {
			core.Err(c, 500, fmt.Sprintf("安装 ufw 失败(exit %d): %s", code, out))
			return
		}
		// 防自锁：先放行 SSH 与面板端口再启用
		out, code, _ = execx.Run("firewall", 30*time.Second, "ufw", "allow", "22/tcp")
		keep("allow ssh", out, nil, code)
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
		out, _, err = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--reload")
		keep("reload", out, err, code)
	}
	slog.Info("防火墙安装完成", "tool", tool, "panel_port", panelPort())
	core.OK(c, gin.H{"ok": true, "tool": tool, "message": "安装完成，已放行 SSH(22/tcp) 与面板端口 " + panelPort() + "/tcp"})
}

func status(c *gin.Context) {
	tool, err := detect()
	if err != nil {
		core.OK(c, gin.H{"tool": "none", "enabled": false, "message": err.Error()})
		return
	}
	enabled := false
	if tool == "ufw" {
		out, _, _ := execx.Run("firewall", 10*time.Second, "ufw", "status")
		enabled = strings.Contains(out, "Status: active")
	} else {
		out, _, err := execx.Run("firewall", 10*time.Second, "firewall-cmd", "--state")
		enabled = err == nil && strings.TrimSpace(out) == "running"
	}
	core.OK(c, gin.H{"tool": tool, "enabled": enabled})
}

func ports(c *gin.Context) {
	tool, err := detect()
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	var list []string
	if tool == "ufw" {
		out, _, _ := execx.Run("firewall", 15*time.Second, "ufw", "status")
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "(v6)") {
				continue
			}
			f := strings.Fields(line)
			if len(f) >= 2 && (f[1] == "ALLOW" || f[1] == "LIMIT") && strings.Contains(f[0], "/") {
				list = append(list, f[0])
			}
		}
	} else {
		out, _, _ := execx.Run("firewall", 15*time.Second, "firewall-cmd", "--list-ports", "--permanent")
		for _, p := range strings.Fields(out) {
			if p != "" {
				list = append(list, p)
			}
		}
	}
	core.OK(c, gin.H{"tool": tool, "ports": list})
}

func changePort(c *gin.Context) {
	var req struct {
		Port   string `binding:"required"` // 如 8080/tcp
		Action string `binding:"required,oneof=add remove"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !portSpecRe.MatchString(req.Port) {
		core.Err(c, 400, "端口格式应为 8080/tcp 或 53/udp")
		return
	}
	tool, err := detect()
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	var out string
	var code int
	if tool == "ufw" {
		if req.Action == "add" {
			out, code, err = execx.Run("firewall", 30*time.Second, "ufw", "allow", req.Port)
		} else {
			out, code, err = execx.Run("firewall", 30*time.Second, "ufw", "delete", "allow", req.Port)
		}
	} else {
		if req.Action == "add" {
			out, code, err = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--add-port="+req.Port, "--permanent")
			if err == nil {
				_, _, err = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--reload")
			}
		} else {
			out, code, err = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--remove-port="+req.Port, "--permanent")
			if err == nil {
				_, _, err = execx.Run("firewall", 30*time.Second, "firewall-cmd", "--reload")
			}
		}
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("执行失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, nil)
}

func switchOn(c *gin.Context) {
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
	var out string
	var code int
	if tool == "ufw" {
		if req.Enabled {
			out, code, err = execx.Run("firewall", 30*time.Second, "ufw", "--force", "enable")
		} else {
			out, code, err = execx.Run("firewall", 30*time.Second, "ufw", "disable")
		}
	} else {
		if req.Enabled {
			out, code, err = execx.Run("firewall", 60*time.Second, "systemctl", "start", "firewalld")
			if err == nil {
				_, _, _ = execx.Run("firewall", 30*time.Second, "systemctl", "enable", "firewalld")
			}
		} else {
			out, code, err = execx.Run("firewall", 60*time.Second, "systemctl", "stop", "firewalld")
		}
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("执行失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, nil)
}
