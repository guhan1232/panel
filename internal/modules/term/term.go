// Package term Web 终端：WebSocket ↔ PTY。
package term

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

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

// Register 注册终端路由（须挂在鉴权中间件之后）。
func Register(api gin.IRouter) {
	api.GET("/ws/terminal", handle)
}

func handle(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()

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
