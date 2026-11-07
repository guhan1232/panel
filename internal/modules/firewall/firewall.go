// Package firewall 防火墙管理：ufw (Debian/Ubuntu) 与 firewalld (RHEL) 统一抽象。
package firewall

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

var portSpecRe = regexp.MustCompile(`^\d+(-\d+)?/(tcp|udp)$`)

func Register(api gin.IRouter) {
	g := api.Group("/firewall")
	g.GET("/status", status)
	g.GET("/ports", ports)
	g.POST("/port", changePort)
	g.POST("/switch", switchOn)
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
