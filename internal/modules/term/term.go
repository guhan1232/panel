// Package term Web 终端：WebSocket ↔ PTY。
package term

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/platform"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		if r.Header.Get("Origin") == "" {
			return true
		}
		u, err := url.Parse(r.Header.Get("Origin"))
		return err == nil && u.Host == r.Host
	},
}

type wsMsg struct {
	Type string `json:"type"` // input | resize | cred | idle | connected | exit
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
	// cred（快速连接）专用字段
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Username   string `json:"username,omitempty"`
	AuthType   string `json:"auth_type,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

// TerminalLog 终端命令行记录。
type TerminalLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:64;index" json:"username"`
	Line      string    `gorm:"size:512" json:"line"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07\x1b]*\x07|\x1b[()][B0]`)

// Register 注册终端路由（须挂在鉴权中间件之后）与命令记录查询。
func Register(api gin.IRouter, db *gorm.DB) {
	if err := db.AutoMigrate(&TerminalLog{}); err != nil {
		slog.Warn("终端记录表迁移失败", "err", err)
	}
	api.GET("/ws/terminal", func(c *gin.Context) { handle(c, db) })
}

// RegisterSSH 多标签 SSH 终端 WS：?host_id=N 或 ?quick=1（临时连接，凭据在首条消息里，用后即弃）。
func RegisterSSH(api gin.IRouter, db *gorm.DB, cfg *config.Config) {
	initCrypt(cfg)
	if err := db.AutoMigrate(&SSHHost{}); err != nil {
		slog.Warn("SSH 主机表迁移失败", "err", err)
	}
	api.GET("/ws/ssh", func(c *gin.Context) { handleSSH(c, db) })
}

// handleSSH SSH 远程终端会话。
// 连接方式二选一：
//  1. ?host_id=N —— 使用收藏主机存储的加密凭据；
//  2. 首条 wsMsg{Type:"cred", ...} —— 临时快速连接，凭据仅存于本次会话内存，连接结束即丢弃。
// 会话空闲（无 input）超过 idleSeconds 触发提醒帧 {type:"idle"}，前端弹窗确认；再超时服务端断开。
func handleSSH(c *gin.Context, db *gorm.DB) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	user := c.GetString("user")
	rec := &lineRecorder{user: user, db: db}

	cols := uint16(parseIntDefault(c.Query("cols"), 120))
	rows := uint16(parseIntDefault(c.Query("rows"), 32))

	var cli *SSHSession
	var hostName string

	if hid := parseIntDefault(c.Query("host_id"), 0); hid > 0 {
		var h SSHHost
		if err := db.First(&h, hid).Error; err != nil {
			_ = ws.WriteJSON(wsMsg{Type: "exit", Data: "主机不存在或已删除"})
			return
		}
		if h.Name == "" {
			h.Name = h.Host
		}
		hostName = h.Name
		cli, err = sshDial(&h, cols, rows)
	} else {
		// 等待首条凭据消息
		var m wsMsg
		if err := ws.ReadJSON(&m); err != nil || m.Type != "cred" {
			return
		}
		hostName = m.Host
		cli, err = dialQuick(m, cols, rows)
	}
	if err != nil {
		_ = ws.WriteJSON(wsMsg{Type: "exit", Data: "SSH 连接失败: " + err.Error()})
		return
	}
	defer cli.Close()
	slog.Info("SSH 终端会话建立", "user", user, "host", hostName, "ip", c.ClientIP())
	_ = ws.WriteJSON(wsMsg{Type: "connected", Data: hostName})

	// 空闲监控：最后一次 input 时间；提醒后仍未活动则断开
	const idleWarn = 10 * time.Minute
	const idleDrop = 15 * time.Minute
	var mu sync.Mutex
	lastInput := time.Now()
	warned := false
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				mu.Lock()
				idle := time.Since(lastInput)
				w := warned
				mu.Unlock()
				if idle > idleWarn && !w {
					_ = ws.WriteJSON(wsMsg{Type: "idle", Data: fmt.Sprintf("已空闲 %s，确认继续使用？超过 %s 将自动断开", "10 分钟", "15 分钟")})
					mu.Lock()
					warned = true
					mu.Unlock()
				} else if idle > idleDrop {
					_ = ws.WriteJSON(wsMsg{Type: "exit", Data: "会话空闲超时，已自动断开"})
					_ = ws.Close()
					return
				}
			}
		}
	}()

	var wg sync.WaitGroup
	// ssh pty → ws
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 8192)
		for {
			n, err := cli.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.TextMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				_ = ws.Close()
				return
			}
		}
	}()

	// ws → ssh pty
	for {
		var m wsMsg
		if err := ws.ReadJSON(&m); err != nil {
			break
		}
		switch m.Type {
		case "input":
			mu.Lock()
			lastInput = time.Now()
			warned = false
			mu.Unlock()
			if _, werr := cli.Write([]byte(m.Data)); werr != nil {
				slog.Debug("SSH 终端写入失败", "err", werr)
			}
			rec.feed(m.Data)
		case "resize":
			_ = cli.Resize(m.Cols, m.Rows)
		}
	}
	cli.Close()
	wg.Wait()
	slog.Info("SSH 终端会话结束", "user", user, "host", hostName, "ip", c.ClientIP())
}

