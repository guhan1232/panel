// 应用源（远程目录）：从镜像站/自建源拉取 JSON 应用清单，合并进模板列表，
// 并支持按清单直接安装。清单格式见 docs/catalog.example.json。
package appstore

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	maxCatalogSize = 1 << 20 // 1MB
	maxCatalogApps = 50
)

type remoteNative struct {
	URLs               map[string]string            `json:"urls"`
	Archive            bool                         `json:"archive"`
	InArchive          string                       `json:"in_archive"`
	Binary             string                       `json:"binary"`
	Args               []string                     `json:"args"`
	Env                map[string]string            `json:"env"`
	Files              map[string]string            `json:"files"`
	Dirs               []string                     `json:"dirs"`
	Tool               bool                         `json:"tool"`
	ArchAlias          map[string]string            `json:"arch_alias"`
	MirrorPaths        map[string]map[string]string `json:"mirror_paths"`
	VersionMirrorPaths map[string]map[string]string `json:"version_mirror_paths"`
}

// remoteApp 远程清单中的一个应用定义（纯声明式，无代码）。
type remoteApp struct {
	Key     string        `json:"key"`
	Name    string        `json:"name"`
	Desc    string        `json:"desc"`
	Kind    string        `json:"kind"` // compose | native | script
	Version string        `json:"version"`
	Params  []Param       `json:"params"`
	Compose string        `json:"compose"`
	Native  *remoteNative `json:"native"`
	Script  string        `json:"script"` // script 用：bash 内容或 http(s) URL（仓库托管）
}

