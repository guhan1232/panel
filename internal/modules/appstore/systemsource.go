// systemsource.go —— 系统软件源（apt/dnf）持久化更换与恢复。
// 与安装时的临时源（aptMirrorInstallShell）不同：这里直接改写系统源文件并持久生效，
// 系统内所有 apt/dnf 安装天然走所选镜像。改写前自动备份原文件，可一键恢复官方源。
package appstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
)

// sysBackupSuffix 原始源文件备份后缀（首次换源时写入，恢复官方源用；不覆盖）。
const sysBackupSuffix = ".hanyuxin-orig"

// sysTargets 可选的系统源目标（official + 有 apt 镜像收录的 SoftwareSource）。
func sysTargets() []map[string]string {
	out := []map[string]string{{"id": "official", "name": "官方源"}}
	for _, s := range softwareSources {
		if s.ID == "auto" || s.AptUbuntu == "" {
			continue
		}
		out = append(out, map[string]string{"id": s.ID, "name": s.Name})
	}
	return out
}

// osInfo 系统标识：ID（ubuntu/debian/centos/rocky...）与版本号。
func osInfo() (osID, osVer string) {
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, ln := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(ln, "ID=") {
				osID = strings.Trim(strings.TrimPrefix(ln, "ID="), `"`)
			}
			if strings.HasPrefix(ln, "VERSION_ID=") {
				osVer = strings.Trim(strings.TrimPrefix(ln, "VERSION_ID="), `"`)
			}
		}
	}
	return strings.ToLower(strings.TrimSpace(osID)), strings.TrimSpace(osVer)
}

// aptSourceFiles 返回需要改写的 apt 源文件（DEB822 优先，其次传统 sources.list）。
func aptSourceFiles() []string {
	out := []string{}
	for _, f := range []string{
		"/etc/apt/sources.list.d/ubuntu.sources",
		"/etc/apt/sources.list.d/debian.sources",
		"/etc/apt/sources.list",
	} {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// detectCurrentDomain 从源文件内容中识别当前使用的主域名（用于状态展示）。
func detectCurrentDomain(content string) string {
	for _, d := range aptKnownDomains {
		if strings.Contains(content, d) {
			return d
		}
	}
	// 未识别：取第一条 deb 行的域名
	for _, ln := range strings.Split(content, "\n") {
		f := strings.Fields(strings.TrimSpace(ln))
		for _, p := range f {
			if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
				u := strings.TrimPrefix(strings.TrimPrefix(p, "https://"), "http://")
				return strings.Split(u, "/")[0]
			}
		}
	}
	return ""
}

// aptKnownDomains 源文件中可能出现的域名（官方/各镜像/云内网），域名级重写时全部清除。
var aptKnownDomains = []string{
	"security.ubuntu.com", "archive.ubuntu.com", "deb.debian.org",
	"mirrors.tencentyun.com", "mirrors.cloud.tencent.com", "mirrors.aliyun.com",
	"mirrors.tuna.tsinghua.edu.cn", "mirrors.ustc.edu.cn",
}

// mirrorRoot 目标镜像站的根地址。
func mirrorRoot(target string) (ubuntuRoot, debianRoot string, ok bool) {
	switch target {
	case "aliyun":
		return "http://mirrors.aliyun.com/ubuntu", "http://mirrors.aliyun.com/debian", true
	case "tencent":
		return "https://mirrors.cloud.tencent.com/ubuntu", "https://mirrors.cloud.tencent.com/debian", true
	case "tuna":
		return "https://mirrors.tuna.tsinghua.edu.cn/ubuntu", "https://mirrors.tuna.tsinghua.edu.cn/debian", true
	case "ustc":
		return "https://mirrors.ustc.edu.cn/ubuntu", "https://mirrors.ustc.edu.cn/debian", true
	}
	return "", "", false
}

// backupOnce 首次换源前备份源文件（原始文件只备份一次，恢复官方源用）。
func backupOnce(path string) error {
	bak := path + sysBackupSuffix
	if _, err := os.Stat(bak); err == nil {
		return nil // 已有备份：保留最初原始版本
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(bak, data, 0o644)
}

// rewriteAPT 按目标重写 apt 源内容（行级重组，保留 options/suite/components 结构）。
// osID: ubuntu | debian（security 仓库根处理不同）。
func rewriteAPT(content, target, osID string) (string, string) {
	ubRoot, debRoot, ok := mirrorRoot(target)
	if !ok {
		return content, ""
	}
	secRoot := debRoot + "-security" // Debian security 独立根
	lines := strings.Split(content, "\n")
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "deb ") && !strings.HasPrefix(t, "deb-src ") {
			continue
		}
		f := strings.Fields(t)
		if len(f) < 3 {
			continue
		}
		kind := f[0]
		idx := 1
		opts := ""
		for idx < len(f) && strings.HasPrefix(f[idx], "[") {
			opts += " " + f[idx]
			idx++
		}
		if idx+1 >= len(f) {
			continue
		}
		suite := f[idx+1]
		url := ubRoot
		if osID == "debian" {
			url = debRoot
			if strings.Contains(suite, "security") || strings.Contains(strings.Join(f, " "), "debian-security") {
				url = secRoot
			}
		}
		lines[i] = kind + opts + " " + url + " " + strings.Join(f[idx+1:], " ")
	}
	name := map[string]string{"aliyun": "阿里云", "tencent": "腾讯云", "tuna": "清华", "ustc": "中科大", "official": "官方"}[target]
	if osID == "debian" {
		name = strings.TrimSuffix(name, "")
	}
	return strings.Join(lines, "\n"), name
}

// ---- 路由 ----

