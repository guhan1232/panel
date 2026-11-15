// 脚本类应用：仓库托管脚本一键安装 + 用户三方脚本包（元数据+参数声明，商店一等公民）。
// 存储为目录式脚本包：data/appstore/scripts/<key>/{install.sh, meta.json}；旧单文件 .sh 自动迁移。
package appstore

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
)

var scriptNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,62}$`)

// ScriptMeta 三方脚本包元数据（商店展示 + 参数表单声明）。
type ScriptMeta struct {
	Name     string  `json:"name"`     // 包 key（小写字母数字_-）
	Title    string  `json:"title"`    // 显示名
	Desc     string  `json:"desc"`
	Version  string  `json:"version"`
	Category string  `json:"category"` // 分类（前端聚合筛选）
	Icon     string  `json:"icon"`     // 图标（emoji/单字，可空）
	Params   []Param `json:"params"`   // 参数声明（安装时表单）
	Updated  string  `json:"updated"`
}

func (m *Module) scriptsDir() string {
	// 绝对路径：bash 子进程的 cwd 可能与面板工作目录不同，相对路径会找不到文件
	d, err := filepath.Abs(filepath.Join(m.dataDir, "appstore", "scripts"))
	if err != nil {
		return filepath.Join(m.dataDir, "appstore", "scripts")
	}
	return d
}

func (m *Module) scriptPkgDir(name string) string {
	return filepath.Join(m.scriptsDir(), name)
}

// readScriptMeta 读取脚本包元数据（缺失时返回仅含 name 的默认值）。
func readScriptMeta(dir, name string) ScriptMeta {
	meta := ScriptMeta{Name: name, Title: name}
	if b, err := os.ReadFile(filepath.Join(dir, "meta.json")); err == nil {
		var m ScriptMeta
		if json.Unmarshal(b, &m) == nil && m.Name == name {
			return m
		}
	}
	return meta
}

// migrateLegacyScript 旧单文件 <name>.sh 迁移为目录式脚本包（一次性，成功后删除旧文件）。
func (m *Module) migrateLegacyScript(name string) {
	old := filepath.Join(m.scriptsDir(), name+".sh")
	if _, err := os.Stat(old); err != nil {
		return
	}
	dir := m.scriptPkgDir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	if b, err := os.ReadFile(old); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "install.sh"), b, 0o700)
		meta := ScriptMeta{Name: name, Title: name, Updated: time.Now().Format("2006-01-02 15:04")}
		if raw, jerr := json.Marshal(meta); jerr == nil {
			_ = os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0o600)
		}
		_ = os.Remove(old)
		slog.Info("旧版脚本已迁移为脚本包", "name", name)
	}
}

// scriptContent 读取脚本包内容（含旧单文件迁移兜底）。
func (m *Module) scriptContent(name string) (string, error) {
	if !scriptNameRe.MatchString(name) {
		return "", fmt.Errorf("脚本名非法")
	}
	m.migrateLegacyScript(name)
	b, err := os.ReadFile(filepath.Join(m.scriptPkgDir(name), "install.sh"))
	if err != nil {
		return "", fmt.Errorf("脚本不存在")
	}
	return string(b), nil
}

// customTemplate 将自定义脚本包转为商店模板（key = custom:<name>，统一走 install 引擎）。
func (m *Module) customTemplate(name string) *Template {
	if !scriptNameRe.MatchString(name) {
		return nil
	}
	m.migrateLegacyScript(name)
	dir := m.scriptPkgDir(name)
	content, err := os.ReadFile(filepath.Join(dir, "install.sh"))
	if err != nil {
		return nil
	}
	meta := readScriptMeta(dir, name)
	return &Template{
		Key:    "custom:" + name,
		Name:   meta.Title,
		Desc:   meta.Desc,
		Kind:   "script",
		Params: meta.Params,
		Script: string(content),
	}
}

// resolveScriptContent 模板 Script 字段：http(s) URL 则拉取（仓库托管），否则视为内嵌内容。
func resolveScriptContent(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("脚本内容为空")
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(s)
		if err != nil {
			return "", fmt.Errorf("脚本下载失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("脚本下载失败: HTTP %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return s, nil
}

// installScript 渲染占位符 → 落盘 → bash 执行。返回状态说明。
func (m *Module) installScript(tpl *Template, name string, p map[string]string) (string, error) {
	content, err := resolveScriptContent(tpl.Script)
	if err != nil {
		return "", err
	}
	content = substitute(content, p)
	dir := m.scriptsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".sh")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		return "", err
	}
	cmd := exec.Command("bash", path)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PANEL_APP_NAME="+name)
	out, err := CaptureRun(cmd, 30*time.Minute)
	if err != nil {
		// 保留脚本与输出便于排障
		_ = os.WriteFile(path+".log", out, 0o600)
		slog.Warn("脚本应用安装失败", "app", name, "log", filepath.Base(path)+".log")
		return "", fmt.Errorf("脚本执行失败（日志见 %s.log）: %s", name, tailStr(string(out), 300))
	}
	_ = os.WriteFile(path+".log", out, 0o600)
	slog.Info("脚本应用安装完成", "app", name)
	return "脚本执行成功（日志 data/appstore/scripts/" + name + ".sh.log）", nil
}

func tailStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// CaptureRun 包装 cmd 运行并捕获合并输出。
func CaptureRun(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	done := make(chan error, 1)
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return []byte(buf.String()), err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return []byte(buf.String()), fmt.Errorf("脚本执行超时（%s）", timeout)
	}
}

// ---- 三方脚本包 CRUD ----

// listCustomScripts 列出脚本包（含元数据；旧单文件自动迁移）。
func (m *Module) listCustomScripts(c *gin.Context) {
	dir := m.scriptsDir()
	items := []gin.H{}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			name := ""
			if e.IsDir() && scriptNameRe.MatchString(e.Name()) {
				name = e.Name()
			} else if !e.IsDir() && strings.HasSuffix(e.Name(), ".sh") && scriptNameRe.MatchString(strings.TrimSuffix(e.Name(), ".sh")) {
				name = strings.TrimSuffix(e.Name(), ".sh")
			}
			if name == "" {
				continue
			}
			m.migrateLegacyScript(name)
			pkgDir := m.scriptPkgDir(name)
			content, err := os.ReadFile(filepath.Join(pkgDir, "install.sh"))
			if err != nil {
				continue
			}
			meta := readScriptMeta(pkgDir, name)
			head := string(content)
			if len(head) > 160 {
				head = head[:160]
			}
			items = append(items, gin.H{
				"name":     meta.Name,
				"title":    meta.Title,
				"desc":     meta.Desc,
				"version":  meta.Version,
				"category": meta.Category,
				"params":   meta.Params,
				"head":     strings.SplitN(head, "\n", 2)[0],
				"size":     len(content),
				"updated":  meta.Updated,
			})
		}
	}
	core.OK(c, items)
}

// saveCustomScript 新建/更新脚本包（元数据 + 内容）。
func (m *Module) saveCustomScript(c *gin.Context) {
	var req struct {
		Name     string  `json:"name" binding:"required"`
		Title    string  `json:"title"`
		Desc     string  `json:"desc"`
		Version  string  `json:"version"`
		Category string  `json:"category"`
		Icon     string  `json:"icon"`
		Params   []Param `json:"params"`
		Content  string  `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	name := strings.TrimSuffix(strings.TrimSpace(req.Name), ".sh")
	if !scriptNameRe.MatchString(name) {
		core.Err(c, 400, "脚本名仅允许小写字母数字_-（2-64 位）")
		return
	}
	if len(req.Content) > 1<<20 {
		core.Err(c, 400, "脚本内容超过 1MB 上限")
		return
	}
	// 参数 key 合法性校验（占位符替换依赖 {{key}}）
	seen := map[string]bool{}
	for _, p := range req.Params {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`).MatchString(p.Key) || seen[p.Key] {
			core.Err(c, 400, "参数 key 非法或重复: "+p.Key)
			return
		}
		seen[p.Key] = true
	}
	dir := m.scriptPkgDir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte(req.Content), 0o700); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = name
	}
	meta := ScriptMeta{Name: name, Title: title, Desc: strings.TrimSpace(req.Desc), Version: strings.TrimSpace(req.Version), Category: strings.TrimSpace(req.Category), Icon: strings.TrimSpace(req.Icon), Params: req.Params, Updated: time.Now().Format("2006-01-02 15:04")}
	raw, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0o600); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	// 旧版同名单文件日志一并清理
	_ = os.Remove(filepath.Join(m.scriptsDir(), name+".sh.log"))
	slog.Info("脚本包已保存", "name", name, "by", c.GetString("user"))
	core.OK(c, gin.H{"name": name})
}

// getCustomScript 读取脚本包（编辑回填：元数据 + 内容）。
func (m *Module) getCustomScript(c *gin.Context) {
	name := c.Param("name")
	if !scriptNameRe.MatchString(name) {
		core.Err(c, 400, "脚本名非法")
		return
	}
	m.migrateLegacyScript(name)
	dir := m.scriptPkgDir(name)
	content, err := os.ReadFile(filepath.Join(dir, "install.sh"))
	if err != nil {
		core.Err(c, 404, "脚本不存在")
		return
	}
	meta := readScriptMeta(dir, name)
	core.OK(c, gin.H{"name": meta.Name, "title": meta.Title, "desc": meta.Desc, "version": meta.Version, "category": meta.Category, "params": meta.Params, "content": string(content), "updated": meta.Updated})
}

// deleteCustomScript 删除脚本包（及其日志）。
func (m *Module) deleteCustomScript(c *gin.Context) {
	name := c.Param("name")
	if !scriptNameRe.MatchString(name) {
		core.Err(c, 400, "脚本名非法")
		return
	}
	_ = os.RemoveAll(m.scriptPkgDir(name))
	_ = os.Remove(filepath.Join(m.scriptsDir(), name+".sh"))
	_ = os.Remove(filepath.Join(m.scriptsDir(), name+".sh.log"))
	slog.Info("脚本包已删除", "name", name, "by", c.GetString("user"))
	core.OK(c, nil)
}

// runCustomScript 直接运行脚本包（登记为 script 类应用实例；商店统一安装走 POST /apps custom: 前缀）。
func (m *Module) runCustomScript(c *gin.Context) {
	name := c.Param("name")
	if !scriptNameRe.MatchString(name) {
		core.Err(c, 400, "脚本名非法")
		return
	}
	var req struct {
		Instance string            `json:"instance" binding:"required"`
		Params   map[string]string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !nameRe.MatchString(req.Instance) {
		core.Err(c, 400, "实例名非法（小写字母数字短横线，3-40 位）")
		return
	}
	var count int64
	m.db.Model(&AppInst{}).Where("name = ?", req.Instance).Count(&count)
	if count > 0 {
		core.Err(c, 400, "同名实例已存在")
		return
	}
	content, err := m.scriptContent(name)
	if err != nil {
		core.Err(c, 404, err.Error())
		return
	}
	tpl := &Template{Key: "script:" + name, Name: name, Kind: "script", Script: content}
	p := orParams(req.Params)
	status, err := m.installScript(tpl, req.Instance, p)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	row := AppInst{Name: req.Instance, AppKey: tpl.Key, Kind: "script"}
	if data, jerr := json.Marshal(p); jerr == nil {
		row.Params = string(data)
	}
	if err := m.db.Create(&row).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"app": row, "status": status})
}

// scriptLog 查看脚本运行日志。
func (m *Module) scriptLog(c *gin.Context) {
	name := c.Param("name")
	if !scriptNameRe.MatchString(name) {
		core.Err(c, 400, "脚本名非法")
		return
	}
	data, err := os.ReadFile(filepath.Join(m.scriptsDir(), name+".sh.log"))
	if err != nil {
		core.OK(c, gin.H{"log": ""})
		return
	}
	core.OK(c, gin.H{"log": string(data)})
}

// registerScriptRoutes 注册脚本应用路由（挂到已有 apps 组）。
func (m *Module) registerScriptRoutes(g gin.IRouter) {
	g.GET("/scripts", m.listCustomScripts)
	g.POST("/scripts", m.saveCustomScript)
	g.GET("/scripts/:name", m.getCustomScript)
	g.DELETE("/scripts/:name", m.deleteCustomScript)
	g.POST("/scripts/:name/run", m.runCustomScript)
	g.GET("/scripts/:name/log", m.scriptLog)
}
