// Package proc 进程与 systemd 服务管理。
package proc

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v4/process"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

var unitNameRe = regexp.MustCompile(`^[A-Za-z0-9@._\\-]+\.service$`)

type ProcInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu"`
	MemPct float32 `json:"mem_pct"`
}

type ServiceInfo struct {
	Unit        string `json:"unit"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Enabled     string `json:"enabled"`
	Description string `json:"description"`
}

func Register(api gin.IRouter) {
	g := api.Group("/proc")
	g.GET("/list", mlist)
	g.POST("/kill", mkill)
	g.GET("/services", mservices)
	g.POST("/service", mservice)
}

func mlist(c *gin.Context) {
	sortBy := c.DefaultQuery("sort", "cpu")
	procs, err := process.Processes()
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	list := make([]ProcInfo, 0, len(procs))
	for _, p := range procs {
		name, _ := p.Name()
		if name == "" {
			continue
		}
		cp, _ := p.CPUPercent()
		mp, _ := p.MemoryPercent()
		list = append(list, ProcInfo{PID: p.Pid, Name: name, CPU: cp, MemPct: mp})
	}
	sort.Slice(list, func(i, j int) bool {
		if sortBy == "mem" {
			return list[i].MemPct > list[j].MemPct
		}
		return list[i].CPU > list[j].CPU
	})
	if len(list) > 100 {
		list = list[:100]
	}
	core.OK(c, list)
}

func mkill(c *gin.Context) {
	var req struct {
		PID   int  `binding:"required"`
		Force bool `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	p, err := process.NewProcess(int32(req.PID))
	if err != nil {
		core.Err(c, 404, "进程不存在")
		return
	}
	if req.Force {
		err = p.Kill()
	} else {
		err = p.Terminate()
	}
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func mservices(c *gin.Context) {
	out, _, err := execx.Run("proc", 30*time.Second, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	if err != nil && out == "" {
		core.Err(c, 500, "systemctl 不可用（面板需部署在 Linux 上）: "+err.Error())
		return
	}
	enabled := map[string]string{}
	if out2, _, err2 := execx.Run("proc", 30*time.Second, "systemctl", "list-unit-files", "--type=service", "--no-pager", "--no-legend"); err2 == nil {
		for _, line := range strings.Split(out2, "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				enabled[f[0]] = f[1]
			}
		}
	}
	items := []ServiceInfo{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		desc := ""
		if idx := strings.Index(line, f[3]); idx >= 0 {
			desc = strings.TrimSpace(line[idx+len(f[3]):])
		}
		items = append(items, ServiceInfo{
			Unit: f[0], Active: f[2], Sub: f[3],
			Enabled: enabled[f[0]], Description: desc,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Unit < items[j].Unit })
	core.OK(c, items)
}

func loadOf(line string) string {
	f := strings.Fields(line)
	if len(f) >= 2 {
		return f[1]
	}
	return ""
}

func mservice(c *gin.Context) {
	var req struct {
		Name   string `binding:"required"`
		Action string `binding:"required,oneof=start stop restart enable disable"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !unitNameRe.MatchString(req.Name) {
		core.Err(c, 400, "服务名非法")
		return
	}
	out, code, err := execx.Run("proc", 60*time.Second, "systemctl", req.Action, req.Name)
	if err != nil {
		core.Err(c, 500, "执行失败(exit "+strconv.Itoa(code)+"): "+out)
		return
	}
	core.OK(c, nil)
}
