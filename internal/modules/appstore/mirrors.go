// 镜像源：安装应用时可选的下载/拉取加速。
//   - compose（Docker 镜像）：将 YAML 中的 image 改写为加速代理仓库
//   - native（原生二进制）：GitHub Release 下载地址追加加速代理前缀
package appstore

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/core"
)

// MirrorDef 一个镜像源定义：Docker 代理主机与 GitHub 加速前缀可同时提供。
type MirrorDef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Docker string `json:"docker"`  // 如 docker.m.daocloud.io（空=不支持 Docker 加速）
	GitHub string `json:"github"`  // 如 https://ghfast.top（空=不支持 GitHub 加速）
	Desc   string `json:"desc"`
}

var mirrors = []MirrorDef{
	{ID: "auto", Name: "自动（优先加速源）", Desc: "按应用类型自动选择第一个可用加速源"},
	{ID: "off", Name: "官方直连", Desc: "不使用任何加速，直连官方源"},
	{ID: "daocloud", Name: "DaoCloud", Docker: "docker.m.daocloud.io", Desc: "Docker Hub 镜像加速"},
	{ID: "1ms", Name: "毫秒镜像", Docker: "docker.1ms.run", Desc: "Docker Hub 镜像加速"},
	{ID: "ghfast", Name: "GitHub 加速 ghfast", GitHub: "https://ghfast.top", Desc: "GitHub Release 下载加速"},
	{ID: "ghproxy", Name: "GitHub 加速 gh-proxy", GitHub: "https://gh-proxy.com", Desc: "GitHub Release 下载加速"},
}

func findMirror(id string) (MirrorDef, bool) {
	for _, m := range mirrors {
		if m.ID == id {
			return m, true
		}
	}
	return MirrorDef{}, false
}

// resolveMirror 解析用户选择的镜像源。kind: compose | native。
// 返回实际生效的定义与描述文案（auto 会自动挑选）。
func resolveMirror(id, kind string) (MirrorDef, string) {
	switch id {
	case "off":
		return MirrorDef{ID: "off", Name: "官方直连"}, "官方直连"
	case "", "auto":
		for _, m := range mirrors {
			if kind == "compose" && m.Docker != "" {
				return m, "自动 → " + m.Name
			}
			if kind == "native" && m.GitHub != "" {
				return m, "自动 → " + m.Name
			}
		}
		return MirrorDef{}, "官方直连（无可用加速源）"
	default:
		if m, ok := findMirror(id); ok {
			if kind == "compose" && m.Docker == "" {
				return m, m.Name + "（该源不支持 Docker 加速，已按官方直连拉取）"
			}
			if kind == "native" && m.GitHub == "" {
				return m, m.Name + "（该源不支持 GitHub 加速，已按官方直连下载）"
			}
			return m, m.Name
		}
		return MirrorDef{ID: "off", Name: "官方直连"}, "官方直连（未知镜像源 ID）"
	}
}

// 注意：全部用 [ \t] 而非 \s，避免跨行吞掉换行破坏 YAML 结构。
var imageLineRe = regexp.MustCompile(`(?m)^([ \t]*-?[ \t]*image:[ \t]*)(\S+)[ \t]*$`)

// applyDockerMirror 将 compose YAML 中所有 image 行改写为经代理仓库拉取。
func applyDockerMirror(content, proxy string) string {
	proxy = strings.Trim(strings.TrimSpace(proxy), "/")
	if proxy == "" {
		return content
	}
	return imageLineRe.ReplaceAllStringFunc(content, func(line string) string {
		m := imageLineRe.FindStringSubmatch(line)
		if m == nil || strings.Contains(m[2], "{{") {
			return line
		}
		return m[1] + proxyImage(m[2], proxy)
	})
}

