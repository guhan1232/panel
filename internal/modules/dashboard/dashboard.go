// Package dashboard 系统监控仪表盘：后台采样器 + 内存环形历史 + REST 查询。
package dashboard

import (
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	interval   = 3 * time.Second
	historyLen = 600 // ≈ 30 分钟
)

// Sample 一次系统采样。
type Sample struct {
	Time     int64   `json:"time"`
	CPU      float64 `json:"cpu"`        // CPU 占用 %
	MemPct   float64 `json:"mem_pct"`    // 内存占用 %
	MemUsed  uint64  `json:"mem_used"`   // 字节
	MemTotal uint64  `json:"mem_total"`  // 字节
	Recv     uint64  `json:"recv"`       // 网络接收 B/s
	Sent     uint64  `json:"sent"`       // 网络发送 B/s
	Load1    float64 `json:"load1"`      // 1 分钟负载
}

type DiskInfo struct {
	Mount   string  `json:"mount"`
	Fs      string  `json:"fs"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	UsedPct float64 `json:"used_pct"`
}

type ProcInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu"`
	MemPct float32 `json:"mem_pct"`
}

type Module struct {
	mu          sync.Mutex
	history     []Sample
	lastNet     gnet.IOCountersStat
	lastNetTime time.Time
}

func New() *Module {
	m := &Module{}
	go m.run()
	return m
}

func (m *Module) run() {
	for {
		s := m.sample()
		m.mu.Lock()
		m.history = append(m.history, s)
		if len(m.history) > historyLen {
			m.history = m.history[len(m.history)-historyLen:]
		}
		m.mu.Unlock()
		time.Sleep(interval)
	}
}

func (m *Module) sample() Sample {
	s := Sample{Time: time.Now().Unix()}
	if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
		s.CPU = p[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemPct = vm.UsedPercent
		s.MemUsed = vm.Used
		s.MemTotal = vm.Total
	}
	if n, err := gnet.IOCounters(false); err == nil && len(n) > 0 {
		now := time.Now()
		if !m.lastNetTime.IsZero() {
			if dt := now.Sub(m.lastNetTime).Seconds(); dt > 0 {
				dRecv := int64(n[0].BytesRecv - m.lastNet.BytesRecv)
				dSent := int64(n[0].BytesSent - m.lastNet.BytesSent)
				if dRecv < 0 {
					dRecv = 0
				}
				if dSent < 0 {
					dSent = 0
				}
				s.Recv = uint64(float64(dRecv) / dt)
				s.Sent = uint64(float64(dSent) / dt)
			}
		}
		m.lastNet = n[0]
		m.lastNetTime = now
	}
	if l, err := load.Avg(); err == nil {
		s.Load1 = l.Load1
	}
	return s
}

func (m *Module) Register(api gin.IRouter) {
	api.GET("/dashboard/summary", m.summary)
}

func (m *Module) summary(c *gin.Context) {
	m.mu.Lock()
	h := append([]Sample(nil), m.history...)
	m.mu.Unlock()
	if len(h) > 120 {
		h = h[len(h)-120:]
	}

	hi, _ := host.Info()

	var disks []DiskInfo
	if parts, err := disk.Partitions(false); err == nil {
		seen := map[string]bool{}
		for _, p := range parts {
			if seen[p.Mountpoint] {
				continue
			}
			seen[p.Mountpoint] = true
			if u, err := disk.Usage(p.Mountpoint); err == nil && u.Total > 0 {
				disks = append(disks, DiskInfo{
					Mount: p.Mountpoint, Fs: p.Fstype,
					Total: u.Total, Used: u.Used, UsedPct: u.UsedPercent,
				})
			}
		}
	}

	tops := m.topProcesses()

	// 当前内存详情
	var memTotal, memUsed uint64
	if vm, err := mem.VirtualMemory(); err == nil {
		memTotal, memUsed = vm.Total, vm.Used
	}

	core.OK(c, gin.H{
		"host":     hi,
		"current":  lastOf(h),
		"history":  h,
		"disks":    disks,
		"procs":    tops,
		"mem_used": memUsed,
		"mem_total": memTotal,
	})
}

func lastOf(h []Sample) Sample {
	if len(h) == 0 {
		return Sample{Time: time.Now().Unix()}
	}
	return h[len(h)-1]
}

func (m *Module) topProcesses() []ProcInfo {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	list := make([]ProcInfo, 0, len(procs))
	for _, p := range procs {
		cp, err1 := p.CPUPercent()
		name, _ := p.Name()
		if err1 != nil || (cp == 0 && name == "") {
			continue
		}
		mp, _ := p.MemoryPercent()
		list = append(list, ProcInfo{PID: p.Pid, Name: name, CPU: cp, MemPct: mp})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CPU > list[j].CPU })
	if len(list) > 10 {
		list = list[:10]
	}
	return list
}