// sysSourceGet GET /api/apps/sys-source：系统源现状。
func sysSourceGet(c *gin.Context) {
	osID, osVer := osInfo()
	pkg := "apt"
	if _, err := exec.LookPath("apt-get"); err != nil {
		if _, err2 := exec.LookPath("dnf"); err2 == nil {
			pkg = "dnf"
		} else if _, err3 := exec.LookPath("yum"); err3 == nil {
			pkg = "yum"
		}
	}
	files := []map[string]string{}
	current := ""
	if pkg == "apt" {
		for _, f := range aptSourceFiles() {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			content := string(data)
			if current == "" {
				current = detectCurrentDomain(content)
			}
			files = append(files, map[string]string{"path": f, "content": content})
		}
	}
	_, hasBackup := os.Stat("/etc/apt/sources.list" + sysBackupSuffix)
	_ = hasBackup
	backup := false
	for _, f := range aptSourceFiles() {
		if _, err := os.Stat(f + sysBackupSuffix); err == nil {
			backup = true
			break
		}
	}
	core.OK(c, gin.H{
		"pkg":     pkg,
		"os_id":   osID,
		"os_ver":  osVer,
		"files":   files,
		"current": current,
		"backup":  backup,
		"targets": sysTargets(),
	})
}

// sysSourceSet POST /api/apps/sys-source {target}：更换并持久化系统源（仅 admin）。
func sysSourceSet(c *gin.Context) {
	var req struct {
		Target string `json:"target" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	target := strings.TrimSpace(req.Target)
	osID, _ := osInfo()
	pkg := "apt"
	if _, err := exec.LookPath("apt-get"); err != nil {
		pkg = ""
		if _, err2 := exec.LookPath("dnf"); err2 == nil {
			pkg = "dnf"
		} else if _, err3 := exec.LookPath("yum"); err3 == nil {
			pkg = "yum"
		}
		if pkg == "" {
			core.Err(c, 400, "未找到 apt-get/dnf 包管理器")
			return
		}
	}

	if pkg == "apt" {
		files := aptSourceFiles()
		if len(files) == 0 {
			core.Err(c, 400, "未找到 apt 源文件（/etc/apt/sources.list）")
			return
		}
		for _, f := range files {
			if err := backupOnce(f); err != nil {
				core.Err(c, 500, "备份失败: "+err.Error())
				return
			}
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			rewritten, _ := rewriteAPT(string(data), target, osID)
			if err := os.WriteFile(f, []byte(rewritten), 0o644); err != nil {
				core.Err(c, 500, "写入源文件失败: "+err.Error())
				return
			}
		}
		out, _ := exec.Command("/bin/bash", "-c", "timeout 150 apt-get update 2>&1 | tail -6").CombinedOutput()
		name := map[string]string{"aliyun": "阿里云", "tencent": "腾讯云", "tuna": "清华大学", "ustc": "中科大", "official": "官方"}[target]
		core.OK(c, gin.H{"ok": true, "target": target, "name": name, "update": strings.TrimSpace(string(out))})
		return
	}

	// dnf/yum：仅支持 CentOS/RHEL 系换阿里云 base 源
	if target != "aliyun" && target != "official" {
		core.Err(c, 400, "dnf/yum 系当前仅支持阿里云镜像或恢复官方源")
		return
	}
	osID2, osVer := osInfo()
	_ = osID2
	el := "9"
	if i := strings.Index(osVer, "."); i > 0 {
		el = osVer[:i]
	}
	repoFiles, _ := filepath.Glob("/etc/yum.repos.d/*.repo")
	for _, f := range repoFiles {
		_ = backupOnce(f)
	}
	if target == "aliyun" {
		url := "https://mirrors.aliyun.com/repo/Centos-" + el + ".repo"
		out, _ := exec.Command("/bin/bash", "-c",
			"curl -fsSL --connect-timeout 20 -m 60 -o /etc/yum.repos.d/hanyuxin-base.repo "+url+" && dnf clean all >/dev/null 2>&1; timeout 150 dnf makecache 2>&1 | tail -4").CombinedOutput()
		name := "阿里云"
		core.OK(c, gin.H{"ok": true, "target": target, "name": name, "update": strings.TrimSpace(string(out))})
		return
	}
	// official：恢复备份
	for _, f := range repoFiles {
		bak := f + sysBackupSuffix
		if data, err := os.ReadFile(bak); err == nil {
			_ = os.WriteFile(f, data, 0o644)
		} else if strings.HasSuffix(f, "hanyuxin-base.repo") {
			_ = os.Remove(f)
		}
	}
	out, _ := exec.Command("/bin/bash", "-c", "timeout 150 dnf makecache 2>&1 | tail -4").CombinedOutput()
	core.OK(c, gin.H{"ok": true, "target": "official", "name": "官方源", "update": strings.TrimSpace(string(out))})
}

// sysSourceRestore POST /api/apps/sys-source/restore：从备份恢复原始源文件（仅 admin）。
func sysSourceRestore(c *gin.Context) {
	files := aptSourceFiles()
	restored := 0
	for _, f := range files {
		bak := f + sysBackupSuffix
		data, err := os.ReadFile(bak)
		if err != nil {
			continue
		}
		if err := os.WriteFile(f, data, 0o644); err == nil {
			restored++
		}
	}
	if restored == 0 {
		core.Err(c, 404, "没有可恢复的备份（从未更换过系统源）")
		return
	}
	out, _ := exec.Command("/bin/bash", "-c", "timeout 150 apt-get update 2>&1 | tail -6").CombinedOutput()
	core.OK(c, gin.H{"ok": true, "restored": restored, "update": strings.TrimSpace(string(out))})
}

