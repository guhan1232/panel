// 应用源（多仓库）：软件商店从多个 git 仓库清单源获取软件安装脚本与源码声明。
// 内置默认源（寒语馨脚本仓库）+ 用户自建仓库源（任意 http(s) 清单 URL，可启停/刷新/删除）。
// 每源独立缓存清单（内嵌在源记录里），合并进模板列表并按源顺序去重。清单格式见 docs/catalog.example.json。
package appstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	maxCatalogSize = 1 << 20 // 单清单 1MB
	maxCatalogApps = 200
	maxSources     = 20
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
	Key      string        `json:"key"`
	Name     string        `json:"name"`
	Desc     string        `json:"desc"`
	Kind     string        `json:"kind"` // compose | native | script
	Version  string        `json:"version"`
	Category string        `json:"category"` // 可选分类（前端聚合筛选）
	Icon     string        `json:"icon"`     // 可选图标（emoji/单字）
	Params   []Param       `json:"params"`
	Compose  string        `json:"compose"`
	Native   *remoteNative `json:"native"`
	Script   string        `json:"script"` // script 用：bash 内容或 http(s) URL（仓库托管）

	// 运行时填充（不序列化）：来源信息，前端来源徽章用
	SourceID   string `json:"-"`
	SourceName string `json:"-"`
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

// DefaultAppSource 寒语馨脚本仓库（默认内置源，jiaoben 目录清单）。
const DefaultAppSource = "https://cnb.cool/hanyuxin/panel/-/git/raw/main/jiaoben/catalog.json"

const DefaultSourceName = "寒语馨脚本仓库"

// AppSource 仓库源记录：URL 指向清单 JSON（裸数组或 {"apps":[...]}），缓存内嵌。
type AppSource struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	URL     string      `json:"url"`
	Enabled bool        `json:"enabled"`
	Builtin bool        `json:"builtin"` // 内置源不可删除，可停用
	Cache   []remoteApp `json:"cache"`   // 最近一次成功拉取的清单
	Updated string      `json:"updated"` // 最近成功刷新时间
	Err     string      `json:"err"`     // 最近一次刷新失败原因（空 = 正常）
}

// loadSources 读取源列表；空且无旧单源时注入内置默认源（启动迁移）。
func (m *Module) loadSources() []AppSource {
	raw := core.GetSetting(m.db, core.KAppSources)
	var list []AppSource
	if raw != "" {
		if json.Unmarshal([]byte(raw), &list) == nil && list != nil {
			return list
		}
		return []AppSource{}
	}
	// 迁移：旧单源设置（KAppSourceURL/Cache）转成一条源记录
	oldURL := core.GetSetting(m.db, core.KAppSourceURL)
	oldCache := core.GetSetting(m.db, core.KAppSourceCache)
	if oldURL != "" {
		var defs []remoteApp
		_ = json.Unmarshal([]byte(oldCache), &defs)
		list = []AppSource{{ID: newSourceID(), Name: "我的应用源", URL: oldURL, Enabled: true, Cache: defs, Updated: time.Now().Format("2006-01-02 15:04")}}
	} else {
		// 首次启动：注入内置默认源并尝试拉取
		src := AppSource{ID: newSourceID(), Name: DefaultSourceName, URL: DefaultAppSource, Enabled: true, Builtin: true}
		if defs, err := fetchCatalog(DefaultAppSource); err == nil {
			src.Cache = defs
			src.Updated = time.Now().Format("2006-01-02 15:04")
		} else {
			src.Err = err.Error()
		}
		list = []AppSource{src}
	}
	m.saveSources(list)
	slog.Info("软件商店源已初始化", "sources", len(list))
	return list
}

func (m *Module) saveSources(list []AppSource) {
	raw, _ := json.Marshal(list)
	if err := core.SetSetting(m.db, core.KAppSources, string(raw)); err != nil {
		slog.Warn("保存应用源失败", "err", err.Error())
	}
}

func newSourceID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "src-" + hex.EncodeToString(b)
}

