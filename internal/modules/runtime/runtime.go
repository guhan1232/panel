// Package runtime 编程语言运行时版本管理器（宝塔式）：
// 支持多版本共存、设默认版本（软链 /usr/local/bin）、卸载。
// 版本清单从官方或各大镜像站（阿里云/腾讯云/清华/中科大）拉取。
package rtm

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

const httpLimit = 64 << 20

// RuntimeInst 已安装的运行时实例。
type RuntimeInst struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Lang      string    `gorm:"size:32;index" json:"lang"`
	Version   string    `gorm:"size:64" json:"version"`
	Dir       string    `gorm:"size:256" json:"dir"`
	Binary    string    `gorm:"size:128" json:"binary"` // 服务单元名（phpX.Y-fpm 等）
	Pkgs      string    `gorm:"size:512" json:"pkgs"`   // 安装的包（卸载用）
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

type sourceDef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Base string `json:"base"`
}

type binLink struct {
	Link string `json:"link"`
	Rel  string `json:"rel"`
}

type runtimeVersion struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	LTS     bool   `json:"lts,omitempty"`
	Date    string `json:"date,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Npm     string `json:"npm,omitempty"`
	V8      string `json:"v8,omitempty"`
}

type langDef struct {
	Key, Name, Desc string
	Sources         []sourceDef
	Bins            []binLink
	// VendorArch goarch → 厂商命名（node: x64；python: x86_64）
	VendorArch func(goarch string) string
	// List 拉取版本清单（按时间降序）
	List func(source, goarch, vendor string) ([]runtimeVersion, error)
	// Resolve 给定版本与源 → 下载地址
	Resolve func(version, source, goarch, vendor string) (string, error)
	// DirName 版本对应的安装目录名
	DirName func(version string) string
	// InstallOverride 非下载式安装（如 PHP 经发行版仓库装 FPM），nil 走通用下载流程
	InstallOverride func(m *Module, version, source string) ([]string, error)
	// UninstallOverride 非下载式卸载
	UninstallOverride func(m *Module, version string) ([]string, error)
	// NoDefault true 表示该语言多版本独立运行，无“默认版本”概念（如 PHP-FPM）
	NoDefault bool
}

var langs = map[string]*langDef{}
var langOrder = []string{"nodejs", "golang", "python", "openjdk", "php"}

var httpUA = "hanyuxin-panel/" + core.Version

func httpGet(url string, timeout time.Duration, limit int64) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", httpUA)
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

var hrefRe = regexp.MustCompile(`href="([^"]+)"`)
var goVerInListingRe = regexp.MustCompile(`go(\d+\.\d+\.\d+)\.linux-amd64\.tar\.gz`)
var jdkFileRe = regexp.MustCompile(`(OpenJDK\d+U-jdk_[^"/]+_linux_hotspot_[\d._]+)\.tar\.gz`)
var cleanNodeVerRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
var cleanGoVerRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func parseHrefs(html string) []string {
	out := []string{}
	for _, m := range hrefRe.FindAllStringSubmatch(html, -1) {
		out = append(out, m[1])
	}
	return out
}

// ---- 语言定义 ----

var nodeSources = []sourceDef{
	{ID: "aliyun", Name: "阿里云镜像站（默认）", Base: "https://mirrors.aliyun.com/nodejs-release"},
	{ID: "tencent", Name: "腾讯云镜像站", Base: "https://mirrors.cloud.tencent.com/nodejs-release"},
	{ID: "official", Name: "官方 nodejs.org", Base: "https://nodejs.org/dist"},
	{ID: "tuna", Name: "清华大学镜像站", Base: "https://mirrors.tuna.tsinghua.edu.cn/nodejs-release"},
	{ID: "ustc", Name: "中科大镜像站", Base: "https://mirrors.ustc.edu.cn/nodejs-release"},
}

var goSources = []sourceDef{
	{ID: "aliyun", Name: "阿里云镜像站（默认）", Base: "https://mirrors.aliyun.com/golang"},
	{ID: "official", Name: "官方 go.dev"},
	{ID: "goproxy", Name: "七牛 Goproxy 中国", Base: "https://goproxy.cn"},
	{ID: "tencent", Name: "腾讯云镜像站", Base: "https://mirrors.cloud.tencent.com/golang"},
	{ID: "ustc", Name: "中科大镜像站", Base: "https://mirrors.ustc.edu.cn/golang"},
}

var pythonSources = []sourceDef{
	{ID: "npmmirror", Name: "npmmirror 镜像（推荐）", Base: "https://registry.npmmirror.com/-/binary/python-build-standalone"},
	{ID: "official", Name: "官方 GitHub Releases"},
	{ID: "ghfast", Name: "GitHub 加速 ghfast"},
	{ID: "ghproxy", Name: "GitHub 加速 gh-proxy"},
}

type jdkSourceDef struct {
	ID     string
	Name   string
	Base   string
	Distro string // adoptium(Temurin) | openjdk(Oracle 通用构建)
}

var jdkSources = []jdkSourceDef{
	{ID: "tuna-adoptium", Name: "清华大学 · Temurin", Base: "https://mirrors.tuna.tsinghua.edu.cn", Distro: "adoptium"},
	{ID: "huawei-openjdk", Name: "华为云 · OpenJDK", Base: "https://mirrors.huaweicloud.com", Distro: "openjdk"},
}

func findJdkSource(id string) *jdkSourceDef {
	for i := range jdkSources {
		if jdkSources[i].ID == id {
			return &jdkSources[i]
		}
	}
	return nil // 兼容旧 ID
}

func nodeSourceBase(source string) (string, error) {
	for _, s := range nodeSources {
		if s.ID == source {
			return s.Base, nil
		}
	}
	return "", fmt.Errorf("未知 Node.js 下载源: %s", source)
}

// listNode 从镜像站 index.json 拉取全部 Node.js 版本。
func listNode(source, goarch, vendor string) ([]runtimeVersion, error) {
	base, err := nodeSourceBase(source)
	if err != nil {
		return nil, err
	}
	data, err := httpGet(base+"/index.json", 30*time.Second, httpLimit)
	if err != nil {
		return nil, err
	}
	return parseNodeIndex(data, base, vendor), nil
}