func substitute(s string, p map[string]string) string {
	for k, v := range p {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

// remoteToTemplate 将远程声明式定义转为可安装模板。
func remoteToTemplate(def remoteApp) *Template {
	t := &Template{Key: def.Key, Name: def.Name, Desc: def.Desc, Kind: def.Kind, Params: def.Params, Script: def.Script}
	if def.Kind == "compose" {
		t.Gen = func(p map[string]string) (string, error) {
			return substitute(def.Compose, p), nil
		}
		return t
	}
	if def.Kind == "script" {
		return t // Script 字段已在构造时带入（installScript 解析 URL/内容）
	}
	nd := def.Native
	t.Native = &NativeSpec{
		Archive:            nd.Archive,
		InArchive:          nd.InArchive,
		Binary:             nd.Binary,
		Dirs:               nd.Dirs,
		URLs:               nd.URLs,
		Tool:               nd.Tool,
		ArchAlias:          nd.ArchAlias,
		MirrorPaths:        nd.MirrorPaths,
		VersionMirrorPaths: nd.VersionMirrorPaths,
		Args: func(p map[string]string) []string {
			out := make([]string, len(nd.Args))
			for i, a := range nd.Args {
				out[i] = substitute(a, p)
			}
			return out
		},
		Env: func(p map[string]string) map[string]string {
			m := map[string]string{}
			for k, v := range nd.Env {
				m[k] = substitute(v, p)
			}
			return m
		},
		Files: func(name string, p map[string]string) map[string]string {
			m := map[string]string{}
			for k, v := range nd.Files {
				m[k] = substitute(v, p)
			}
			return m
		},
	}
	return t
}

// DefaultAppSource 面板脚本仓库默认清单（jiaoben 分支，详见仓库内 jiaoben/README.md）。
const DefaultAppSource = "https://cnb.cool/hanyuxin/panel/-/git/raw/main/jiaoben/catalog.json"

// EnsureDefaultSource 启动时：用户未配置应用源且无缓存时，拉取默认脚本仓库清单并缓存（失败静默，用户可手动在设置里刷新）。
func (m *Module) EnsureDefaultSource() {
	if core.GetSetting(m.db, core.KAppSourceURL) != "" || core.GetSetting(m.db, core.KAppSourceCache) != "" {
		return
	}
	defs, err := fetchCatalog(DefaultAppSource)
	if err != nil {
		slog.Warn("默认脚本仓库清单拉取失败（可稍后在应用源处重试）", "err", err.Error())
		return
	}
	raw, _ := json.Marshal(defs)
	_ = core.SetSetting(m.db, core.KAppSourceURL, DefaultAppSource)
	_ = core.SetSetting(m.db, core.KAppSourceCache, string(raw))
	slog.Info("已加载默认脚本仓库清单", "url", DefaultAppSource, "apps", len(defs))
}

// remoteCatalog 返回缓存的远程应用定义（无源或解析失败返回空）。
func (m *Module) remoteCatalog() []remoteApp {
	raw := core.GetSetting(m.db, core.KAppSourceCache)
	if raw == "" {
		return nil
	}
	var defs []remoteApp
	if json.Unmarshal([]byte(raw), &defs) != nil {
		return nil
	}
	return defs
}

func (m *Module) findTemplate(key string) *Template {
	for i := range templates {
		if templates[i].Key == key {
			return &templates[i]
		}
	}
	for _, def := range m.remoteCatalog() {
		if def.Key == key {
			return remoteToTemplate(def)
		}
	}
	return nil
}

func validateRemoteDefs(defs []remoteApp) error {
	if len(defs) == 0 {
		return fmt.Errorf("清单为空")
	}
	if len(defs) > maxCatalogApps {
		return fmt.Errorf("应用数量超过上限 %d", maxCatalogApps)
	}
	seen := map[string]bool{}
	for _, d := range defs {
		if !nameRe.MatchString(d.Key) {
			return fmt.Errorf("应用 key 非法: %q", d.Key)
		}
		if seen[d.Key] {
			return fmt.Errorf("应用 key 重复: %s", d.Key)
		}
		seen[d.Key] = true
		if d.Name == "" {
			return fmt.Errorf("应用 %s 缺少 name", d.Key)
		}
		switch d.Kind {
		case "compose":
			if strings.TrimSpace(d.Compose) == "" {
				return fmt.Errorf("应用 %s 缺少 compose 内容", d.Key)
			}
		case "native":
			if d.Native == nil || len(d.Native.URLs) == 0 || d.Native.Binary == "" {
				return fmt.Errorf("原生应用 %s 缺少 urls 或 binary", d.Key)
			}
		case "script":
			if strings.TrimSpace(d.Script) == "" {
				return fmt.Errorf("脚本应用 %s 缺少 script", d.Key)
			}
		default:
			return fmt.Errorf("应用 %s kind 非法: %s", d.Key, d.Kind)
		}
	}
	return nil
}

func fetchCatalog(url string) ([]remoteApp, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogSize))
	if err != nil {
		return nil, err
	}
	// 兼容两种格式：裸数组 或 {"apps":[...]}（jiaoben 分支清单格式）
	var wrapper struct {
		Apps []remoteApp 	}
	var defs []remoteApp
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Apps) > 0 {
		defs = wrapper.Apps
	} else if err := json.Unmarshal(data, &defs); err != nil {
		return nil, fmt.Errorf("清单 JSON 解析失败: %w", err)
	}
	if err := validateRemoteDefs(defs); err != nil {
		return nil, err
	}
	return defs, nil
}

// ---- 路由 ----

func (m *Module) getSource(c *gin.Context) {
	url := core.GetSetting(m.db, core.KAppSourceURL)
	core.OK(c, gin.H{
		"url":   url,
		"count": len(m.remoteCatalog()),
	})
}

func (m *Module) setSource(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	url := strings.TrimSpace(req.URL)
	if url == "" {
		// 清除应用源
		_ = core.SetSetting(m.db, core.KAppSourceURL, "")
		_ = core.SetSetting(m.db, core.KAppSourceCache, "")
		slog.Info("应用源已清除", "by", c.GetString("user"))
		core.OK(c, gin.H{"count": 0})
		return
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		core.Err(c, 400, "应用源地址需以 http(s):// 开头")
		return
	}
	defs, err := fetchCatalog(url)
	if err != nil {
		core.Err(c, 500, "拉取失败: "+err.Error())
		return
	}
	raw, _ := json.Marshal(defs)
	if err := core.SetSetting(m.db, core.KAppSourceURL, url); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := core.SetSetting(m.db, core.KAppSourceCache, string(raw)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("应用源已更新", "url", url, "apps", len(defs), "by", c.GetString("user"))
	core.OK(c, gin.H{"count": len(defs)})
}
