// hanyuxin-agent —— 寒语馨面板多节点 Agent：反连面板上报指标并执行下发的命令。
//
// 用法: hanyuxin-agent -s ws://<面板IP>:<端口> -t <节点token> [-name <节点名>]
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

type fileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

type msgOut struct {
	Type     string      `json:"type"`
	Hostname string      `json:"hostname,omitempty"`
	OS       string      `json:"os,omitempty"`
	Arch     string      `json:"arch,omitempty"`
	Version  string      `json:"version,omitempty"`
	ID       string      `json:"id,omitempty"`
	Output   string      `json:"output,omitempty"`
	Exit     int         `json:"exit,omitempty"`
	Err      string      `json:"err,omitempty"`
	Sample   *sample     `json:"sample,omitempty"`
	Data     string      `json:"data,omitempty"`     // file read 的 base64
	Entries  []fileEntry `json:"entries,omitempty"`  // file ls
	DataB64  bool        `json:"data_b64,omitempty"` // Output 是否 base64（term_out 二进制安全）
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
	Path       string `json:"path,omitempty"`
	Data       string `json:"data,omitempty"` // file write 的 base64 内容 / term_in 输入
	Force      bool   `json:"force,omitempty"`
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
		fmt.Fprintln(os.Stderr, "用法: hanyuxin-agent -s ws://<面板IP>:<端口> -t <token> [-name <节点名>]")
		os.Exit(1)
	}
	nodeName := *name
	if nodeName == "" {
		hn, _ := os.Hostname()
		nodeName = hn
	}

	hi, _ := host.Info()
	hostname := hi.Hostname

	slog.Info("hanyuxin-agent 启动", "version", core.Version, "server", *server, "name", nodeName)

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
		switch in.Type {
		case "exec":
			go handleExec(c, in)
		case "file":
			go handleFile(c, in)
		case "term_open":
			handleTermOpen(c, in)
		case "term_in":
			handleTermIn(in)
		case "term_close":
			handleTermClose(in)
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

// handleFile 文件操作：ls | read | write | delete。
func handleFile(c *conn, in msgIn) {
	res := msgOut{Type: "file_result", ID: in.ID}
	defer func() {
		if r := recover(); r != nil {
			res.Err = fmt.Sprintf("panic: %v", r)
			_ = c.send(res)
		}
	}()
	// 路径安全：禁止 .. 穿越
	if strings.Contains(in.Path, "..") {
		res.Err = "路径不允许包含 .."
		_ = c.send(res)
		return
	}
	// Script 字段在此协议中作为 file op 传递（ls/read/write/delete）
	switch in.Script {
	case "ls":
		entries, err := os.ReadDir(in.Path)
		if err != nil {
			res.Err = err.Error()
			break
		}
		out := []fileEntry{}
		for _, e := range entries {
			fi, _ := e.Info()
			var size int64
			var mode, mt string
			if fi != nil {
				size = fi.Size()
				mode = fi.Mode().String()
				mt = fi.ModTime().Format("2006-01-02 15:04:05")
			}
			out = append(out, fileEntry{Name: e.Name(), Size: size, IsDir: e.IsDir(), Mode: mode, ModTime: mt})
		}
		res.Entries = out
	case "read":
		data, err := os.ReadFile(in.Path)
		if err != nil {
			res.Err = err.Error()
			break
		}
		if len(data) > 8<<20 {
			res.Err = "文件超过 8MB，请用其他方式传输"
			break
		}
		res.Data = base64.StdEncoding.EncodeToString(data)
		res.DataB64 = true
	case "write":
		data, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil {
			res.Err = err.Error()
			break
		}
		if err := os.WriteFile(in.Path, data, 0o644); err != nil {
			res.Err = err.Error()
			break
		}
	case "delete":
		if !in.Force {
			res.Err = "删除需确认"
			break
		}
		if err := os.RemoveAll(in.Path); err != nil {
			res.Err = err.Error()
		}
	default:
		res.Err = "未知文件操作"
	}
	if err := c.send(res); err != nil {
		slog.Warn("回传文件结果失败", "id", in.ID, "err", err)
	}
}

// ---- 节点终端：bash 子进程流式转发 ----

type agentTerm struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	c     *conn
}

var terms = struct {
	mu sync.Mutex
	m  map[string]*agentTerm
}{m: map[string]*agentTerm{}}

func handleTermOpen(c *conn, in msgIn) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if _, err := os.Stat("/bin/bash"); err == nil {
			shell = "/bin/bash"
		} else {
			shell = "/bin/sh"
		}
	}
	cmd := exec.Command(shell, "-i")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = c.send(msgOut{Type: "term_out", ID: in.ID, Output: "打开终端失败: " + err.Error()})
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = c.send(msgOut{Type: "term_out", ID: in.ID, Output: "打开终端失败: " + err.Error()})
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		_ = c.send(msgOut{Type: "term_out", ID: in.ID, Output: "启动 shell 失败: " + err.Error()})
		return
	}
	t := &agentTerm{cmd: cmd, stdin: stdin, c: c}
	terms.mu.Lock()
	terms.m[in.ID] = t
	terms.mu.Unlock()
	slog.Info("节点终端会话开启", "id", in.ID)
	go func() {
		buf := make([]byte, 4096)
		for {
			nr, err := stdout.Read(buf)
			if nr > 0 {
				chunk := base64.StdEncoding.EncodeToString(buf[:nr])
				if err := c.send(msgOut{Type: "term_out", ID: in.ID, Output: chunk, DataB64: true}); err != nil {
					break
				}
			}
			if err != nil {
				_ = c.send(msgOut{Type: "term_out", ID: in.ID, Output: "\r\n[会话已结束]"})
				break
			}
		}
		terms.mu.Lock()
		delete(terms.m, in.ID)
		terms.mu.Unlock()
		_ = cmd.Wait()
	}()
}

func handleTermIn(in msgIn) {
	terms.mu.Lock()
	t := terms.m[in.ID]
	terms.mu.Unlock()
	if t == nil {
		return
	}
	data, err := base64.StdEncoding.DecodeString(in.Data)
	if err == nil {
		_, _ = t.stdin.Write(data)
	}
}

func handleTermClose(in msgIn) {
	terms.mu.Lock()
	t := terms.m[in.ID]
	terms.mu.Unlock()
	if t != nil {
		_ = t.cmd.Process.Kill()
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