// parseNodeIndex 解析 nodejs-release index.json（纯函数，便于测试）。
func parseNodeIndex(data []byte, base, vendor string) []runtimeVersion {
	var arr []struct {
		Version string `json:"version"`
		Date    string `json:"date"`
		LTS     any    `json:"lts"`
		Npm     string `json:"npm"`
		V8      string `json:"v8"`
	}
	if json.Unmarshal(data, &arr) != nil {
		return nil
	}
	out := []runtimeVersion{}
	for i := 0; i < len(arr) && len(out) < 300; i++ { // 清单新版本在前
		v := arr[i]
		if !cleanNodeVerRe.MatchString(v.Version) {
			continue
		}
		lts := false
		switch t := v.LTS.(type) {
		case bool:
			lts = t
		case string:
			lts = t != "" && t != "false"
		}
		out = append(out, runtimeVersion{
			Version: v.Version, Date: v.Date, LTS: lts, Npm: v.Npm, V8: v.V8,
			URL: base + "/" + v.Version + "/node-" + v.Version + "-linux-" + vendor + ".tar.gz",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := versionNums(strings.TrimPrefix(out[i].Version, "v")), versionNums(strings.TrimPrefix(out[j].Version, "v"))
		for k := 0; k < 3; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return false
	})
	return out
}

func resolveNode(version, source, goarch, vendor string) (string, error) {
	base, err := nodeSourceBase(source)
	if err != nil {
		return "", err
	}
	return base + "/" + version + "/node-" + version + "-linux-" + vendor + ".tar.gz", nil
}

// listGo 镜像站走目录列表解析；官方走 JSON API。
func listGo(source, goarch, vendor string) ([]runtimeVersion, error) {
	out := []runtimeVersion{}
	switch source {
	case "goproxy":
		data, err := httpGet("https://goproxy.cn/golang.org/toolchain/@v/list", 30*time.Second, httpLimit)
		if err != nil {
			return nil, err
		}
		return parseToolchainList(string(data), goarch), nil
	case "official":
		data, err := httpGet("https://go.dev/dl/?mode=json&include=all", 30*time.Second, httpLimit)
		if err != nil {
			return nil, err
		}
		var arr []struct {
			Version string `json:"version"`
			Files   []struct {
				Filename string `json:"filename"`
				OS       string `json:"os"`
				Arch     string `json:"arch"`
				Kind     string `json:"kind"`
				SHA256   string `json:"sha256"`
			} `json:"files"`
		}
		if err := json.Unmarshal(data, &arr); err != nil {
			return nil, err
		}
		for _, rel := range arr {
			if !strings.HasPrefix(rel.Version, "go") {
				continue
			}
			ver := strings.TrimPrefix(rel.Version, "go")
			if !cleanGoVerRe.MatchString(ver) {
				continue
			}
			for _, f := range rel.Files {
				if f.OS == "linux" && f.Arch == goarch && f.Kind == "archive" && strings.HasSuffix(f.Filename, ".tar.gz") {
					out = append(out, runtimeVersion{Version: ver, URL: "https://go.dev/dl/" + f.Filename})
					break
				}
			}
		}
		return out, nil
	default:
		var base string
		for _, s := range goSources {
			if s.ID == source && s.Base != "" {
				base = s.Base
			}
		}
		if base == "" {
			return nil, fmt.Errorf("未知 Go 下载源: %s", source)
		}
		html, err := httpGet(base+"/", 30*time.Second, httpLimit)
		if err != nil {
			return nil, err
		}
		return parseGoListing(string(html), base, goarch), nil
	}
}

func resolveGo(version, source, goarch, vendor string) (string, error) {
	switch source {
	case "goproxy":
		// Go 1.21+ 工具链以 golang.org/toolchain 模块发布（zip 格式）
		return "https://goproxy.cn/golang.org/toolchain/@v/v0.0.1-go" + version + ".linux-" + goarch + ".zip", nil
	case "", "auto":
		// 默认走阿里云镜像（go.dev 国内直连慢，tarball 结构与官方一致）
		return "https://mirrors.aliyun.com/golang/go" + version + ".linux-" + goarch + ".tar.gz", nil
	case "official":
		return "https://go.dev/dl/go" + version + ".linux-" + goarch + ".tar.gz", nil
	default:
		for _, s := range goSources {
			if s.ID == source && s.Base != "" {
				return s.Base + "/go" + version + ".linux-" + goarch + ".tar.gz", nil
			}
		}
		return "", fmt.Errorf("未知 Go 下载源: %s", source)
	}
}

// listPython 走 python-build-standalone GitHub Releases（清单固定官方 API，下载可加速）。
func listPython(source, goarch, vendor string) ([]runtimeVersion, error) {
	if source == "npmmirror" {
		return listPythonNpmmirror(vendor)
	}
	data, err := httpGet("https://api.github.com/repos/astral-sh/python-build-standalone/releases?per_page=50", 30*time.Second, httpLimit)
	if err != nil {
		return nil, err
	}
	var arr []struct {
		TagName   string `json:"tag_name"`
		Published string `json:"published_at"`
		Assets    []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, err
	}
	out := []runtimeVersion{}
	for _, rel := range arr {
		for _, a := range rel.Assets {
			name := a.Name
			if !strings.HasPrefix(name, "cpython-") || !strings.Contains(name, "-"+vendor+"-unknown-linux-gnu-install_only.tar.gz") {
				continue
			}
			body := strings.TrimSuffix(strings.TrimPrefix(name, "cpython-"), "-"+vendor+"-unknown-linux-gnu-install_only.tar.gz")
			ver, _, ok := strings.Cut(body, "+")
			if !ok || !strings.Contains(ver, ".") {
				continue
			}
			out = append(out, runtimeVersion{
				Version: ver, Tag: rel.TagName,
				Date: strings.Split(rel.Published, "T")[0],
				URL:  a.BrowserDownloadURL,
			})
			break // 同 tag 同版本取第一个匹配架构
		}
		if len(out) >= 60 {
			break
		}
	}
	if source == "ghfast" || source == "ghproxy" {
		for i := range out {
			for _, s := range []struct{ ID, Prefix string }{{"ghfast", "https://ghfast.top/"}, {"ghproxy", "https://gh-proxy.com/"}} {
				if s.ID == source && strings.Contains(out[i].URL, "github.com/") {
					out[i].URL = s.Prefix + out[i].URL
				}
			}
		}
	}
	return out, nil
}

func resolvePython(version, source, goarch, vendor string) (string, error) {
	ver, tag, _ := strings.Cut(version, "+")
	if tag == "" {
		return "", fmt.Errorf("Python 版本需带构建标签，如 3.12.7+20241016")
	}
	file := fmt.Sprintf("cpython-%s+%s-%s-unknown-linux-gnu-install_only.tar.gz", ver, tag, vendor)
	if source == "npmmirror" {
		return "https://registry.npmmirror.com/-/binary/python-build-standalone/" + tag + "/" + file, nil
	}
	u := "https://github.com/astral-sh/python-build-standalone/releases/download/" + tag + "/" + file
	if source == "ghfast" || source == "ghproxy" {
		prefix := "https://ghfast.top/"
		if source == "ghproxy" {
			prefix = "https://gh-proxy.com/"
		}
		u = prefix + u
	} else if source != "official" && source != "" {
		return "", fmt.Errorf("未知 Python 下载源: %s", source)
	}
	return u, nil
}

// listOpenJDK 从清华/中科大 Adoptium 目录解析：先列主版本，再列各主版本文件。
func listOpenJDK(source, goarch, vendor string) ([]runtimeVersion, error) {
	def := findJdkSource(source)
	if source == "tuna" {
		def = findJdkSource("tuna-adoptium") // 兼容旧源 ID
	}
	if def == nil {
		return nil, fmt.Errorf("未知 OpenJDK 下载源: %s", source)
	}
	if def.Distro == "openjdk" {
		return listHuaweiOpenJDK(def.Base, goarch, vendor)
	}
	base := def.Base
	html, err := httpGet(base+"/Adoptium/?C=N;O=A", 30*time.Second, httpLimit)
	if err != nil {
		return nil, err
	}
	majors := []int{}
	for _, h := range parseHrefs(string(html)) {
		h = strings.Trim(h, "/")
		if n, err := strconv.Atoi(h); err == nil && n >= 8 {
			majors = append(majors, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(majors)))
	if len(majors) > 5 {
		majors = majors[:5] // 最新 5 个大版本，避免过多请求
	}
	out := []runtimeVersion{}
	for _, major := range majors {
		filesHTML, err := httpGet(fmt.Sprintf("%s/Adoptium/%d/jdk/%s/linux/?C=N;O=A", base, major, vendor), 30*time.Second, httpLimit)
		if err != nil {
			continue // 个别大版本可能无此架构
		}
		for _, h := range parseHrefs(string(filesHTML)) {
			m := jdkFileRe.FindStringSubmatch(h)
			if m == nil || !strings.Contains(m[1], "_"+vendor+"_") {
				continue
			}
			file := m[1] + ".tar.gz"
			tail := file[strings.Index(file, "hotspot_")+len("hotspot_"):]
			tail = strings.TrimSuffix(tail, ".tar.gz")
			ver := strings.ReplaceAll(tail, "_", "+")
			out = append(out, runtimeVersion{
				Version: ver,
				URL:     fmt.Sprintf("%s/Adoptium/%d/jdk/%s/linux/%s", base, major, vendor, file),
				Date:    "",
			})
		}
	}
	// 按版本号数值排序（字符串比较会跨位数出错，如 9.x vs 25.x）
	sort.Slice(out, func(i, j int) bool {
		a, b := versionNums(out[i].Version), versionNums(out[j].Version)
		for k := 0; k < 2; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return false
	})
	return out, nil
}

func resolveOpenJDK(version, source, goarch, vendor string) (string, error) {
	def := findJdkSource(source)
	if source == "tuna" {
		def = findJdkSource("tuna-adoptium")
	}
	if def == nil {
		return "", fmt.Errorf("未知 OpenJDK 下载源: %s", source)
	}
	major := strings.Split(version, ".")[0]
	if def.Distro == "openjdk" {
		// 华为云通用构建：/openjdk/<版本>/openjdk-<版本>_linux-<架构>_bin.tar.gz
		return def.Base + "/openjdk/" + version + "/openjdk-" + version + "_linux-" + vendor + "_bin.tar.gz", nil
	}
	file := fmt.Sprintf("OpenJDK%sU-jdk_%s_linux_hotspot_%s.tar.gz",
		major, vendor, strings.ReplaceAll(version, "+", "_"))
	return def.Base + "/Adoptium/" + major + "/jdk/" + vendor + "/linux/" + file, nil
}

func vendorArchOf(lang, goarch string) string {
	switch lang {
	case "nodejs", "openjdk":
		if goarch == "amd64" {
			return "x64"
		}
		return goarch
	case "python":
		if goarch == "amd64" {
			return "x86_64"
		}
		return goarch
	}
	return goarch
}

func init() {
	langs["nodejs"] = &langDef{
		Key: "nodejs", Name: "Node.js",
		Desc:       "JavaScript 运行时；版本清单来自官方或镜像站 index.json",
		Sources:    nodeSources,
		Bins:       nil, // 依赖版本，运行时计算
		VendorArch: func(g string) string { return vendorArchOf("nodejs", g) },
		List:       listNode,
		Resolve:    resolveNode,
	}
	langs["golang"] = &langDef{
		Key: "golang", Name: "Golang",
		Desc:       "Go 编译工具链；镜像站目录解析版本清单",
		Sources:    goSources,
		Bins:       []binLink{{Link: "go", Rel: "go/bin/go"}, {Link: "gofmt", Rel: "go/bin/gofmt"}},
		VendorArch: func(g string) string { return g },
		List:       listGo,
		Resolve:    resolveGo,
	}
	langs["python"] = &langDef{
		Key: "python", Name: "Python",
		Desc:       "python-build-standalone 独立发行版；清单来自 GitHub Releases，下载可加速",
		Sources:    pythonSources,
		Bins:       []binLink{{Link: "python3", Rel: "python/bin/python3"}, {Link: "python", Rel: "python/bin/python3"}},
		VendorArch: func(g string) string { return vendorArchOf("python", g) },
		List:       listPython,
		Resolve:    resolvePython,
	}
	jdkSrcs := []sourceDef{}
	for _, js := range jdkSources {
		jdkSrcs = append(jdkSrcs, sourceDef{ID: js.ID, Name: js.Name})
	}
	langs["php"] = &langDef{
		Key: "php", Name: "PHP",
		Desc:    "多版本 PHP-FPM；Debian/Ubuntu 经 ondrej/sury 仓库，RHEL 经 Remi 仓库安装，各版本独立 systemd 服务",
		Sources: []sourceDef{{ID: "repo", Name: "发行版仓库 (ondrej/Remi)"}},
		Bins:    []binLink{},
		List:    listPHP,
		InstallOverride: func(m *Module, version, source string) ([]string, error) {
			return m.installPHP(version)
		},
		UninstallOverride: (*Module).uninstallPHP,
		NoDefault:         true,
	}
	langs["openjdk"] = &langDef{
		Key: "openjdk", Name: "OpenJDK",
		Desc:       "Java 开发工具包；Temurin(清华)与华为云通用构建",
		Sources:    jdkSrcs,
		Bins:       nil, // 依赖版本
		VendorArch: func(g string) string { return vendorArchOf("openjdk", g) },
		List:       listOpenJDK,
		Resolve:    resolveOpenJDK,
	}
}

// binsFor 计算某语言某版本需要软链的二进制（相对运行时安装根目录）。
func binsFor(lang, version, vendor string) []binLink {
	l := langs[lang]
	switch lang {
	case "nodejs":
		return []binLink{{Link: "node", Rel: "node-" + version + "-linux-" + vendor + "/bin/node"}}
	case "openjdk":
		return []binLink{{Link: "java", Rel: "jdk-" + version + "/bin/java"}}
	default:
		return l.Bins
	}
}

// dirNameFor 安装目录名（data/runtimes/<lang>/<dirName>）。
func dirNameFor(lang, version string) string {
	switch lang {
	case "golang":
		return "go" + version
	case "python":
		return "cpython-" + version
	case "openjdk":
		return "jdk-" + version
	}
	return version
}

// ---- 模块 ----

type Module struct {
	db      *gorm.DB
	dataDir string
}

func New(db *gorm.DB, dataDir string) *Module {
	return &Module{db: db, dataDir: dataDir}
}

func (m *Module) Start() error {
	return m.db.AutoMigrate(&RuntimeInst{})
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/runtimes")
	g.GET("", m.installed)
	g.GET("/langs", m.langs)
	g.GET("/:lang/versions", m.versions)
	g.GET("/:lang/registry", m.getRegistry)
	g.POST("/:lang/registry", m.setRegistry)
	g.GET("/:lang/pypi", m.getPypi)
	g.POST("/:lang/pypi", m.setPypi)
	g.GET("/:lang/composer", m.getComposer)
	g.POST("/:lang/composer", m.setComposer)
	g.POST("/:lang/composer/install", m.installComposer)
	g.GET("/:lang/pecl", m.getPecl)
	g.POST("/:lang/pecl", m.setPecl)
	g.POST("/:lang/install", m.install)
	g.POST("/:lang/set-default", m.setDefault)
	g.DELETE("/:id", m.remove)
}

func (m *Module) langs(c *gin.Context) {
	items := []gin.H{}
	for _, key := range langOrder {
		l := langs[key]
		srcs := []gin.H{}
		for _, s := range l.Sources {
			srcs = append(srcs, gin.H{"id": s.ID, "name": s.Name})
		}
		items = append(items, gin.H{"key": l.Key, "name": l.Name, "desc": l.Desc, "sources": srcs})
	}
	core.OK(c, items)
}

func (m *Module) versions(c *gin.Context) {
	lang := c.Param("lang")
	l, ok := langs[lang]
	if !ok {
		core.Err(c, 404, "未知语言: "+lang)
		return
	}
	source := c.DefaultQuery("source", l.Sources[0].ID)
	vendor := vendorArchOf(lang, goruntime.GOARCH)
	list, err := l.List(source, goruntime.GOARCH, vendor)
	if err != nil {
		core.Err(c, 500, "获取版本清单失败: "+err.Error())
		return
	}
	core.OK(c, list)
}

var dirSanitizer = regexp.MustCompile(`[^A-Za-z0-9.+_-]+`)

func (m *Module) install(c *gin.Context) {
	lang := c.Param("lang")
	if _, ok := langs[lang]; !ok {
		core.Err(c, 404, "未知语言: "+lang)
		return
	}
	if goruntime.GOOS != "linux" {
		core.Err(c, 400, "运行时安装仅支持在 Linux 服务器上执行")
		return
	}
	var req struct {
		Version string `binding:"required"`
		Source  string `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	links, err := m.installRuntime(lang, req.Version, req.Source)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"links": links})
}

func (m *Module) setDefault(c *gin.Context) {
	var body struct {
		InstanceID uint `json:"instance_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		core.BindErr(c, err)
		return
	}
	id := int(body.InstanceID)
	var inst RuntimeInst
	if err := m.db.First(&inst, id).Error; err != nil {
		core.Err(c, 404, "实例不存在")
		return
	}
	l := langs[inst.Lang]
	if l == nil {
		core.Err(c, 404, "未知语言")
		return
	}
	if l.NoDefault {
		core.Err(c, 400, "该运行时各版本独立 systemd 服务，无需设默认")
		return
	}
	vendor := vendorArchOf(inst.Lang, goruntime.GOARCH)
	dirAbs, _ := filepath.Abs(inst.Dir)
	for _, b := range binsFor(inst.Lang, inst.Version, vendor) {
		target := filepath.Join(dirAbs, filepath.FromSlash(b.Rel))
		link := filepath.Join("/usr/local/bin", b.Link)
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			core.Err(c, 500, "切换软链失败: "+err.Error())
			return
		}
	}
	m.db.Model(&RuntimeInst{}).Where("lang = ?", inst.Lang).Update("is_default", false)
	m.db.Model(&inst).Update("is_default", true)
	core.OK(c, nil)
}

func (m *Module) remove(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var inst RuntimeInst
	if err := m.db.First(&inst, id).Error; err != nil {
		core.Err(c, 404, "实例不存在")
		return
	}
	l := langs[inst.Lang]
	if l != nil && l.UninstallOverride != nil {
		if _, err := l.UninstallOverride(m, inst.Version); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		m.db.Delete(&inst)
		core.OK(c, nil)
		return
	}
	if l != nil {
		vendor := vendorArchOf(inst.Lang, goruntime.GOARCH)
		for _, b := range binsFor(inst.Lang, inst.Version, vendor) {
			_ = os.Remove(filepath.Join("/usr/local/bin", b.Link))
		}
	}
	_ = os.RemoveAll(inst.Dir)
	m.db.Delete(&inst)
	core.OK(c, nil)
}

func (m *Module) installed(c *gin.Context) {
	var rows []RuntimeInst
	m.db.Order("lang, is_default DESC, id DESC").Find(&rows)
	items := []gin.H{}
	for _, key := range langOrder {
		l := langs[key]
		mine := []RuntimeInst{}
		for _, r := range rows {
			if r.Lang == key {
				mine = append(mine, r)
			}
		}
		srcs := []string{}
		for _, s := range l.Sources {
			srcs = append(srcs, s.ID)
		}
		items = append(items, gin.H{
			"key": l.Key, "name": l.Name, "desc": l.Desc,
			"sources": l.Sources, "installed": mine,
		})
	}
	core.OK(c, items)
}

// execDownload 通过 curl 下载文件（统一走执行层，超时 15 分钟）。
func execDownload(local, url string) (string, int, error) {
	return execx.Run("runtime", 15*time.Minute, "curl", "-fSL", "--retry", "3", "--connect-timeout", "30", "-o", local, url)
}

// untar 解压 tar.gz 到目标目录（整包解压，含顶层目录）。
func untar(local, dir string) error {
	return archive.UnTarGz(local, dir)
}

// versionNums 提取三段版本号用于排序比较。
func versionNums(v string) [3]int {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "go"), ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		out[i] = n
	}
	return out
}

