// Package webui 嵌入前端构建产物（web/dist），随二进制发布。
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
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
	// index.html 禁止缓存：升级换版后引用的资源哈希会变，浏览器缓存旧页会导致 404；
	// 带哈希的 /assets/* 由 ServeStatic 设长缓存。
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 显式写状态头：gin NoRoute 下依赖隐式 200 会被先行 WriteHeader(404) 污染
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ServeStatic 输出嵌入的静态资源并按类型设置缓存策略（带哈希文件名 → 长缓存）。
func ServeStatic(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, path.Base(p), time.Time{}, bytes.NewReader(data))
}
