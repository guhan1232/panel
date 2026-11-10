package appstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

// dockerJSONList 执行 docker 子命令并按行解析 --format "{{json .}}" 输出。
func dockerJSONList(args ...string) ([]map[string]any, error) {
	full := append(args, "--format", "{{json .}}")
	out, _, err := execx.Run("appstore", 30*time.Second, "docker", full...)
	if err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) == nil {
			rows = append(rows, obj)
		}
	}
	return rows, nil
}

// dockerContainers 容器列表（docker ps -a）。
func (m *Module) dockerContainers(c *gin.Context) {
	rows, err := dockerJSONList("ps", "-a")
	if err != nil {
		core.Err(c, 500, "docker 不可用或未运行: "+err.Error())
		return
	}
	core.OK(c, rows)
}

// dockerContainerAction 容器操作：start | stop | restart | remove（remove 需 force）。
func (m *Module) dockerContainerAction(c *gin.Context) {
	name := c.Param("name")
	action := c.Param("action")
	switch action {
	case "start", "stop", "restart":
	case "remove":
		if c.Query("force") != "1" {
			core.Err(c, 400, "删除容器需确认（force=1）")
			return
		}
	default:
		core.Err(c, 400, "不支持的操作")
		return
	}
	args := []string{action}
	if action == "remove" {
		args = []string{"rm", "-f"}
	}
	if out, code, err := execx.Run("appstore", 60*time.Second, "docker", append(args, name)...); err != nil {
		core.Err(c, 500, fmt.Sprintf("容器%s失败(exit %d): %s", action, code, strings.TrimSpace(out)))
		return
	}
	core.OK(c, gin.H{"ok": true})
}

// dockerContainerLogs 容器日志（docker logs --tail）。
func (m *Module) dockerContainerLogs(c *gin.Context) {
	name := c.Param("name")
	tail := c.DefaultQuery("tail", "200")
	out, _, err := execx.Run("appstore", 30*time.Second, "docker", "logs", "--tail", tail, "--timestamps", name)
	if err != nil && out == "" {
		core.Err(c, 500, "日志获取失败: "+err.Error())
		return
	}
	core.OK(c, gin.H{"logs": out})
}

// dockerImages 镜像列表。
func (m *Module) dockerImages(c *gin.Context) {
	rows, err := dockerJSONList("images")
	if err != nil {
		core.Err(c, 500, "docker 不可用: "+err.Error())
		return
	}
	core.OK(c, rows)
}

// dockerImagesPrune 清理未使用镜像（危险操作，需 force 确认）。
func (m *Module) dockerImagesPrune(c *gin.Context) {
	if c.Query("force") != "1" {
		core.Err(c, 400, "清理镜像需确认（force=1）")
		return
	}
	out, code, err := execx.Run("appstore", 120*time.Second, "docker", "image", "prune", "-af")
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("清理失败(exit %d): %s", code, strings.TrimSpace(out)))
		return
	}
	core.OK(c, gin.H{"ok": true, "output": strings.TrimSpace(out)})
}

// dockerComposeGet 读取某实例的 docker-compose.yml（在线编辑用）。
func (m *Module) dockerComposeGet(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		core.Err(c, 400, "缺少实例名")
		return
	}
	data, err := os.ReadFile(m.composeFile(name))
	if err != nil {
		core.Err(c, 404, "compose 文件不存在（仅商店 Compose 应用支持）")
		return
	}
	core.OK(c, gin.H{"name": name, "content": string(data)})
}

// dockerComposeSave 保存 docker-compose.yml 并 up -d 应用变更。
func (m *Module) dockerComposeSave(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if strings.Contains(req.Name, "/") || strings.Contains(req.Name, "..") {
		core.Err(c, 400, "实例名非法")
		return
	}
	if err := os.WriteFile(m.composeFile(req.Name), []byte(req.Content), 0o644); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if out, code, err := execx.Run("appstore", 15*time.Minute, "docker", composeArgs(m.composeFile(req.Name), "up", "-d")...); err != nil {
		core.Err(c, 500, fmt.Sprintf("应用变更失败(exit %d): %s", code, strings.TrimSpace(out)))
		return
	}
	core.OK(c, gin.H{"ok": true})
}

// dockerComposeDeploy 自定义 Compose 应用：写入数据目录并 up -d，注册实例。
func (m *Module) dockerComposeDeploy(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !nameRe.MatchString(req.Name) {
		core.Err(c, 400, "实例名非法（小写字母数字短横线，3-40 位）")
		return
	}
	if !strings.Contains(req.Content, "services:") {
		core.Err(c, 400, "YAML 需包含 services: 段")
		return
	}
	var count int64
	m.db.Model(&AppInst{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		core.Err(c, 400, "同名实例已存在")
		return
	}
	dir := m.dir(req.Name)
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.WriteFile(m.composeFile(req.Name), []byte(req.Content), 0o644); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if out, code, err := execx.Run("appstore", 15*time.Minute, "docker", composeArgs(m.composeFile(req.Name), "up", "-d")...); err != nil {
		_ = os.RemoveAll(dir)
		core.Err(c, 500, fmt.Sprintf("部署失败(exit %d): %s", code, strings.TrimSpace(out)))
		return
	}
	params, _ := json.Marshal(map[string]string{"source": "custom"})
	row := AppInst{Name: req.Name, AppKey: "custom-compose", Kind: "compose", Params: string(params), Version: ""}
	if err := m.db.Create(&row).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("自定义 Compose 应用已部署", "name", req.Name)
	core.OK(c, gin.H{"app": row, "status": "容器已启动"})
}

// dockerStats 容器资源统计（一次性快照）。
func dockerStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		core.Err(c, 500, "docker 不可用: "+err.Error())
		return
	}
	rows := []map[string]any{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) == nil {
			rows = append(rows, obj)
		}
	}
	core.OK(c, rows)
}