// installRuntime 下载→解压→软链→入库，返回软链路径列表。
func (m *Module) installRuntime(lang, version, source string) ([]string, error) {
	l := langs[lang]
	if l.InstallOverride != nil {
		return l.InstallOverride(m, version, source)
	}
	var count int64
	m.db.Model(&RuntimeInst{}).Where("lang = ? AND version = ?", lang, version).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("该版本已安装")
	}
	vendor := vendorArchOf(lang, goruntime.GOARCH)
	url, err := l.Resolve(version, source, goruntime.GOARCH, vendor)
	if err != nil {
		return nil, err
	}
	dirName := dirSanitizer.ReplaceAllString(dirNameFor(lang, version), "_")
	if dirName == "" || dirName == "." {
		return nil, fmt.Errorf("版本目录名非法")
	}
	dir, _ := filepath.Abs(filepath.Join(m.dataDir, "runtimes", lang, dirName))
	_ = os.MkdirAll(dir, 0o755)
	fail := func(err error) error {
		_ = os.RemoveAll(dir)
		return err
	}
	local := filepath.Join(dir, "download.tmp")
	if _, code, derr := execDownload(local, url); derr != nil {
		return nil, fail(fmt.Errorf("下载失败(exit %d): %s", code, url))
	}
	defer os.Remove(local)
	if strings.HasSuffix(url, ".zip") {
		if err := archive.UnZip(local, dir); err != nil {
			return nil, fail(fmt.Errorf("解压失败: %w", err))
		}
	} else if err := untar(local, dir); err != nil {
		return nil, fail(fmt.Errorf("解压失败: %w", err))
	}
	links := []string{}
	for _, b := range binsFor(lang, version, vendor) {
		target := filepath.Join(dir, filepath.FromSlash(b.Rel))
		if _, err := os.Stat(target); err != nil {
			// zip 顶层目录结构不同（如 goproxy 工具链包），按文件名在解压树中探索
			found, ferr := findInTree(dir, filepath.Base(b.Rel))
			if ferr != nil {
				return nil, fail(fmt.Errorf("解压包内未找到 %s", b.Rel))
			}
			target = found
		}
		_ = os.Chmod(target, 0o755)
		link := filepath.Join("/usr/local/bin", b.Link)
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			return nil, fail(fmt.Errorf("创建软链失败: %w", err))
		}
		links = append(links, link)
	}
	m.db.Model(&RuntimeInst{}).Where("lang = ?", lang).Update("is_default", false)
	inst := RuntimeInst{Lang: lang, Version: version, Dir: dir, IsDefault: true}
	if err := m.db.Create(&inst).Error; err != nil {
		return nil, fail(err)
	}
	return links, nil
}