// remoteCatalog 合并所有启用源的应用定义（源顺序优先，key 重复时先注册源生效），并填充来源信息。
func (m *Module) remoteCatalog() []remoteApp {
	seen := map[string]bool{}
	var out []remoteApp
	for _, src := range m.loadSources() {
		if !src.Enabled {
			continue
		}
		for _, def := range src.Cache {
			if seen[def.Key] {
				continue
			}
			seen[def.Key] = true
			def.SourceID = src.ID
			def.SourceName = src.Name
			out = append(out, def)
		}
	}
	return out
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
	// 本地自定义脚本（custom:<name>）
	if strings.HasPrefix(key, "custom:") {
		if t := m.customTemplate(strings.TrimPrefix(key, "custom:")); t != nil {
			return t
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
	// 兼容两种格式：裸数组 或 {"apps":[...]}（jiaoben 仓库清单格式）
	var wrapper struct {
		Apps []remoteApp
	}
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

// ---- 路由：多仓库源管理 ----

func sourceDTO(s AppSource) gin.H {
	return gin.H{"id": s.ID, "name": s.Name, "url": s.URL, "enabled": s.Enabled, "builtin": s.Builtin, "count": len(s.Cache), "updated": s.Updated, "err": s.Err}
}

// listSources 列出全部仓库源（按内置优先、名称排序）。
func (m *Module) listSources(c *gin.Context) {
	list := m.loadSources()
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Builtin != list[j].Builtin {
			return list[i].Builtin
		}
		return list[i].Name < list[j].Name
	})
	out := make([]gin.H, 0, len(list))
	for _, s := range list {
		out = append(out, sourceDTO(s))
	}
	core.OK(c, out)
}

// addSource 添加仓库源：立即拉取校验，成功才入库。
func (m *Module) addSource(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
		URL  string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	url := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		core.Err(c, 400, "仓库源地址需以 http(s):// 开头")
		return
	}
	list := m.loadSources()
	if len(list) >= maxSources {
		core.Err(c, 400, "仓库源数量已达上限")
		return
	}
	defs, err := fetchCatalog(url)
	if err != nil {
		core.Err(c, 500, "清单拉取失败: "+err.Error())
		return
	}
	src := AppSource{ID: newSourceID(), Name: strings.TrimSpace(req.Name), URL: url, Enabled: true, Cache: defs, Updated: time.Now().Format("2006-01-02 15:04")}
	list = append(list, src)
	m.saveSources(list)
	slog.Info("仓库源已添加", "name", src.Name, "url", url, "apps", len(defs), "by", c.GetString("user"))
	core.OK(c, sourceDTO(src))
}

// updateSource 启停/改名/改址（改址时重新拉取）。
func (m *Module) updateSource(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name    *string `json:"name"`
		URL     *string `json:"url"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	list := m.loadSources()
	for i := range list {
		if list[i].ID != id {
			continue
		}
		if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
			list[i].Name = strings.TrimSpace(*req.Name)
		}
		if req.Enabled != nil {
			list[i].Enabled = *req.Enabled
		}
		if req.URL != nil {
			url := strings.TrimSpace(*req.URL)
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				core.Err(c, 400, "仓库源地址需以 http(s):// 开头")
				return
			}
			defs, err := fetchCatalog(url)
			if err != nil {
				core.Err(c, 500, "清单拉取失败: "+err.Error())
				return
			}
			list[i].URL = url
			list[i].Cache = defs
			list[i].Updated = time.Now().Format("2006-01-02 15:04")
			list[i].Err = ""
		}
		m.saveSources(list)
		core.OK(c, sourceDTO(list[i]))
		return
	}
	core.Err(c, 404, "仓库源不存在")
}

// deleteSource 删除仓库源（内置源拒绝）。
func (m *Module) deleteSource(c *gin.Context) {
	id := c.Param("id")
	list := m.loadSources()
	for i := range list {
		if list[i].ID != id {
			continue
		}
		if list[i].Builtin {
			core.Err(c, 400, "内置源不可删除，可停用")
			return
		}
		list = append(list[:i], list[i+1:]...)
		m.saveSources(list)
		slog.Info("仓库源已删除", "id", id, "by", c.GetString("user"))
		core.OK(c, nil)
		return
	}
	core.Err(c, 404, "仓库源不存在")
}

// refreshSource 手动刷新源清单。
func (m *Module) refreshSource(c *gin.Context) {
	id := c.Param("id")
	list := m.loadSources()
	for i := range list {
		if list[i].ID != id {
			continue
		}
		defs, err := fetchCatalog(list[i].URL)
		if err != nil {
			list[i].Err = err.Error()
			m.saveSources(list)
			core.Err(c, 500, "刷新失败: "+err.Error())
			return
		}
		list[i].Cache = defs
		list[i].Updated = time.Now().Format("2006-01-02 15:04")
		list[i].Err = ""
		m.saveSources(list)
		slog.Info("仓库源已刷新", "name", list[i].Name, "apps", len(defs), "by", c.GetString("user"))
		core.OK(c, sourceDTO(list[i]))
		return
	}
	core.Err(c, 404, "仓库源不存在")
}

// EnsureDefaultSource 启动时加载/初始化源列表（含旧单源迁移），失败静默。
func (m *Module) EnsureDefaultSource() {
	list := m.loadSources()
	total := 0
	for _, s := range list {
		if s.Enabled {
			total += len(s.Cache)
		}
	}
	slog.Info("软件商店源就绪", "sources", len(list), "apps", total)
}

// registerSourceRoutes 注册仓库源路由：读挂 apps 组（/api/apps/sources），写挂 admin 组。
func (m *Module) registerSourceRoutes(g gin.IRouter, admin gin.IRouter) {
	g.GET("/sources", m.listSources)
	admin.POST("/apps/sources", m.addSource)
	admin.PUT("/apps/sources/:id", m.updateSource)
	admin.DELETE("/apps/sources/:id", m.deleteSource)
	admin.POST("/apps/sources/:id/refresh", m.refreshSource)
}
