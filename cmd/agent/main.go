// webmblx-agent —— 多节点 Agent：反连面板上报指标并执行下发的命令。
//
// 用法: webmblx-agent -s ws://<面板IP>:<端口> -t <节点token> [-name <节点名>]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

type msgOut struct {
	Type     string    `json:"type"`
	Hostname string    `json:"hostname,omitempty"`
	OS       string    `json:"os,omitempty"`
	Arch     string    `json:"arch,omitempty"`
	Version  string    `json:"version,omitempty"`
	ID       string    `json:"id,omitempty"`
	Output   string    `json:"output,omitempty"`
	Exit     int       `json:"exit,omitempty"`
	Err      string    `json:"err,omitempty"`
	Sample   *sample   `json:"sample,omitempty"`
}

type sample struct {
	CPU      float64 `json:"cpu"`
	MemPct   float64 `json:"mem_pct"`
	MemUsed  uint64  `json:"mem_used"`
	MemTotal uint64  `json:"mem_total"`
	Recv     uint64  `json:"recv"`
	Sent     uint64  `json:"sent"`
	Load1    float64 `json:"load1"`
}

type msgIn struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Script     string `json:"script"`
	TimeoutSec int    `json:"timeout_sec"`
}

type conn struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (c *conn) send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(v)
}

func main() {
	server := flag.String("s", "", "面板地址，如 ws://1.2.3.4:24567")
	token := flag.String("t", "", "节点接入 token（面板-节点管理页获取）")
	name := flag.String("name", "", "节点名称（默认取主机名）")
	flag.Parse()

	if *server == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "用法: webmblx-agent -s ws://<面板IP>:<端口> -t <token> [-name <节点名>]")
		os.Exit(1)
	}
	nodeName := *name
	if nodeName == "" {
		hn, _ := os.Hostname()
		nodeName = hn
	}

	hi, _ := host.Info()
	hostname := hi.Hostname

	slog.Info("webmblx-agent 启动", "version", core.Version, "server", *server, "name", nodeName)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	backoff := 3 * time.Second
	for {
		select {
		case <-quit:
			slog.Info("agent 退出")
			return
		default:
		}
		if err := run(*server, *token, nodeName, hostname, quit); err != nil {
			slog.Warn("连接中断", "err", err, "reconnect_in", backoff)
		}
		select {
		case <-quit:
			slog.Info("agent 退出")
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func run(server, token, nodeName, hostname string, quit <-chan os.Signal) error {
	wsURL, err := toWSUrl(server)
	if err != nil {
		return err
	}
	dialURL := fmt.Sprintf("%s/api/ws/agent?token=%s&name=%s", wsURL, url.QueryEscape(token), url.QueryEscape(nodeName))
	ws, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		return fmt.Errorf("连接面板失败: %w", err)
	}
	defer ws.Close()
	c := &conn{ws: ws}

	hi, _ := host.Info()
	if err := c.send(msgOut{
		Type: "hello", Hostname: hostname, OS: hi.OS + " " + hi.Platform,
		Arch: hi.KernelArch, Version: core.Version,
	}); err != nil {
		return err
	}
	slog.Info("已连接面板并注册")

	// 指标采样上报（15s）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sampler(ctx, c)

	// 读取面板下发
	for {
		var in msgIn
		if err := ws.ReadJSON(&in); err != nil {
			return err
		}
		if in.Type == "exec" {
			go handleExec(c, in)
		}
	}
}

func handleExec(c *conn, in msgIn) {
	timeout := time.Duration(in.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, exit, err := execx.RunShellContext(ctx, "agent:"+in.ID, in.Script)
	res := msgOut{Type: "exec_result", ID: in.ID, Output: output, Exit: exit}
	if err != nil {
		res.Err = err.Error()
	}
	if err := c.send(res); err != nil {
		slog.Warn("回传执行结果失败", "id", in.ID, "err", err)
	}
}

func sampler(ctx context.Context, c *conn) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var lastNetTime time.Time
	var last gnet.IOCountersStat
	emit := func() {
		s := &sample{}
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
			if !lastNetTime.IsZero() {
				if dt := now.Sub(lastNetTime).Seconds(); dt > 0 {
					dR := int64(n[0].BytesRecv - last.BytesRecv)
					dS := int64(n[0].BytesSent - last.BytesSent)
					if dR < 0 {
						dR = 0
					}
					if dS < 0 {
						dS = 0
					}
					s.Recv = uint64(float64(dR) / dt)
					s.Sent = uint64(float64(dS) / dt)
				}
			}
			last = n[0]
			lastNetTime = now
		}
		if l, err := load.Avg(); err == nil {
			s.Load1 = l.Load1
		}
		if err := c.send(msgOut{Type: "metric", Sample: s}); err != nil {
			slog.Debug("指标上报失败", "err", err)
		}
	}
	emit() // 连接后立即上报一次（首次 CPU 为 0 属正常）
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			emit()
		}
	}
}

func toWSUrl(server string) (string, error) {
	s := strings.TrimSpace(server)
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") &&
		!strings.HasPrefix(s, "ws://") && !strings.HasPrefix(s, "wss://") {
		s = "ws://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("面板地址非法: %s", server)
	}
	switch u.Scheme {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	}
	return u.String(), nil
}