// Tool 判断该语言是否解压型（下载解压即用，无 systemd 服务单元）。
func (l *langDef) Tool() bool {
	return l.Key == "nodejs" || l.Key == "golang" || l.Key == "python" || l.Key == "openjdk"
}

// parseGoListing 解析镜像站 golang/ 目录列表（纯函数）。
func parseGoListing(html, base, goarch string) []runtimeVersion {
	out := []runtimeVersion{}
	seen := map[string]bool{}
	for _, m := range goVerInListingRe.FindAllStringSubmatch(html, -1) {
		ver := m[1]
		if seen[ver] {
			continue
		}
		seen[ver] = true
		out = append(out, runtimeVersion{Version: ver, URL: base + "/go" + ver + ".linux-" + goarch + ".tar.gz"})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := versionNums(out[i].Version), versionNums(out[j].Version)
		for k := 0; k < 3; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return false
	})
	return out
}

// jdkVerFromFile 从 Temurin 文件名推出版本号（纯函数）。
// OpenJDK17U-jdk_x64_linux_hotspot_17.0.12_7.tar.gz → "17.0.12+7"。
func jdkVerFromFile(file string) (string, bool) {
	idx := strings.Index(file, "hotspot_")
	if idx < 0 {
		return "", false
	}
	tail := strings.TrimSuffix(file[idx+len("hotspot_"):], ".tar.gz")
	if tail == "" || !strings.Contains(tail, "_") {
		return "", false
	}
	return strings.ReplaceAll(tail, "_", "+"), true
}

