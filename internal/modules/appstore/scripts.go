// 脚本类应用：仓库托管脚本一键安装 + 用户自定义脚本上传/运行。
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

func (m *Module) scriptsDir() string {
	// 绝对路径：bash 子进程的 cwd 可能与面板工作目录不同，相对路径会找不到文件
	d, err := filepath.Abs(filepath.Join(m.dataDir, "appstore", "scripts"))
	if err != nil {
		return filepath.Join(m.dataDir, "appstore", "scripts")
	}
	return d
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

// ---- 自定义脚本 CRUD ----

// listCustomScripts 列出用户上传的脚本。
func (m *Module) listCustomScripts(c *gin.Context) {
	dir := m.scriptsDir()
	items := []gin.H{}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
				continue
			}
			info, _ := e.Info()
			item := gin.H{"name": strings.TrimSuffix(e.Name(), ".sh"), "size": info.Size(), "updated": info.ModTime().Format("2006-01-02 15:04")}
			if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
				head := string(b)
				if len(head) > 120 {
					head = head[:120]
				}
				item["head"] = strings.SplitN(head, "\n", 2)[0]
			}
			items = append(items, item)
		}
	}
	core.OK(c, items)
}

// saveCustomScript 保存/更新用户脚本。
func (m *Module) saveCustomScript(c *gin.Context) {
	var req struct {
		Name    string `binding:"required"`
		Content string `binding:"required"`
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
	if err := os.MkdirAll(m.scriptsDir(), 0o700); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(m.scriptsDir(), name+".sh"), []byte(req.Content), 0o700); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"name": name})
}

// deleteCustomScript 删除脚本（及其日志）。
func (m *Module) deleteCustomScript(c *gin.Context) {
	name := c.Param("name")
	if !scriptNameRe.MatchString(name) {
		core.Err(c, 400, "脚本名非法")
		return
	}
	_ = os.Remove(filepath.Join(m.scriptsDir(), name+".sh"))
	_ = os.Remove(filepath.Join(m.scriptsDir(), name+".sh.log"))
	core.OK(c, nil)
}

// runCustomScript 运行用户上传的脚本（登记为 script 类应用实例）。
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
	content, err := os.ReadFile(filepath.Join(m.scriptsDir(), name+".sh"))
	if err != nil {
		core.Err(c, 404, "脚本不存在")
		return
	}
	tpl := &Template{Key: "script:" + name, Name: name, Kind: "script", Script: string(content)}
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

// scriptLog 查看脚本安装日志。
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
	g.DELETE("/scripts/:name", m.deleteCustomScript)
	g.POST("/scripts/:name/run", m.runCustomScript)
	g.GET("/scripts/:name/log", m.scriptLog)
}