// ---- Docker 全面管理（总览/网站/网络/存储卷/镜像仓库）----

// dockerInfo 单条 docker info/version 输出。
func dockerInfo(args ...string) (map[string]any, error) {
	out, _, err := execx.Run("appstore", 30*time.Second, "docker", args...)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// dockerOverview 总览：版本/信息/容器统计。
func dockerOverview(c *gin.Context) {
	if _, _, err := execx.Run("appstore", 8*time.Second, "docker", "version"); err != nil {
		core.OK(c, gin.H{"installed": false})
		return
	}
	ver, _ := dockerInfo("version", "--format", "{{json .}}")
	info, _ := dockerInfo("info", "--format", "{{json .}}")
	containers, _ := dockerJSONList("ps", "-a")
	images, _ := dockerJSONList("images")
	running, stopped := 0, 0
	for _, ct := range containers {
		if s, _ := ct["State"].(string); s == "running" {
			running++
		} else {
			stopped++
		}
	}
	core.OK(c, gin.H{
		"version":   ver,
		"info":      info,
		"running":   running,
		"stopped":   stopped,
		"images":    len(images),
		"templates": true,
	})
}

// dockerSites 网站视图：发布了端口映射的容器（建站类）。
func dockerSites(c *gin.Context) {
	rows, err := dockerJSONList("ps", "-a")
	if err != nil {
		core.OK(c, gin.H{"installed": false})
		return
	}
	sites := []map[string]any{}
	for _, r := range rows {
		ports, _ := r["Ports"].(string)
		name, _ := r["Names"].(string)
		sites = append(sites, gin.H{"name": name, "image": r["Image"], "state": r["State"], "ports": ports})
	}
	core.OK(c, sites)
}

// dockerNetworks 网络列表（Docker 未安装时优雅降级）。
func dockerNetworks(c *gin.Context) {
	rows, err := dockerJSONList("network", "ls")
	if err != nil {
		core.OK(c, gin.H{"installed": false})
		return
	}
	core.OK(c, rows)
}

// dockerNetworkCreate 创建网络。
func dockerNetworkCreate(c *gin.Context) {
	var req struct {
		Name   string `binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !nameRe.MatchString(req.Name) {
		core.Err(c, 400, "网络名非法")
		return
	}
	driver := req.Driver
	if driver == "" {
		driver = "bridge"
	}
	out, code, err := execx.Run("appstore", 30*time.Second, "docker", "network", "create", "--driver", driver, req.Name)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("创建失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, nil)
}

// dockerNetworkRemove 删除网络（内置网络禁删）。
func dockerNetworkRemove(c *gin.Context) {
	name := c.Param("name")
	switch name {
	case "bridge", "host", "none":
		core.Err(c, 400, "内置网络不可删除")
		return
	}
	out, code, err := execx.Run("appstore", 30*time.Second, "docker", "network", "rm", name)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("删除失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, nil)
}

// dockerVolumes 存储卷列表（Docker 未安装时优雅降级）。
func dockerVolumes(c *gin.Context) {
	rows, err := dockerJSONList("volume", "ls")
	if err != nil {
		core.OK(c, gin.H{"installed": false})
		return
	}
	core.OK(c, rows)
}

// dockerVolumeRemove 删除存储卷。
func dockerVolumeRemove(c *gin.Context) {
	name := c.Param("name")
	out, code, err := execx.Run("appstore", 30*time.Second, "docker", "volume", "rm", name)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("删除失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, nil)
}

// dockerSearch 线上镜像搜索。
func dockerSearch(c *gin.Context) {
	term := strings.TrimSpace(c.Query("term"))
	if term == "" {
		core.OK(c, []map[string]any{})
		return
	}
	rows, err := dockerJSONList("search", "--limit", "12", term)
	if err != nil {
		core.Err(c, 500, "docker 不可用: "+err.Error())
		return
	}
	core.OK(c, rows)
}

// dockerPull 拉取镜像（同步限时执行）。
func dockerPull(c *gin.Context) {
	var req struct {
		Image string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	image := strings.TrimSpace(req.Image)
	if image == "" || strings.ContainsAny(image, ";|&`$") {
		core.Err(c, 400, "镜像名非法")
		return
	}
	out, code, err := execx.Run("appstore", 15*time.Minute, "docker", "pull", image)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("拉取失败(exit %d): %s", code, tailOut(out)))
		return
	}
	core.OK(c, gin.H{"ok": true, "output": tailOut(out)})
}

// dockerLogin 登录镜像仓库（凭据不落库，仅本机 docker credential store）。
func dockerLogin(c *gin.Context) {
	var req struct {
		Registry string `binding:"required"`
		Username string `binding:"required"`
		Password string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	registry := strings.TrimSpace(req.Registry)
	if strings.ContainsAny(registry, ";|&`$ ") {
		core.Err(c, 400, "仓库地址非法")
		return
	}
	out, code, err := execx.Run("appstore", 60*time.Second, "docker", "login", "-u", req.Username, "-p", req.Password, registry)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("登录失败(exit %d): %s", code, tailOut(out)))
		return
	}
	// 记录仓库地址清单（不含凭据）
	core.OK(c, gin.H{"ok": true})
}

func tailOut(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return "..." + s[len(s)-400:]
	}
	return s
}