// proxyImage 将单个镜像引用改写为经代理仓库的引用。
//   - mysql:8            → <proxy>/library/mysql:8
//   - user/app:1         → <proxy>/user/app:1
//   - ghcr.io/x/y:2      → <proxy>/ghcr.io/x/y:2
//   - localhost:5000/a   → <proxy>/localhost:5000/a
func proxyImage(img, proxy string) string {
	img = strings.TrimPrefix(img, "docker.io/")
	// registry 判定只看第一段路径（避免把 tag 的冒号误判为 registry 端口）
	first := ""
	if i := strings.Index(img, "/"); i >= 0 {
		first = img[:i]
	}
	hasRegistry := first != "" &&
		(strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost")
	var name string
	switch {
	case hasRegistry:
		name = img
	case first == "":
		name = "library/" + img
	default:
		name = img
	}
	return proxy + "/" + name
}

// applyGitHubMirror 对 GitHub 下载地址追加加速前缀（ghproxy 风格）。
func applyGitHubMirror(rawURL, ghProxy string) string {
	ghProxy = strings.TrimRight(strings.TrimSpace(ghProxy), "/")
	if ghProxy == "" || !strings.Contains(rawURL, "github.com/") {
		return rawURL
	}
	return ghProxy + "/" + rawURL
}

// ---- 路由 ----

func listMirrors(c *gin.Context) {
	core.OK(c, mirrors)
}

// ---- 软件商店源（原生二进制下载通道）----

// SoftwareSource 软件商店下载源：GitHub 加速前缀或大镜像站根地址。
type SoftwareSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// GitHub 加速前缀（ghproxy 风格），与 Base 互斥
	GitHub string `json:"github,omitempty"`
	// 镜像站根地址（如 https://mirrors.aliyun.com），命中模板 MirrorPaths 时使用
	Base string `json:"base,omitempty"`
	Desc string `json:"desc"`
}

var softwareSources = []SoftwareSource{
	{ID: "auto", Name: "自动（GitHub 加速优先）", Desc: "GitHub 发布走加速，其余官方直连"},
	{ID: "official", Name: "官方直连", Desc: "不经任何加速"},
	{ID: "ghfast", Name: "GitHub 加速 ghfast", GitHub: "https://ghfast.top", Desc: "GitHub Release 下载加速"},
	{ID: "ghproxy", Name: "GitHub 加速 gh-proxy", GitHub: "https://gh-proxy.com", Desc: "GitHub Release 下载加速"},
	{ID: "aliyun", Name: "阿里云镜像站", Base: "https://mirrors.aliyun.com", Desc: "mirrors.aliyun.com"},
	{ID: "tencent", Name: "腾讯云镜像站", Base: "https://mirrors.cloud.tencent.com", Desc: "mirrors.cloud.tencent.com"},
	{ID: "tuna", Name: "清华大学镜像站", Base: "https://mirrors.tuna.tsinghua.edu.cn", Desc: "mirrors.tuna.tsinghua.edu.cn"},
	{ID: "ustc", Name: "中科大镜像站", Base: "https://mirrors.ustc.edu.cn", Desc: "mirrors.ustc.edu.cn"},
}

func listSoftSources(c *gin.Context) {
	core.OK(c, softwareSources)
}

// resolveNativeURL 按软件源解析原生应用下载地址。
//   - auto：GitHub 发布走加速前缀，其余官方直连
//   - ghfast/ghproxy：GitHub 加速前缀（非 GitHub 发布报错）
//   - aliyun/tencent/tuna/ustc：查模板 MirrorPaths[源ID][架构]，未收录报错
func resolveNativeURL(urls map[string]string, mirrorPaths map[string]map[string]string, arch, srcID string, p map[string]string) (string, string, error) {
	raw := substitute(urls[arch], p)
	if raw == "" {
		return "", "", fmt.Errorf("模板不支持架构 %s", arch)
	}
	switch srcID {
	case "", "auto":
		for _, s := range softwareSources {
			if s.GitHub != "" && strings.Contains(raw, "github.com/") {
				return applyGitHubMirror(raw, s.GitHub), "自动 → " + s.Name, nil
			}
		}
		return raw, "官方直连", nil
	case "official":
		return raw, "官方直连", nil
	case "ghfast", "ghproxy":
		for _, s := range softwareSources {
			if s.ID == srcID {
				if !strings.Contains(raw, "github.com/") {
					return "", "", fmt.Errorf("该应用不从 GitHub 发布，请选择官方直连或镜像站")
				}
				return applyGitHubMirror(raw, s.GitHub), s.Name, nil
			}
		}
		return "", "", fmt.Errorf("未知软件源: %s", srcID)
	default:
		for _, s := range softwareSources {
			if s.ID == srcID && s.Base != "" {
			paths, ok := mirrorPaths[srcID]
			if !ok {
				return "", "", fmt.Errorf("%s 未收录此软件（该源无对应目录），请改用 GitHub 加速或官方直连", s.Name)
			}
			u, ok := paths[arch]
			if !ok || u == "" {
				return "", "", fmt.Errorf("%s 未收录架构 %s 的此软件", s.Name, arch)
			}
			u = substitute(u, p)
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				u = strings.TrimRight(s.Base, "/") + "/" + strings.TrimLeft(u, "/")
			}
			return u, s.Name, nil
			}
		}
		return "", "", fmt.Errorf("未知软件源: %s", srcID)
	}
}
