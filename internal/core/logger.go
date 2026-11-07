// Package core 面板公共基础：日志、数据库、模型、响应封装、初始化。
package core

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// dailyWriter 按天切割的日志文件写入器。
type dailyWriter struct {
	mu  sync.Mutex
	dir string
	day string
	f   *os.File
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	day := time.Now().Format("20060102")
	if w.f == nil || day != w.day {
		if w.f != nil {
			_ = w.f.Close()
		}
		if err := os.MkdirAll(w.dir, 0o755); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(filepath.Join(w.dir, "panel-"+day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return 0, err
		}
		w.f, w.day = f, day
	}
	return w.f.Write(p)
}

// Setup 初始化全局 slog：stdout + 按天切割文件。
func Setup(logDir, level string) {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	w := &dailyWriter{dir: logDir}
	h := slog.NewTextHandler(io.MultiWriter(os.Stdout, w), &slog.HandlerOptions{Level: lv})
	slog.SetDefault(slog.New(h))
}
