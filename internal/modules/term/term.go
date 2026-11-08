// Package term Web 终端：WebSocket ↔ PTY。
package term

import (
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
	Type string `json:"type"` // input | resize
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
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

// lineRecorder 逐行记录终端输入（回车触发落库，剥离 ANSI 转义）。
type lineRecorder struct {
	user string
	db   *gorm.DB
	mu   sync.Mutex
	buf  strings.Builder
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