// listPythonNpmmirror 从 npmmirror 镜像目录拉取 python-build-standalone 版本清单。
func listPythonNpmmirror(vendor string) ([]runtimeVersion, error) {
	base := "https://registry.npmmirror.com/-/binary/python-build-standalone"
	rootJSON, err := httpGet(base+"/", 30*time.Second, httpLimit)
	if err != nil {
		return nil, err
	}
	var dirs []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if json.Unmarshal(rootJSON, &dirs) != nil {
		return nil, fmt.Errorf("npmmirror 目录解析失败")
	}
	tags := []string{}
	for _, d := range dirs {
		if d.Type == "dir" && strings.HasSuffix(d.Name, "/") {
			tags = append(tags, strings.TrimSuffix(d.Name, "/"))
		}
	}
	sort.Strings(tags)
	if len(tags) > 20 {
		tags = tags[len(tags)-20:]
	}
	out := []runtimeVersion{}
	for _, tag := range tags {
		tagJSON, err := httpGet(base+"/"+tag+"/", 30*time.Second, httpLimit)
		if err != nil {
			continue
		}
		var files []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if json.Unmarshal(tagJSON, &files) != nil {
			continue
		}
		for _, f := range files {
			m := strings.HasPrefix(f.Name, "cpython-") && strings.Contains(f.Name, "-"+vendor+"-unknown-linux-gnu-install_only.tar.gz")
			if !m {
				continue
			}
			body := strings.TrimSuffix(strings.TrimPrefix(f.Name, "cpython-"), "-"+vendor+"-unknown-linux-gnu-install_only.tar.gz")
			ver, tag0, ok := strings.Cut(body, "+")
			if !ok || tag0 != tag {
				continue
			}
			out = append(out, runtimeVersion{Version: ver + "+" + tag0, Tag: tag, URL: f.URL})
			break
		}
		if len(out) >= 40 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := versionNums(strings.Split(out[i].Version, "+")[0]), versionNums(strings.Split(out[j].Version, "+")[0])
		for k := 0; k < 3; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return false
	})
	return out, nil
}

// nodeNpmPath 默认 nodejs 实例的 npm 可执行路径（无默认实例返回空）。
func (m *Module) nodeNpmPath() string {
	var inst RuntimeInst
	if err := m.db.Where("lang = ? AND is_default = ?", "nodejs", true).First(&inst).Error; err != nil {
		return ""
	}
	vendor := vendorArchOf("nodejs", goruntime.GOARCH)
	return filepath.Join(inst.Dir, "node-"+inst.Version+"-linux-"+vendor, "bin", "npm")
}