// dialQuick 临时快速连接（凭据仅在内存中短暂使用，不落库）。
func dialQuick(m wsMsg, cols, rows uint16) (*SSHSession, error) {
	h := &SSHHost{
		Host: m.Host, Port: m.Port, Username: m.Username,
		AuthType: m.AuthType, Password: encrypt(m.Password),
		PrivateKey: encrypt(m.PrivateKey), Passphrase: encrypt(m.Passphrase),
	}
	if h.Port <= 0 {
		h.Port = 22
	}
	return sshDial(h, cols, rows)
}

// lineRecorder 逐行记录终端输入（回车触发落库，剥离 ANSI 转义）。
type lineRecorder struct {
	user string
	db   *gorm.DB
	mu   sync.Mutex
	buf  strings.Builder
}

// secretRe 命令行中的敏感参数（密码/令牌/密钥）。
var secretRe = regexp.MustCompile(`(-p|--password[= ]|PWD=|PASSWD=|PASSWORD=|TOKEN=|SECRET=|API[_-]?KEY=)\S*`)

// maskSecrets 终端命令落库前掩码敏感参数。
func maskSecrets(line string) string {
	return secretRe.ReplaceAllString(line, "$1***")
}

func (r *lineRecorder) feed(data string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.WriteString(data)
	for {
		b := r.buf.String()
		idx := strings.IndexAny(b, "\r\n")
		if idx < 0 {
			return
		}
		line := b[:idx]
		r.buf.Reset()
		r.buf.WriteString(b[idx+1:])
		line = ansiRe.ReplaceAllString(line, "")
		line = maskSecrets(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 500 {
			line = line[:500]
		}
		db := r.db
		user := r.user
		entry := TerminalLog{Username: user, Line: line, CreatedAt: time.Now()}
		db.Create(&entry)
	}
}

func handle(c *gin.Context, db *gorm.DB) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	user := c.GetString("user")
	rec := &lineRecorder{user: user, db: db}

	cols := uint16(parseIntDefault(c.Query("cols"), 120))
	rows := uint16(parseIntDefault(c.Query("rows"), 32))
	term, err := platform.StartTerminal(cols, rows)
	if err != nil {
		_ = ws.WriteJSON(wsMsg{Type: "exit", Data: err.Error()})
		return
	}
	defer term.Close()
	slog.Info("终端会话建立", "user", c.GetString("user"), "ip", c.ClientIP())

	var wg sync.WaitGroup
	// pty → ws
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 8192)
		for {
			n, err := term.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.TextMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				_ = ws.Close()
				return
			}
		}
	}()

	// ws → pty
	for {
		var m wsMsg
		if err := ws.ReadJSON(&m); err != nil {
			break
		}
		switch m.Type {
		case "input":
			if _, err := term.Write([]byte(m.Data)); err != nil {
				slog.Debug("终端写入失败", "err", err)
			}
			rec.feed(m.Data)
		case "resize":
			_ = term.Resize(m.Cols, m.Rows)
		}
	}
	term.Close()
	wg.Wait()
	slog.Info("终端会话结束", "user", c.GetString("user"), "ip", c.ClientIP())
}

func parseIntDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 && n < 500 {
		return n
	}
	return def
}
