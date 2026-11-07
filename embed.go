// Package webui 嵌入前端构建产物（web/dist），随二进制发布。
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web/dist
var dist embed.FS

// FS 返回以 web/dist 为根的前端文件系统。
func FS() fs.FS {
	sub, err := fs.Sub(dist, "web/dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// ServeIndex 输出 SPA 入口页（history 路由回退）。
func ServeIndex(w http.ResponseWriter, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "前端未构建：请先执行 make build-web", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