func (m *Module) getRegistry(c *gin.Context) {
	registry := ""
	if npm := m.nodeNpmPath(); npm != "" {
		if out, code, err := execx.Run("runtime", 60*time.Second, npm, "config", "get", "registry"); err == nil && code == 0 {
			registry = strings.TrimSpace(out)
		}
	}
	core.OK(c, gin.H{"registry": registry})
}

func (m *Module) setRegistry(c *gin.Context) {
	var req struct {
		Registry string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !strings.HasPrefix(req.Registry, "http://") && !strings.HasPrefix(req.Registry, "https://") {
		core.Err(c, 400, "registry 需以 http(s):// 开头")
		return
	}
	npm := m.nodeNpmPath()
	if npm == "" {
		core.Err(c, 400, "未找到默认 Node.js（请先安装并设为默认）")
		return
	}
	if out, code, err := execx.Run("runtime", 60*time.Second, npm, "config", "set", "registry", req.Registry); err != nil {
		core.Err(c, 500, fmt.Sprintf("设置失败(exit %d): %s", code, out))
		return
	}
	slog.Info("npm registry 已设置", "registry", req.Registry, "by", c.GetString("user"))
	core.OK(c, gin.H{"registry": req.Registry})
}

// parseToolchainList 解析 goproxy toolchain @v/list（纯函数，仅保留 linux-arch 条目）。
func parseToolchainList(list string, goarch string) []runtimeVersion {
	suffix := ".linux-" + goarch
	out := []runtimeVersion{}
	for _, line := range strings.Split(list, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, suffix) {
			continue
		}
		// v0.0.1-go1.23.4.linux-amd64 → 1.23.4
		body := strings.TrimSuffix(strings.TrimPrefix(line, "v0.0.1-go"), suffix)
		if !cleanGoVerRe.MatchString(body) {
			continue
		}
		out = append(out, runtimeVersion{Version: body, URL: "https://goproxy.cn/golang.org/toolchain/@v/" + line + ".zip"})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := versionNums(out[i].Version), versionNums(out[j].Version)
		for k := 0; k < 3; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return false
	})
	return out
}

// findInTree 在解压树中按文件名查找第一个匹配文件。
func findInTree(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", err
	}
	return found, nil
}

// pythonPipPath 默认 python 实例的 pip3 路径（无默认实例返回空）。
func (m *Module) pythonPipPath() string {
	var inst RuntimeInst
	if err := m.db.Where("lang = ? AND is_default = ?", "python", true).First(&inst).Error; err != nil {
		return ""
	}
	return filepath.Join(inst.Dir, "python", "bin", "pip3")
}

func (m *Module) getPypi(c *gin.Context) {
	indexURL := ""
	if pip := m.pythonPipPath(); pip != "" {
		if out, code, err := execx.Run("runtime", 60*time.Second, pip, "config", "get", "global.index-url"); err == nil && code == 0 {
			out = strings.TrimSpace(out)
			if strings.HasPrefix(out, "http://") || strings.HasPrefix(out, "https://") {
				indexURL = out
			}
		}
	}
	core.OK(c, gin.H{"index_url": indexURL})
}

func (m *Module) setPypi(c *gin.Context) {
	var req struct {
		IndexURL string `json:"index_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !strings.HasPrefix(req.IndexURL, "http://") && !strings.HasPrefix(req.IndexURL, "https://") {
		core.Err(c, 400, "pypi 源需以 http(s):// 开头")
		return
	}
	pip := m.pythonPipPath()
	if pip == "" {
		core.Err(c, 400, "未找到默认 Python（请先安装并设为默认）")
		return
	}
	if out, code, err := execx.Run("runtime", 60*time.Second, pip, "config", "set", "global.index-url", req.IndexURL); err != nil {
		core.Err(c, 500, fmt.Sprintf("设置失败(exit %d): %s", code, out))
		return
	}
	slog.Info("pip index-url 已设置", "url", req.IndexURL, "by", c.GetString("user"))
	core.OK(c, gin.H{"index_url": req.IndexURL})
}

// parseHuaweiOpenJDKDirs 从华为 /openjdk/ 目录列表提取版本目录（纯函数）。
func parseHuaweiOpenJDKDirs(html string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, h := range parseHrefs(html) {
		h = strings.Trim(h, "/")
		if h == "" || !isVerDir(h) || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := versionNums(out[i]), versionNums(out[j])
		for k := 0; k < 4; k++ {
			var ai, bi int
			if k < len(a) {
				ai = a[k]
			}
			if k < len(b) {
				bi = b[k]
			}
			if ai != bi {
				return ai > bi
			}
		}
		return false
	})
	return out
}

func isVerDir(s string) bool {
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return false
		}
		for _, r := range seg {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// listHuaweiOpenJDK 华为通用构建清单（目录名即版本，文件名可推导）。
func listHuaweiOpenJDK(base, goarch, vendor string) ([]runtimeVersion, error) {
	html, err := httpGet(base+"/openjdk/", 30*time.Second, httpLimit)
	if err != nil {
		return nil, err
	}
	dirs := parseHuaweiOpenJDKDirs(string(html))
	if len(dirs) > 30 {
		dirs = dirs[:30]
	}
	out := []runtimeVersion{}
	for _, v := range dirs {
		out = append(out, runtimeVersion{
			Version: v,
			URL:     base + "/openjdk/" + v + "/openjdk-" + v + "_linux-" + vendor + "_bin.tar.gz",
		})
	}
	return out, nil
}

// phpBranches 已知的 PHP 稳定/EOL 分支（安装按分支装发行版包）。
// phpSourceDirs 提供 PHP 全历史版本(tar.gz)的镜像站目录。
var phpSourceDirs = []sourceDef{
	{ID: "sohu", Name: "搜狐镜像", Base: "https://mirrors.sohu.com/php"},
	{ID: "official", Name: "php.net", Base: "https://www.php.net/distributions"},
}

var phpBranchesFallback = []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4", "7.3", "7.2", "7.1", "7.0", "5.6"}

// listPHP 版本清单：全历史分支（搜狐目录解析，失败回退静态表）+ php.net JSON 补日期。
func listPHP(source, goarch, vendor string) ([]runtimeVersion, error) {
	dates := map[string]string{}
	if data, err := httpGet("https://www.php.net/releases/index.php?json&max=100", 20*time.Second, 8<<20); err == nil {
		dates = parsePHPJson(data)
	}
	branches := phpBranchesFallback
	if html, err := httpGet(phpSourceDirs[0].Base+"/", 30*time.Second, 8<<20); err == nil {
		if dyn := branchesFromPHPListing(string(html), phpBranchesFallback); len(dyn) > 0 {
			branches = dyn
		}
	}
	out := []runtimeVersion{}
	for _, br := range branches {
		out = append(out, runtimeVersion{Version: br, Date: dates[br]})
	}
	return out, nil
}

// parsePHPJson 解析 php.net releases JSON（键为major），返回分支 → 最新补丁日期。
func parsePHPJson(data []byte) map[string]string {
	out := map[string]string{}
	var m map[string]struct {
		Date   string `json:"date"`
		Source []struct {
			Filename string `json:"filename"`
			Date     string `json:"date"`
		} `json:"source"`
	}
	if json.Unmarshal(data, &m) != nil {
		return out
	}
	for _, files := range m {
		for _, f := range files.Source {
			name := f.Filename
			if !strings.HasPrefix(name, "php-") || !strings.HasSuffix(name, ".tar.gz") {
				continue
			}
			ver := strings.TrimSuffix(strings.TrimPrefix(name, "php-"), ".tar.gz")
			parts := strings.Split(ver, ".")
			if len(parts) < 2 {
				continue
			}
			br := parts[0] + "." + parts[1]
			if _, exists := out[br]; !exists {
				out[br] = f.Date
			}
			break
		}
	}
	return out
}

// installPHP 经发行版仓库安装指定分支的 PHP-FPM（多版本共存，各自独立 systemd 服务）。
func (m *Module) installPHP(version string) ([]string, error) {
	if goruntime.GOOS != "linux" {
		return nil, fmt.Errorf("PHP 安装仅支持在 Linux 服务器上执行")
	}
	osID, verID, codename := osRelease()
	var cmds []string
	var unit string
	var pkgs []string
	cmds = append(cmds, "systemctl stop unattended-upgrades || true")
	switch osID {
	case "ubuntu":
		cmds = append(cmds,
			"DEBIAN_FRONTEND=noninteractive apt-get install -y software-properties-common || true",
			"add-apt-repository ppa:ondrej/php -y",
			"apt-get update",
		)
		pkgs = []string{"php" + version + "-fpm", "php" + version + "-mysql", "php" + version + "-curl", "php" + version + "-gd", "php" + version + "-xml", "php" + version + "-mbstring"}
		unit = "php" + version + "-fpm"
	case "debian":
		if codename == "" {
			codename = shellOut("lsb_release -cs")
		}
		if codename == "" {
			return nil, fmt.Errorf("无法识别 Debian 发行版代号")
		}
		cmds = append(cmds,
			"DEBIAN_FRONTEND=noninteractive apt-get install -y gnupg curl ca-certificates",
			"curl -fsSL https://packages.sury.org/php/apt.gpg -o /usr/share/keyrings/sury-php.gpg",
			"echo \"deb [signed-by=/usr/share/keyrings/sury-php.gpg] https://packages.sury.org/php/ "+codename+" main\" > /etc/apt/sources.list.d/sury-php.list",
			"apt-get update",
		)
		pkgs = []string{"php" + version + "-fpm", "php" + version + "-mysql", "php" + version + "-curl", "php" + version + "-gd", "php" + version + "-xml", "php" + version + "-mbstring"}
		unit = "php" + version + "-fpm"
	case "centos", "rocky", "almalinux", "rhel", "ol", "fedora":
		el := verID
		if i := strings.Index(el, "."); i > 0 {
			el = el[:i]
		}
		cmds = append(cmds,
			"dnf install -y epel-release https://mirrors.tuna.tsinghua.edu.cn/remi/enterprise/remi-release-"+el+".rpm",
			"dnf module reset php -y",
			"dnf module enable php:remi-"+version+" -y",
		)
		pkgs = []string{"php-fpm", "php-mysqlnd", "php-gd", "php-curl", "php-xml", "php-mbstring"}
		unit = "php-fpm"
	default:
		return nil, fmt.Errorf("暂不支持发行版 %s 的 PHP 仓库安装", osID)
	}
	installCmd := "DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=1800 install -y " + strings.Join(pkgs, " ")
	if osID != "ubuntu" && osID != "debian" {
		installCmd = "dnf install -y " + strings.Join(pkgs, " ")
	}
	// 逐条执行仓库配置命令（PPA/sury/Remi + 更新索引），失败即中止并带回显输出
	for _, cmd := range cmds {
		out, code, err := execx.RunShell("runtime:php", 15*time.Minute, cmd)
		if err != nil || code != 0 {
			return nil, fmt.Errorf("仓库配置失败(exit %d): %s\n%s", code, cmd, tailOf(out))
		}
	}
	if out, code, err := execx.RunShell("runtime:php", 30*time.Minute, installCmd); err != nil {
		return nil, fmt.Errorf("php%s-fpm 安装失败(exit %d): %s", version, code, tailOf(out))
	}
	if _, code, err := execx.Run("runtime", 60*time.Second, "systemctl", "enable", "--now", unit); err != nil {
		return nil, fmt.Errorf("服务启动失败(exit %d): %s", code, unit)
	}
	time.Sleep(2 * time.Second)
	if stateOut, _, _ := execx.Run("runtime", 10*time.Second, "systemctl", "is-active", unit); strings.TrimSpace(stateOut) != "active" {
		return nil, fmt.Errorf("php%s-fpm 未进入 active 状态，请检查 journalctl -u %s", version, unit)
	}
	_, _, _ = execx.Run("runtime", 30*time.Second, "systemctl", "start", "unattended-upgrades")
	if err := ensureComposer(); err != nil {
		slog.Warn("Composer 自动安装失败（不影响 PHP 使用）", "err", err)
	}
	var dup int64
	m.db.Model(&RuntimeInst{}).Where("lang = ? AND version = ?", "php", version).Count(&dup)
	if dup == 0 {
		inst := RuntimeInst{Lang: "php", Version: version, Binary: unit, Pkgs: strings.Join(pkgs, " "), IsDefault: true}
		m.db.Create(&inst)
	}
	return []string{unit + " 服务已启动"}, nil
}

// uninstallPHP 卸载指定分支的 PHP（停服 + 移除包）。
func (m *Module) uninstallPHP(version string) ([]string, error) {
	osID, _, _ := osRelease()
	unit := "php" + version + "-fpm"
	_, _, _ = execx.Run("runtime", 60*time.Second, "systemctl", "disable", "--now", unit)
	switch osID {
	case "ubuntu", "debian":
		_, _, _ = execx.RunShell("runtime:php", 10*time.Minute, "DEBIAN_FRONTEND=noninteractive apt-get purge -y php"+version+"-*")
	case "centos", "rocky", "almalinux", "rhel", "ol", "fedora":
		_, _, _ = execx.RunShell("runtime:php", 10*time.Minute, "dnf remove -y php-*")
	}
	return []string{"php" + version + " 已卸载"}, nil
}

// osRelease 读取 /etc/os-release 关键字段。
func osRelease() (id, versionID, codename string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "ID="):
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		case strings.HasPrefix(line, "VERSION_ID="):
			versionID = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		case strings.HasPrefix(line, "VERSION_CODENAME="):
			codename = strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), "\"")
		}
	}
	return
}

// shellOut 执行 shell 命令并返回 trim 后输出（内部辅助）。
func shellOut(cmd string) string {
	out, _, err := execx.RunShell("runtime", 30*time.Second, cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// tailOf 截取输出末尾（错误提示用）。
func tailOf(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1024 {
		return "..." + s[len(s)-1024:]
	}
	return s
}

// branchesFromPHPListing 从镜像站目录 HTML 提取 PHP 分支列表（如 7.4/8.3），纯函数。
var phpTarRe = regexp.MustCompile(`php-(\d+)\.(\d+)\.\d+\.tar\.gz`)

func branchesFromPHPListing(html string, fallback []string) []string {
	seen := map[string]bool{}
	for _, m := range phpTarRe.FindAllStringSubmatch(html, -1) {
		br := m[1] + "." + m[2]
		seen[br] = true
	}
	if len(seen) == 0 {
		return fallback
	}
	out := []string{}
	for br := range seen {
		out = append(out, br)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := versionNums(out[i]), versionNums(out[j])
		for k := 0; k < 2; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return false
	})
	return out
}

// composerPhar / composerBin 路径。
func composerPhar() string { return "/usr/local/share/composer.phar" }
func composerBin() string  { return "/usr/local/bin/composer" }

// ensureComposer 下载 composer.phar（阿里云优先，官方回退）并创建包装脚本。
func ensureComposer() error {
	if _, err := os.Stat(composerPhar()); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(composerPhar()), 0o755); err != nil {
		return err
	}
	urls := []string{
		"https://mirrors.aliyun.com/composer/composer.phar",
		"https://getcomposer.org/composer-stable.phar",
	}
	var lastErr error
	for _, u := range urls {
		if _, _, err := execx.Run("runtime", 10*time.Minute, "curl", "-fSL", "--retry", "2", "--connect-timeout", "20", "-o", composerPhar(), u); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = fmt.Errorf("下载失败: %s", u)
		}
	}
	if lastErr != nil {
		return lastErr
	}
	wrapper := "#!/bin/bash\nexec /usr/bin/php " + composerPhar() + " \"$@\"\n"
	if err := os.WriteFile(composerBin(), []byte(wrapper), 0o755); err != nil {
		return err
	}
	return nil
}

// getComposer Composer 安装状态与当前 packagist 源。
func (m *Module) getComposer(c *gin.Context) {
	installed := false
	if _, err := os.Stat(composerPhar()); err == nil {
		if _, err2 := os.Stat(composerBin()); err2 == nil {
			installed = true
		}
	}
	url := ""
	if installed {
		home := os.Getenv("HOME")
		for _, cfgPath := range []string{filepath.Join(home, ".config", "composer", "config.json"), filepath.Join(home, ".composer", "config.json")} {
			data, err := os.ReadFile(cfgPath)
			if err != nil {
				continue
			}
			var cfg struct {
				Repositories []struct {
					Packagist any    `json:"packagist.org"`
					URL       string `json:"url"`
				} `json:"repositories"`
			}
			if json.Unmarshal(data, &cfg) != nil {
				continue
			}
			for _, repo := range cfg.Repositories {
				if repo.URL != "" {
					url = repo.URL
					break
				}
			}
			if url != "" {
				break
			}
		}
	}
	core.OK(c, gin.H{"installed": installed, "url": url})
}

// setComposer 设置 Composer packagist 源（url 为空则清除自定义源）。
func (m *Module) setComposer(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if _, err := os.Stat(composerBin()); err != nil {
		core.Err(c, 400, "Composer 未安装，请先安装 PHP（会自动附带）或使用安装按钮")
		return
	}
	if req.URL != "" && !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		core.Err(c, 400, "源地址需以 http(s):// 开头")
		return
	}
	var args []string
	if req.URL == "" {
		args = []string{"config", "-g", "--unset", "repos.packagist"}
	} else {
		args = []string{"config", "-g", "repo.packagist", "composer", req.URL}
	}
	if out, code, err := execx.Run("runtime", 60*time.Second, composerBin(), args...); err != nil {
		core.Err(c, 500, fmt.Sprintf("设置失败(exit %d): %s", code, out))
		return
	}
	slog.Info("Composer 源已设置", "url", req.URL, "by", c.GetString("user"))
	core.OK(c, gin.H{"url": req.URL})
}

// installComposer 为已装 PHP 补装 Composer。
func (m *Module) installComposer(c *gin.Context) {
	if goruntime.GOOS != "linux" {
		core.Err(c, 400, "Composer 安装仅支持在 Linux 服务器上执行")
		return
	}
	if err := ensureComposer(); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if out, code, err := execx.Run("runtime", 60*time.Second, composerBin(), "-V"); err != nil {
		core.Err(c, 500, fmt.Sprintf("Composer 验证失败(exit %d): %s", code, out))
		return
	}
	core.OK(c, gin.H{"bin": composerBin()})
}

// getPecl PECL 源状态（需 php-pear 包提供 pear/pecl 命令）。
func (m *Module) getPecl(c *gin.Context) {
	installed := false
	mirror := ""
	if _, err := os.Stat("/usr/bin/pecl"); err == nil {
		installed = true
		if out, code, err := execx.Run("runtime", 30*time.Second, "pear", "config-get", "preferred_mirror"); err == nil && code == 0 {
			mirror = strings.TrimSpace(out)
		}
	}
	core.OK(c, gin.H{"installed": installed, "mirror": mirror})
}

// setPecl 设置 PECL 镜像（pear config-set preferred_mirror）。
func (m *Module) setPecl(c *gin.Context) {
	var req struct {
		Mirror string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if _, err := os.Stat("/usr/bin/pecl"); err != nil {
		core.Err(c, 400, "未找到 pear/pecl（需先安装 php-pear 包）")
		return
	}
	host := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(req.Mirror), "https://"), "http://")
	host = strings.Trim(host, "/")
	if strings.ContainsAny(host, " /\\") {
		core.Err(c, 400, "镜像地址非法")
		return
	}
	_, _, _ = execx.Run("runtime", 60*time.Second, "pear", "channel-update", "pecl.php.net")
	if out, code, err := execx.Run("runtime", 30*time.Second, "pear", "config-set", "preferred_mirror", host); err != nil {
		core.Err(c, 500, fmt.Sprintf("设置失败(exit %d): %s", code, out))
		return
	}
	slog.Info("PECL 镜像已设置", "mirror", host, "by", c.GetString("user"))
	core.OK(c, gin.H{"mirror": host})
}
