// Package appstore 应用商店：两种部署形态。
//   - compose：Docker Compose 容器化部署（模板生成 compose 文件）
//   - native：Linux 原生二进制部署（下载/解压 + systemd 托管），自动适配 amd64/arm64
package appstore

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

// AppVersion 应用可选版本。
type AppVersion struct {
	Version string `json:"version"`
	Note    string `json:"note,omitempty"`
}

// Param 模板参数定义。
type Param struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Def      string `json:"def"`
	Required bool   `json:"required"`
}

// NativeSpec 原生二进制应用的安装描述。
type NativeSpec struct {
	URLs      map[string]string                  // arch(amd64/arm64) → 下载地址（tar.gz 或裸二进制）
	Archive   bool                               // 是否为 tar.gz 压缩包
	InArchive string                             // 压缩包内二进制相对路径，{arch}=goarch，{vendor_arch}=厂商命名
	Binary    string                             // 目标二进制文件名
	Args      func(p map[string]string) []string // ExecStart 参数
	Env       func(p map[string]string) map[string]string
	Files     func(name string, p map[string]string) map[string]string // 附加配置文件（相对安装目录）
	Dirs      []string                                                 // 需预创建的子目录（如 www/data）

	// Tool=true 表示工具类（如 Node.js/Go 工具链）：解压后软链到 /usr/local/bin，
	// 不创建 systemd 服务；false 为服务类，systemd 托管。
	Tool bool
	// ArchAlias 厂商架构命名映射（如 amd64→x64，Node.js 使用），仅用于 {vendor_arch} 占位符
	ArchAlias map[string]string
	// MirrorPaths 大镜像站收录路径：源ID → 架构 → 相对镜像站根的路径（或完整 URL）
	MirrorPaths map[string]map[string]string
	// VersionMirrorPaths 指定版本的大镜像站路径：源ID → 架构 → 路径模板，支持 {{ver}}。
	VersionMirrorPaths map[string]map[string]string
}

// Template 应用模板。
type Template struct {
	Key    string                                        `json:"key"`
	Name   string                                        `json:"name"`
	Desc   string                                        `json:"desc"`
	Kind   string                                        `json:"kind"` // compose | native | package | script
	Params []Param                                       `json:"params"`
	Gen    func(p map[string]string) (string, error)     // compose 用
	Native *NativeSpec                                   // native 用
	Pkg    func(distro string) ([]string, string, error) // package 用: 返回(包列表, systemd 单元)
	Script string                                        // script 用: bash 内容或 http(s) URL

	// ---- 版本选择支持（nil = 仅默认版本）----
	DefaultVersion   string                                                                                                            // 默认版本（未选时使用）
	VersionList      func() ([]AppVersion, error)                                                                                      // 拉取可选版本清单
	NativeVersionURL func(ver, goarch string) (string, error)                                                                          // native: 按版本返回下载地址
	PkgVersions      func(distro string) ([]AppVersion, error)                                                                         // package: 按发行版返回版本选项
	PkgInstallVer    func(ver, distro, srcID string) (pre []string, pkgs []string, post []string, unit string, note string, err error) // package: 按版本安装（note=实际生效源说明）
}

// AppInst 已安装实例。
type AppInst struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:40" json:"name"`
	AppKey    string    `gorm:"size:32" json:"app_key"`
	Kind      string    `gorm:"size:16" json:"kind"`   // compose | native（历史空值按 compose）
	Tool      bool      `json:"tool"`                  // native 工具类（无 systemd 服务）
	Binary    string    `gorm:"size:64" json:"binary"` // native 二进制名 / package 的 systemd 单元
	Pkgs      string    `gorm:"size:256" json:"pkgs"`  // package 空格分隔的包名（卸载用）
	Params    string    `gorm:"type:text" json:"params"`
	Version   string    `gorm:"size:64" json:"version"` // 安装/升级时选定的版本
	CreatedAt time.Time `json:"created_at"`
}

// ---- 工具 ----

// yamlStr YAML 双引号安全包裹。
func yamlStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// unitQuote systemd 单元文件值转义。
func unitQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func portParam(p map[string]string, def string) (string, error) {
	port := p["port"]
	if port == "" {
		port = def
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("端口非法: %s", port)
	}
	return port, nil
}

func portOf(p map[string]string, key, def string) (string, error) {
	return portParam(map[string]string{"port": p[key]}, def)
}

func required(p map[string]string, key, label string) (string, error) {
	v := p[key]
	if v == "" {
		return "", fmt.Errorf("缺少参数: %s", label)
	}
	return v, nil
}

// ---- 模板库 ----

var templates = []Template{
	{
		Key: "nginx", Name: "Nginx", Desc: "Web 服务器/反向代理（容器）", Kind: "compose",
		Params: []Param{{Key: "port", Label: "宿主机端口", Def: "80", Required: true}},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "80")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  nginx:
    image: nginx:stable
    ports:
      - %q:80
    volumes:
      - ./conf:/etc/nginx/conf.d:ro
      - ./www:/usr/share/nginx/html:ro
    restart: always
`, port), nil
		},
	},
	{
		Key: "redis", Name: "Redis 7", Desc: "内存数据库/缓存（容器）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "6379", Required: true},
			{Key: "password", Label: "访问密码（留空=无密码）", Required: false},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "6379")
			if err != nil {
				return "", err
			}
			cmd := ""
			if pass := p["password"]; pass != "" {
				cmd = fmt.Sprintf("    command: [\"redis-server\", \"--requirepass\", %s]\n", yamlStr(pass))
			}
			return fmt.Sprintf(`services:
  redis:
    image: redis:7-alpine
    ports:
      - %q:6379
%s    volumes:
      - ./data:/data
    restart: always
`, port, cmd), nil
		},
	},
	{
		Key: "wordpress", Name: "WordPress", Desc: "博客/CMS（容器，内置数据库）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "8080", Required: true},
			{Key: "root_pass", Label: "数据库 root 密码", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "8080")
			if err != nil {
				return "", err
			}
			pass, err := required(p, "root_pass", "数据库 root 密码")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: %s
      MYSQL_DATABASE: wordpress
    volumes:
      - db_data:/var/lib/mysql
    restart: always
  app:
    image: wordpress:latest
    ports:
      - %q:80
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: root
      WORDPRESS_DB_PASSWORD: %s
    volumes:
      - wp_data:/var/www/html
    depends_on:
      - db
    restart: always
volumes:
  db_data:
  wp_data:
`, yamlStr(pass), port, yamlStr(pass)), nil
		},
	},
	{
		Key: "postgresql", Name: "PostgreSQL 16", Desc: "关系型数据库（容器）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "5432", Required: true},
			{Key: "postgres_pass", Label: "postgres 密码", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "5432")
			if err != nil {
				return "", err
			}
			pass, err := required(p, "postgres_pass", "postgres 密码")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  postgres:
    image: postgres:16
    ports:
      - %q:5432
    environment:
      POSTGRES_PASSWORD: %s
    volumes:
      - ./data:/var/lib/postgresql/data
    restart: always
`, port, yamlStr(pass)), nil
		},
	},
	{
		Key: "mongodb", Name: "MongoDB 7", Desc: "文档数据库（容器）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "27017", Required: true},
			{Key: "root_user", Label: "root 用户", Def: "admin", Required: true},
			{Key: "root_pass", Label: "root 密码", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "27017")
			if err != nil {
				return "", err
			}
			user, err := required(p, "root_user", "root 用户")
			if err != nil {
				return "", err
			}
			pass, err := required(p, "root_pass", "root 密码")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  mongodb:
    image: mongo:7
    ports:
      - %q:27017
    environment:
      MONGO_INITDB_ROOT_USERNAME: %s
      MONGO_INITDB_ROOT_PASSWORD: %s
    volumes:
      - ./data:/data/db
    restart: always
`, port, yamlStr(user), yamlStr(pass)), nil
		},
	},
	{
		Key: "memcached", Name: "Memcached", Desc: "内存缓存（容器）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "宿主机端口", Def: "11211", Required: true},
			{Key: "memory", Label: "内存上限(MB)", Def: "64", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portParam(p, "11211")
			if err != nil {
				return "", err
			}
			mem, err := portParam(map[string]string{"port": p["memory"]}, "64")
			if err != nil {
				return "", fmt.Errorf("内存上限非法: %s", p["memory"])
			}
			return fmt.Sprintf(`services:
  memcached:
    image: memcached:alpine
    ports:
      - %q:11211
    command: ["-m", "%s"]
    restart: always
`, port, mem), nil
		},
	},
	{
		Key: "rabbitmq", Name: "RabbitMQ", Desc: "消息队列（含管理界面，容器）", Kind: "compose",
		Params: []Param{
			{Key: "amqp_port", Label: "AMQP 端口", Def: "5672", Required: true},
			{Key: "mgmt_port", Label: "管理界面端口", Def: "15672", Required: true},
			{Key: "user", Label: "默认用户", Def: "admin", Required: true},
			{Key: "pass", Label: "默认密码", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			amqp, err := portOf(p, "amqp_port", "5672")
			if err != nil {
				return "", err
			}
			mgmt, err := portOf(p, "mgmt_port", "15672")
			if err != nil {
				return "", err
			}
			user, err := required(p, "user", "默认用户")
			if err != nil {
				return "", err
			}
			pass, err := required(p, "pass", "默认密码")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  rabbitmq:
    image: rabbitmq:3-management
    ports:
      - %q:5672
      - %q:15672
    environment:
      RABBITMQ_DEFAULT_USER: %s
      RABBITMQ_DEFAULT_PASS: %s
    volumes:
      - ./data:/var/lib/rabbitmq
    restart: always
`, amqp, mgmt, yamlStr(user), yamlStr(pass)), nil
		},
	},
	{
		Key: "portainer", Name: "Portainer CE", Desc: "容器可视化管理（容器）", Kind: "compose",
		Params: []Param{{Key: "port", Label: "HTTPS 端口", Def: "9443", Required: true}},
		Gen: func(p map[string]string) (string, error) {
			port, err := portOf(p, "port", "9443")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  portainer:
    image: portainer/portainer-ce:latest
    ports:
      - %q:9443
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/data
    restart: always
`, port), nil
		},
	},
	{
		Key: "uptime-kuma", Name: "Uptime Kuma", Desc: "站点可用性监控（容器）", Kind: "compose",
		Params: []Param{{Key: "port", Label: "访问端口", Def: "3001", Required: true}},
		Gen: func(p map[string]string) (string, error) {
			port, err := portOf(p, "port", "3001")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  uptime-kuma:
    image: louislam/uptime-kuma:latest
    ports:
      - %q:3001
    volumes:
      - ./data:/app/data
    restart: always
`, port), nil
		},
	},
	{
		Key: "phpmyadmin", Name: "phpMyAdmin", Desc: "MySQL Web 管理工具（容器）", Kind: "compose",
		Params: []Param{
			{Key: "port", Label: "访问端口", Def: "8081", Required: true},
			{Key: "pma_host", Label: "MySQL 地址", Def: "host.docker.internal", Required: true},
		},
		Gen: func(p map[string]string) (string, error) {
			port, err := portOf(p, "port", "8081")
			if err != nil {
				return "", err
			}
			host, err := required(p, "pma_host", "MySQL 地址")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  phpmyadmin:
    image: phpmyadmin:latest
    ports:
      - %q:80
    environment:
      PMA_HOST: %s
      PMA_ARBITRARY: "1"
    restart: always
`, port, yamlStr(host)), nil
		},
	},
	{
		Key: "grafana", Name: "Grafana", Desc: "可视化监控面板（容器）", Kind: "compose",
		Params: []Param{{Key: "port", Label: "访问端口", Def: "3000", Required: true}},
		Gen: func(p map[string]string) (string, error) {
			port, err := portOf(p, "port", "3000")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  grafana:
    image: grafana/grafana:latest
    ports:
      - %q:3000
    volumes:
      - ./data:/var/lib/grafana
    restart: always
`, port), nil
		},
	},
	{
		Key: "vaultwarden", Name: "Vaultwarden", Desc: "密码管理器（Bitwarden 兼容，容器）", Kind: "compose",
		Params: []Param{{Key: "port", Label: "访问端口", Def: "8222", Required: true}},
		Gen: func(p map[string]string) (string, error) {
			port, err := portOf(p, "port", "8222")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`services:
  vaultwarden:
    image: vaultwarden/server:latest
    ports:
      - %q:80
    volumes:
      - ./data:/data
    restart: always
`, port), nil
		},
	},
	{
		Key: "nginx-pkg", Name: "Nginx", DefaultVersion: "distro", Desc: "Web 服务器/反向代理（经系统镜像源安装）", Kind: "package",
		Params: []Param{},
		VersionList: func() ([]AppVersion, error) {
			out := []AppVersion{{Version: "distro", Note: "发行版源"}}
			entries := nginxPoolEntries()
			if len(entries) == 0 {
				// 兜底：download 页全量版本（mainline+stable）
				for _, v := range nginxDownloadVersions() {
					out = append(out, AppVersion{Version: v, Note: "nginx.org 官方发布版本"})
				}
				return out, nil
			}
			// minor 奇偶区分主线（奇）/稳定（偶）分支，各分支内最大完整版本为「当前」
			maxStable, maxMain := "", ""
			for _, e := range entries {
				parts := strings.Split(e.Ver, ".")
				if len(parts) < 2 {
					continue
				}
				mi, _ := strconv.Atoi(parts[1])
				if mi%2 == 0 {
					if maxStable == "" || verLess(maxStable, e.Ver) {
						maxStable = e.Ver
					}
				} else if maxMain == "" || verLess(maxMain, e.Ver) {
					maxMain = e.Ver
				}
			}
			for _, e := range entries {
				parts := strings.Split(e.Ver, ".")
				mi := 0
				if len(parts) >= 2 {
					mi, _ = strconv.Atoi(parts[1])
				}
				note := "nginx.org 稳定版"
				if mi%2 == 1 {
					note = "nginx.org 主线版"
				}
				if (mi%2 == 0 && e.Ver == maxStable) || (mi%2 == 1 && e.Ver == maxMain) {
					note = "nginx.org 当前" + strings.TrimPrefix(note, "nginx.org ")
				}
				out = append(out, AppVersion{Version: e.Ver, Note: note})
			}
			return out, nil
		},
		Pkg: func(distro string) ([]string, string, error) {
			return []string{"nginx"}, "nginx", nil
		},
		PkgInstallVer: func(ver, distro, srcID string) (pre []string, pkgs []string, post []string, unit string, note string, err error) {
			if distro != "apt" {
				return nil, []string{"nginx"}, nil, "nginx", "nginx.org 官方源", nil
			}
			if nginxRe.MatchString(ver) {
				if srcID != "" && srcID != "auto" && srcID != "official" {
					return nil, nil, nil, "", "", fmt.Errorf("Nginx 指定版本来自 nginx.org 官方源，请选择官方源")
				}
				entry := nginxPoolCache.byVer[ver]
				if entry == "" {
					return nil, nil, nil, "", "", fmt.Errorf("nginx %s 在 nginx.org 仓库中不存在（当前发行版/架构）", ver)
				}
				branch, fname := "stable", entry
				if i := strings.Index(entry, "/"); i >= 0 {
					branch, fname = entry[:i], entry[i+1:]
				}
				distroPath := "ubuntu"
				if id, _ := osRelease(); id == "debian" {
					distroPath = "debian"
				}
				deb := "/tmp/hanyuxin-nginx-" + ver + ".deb"
				// 下载候选：USTC 镜像优先，官方 nginx.org 回退（curl 逐候选，成功即停）
				urls := nginxPoolBaseCandidates(distroPath, branch)
				dl := "false"
				for _, u := range urls {
					dl += fmt.Sprintf(" || curl -fSL --retry 3 --connect-timeout 30 -o %s %s", deb, u+fname)
				}
				pre = []string{
					dl,
					"systemctl stop nginx 2>/dev/null || true",
					"DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 purge -y nginx nginx-common nginx-core 2>/dev/null || true",
					"DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 install -y " + deb,
				}
				pkgs = []string{"nginx"}
				unit = "nginx"
				post = []string{"rm -f " + deb}
				note = "nginx.org " + ver
				if branch == "mainline" {
					note = "nginx.org " + ver + "（主线，镜像站下载）"
				}
				return pre, pkgs, post, unit, note, nil
			}
			repo := "http://nginx.org/packages"
			if ver == "mainline" {
				repo += "/mainline"
			}
			pre = []string{
				"DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 install -y curl gnupg2 ca-certificates",
				"curl -fsSL https://nginx.org/keys/nginx_signing.key | gpg --dearmor --yes -o /usr/share/keyrings/nginx_signing.key",
				`os=$(. /etc/os-release && echo $ID); codename=$(. /etc/os-release && echo $VERSION_CODENAME); echo "deb [signed-by=/usr/share/keyrings/nginx_signing.key] ` + repo + `/$os $codename nginx" > /etc/apt/sources.list.d/nginx-org.list`,
			}
			pkgs = []string{"nginx"}
			unit = "nginx"
			note = "nginx.org 官方源（各镜像站未收录 nginx.org 仓库）"
			return pre, pkgs, post, unit, note, nil
		},
	},
	{
		Key: "mysql", Name: "MySQL", DefaultVersion: "distro", Desc: "关系型数据库（系统源版本 + 官方仓库主线/LTS，apt 系走镜像自动）", Kind: "package",
		Params: []Param{},
		Pkg: func(distro string) ([]string, string, error) {
			if distro == "apt" {
				// Debian/Ubuntu 各版本的 MySQL 包名不同：按可用性探测（Ubuntu mysql-server；Debian trixie mysql-innovation；无候选则提示改用 MariaDB）
				out, _, err := execx.Run("appstore", 60*time.Second, "sh", "-c",
					"for c in mysql-server mysql-innovation; do if apt-cache show $c >/dev/null 2>&1; then echo $c; break; fi; done")
				cand := strings.TrimSpace(out)
				if err != nil || cand == "" {
					return nil, "", fmt.Errorf("当前发行版的系统源未提供 MySQL 包（Debian 系多仅提供 MariaDB，请使用 MariaDB 应用或选择官方仓库版本）")
				}
				return []string{cand}, "mysql", nil
			}
			return []string{"mysql-server"}, "mysqld", nil
		},
		PkgInstallVer: func(ver, distro, srcID string) (pre []string, pkgs []string, post []string, unit string, note string, err error) {
			if distro != "apt" {
				return nil, nil, nil, "", "", fmt.Errorf("MySQL 官方仓库版本当前仅支持 apt 系发行版；dnf/yum 系请使用发行版源版本（distro）")
			}
			// 版本线 → madison 前缀过滤：innovation（主线 9.x）/ lts-8.4 / lts-8.0；具体版本直传
			prefix := "^9\\."
			switch ver {
			case "lts-8.4":
				prefix = "^8\\.4\\."
			case "lts-8.0":
				prefix = "^8\\.0\\."
			case "innovation":
			default:
				if strings.HasPrefix(ver, "9.") {
					prefix = "^" + strings.ReplaceAll(ver, ".", "\\.")
				} else {
					return nil, nil, nil, "", "", fmt.Errorf("MySQL 官方仓库版本可选：innovation（主线最新）/ lts-8.4 / lts-8.0")
				}
			}
			pre = []string{
				"DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 install -y curl ca-certificates gnupg",
				"curl -fsSL https://repo.mysql.com/RPM-GPG-KEY-mysql-2023 | gpg --dearmor --yes -o /usr/share/keyrings/mysql.gpg",
				`. /etc/os-release && echo "deb [signed-by=/usr/share/keyrings/mysql.gpg] https://repo.mysql.com/apt/debian $VERSION_CODENAME mysql-innovation mysql-8.4 mysql-8.0 mysql-tools" > /etc/apt/sources.list.d/mysql-org.list`,
				"DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 update",
				// 选取对应版本线最新版本，逐包 pin 安装（server/client 同版本）
				fmt.Sprintf(`v=$(apt-cache madison mysql-server | awk '{print $3}' | grep -E '%s' | head -1); [ -n "$v" ] || { echo "repo.mysql.com 上未找到该版本线的 MySQL 包"; exit 1; }; DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 install -y mysql-server=$v mysql-client=$v`, prefix),
			}
			pkgs = []string{"mysql-server"}
			unit = "mysql"
			note = "MySQL 官方仓库（repo.mysql.com，选线 " + ver + "）"
			return pre, pkgs, post, unit, note, nil
		},
		VersionList: func() ([]AppVersion, error) {
			return []AppVersion{
				{Version: "distro", Note: "系统默认（推荐）"},
				{Version: "innovation", Note: "官方仓库 · 主线最新 Innovation（9.x）"},
				{Version: "lts-8.4", Note: "官方仓库 · LTS 8.4"},
				{Version: "lts-8.0", Note: "官方仓库 · LTS 8.0"},
			}, nil
		},
	},
	{
		Key: "mariadb", Name: "MariaDB", DefaultVersion: "distro", Desc: "关系型数据库（系统源或官方仓库多版本，走镜像站）", Kind: "package",
		Params: []Param{},
		VersionList: func() ([]AppVersion, error) {
			out := []AppVersion{}
			for _, v := range mariadbVersions() {
				note := "官方仓库 · 稳定分支（镜像站）"
				if strings.Contains(v, "rolling") {
					note = "官方仓库 · 滚动分支（镜像站）"
				} else if strings.Count(v, ".") >= 2 {
					note = "官方仓库 · 指定小版本（镜像站）"
				}
				out = append(out, AppVersion{Version: v, Note: note})
			}
			return out, nil
		},
		Pkg: func(distro string) ([]string, string, error) {
			return []string{"mariadb-server", "mariadb-client"}, "mariadb", nil
		},
		PkgInstallVer: func(ver, distro, srcID string) (pre []string, pkgs []string, post []string, unit string, note string, err error) {
			if distro != "apt" {
				// dnf：指定版本走阿里云 MariaDB yum 镜像（目录结构与官方 yum.mariadb.org 一致）
				pre = []string{
					"cat > /etc/yum.repos.d/MariaDB.repo <<MARIADB_REPO_EOF\n" +
						"[mariadb]\n" +
						"name = MariaDB\n" +
						"baseurl = https://mirrors.aliyun.com/mariadb/yum/" + ver + "/centos$(rpm -E %{rhel} 2>/dev/null || echo 9)-amd64\n" +
						"gpgkey = https://yum.mariadb.org/RPM-GPG-KEY-MariaDB\n" +
						"gpgcheck = 1\n" +
						"MARIADB_REPO_EOF",
				}
				pkgs = []string{"MariaDB-server", "MariaDB-client"}
				unit = "mariadb"
				note = "MariaDB " + ver + "（阿里云 yum 镜像）"
				return pre, pkgs, post, unit, note, nil
			}
			repo := "https://mirrors.aliyun.com/mariadb/repo/" + ver
			sourceNote := "阿里云镜像"
			switch srcID {
			case "", "auto", "aliyun":
			case "tencent-in":
				repo = "http://mirrors.tencentyun.com/mariadb/repo/" + ver
				sourceNote = "腾讯云内网镜像"
			case "official":
				repo = "https://deb.mariadb.org/" + ver
				sourceNote = "MariaDB 官方源"
			}
			pre = []string{
				"install -d /etc/apt/keyrings",
				"curl -fsSL https://mariadb.org/mariadb_release_signing_key.asc | gpg --dearmor --yes -o /etc/apt/keyrings/mariadb-keyring.gpg",
				`dp=$(. /etc/os-release && echo $ID); cn=$(. /etc/os-release && echo $VERSION_CODENAME); echo "deb [signed-by=/etc/apt/keyrings/mariadb-keyring.gpg] ` + repo + `/$dp $cn main" > /etc/apt/sources.list.d/mariadb-org.list`,
			}
			pkgs = []string{"mariadb-server", "mariadb-client"}
			unit = "mariadb"
			note = "MariaDB " + ver + " → " + sourceNote
			return pre, pkgs, post, unit, note, nil
		},
	},
	{
		Key: "redis-pkg", Name: "Redis", DefaultVersion: "distro", Desc: "内存数据库/缓存（经系统镜像源安装）", Kind: "package",
		Params: []Param{},
		VersionList: func() ([]AppVersion, error) {
			out := []AppVersion{{Version: "distro", Note: "发行版源"}}
			_, order := redisVersions()
			if len(order) == 0 {
				order = redisArchiveVersions()
			}
			for _, v := range order {
				out = append(out, AppVersion{Version: v, Note: "Redis 官方仓库"})
			}
			return out, nil
		},
		Pkg: func(distro string) ([]string, string, error) {
			if distro == "apt" {
				return []string{"redis-server"}, "redis-server", nil
			}
			return []string{"redis"}, "redis", nil
		},
		PkgInstallVer: func(ver, distro, srcID string) (pre []string, pkgs []string, post []string, unit string, note string, err error) {
			if distro != "apt" {
				return nil, []string{"redis"}, nil, "redis", "发行版源（官方仓库仅提供 deb）", nil
			}
			if srcID != "" && srcID != "auto" && srcID != "official" {
				return nil, nil, nil, "", "", fmt.Errorf("Redis 指定版本仅支持 packages.redis.io 官方源，请选择官方源")
			}
			byDisp, _ := redisVersions()
			full := byDisp[ver]
			if full == "" {
				return nil, nil, nil, "", "", fmt.Errorf("Redis %s 在 packages.redis.io 仓库中不存在（当前发行版/架构）", ver)
			}
			pre = []string{
				"curl -fsSL https://packages.redis.io/gpg | gpg --dearmor --yes -o /usr/share/keyrings/redis-archive-keyring.gpg",
				`cn=$(. /etc/os-release && echo $VERSION_CODENAME); echo "deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $cn main" > /etc/apt/sources.list.d/redis-org.list`,
			}
			pkgs = []string{"redis-server=" + full, "redis-tools=" + full}
			unit = "redis-server"
			note = "Redis 官方仓库 " + ver
			return pre, pkgs, post, unit, note, nil
		},
	},
	{
		Key: "postgresql-pkg", Name: "PostgreSQL", DefaultVersion: "distro", Desc: "关系型数据库（经系统镜像源安装）", Kind: "package",
		Params: []Param{},
		VersionList: func() ([]AppVersion, error) {
			return []AppVersion{
				{Version: "distro", Note: "发行版源"},
				{Version: "13", Note: "PGDG"},
				{Version: "14", Note: "PGDG"},
				{Version: "15", Note: "PGDG"},
				{Version: "16", Note: "PGDG"},
				{Version: "17", Note: "PGDG"},
				{Version: "18", Note: "PGDG 最新主线"},
				{Version: "19", Note: "PGDG 开发版（Beta）"},
			}, nil
		},
		Pkg: func(distro string) ([]string, string, error) {
			if distro == "apt" {
				return []string{"postgresql"}, "postgresql", nil
			}
			return []string{"postgresql-server"}, "postgresql", nil
		},
		PkgInstallVer: func(ver, distro, srcID string) (pre []string, pkgs []string, post []string, unit string, note string, err error) {
			if distro == "apt" {
				if srcID != "" && srcID != "auto" && srcID != "official" && srcID != "aliyun" && srcID != "tencent" {
					return nil, nil, nil, "", "", fmt.Errorf("PostgreSQL 指定版本仅支持 PGDG 官方、阿里云或腾讯云源，请更换下载源")
				}
				repoBase := "https://apt.postgresql.org/pub/repos/apt"
				note = "PGDG 官方源"
				if msrc, ok := aptMirrorSource(srcID); ok && msrc.PgdgApt != "" {
					repoBase = msrc.PgdgApt
					note = "PGDG → " + msrc.Name
				}
				pre = []string{
					"DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 install -y curl ca-certificates gnupg",
					"install -d /usr/share/postgresql-common/pgdg && curl -fsSL -o /usr/share/postgresql-common/pgdg/apt-postgresql.org.asc https://www.postgresql.org/media/keys/ACCC4CF8.asc",
					`. /etc/os-release && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt-postgresql.org.asc] ` + repoBase + ` $VERSION_CODENAME-pgdg main" > /etc/apt/sources.list.d/pgdg.list`,
				}
				pkgs = []string{"postgresql-" + ver}
				unit = "postgresql@" + ver + "-main"
				return pre, pkgs, post, unit, note, nil
			}
            // PGDG yum 仓库：阿里云/腾讯云镜像已收录 repos/yum（含 reporpms），默认走镜像
            var yumBase string
            var yumNote string
            switch srcID {
            case "official", "off":
            	yumBase, yumNote = "https://download.postgresql.org/pub/repos/yum", "PGDG 官方源"
            case "", "auto", "aliyun", "tencent":
            	yumBase, yumNote = "https://download.postgresql.org/pub/repos/yum", "PGDG 官方源"
            	if msrc, ok := aptMirrorSource(srcID); ok && msrc.PgdgYum != "" {
            		yumBase = msrc.PgdgYum
            		yumNote = "PGDG → " + msrc.Name
            	}
            }
            pre = []string{
            	"dnf -qy install " + yumBase + "/reporpms/EL-$(rpm -E %{rhel} 2>/dev/null || echo 9)-x86_64/pgdg-redhat-repo-latest.noarch.rpm || yum install -y " + yumBase + "/reporpms/EL-7-x86_64/pgdg-redhat-repo-latest.noarch.rpm",
            	"dnf -qy module disable postgresql 2>/dev/null || true",
            }
			pkgs = []string{"postgresql" + ver + "-server"}
			unit = "postgresql-" + ver
			post = []string{fmt.Sprintf("/usr/pgsql-%s/bin/postgresql-%s-setup initdb || true", ver, ver)}
			note = "PostgreSQL " + ver + " dnf（" + yumNote + "）"
			return pre, pkgs, post, unit, note, nil
		},
	},
	{
		Key: "memcached-pkg", Name: "Memcached", DefaultVersion: "distro", Desc: "内存缓存（经系统镜像源安装）", Kind: "package",
		Params: []Param{},
		VersionList: func() ([]AppVersion, error) {
			return memcachedVersions(), nil
		},
		Pkg: func(distro string) ([]string, string, error) {
			return []string{"memcached"}, "memcached", nil
		},
		PkgInstallVer: func(ver, distro, srcID string) (pre []string, pkgs []string, post []string, unit string, note string, err error) {
			if distro != "apt" {
				return nil, []string{"memcached"}, nil, "memcached", "发行版源（dnf/yum 暂不锁定历史包版本）", nil
			}
			if !aptVersionAvailable("memcached", ver) {
				return nil, nil, nil, "", "", fmt.Errorf("Memcached %s 不在当前 apt 软件包索引中，请先刷新版本列表", ver)
			}
			pkgs = []string{"memcached=" + ver}
			unit = "memcached"
			note = "Memcached " + ver + "（发行版源）"
			if msrc, ok := aptMirrorSource(srcID); ok {
				note += " → " + msrc.Name
			} else if srcID == "official" || srcID == "" || srcID == "auto" {
				note += " → 官方源"
			}
			return nil, pkgs, nil, unit, note, nil
		},
	},
	{
		Key: "caddy", Name: "Caddy", DefaultVersion: "2.8.4", Desc: "Web 服务器，自动 HTTPS（原生，GitHub 全版本可选）", Kind: "native",
		Params: []Param{{Key: "port", Label: "HTTP 端口", Def: "2015", Required: true}},
		VersionList: func() ([]AppVersion, error) {
			return ghVerList("caddyserver/caddy"), nil
		},
		NativeVersionURL: func(ver, goarch string) (string, error) {
			return fmt.Sprintf("https://github.com/caddyserver/caddy/releases/download/v%s/caddy_%s_linux_%s.tar.gz", ver, ver, goarch), nil
		},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://github.com/caddyserver/caddy/releases/download/v2.8.4/caddy_2.8.4_linux_amd64.tar.gz",
				"arm64": "https://github.com/caddyserver/caddy/releases/download/v2.8.4/caddy_2.8.4_linux_arm64.tar.gz",
			},
			Archive: true, InArchive: "caddy", Binary: "caddy",
			Dirs: []string{"www"},
			Args: func(p map[string]string) []string { return []string{"run", "--config", "Caddyfile"} },
			Files: func(name string, p map[string]string) map[string]string {
				port, _ := portOf(p, "port", "2015")
				return map[string]string{
					"Caddyfile": fmt.Sprintf(`{
	admin off
}

:%s {
	root * ./www
	file_server
}
`, port),
				}
			},
		},
	},
	{
		Key: "minio", Name: "MinIO", DefaultVersion: "latest", Desc: "S3 兼容对象存储（原生，RELEASE 版）", Kind: "native",
		Params: []Param{
			{Key: "port", Label: "API 端口", Def: "9000", Required: true},
			{Key: "console_port", Label: "控制台端口", Def: "9001", Required: true},
			{Key: "access_key", Label: "Access Key", Def: "minioadmin", Required: true},
			{Key: "secret_key", Label: "Secret Key", Required: true},
		},
		VersionList: func() ([]AppVersion, error) {
			return minioVersions(), nil
		},
		NativeVersionURL: func(ver, goarch string) (string, error) {
			return minioVersionURL(ver, goarch)
		},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://dl.min.io/server/minio/release/linux-amd64/minio",
				"arm64": "https://dl.min.io/server/minio/release/linux-arm64/minio",
			},
			Archive: false, Binary: "minio",
			MirrorPaths: map[string]map[string]string{
				"minio-cn": {
					"amd64": "server/minio/release/linux-amd64/minio",
					"arm64": "server/minio/release/linux-arm64/minio",
				},
			},
			VersionMirrorPaths: map[string]map[string]string{
				"minio-cn": {
					"amd64": "server/minio/release/linux-amd64/archive/minio.{{ver}}",
					"arm64": "server/minio/release/linux-arm64/archive/minio.{{ver}}",
				},
			},
			Dirs: []string{"data"},
			Env: func(p map[string]string) map[string]string {
				return map[string]string{
					"MINIO_ROOT_USER":     orStr(p, "access_key", "minioadmin"),
					"MINIO_ROOT_PASSWORD": p["secret_key"],
				}
			},
			Args: func(p map[string]string) []string {
				api, _ := portOf(p, "port", "9000")
				console, _ := portOf(p, "console_port", "9001")
				return []string{"server", "./data", "--address", ":" + api, "--console-address", ":" + console}
			},
		},
	},
	{
		Key: "node_exporter", Name: "node_exporter", DefaultVersion: "1.8.2", Desc: "Prometheus 主机指标采集（原生，GitHub 全版本可选）", Kind: "native",
		Params: []Param{{Key: "port", Label: "监听端口", Def: "9100", Required: true}},
		VersionList: func() ([]AppVersion, error) {
			return ghVerList("prometheus/node_exporter"), nil
		},
		NativeVersionURL: func(ver, goarch string) (string, error) {
			return fmt.Sprintf("https://github.com/prometheus/node_exporter/releases/download/v%s/node_exporter-%s.linux-%s.tar.gz", ver, ver, goarch), nil
		},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-amd64.tar.gz",
				"arm64": "https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-arm64.tar.gz",
			},
			Archive: true, InArchive: "node_exporter-{ver}.linux-{arch}/node_exporter", Binary: "node_exporter",
			Args: func(p map[string]string) []string {
				port, _ := portOf(p, "port", "9100")
				return []string{"--web.listen-address", ":" + port}
			},
		},
	},
	{
		Key: "frps", Name: "FRP 服务端", DefaultVersion: "0.61.1", Desc: "内网穿透服务端（原生，GitHub 全版本可选）", Kind: "native",
		VersionList: func() ([]AppVersion, error) {
			return ghVerList("fatedier/frp"), nil
		},
		NativeVersionURL: func(ver, goarch string) (string, error) {
			return fmt.Sprintf("https://github.com/fatedier/frp/releases/download/v%s/frp_%s_linux_%s.tar.gz", ver, ver, goarch), nil
		},
		Params: []Param{
			{Key: "port", Label: "绑定端口", Def: "7000", Required: true},
			{Key: "token", Label: "认证 Token", Required: true},
		},
		Native: &NativeSpec{
			URLs: map[string]string{
				"amd64": "https://github.com/fatedier/frp/releases/download/v0.61.1/frp_0.61.1_linux_amd64.tar.gz",
				"arm64": "https://github.com/fatedier/frp/releases/download/v0.61.1/frp_0.61.1_linux_arm64.tar.gz",
			},
			Archive: true, InArchive: "frp_{ver}_linux_{arch}/frps", Binary: "frps",
			Args: func(p map[string]string) []string { return []string{"-c", "frps.toml"} },
			Files: func(name string, p map[string]string) map[string]string {
				port, _ := portOf(p, "port", "7000")
				return map[string]string{
					"frps.toml": fmt.Sprintf("bindAddr = \"0.0.0.0\"\nbindPort = %s\nauth.token = %s\n", port, yamlStr(p["token"])),
				}
			},
		},
	},
}

func orStr(p map[string]string, key, def string) string {
	if p[key] == "" {
		return def
	}
	return p[key]
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
	if err := m.db.AutoMigrate(&AppInst{}); err != nil {
		return err
	}
	m.EnsureDefaultSource() // 默认脚本仓库（jiaoben 分支）清单
	// 后台预取版本缓存（nginx pool 海外拉取较慢；启动预热让安装对话框秒开）
	go func() {
		time.Sleep(3 * time.Second)
		start := time.Now()
		_ = nginxPoolEntries()
		_ = mariadbVersions()
		_, _ = redisVersions()
		slog.Info("软件商店版本缓存预热完成", "took", time.Since(start).Round(time.Millisecond))
	}()
	return nil
}

func (m *Module) Register(api gin.IRouter, admin gin.IRouter) {
	g := api.Group("/apps")
	g.GET("/templates", m.listTemplates)
	g.GET("/versions", m.listVersions)
	g.GET("/mirrors", listMirrors)
	g.GET("/soft-sources", listSoftSources)
	g.GET("/docker/status", dockerStatus)
	g.POST("/docker/install", installDocker)
	g.GET("/docker/containers", m.dockerContainers)
	g.POST("/docker/containers/:name/:action", m.dockerContainerAction)
	g.GET("/docker/containers/:name/logs", m.dockerContainerLogs)
	g.GET("/docker/images", m.dockerImages)
	g.POST("/docker/images/prune", m.dockerImagesPrune)
	g.GET("/docker/compose", m.dockerComposeGet)
	g.POST("/docker/compose", m.dockerComposeSave)
	g.POST("/docker/compose-deploy", m.dockerComposeDeploy)
	g.GET("/docker/stats", dockerStats)
	g.GET("/docker/mirrors", m.getMirrors)
	g.POST("/docker/mirrors", m.saveMirrors)
	g.GET("/docker/overview", dockerOverview)
	g.GET("/docker/sites", dockerSites)
	g.GET("/docker/networks", dockerNetworks)
	g.POST("/docker/networks", dockerNetworkCreate)
	g.DELETE("/docker/networks/:name", dockerNetworkRemove)
	g.GET("/docker/volumes", dockerVolumes)
	g.DELETE("/docker/volumes/:name", dockerVolumeRemove)
	g.GET("/docker/search", dockerSearch)
	g.POST("/docker/pull", dockerPull)
	g.POST("/docker/login", dockerLogin)
	g.GET("", m.list)
	g.POST("", m.install)
	g.POST("/:id/start", m.start)
	g.POST("/:id/stop", m.stop)
	g.POST("/:id/restart", m.restart)
	g.GET("/:id/logs", m.logs)
	m.registerScriptRoutes(g)
	g.POST("/:id/uninstall", m.uninstall)
	g.POST("/:id/upgrade", m.upgrade)
	g.GET("/sys-source", sysSourceGet)
	g.POST("/sys-source", sysSourceSet)
	g.POST("/sys-source/restore", sysSourceRestore)
	m.registerSourceRoutes(g, admin)
}

// ---- Docker 镜像源（daemon.json）管理 ----

const daemonJSONPath = "/etc/docker/daemon.json"

// readDaemonJSON 读取 daemon.json；不存在返回空 map 与 exists=false，其余键保留不动。
func readDaemonJSON() (map[string]any, bool, string, error) {
	data, err := os.ReadFile(daemonJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, false, "", nil
		}
		return nil, false, "", err
	}
	raw := string(data)
	if len(strings.TrimSpace(raw)) == 0 {
		return map[string]any{}, true, raw, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, true, raw, fmt.Errorf("daemon.json 解析失败: %w", err)
	}
	return m, true, raw, nil
}

func strListOf(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// liveMirrors 从 docker info 回读当前实际生效的镜像源。
func liveMirrors() []string {
	out, _, err := execx.Run("appstore", 25*time.Second, "docker", "info")
	if err != nil {
		return nil
	}
	var res []string
	in := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Registry Mirrors:") {
			in = true
			continue
		}
		if in {
			t := strings.TrimSpace(line)
			if t == "" || (!strings.HasPrefix(t, "https://") && !strings.HasPrefix(t, "http://")) {
				break
			}
			res = append(res, t)
		}
	}
	return res
}

func (m *Module) getMirrors(c *gin.Context) {
	mm, exists, raw, err := readDaemonJSON()
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	dockerInstalled := false
	if v, _, e := execx.Run("appstore", 20*time.Second, "docker", "--version"); e == nil && strings.Contains(v, "docker") {
		dockerInstalled = true
	}
	core.OK(c, gin.H{
		"path":                daemonJSONPath,
		"exists":              exists,
		"raw":                 raw,
		"registry_mirrors":    strListOf(mm, "registry-mirrors"),
		"insecure_registries": strListOf(mm, "insecure-registries"),
		"live_mirrors":        liveMirrors(),
		"docker_installed":    dockerInstalled,
	})
}

func (m *Module) saveMirrors(c *gin.Context) {
	if runtime.GOOS != "linux" {
		core.Err(c, 400, "Docker 配置仅支持在 Linux 服务器上修改")
		return
	}
	var req struct {
		RegistryMirrors    []string `json:"registry_mirrors"`
		InsecureRegistries []string `json:"insecure_registries"`
		Restart            bool     `json:"restart"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	clean := func(list []string, needURL bool) ([]string, error) {
		seen := map[string]bool{}
		out := []string{}
		for _, s := range list {
			s = strings.TrimSpace(strings.TrimRight(s, ","))
			if s == "" {
				continue
			}
			if needURL && !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
				return nil, fmt.Errorf("镜像源需以 http(s):// 开头: %s", s)
			}
			if strings.ContainsAny(s, " \t\n\"'") {
				return nil, fmt.Errorf("配置项含非法字符: %s", s)
			}
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		return out, nil
	}
	mirrors, err := clean(req.RegistryMirrors, true)
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	insecure, err := clean(req.InsecureRegistries, false)
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}

	mm, exists, _, err := readDaemonJSON()
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	// 合并写入：清空则删除键，其余键保持不动
	if len(mirrors) > 0 {
		mm["registry-mirrors"] = mirrors
	} else {
		delete(mm, "registry-mirrors")
	}
	if len(insecure) > 0 {
		mm["insecure-registries"] = insecure
	} else {
		delete(mm, "insecure-registries")
	}
	data, err := json.MarshalIndent(mm, "", "  ")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}

	// 备份原文件
	if exists {
		if old, rerr := os.ReadFile(daemonJSONPath); rerr == nil {
			_ = os.WriteFile(daemonJSONPath+".hanyuxin.bak", old, 0o644)
		}
	}
	if err := os.MkdirAll(filepath.Dir(daemonJSONPath), 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.WriteFile(daemonJSONPath, data, 0o644); err != nil {
		core.Err(c, 500, "写入 daemon.json 失败: "+err.Error())
		return
	}

	restarted := false
	if req.Restart {
		if out, code, rerr := execx.Run("appstore", 120*time.Second, "systemctl", "restart", "docker"); rerr != nil {
			// 回滚配置并再次重启，保持 Docker 可用
			if exists {
				if bak, berr := os.ReadFile(daemonJSONPath + ".hanyuxin.bak"); berr == nil {
					_ = os.WriteFile(daemonJSONPath, bak, 0o644)
					_, _, _ = execx.Run("appstore", 120*time.Second, "systemctl", "restart", "docker")
				}
			}
			core.Err(c, 500, fmt.Sprintf("重启 Docker 失败(exit %d)，已回滚配置: %s", code, tailOf(out)))
			return
		}
		restarted = true
	}
	slog.Info("Docker 镜像源配置已更新", "mirrors", len(mirrors), "insecure", len(insecure), "restarted", restarted)
	core.OK(c, gin.H{
		"registry_mirrors":    mirrors,
		"insecure_registries": insecure,
		"live_mirrors":        liveMirrors(),
		"restarted":           restarted,
	})
}

// 目录

func (m *Module) dir(name string) string { return filepath.Join(m.dataDir, "apps", name) }

func (m *Module) nativeDir(name string) string {
	return filepath.Join(m.dataDir, "apps-native", name)
}

func (m *Module) composeFile(name string) string {
	return filepath.Join(m.dir(name), "docker-compose.yml")
}

func (m *Module) unitName(name string) string { return "hanyuxin-app-" + name }

func composeArgs(file string, args ...string) []string {
	return append([]string{"compose", "-f", file}, args...)
}

// ---- Docker 环境 ----

func dockerStatus(c *gin.Context) {
	out, _, err := execx.Run("appstore", 20*time.Second, "docker", "--version")
	installed := err == nil
	composeOK := false
	if installed {
		if _, _, err2 := execx.Run("appstore", 20*time.Second, "docker", "compose", "version"); err2 == nil {
			composeOK = true
		}
	}
	core.OK(c, gin.H{"installed": installed, "version": strings.TrimSpace(out), "compose": composeOK})
}

func installDocker(c *gin.Context) {
	// 官方脚本优先，被墙时回退阿里云源（deb 系）；RHEL 系直接用阿里云源
	script := `set -eo pipefail; ` +
		`(curl -fsSL --max-time 60 https://get.docker.com | sh) || ( ` +
		`apt-get update -qq; ` +
		`DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ca-certificates curl gnupg; ` +
		`install -m 0755 -d /etc/apt/keyrings; ` +
		`curl -fsSL https://mirrors.aliyun.com/docker-ce/linux/debian/gpg -o /etc/apt/keyrings/docker.asc; ` +
		`chmod a+r /etc/apt/keyrings/docker.asc; ` +
		`echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://mirrors.aliyun.com/docker-ce/linux/debian $(. /etc/os-release && echo $VERSION_CODENAME) stable" > /etc/apt/sources.list.d/docker.list; ` +
		`apt-get update -qq; ` +
		`DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin ); ` +
		`systemctl enable --now docker; ` +
		`docker compose version`
	out, code, err := execx.RunShell("appstore:install-docker", 30*time.Minute, script)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("Docker 安装失败(exit %d): %s", code, tailOut(out)))
		return
	}
	core.OK(c, nil)
}


// ---- 路由处理 ----

// listVersions 拉取指定模板的可选版本清单。
func (m *Module) listVersions(c *gin.Context) {
	key := c.Query("key")
	tpl := m.findTemplate(key)
	if tpl == nil {
		core.Err(c, 404, "模板不存在")
		return
	}
	// 系统包：版本 = distro（系统默认）+ 系统镜像源版本（pin 安装）+ 官方仓库版本（镜像站定向安装），按 Version 去重
	if tpl.Kind == "package" {
		out := []AppVersion{{Version: "distro", Note: "系统默认（推荐）"}}
		seen := map[string]bool{"distro": true}
		if pkgs, _, perr := tpl.Pkg(detectPkgManager()); perr == nil && len(pkgs) > 0 {
			for _, v := range systemPkgVersions(pkgs) {
				if seen[v.Version] {
					continue
				}
				seen[v.Version] = true
				out = append(out, v)
			}
		}
		if tpl.VersionList != nil {
			if vers, verr := tpl.VersionList(); verr == nil {
				for _, v := range vers {
					if seen[v.Version] {
						continue
					}
					seen[v.Version] = true
					out = append(out, v)
				}
			}
		}
		core.OK(c, gin.H{"versions": out, "default": "distro"})
		return
	}
	def := tpl.DefaultVersion
	if tpl.VersionList == nil {
		core.OK(c, gin.H{"versions": []AppVersion{{Version: def, Note: "最新/默认版本"}}, "default": def})
		return
	}
	vers, err := tpl.VersionList()
	if err != nil {
		core.Err(c, 500, "获取版本清单失败: "+err.Error())
		return
	}
	core.OK(c, gin.H{"versions": vers, "default": def})
}

func (m *Module) listTemplates(c *gin.Context) {
	// 模板的 Gen/Native 为函数字段，JSON 不支持，输出 DTO；
	// 合并顺序：内置 → 仓库源（多源按序去重）→ 本地三方脚本包（custom: 前缀）
	out := make([]gin.H, 0, len(templates))
	for _, t := range templates {
		item := gin.H{"key": t.Key, "name": t.Name, "desc": t.Desc, "kind": t.Kind, "params": t.Params, "source": "builtin", "default_version": t.DefaultVersion}
		if t.Kind == "package" {
			item["soft_sources"] = supportedPkgSources()
		} else if t.Kind == "native" && t.Native != nil {
			item["soft_sources"] = supportedSoftSources(t.Native.URLs, t.Native.MirrorPaths, t.Native.VersionMirrorPaths)
			item["tool"] = t.Native.Tool
		}
		out = append(out, item)
	}
	seen := map[string]bool{}
	for _, t := range templates {
		seen[t.Key] = true
	}
	for _, d := range m.remoteCatalog() {
		if seen[d.Key] {
			continue
		}
		seen[d.Key] = true
		item := gin.H{"key": d.Key, "name": d.Name, "desc": d.Desc, "kind": d.Kind, "params": d.Params, "source": "remote", "source_name": d.SourceName, "version": d.Version, "category": d.Category, "icon": d.Icon, "default_version": d.Version}
		if d.Kind == "package" {
			item["soft_sources"] = supportedPkgSources()
		} else if d.Kind == "native" && d.Native != nil {
			item["soft_sources"] = supportedSoftSources(d.Native.URLs, d.Native.MirrorPaths, d.Native.VersionMirrorPaths)
			item["tool"] = d.Native.Tool
		}
		out = append(out, item)
	}
	// 本地三方脚本包（custom:<name>），与仓库/内置 key 去重（custom: 前缀天然不撞）
	for _, s := range m.customScriptList() {
		out = append(out, s)
	}
	core.OK(c, out)
}

// customScriptList 将本地脚本包转为商店目录条目（source=local，key=custom:<name>）。
func (m *Module) customScriptList() []gin.H {
	dir := m.scriptsDir()
	out := []gin.H{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
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
		t := m.customTemplate(name)
		if t == nil {
			continue
		}
		pkgDir := m.scriptPkgDir(name)
		meta := readScriptMeta(pkgDir, name)
		out = append(out, gin.H{
			"key": t.Key, "name": t.Name, "desc": t.Desc, "kind": "script", "params": t.Params,
			"source": "local", "source_name": "本地脚本", "version": meta.Version, "category": meta.Category, "icon": meta.Icon,
		})
	}
	return out
}

// aptInstallMu 系统包安装全程互斥：并发安装会竞争 apt/dpkg 锁导致 update 失败。
var aptInstallMu sync.Mutex

// supportedPkgSources 系统包应用可用软件源：官方/自动 + 收录了 apt 仓库的镜像站。
func supportedPkgSources() []string {
	ids := []string{"auto", "official"}
	for _, s := range softwareSources {
		if s.AptUbuntu != "" {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// aptMirrorInstallShell 构造 apt 镜像定向安装命令：临时 sources.list 指向镜像站，
// 仅本次安装生效（-o Dir::Etc 覆盖），完成后删除，不改动系统全局源。
// Ubuntu 与 Debian 的仓库基址与安全源布局不同，按 /etc/os-release 区分；
// 镜像未收录 Debian 源时回退官方 Debian 源（安装仍可成功）。
func aptMirrorInstallShell(msrc SoftwareSource, pkgs []string) string {
	return "systemctl stop unattended-upgrades || true; " +
		"os=$(. /etc/os-release && echo $ID); cn=$(. /etc/os-release && echo $VERSION_CODENAME); " +
		"comps=\"main universe restricted multiverse\"; [ \"$os\" = debian ] && comps=\"main contrib non-free non-free-firmware\"; " +
		"if [ \"$os\" = debian ]; then m=\"" + msrc.AptDebian + "\"; [ -z \"$m\" ] && m=\"http://deb.debian.org/debian\"; sec=\"$m-security $cn-security\"; " +
		"else m=\"" + msrc.AptUbuntu + "\"; [ -z \"$m\" ] && m=\"http://archive.ubuntu.com/ubuntu\"; sec=\"$m $cn-security\"; fi; " +
		"{ echo \"deb $m $cn $comps\"; echo \"deb $m $cn-updates $comps\"; echo \"deb $sec $comps\"; } > /etc/apt/sources.list.d/hanyuxin-mirror.list; " +
		"DEBIAN_FRONTEND=noninteractive apt-get -o Dir::Etc::sourcelist=sources.list.d/hanyuxin-mirror.list -o Dir::Etc::sourceparts=- -o APT::Get::List-Cleanup=0 -o DPkg::Lock::Timeout=600 update && " +
		"DEBIAN_FRONTEND=noninteractive apt-get -o Dir::Etc::sourcelist=sources.list.d/hanyuxin-mirror.list -o Dir::Etc::sourceparts=- -o DPkg::Lock::Timeout=600 install -y " +
		strings.Join(pkgs, " ") + "; rc=$?; rm -f /etc/apt/sources.list.d/hanyuxin-mirror.list; " +
		"systemctl start unattended-upgrades || true; exit $rc"
}

// supportedSoftSources 计算原生应用可用的软件源 ID（官方/自动恒可用，其余按发布渠道与镜像站收录判断）。
func supportedSoftSources(urls map[string]string, mirrorPaths, versionMirrorPaths map[string]map[string]string) []string {
	ids := []string{"auto", "official"}
	isGitHub := false
	for _, u := range urls {
		if strings.Contains(u, "github.com/") {
			isGitHub = true
			break
		}
	}
	for _, s := range softwareSources {
		if s.GitHub != "" {
			if isGitHub {
				ids = append(ids, s.ID)
			}
			continue
		}
		if s.Base != "" && (mirrorPaths[s.ID] != nil || versionMirrorPaths[s.ID] != nil) {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func (m *Module) list(c *gin.Context) {
	var rows []AppInst
	m.db.Order("id").Find(&rows)
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		status := m.statusText(&r)
		items = append(items, gin.H{"app": r, "status": status})
	}
	core.OK(c, items)
}

func (m *Module) statusText(r *AppInst) string {
	if r.Kind == "package" {
		out, _, err := execx.Run("appstore", 10*time.Second, "systemctl", "is-active", r.Binary)
		state := strings.TrimSpace(out)
		if err != nil || state == "" {
			return "未知（systemd 不可用）"
		}
		return state
	}
	if r.Kind == "native" && r.Tool {
		return "已安装（工具类）"
	}
	if r.Kind == "native" {
		out, _, err := execx.Run("appstore", 10*time.Second, "systemctl", "is-active", m.unitName(r.Name))
		state := strings.TrimSpace(out)
		if err != nil || state == "" {
			return "未知（systemd 不可用）"
		}
		enableOut, _, _ := execx.Run("appstore", 10*time.Second, "systemctl", "is-enabled", m.unitName(r.Name))
		return fmt.Sprintf("%s (%s)", state, strings.TrimSpace(enableOut))
	}
	out, _, err := execx.Run("appstore", 15*time.Second,
		"docker", composeArgs(m.composeFile(r.Name), "ps")...)
	if err != nil {
		return "未知（docker 不可用或未运行）"
	}
	status := strings.TrimSpace(out)
	if status == "" {
		status = "无运行容器"
	}
	return status
}

func (m *Module) install(c *gin.Context) {
	var req struct {
		AppKey  string            `json:"app_key" binding:"required"`
		Name    string            `json:"name" binding:"required"`
		Version string            `json:"version"`
		Mirror  string            `json:"mirror"`
		Params  map[string]string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !nameRe.MatchString(req.Name) {
		core.Err(c, 400, "实例名非法（小写字母数字短横线，3-40 位）")
		return
	}
	tpl := m.findTemplate(req.AppKey)
	if tpl == nil {
		core.Err(c, 404, "模板不存在")
		return
	}
	var count int64
	m.db.Model(&AppInst{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		core.Err(c, 400, "同名实例已存在")
		return
	}
	p := orParams(req.Params)

	var status string
	row := AppInst{Name: req.Name, AppKey: req.AppKey, Kind: tpl.Kind}
	if tpl.Kind == "script" {
		s, err := m.installScript(tpl, req.Name, p)
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		status = s
	} else if tpl.Kind == "package" {
		// 系统包统一走系统镜像源（apt/dnf 当前源）；版本可从系统源内选择 pin 安装（srcID 不再参与）
		s, pkgs, unit, err := m.installPackage(tpl, req.Name, req.Version, "auto", p)
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		status = s
		row.Binary = unit
		row.Pkgs = strings.Join(pkgs, " ")
	} else if tpl.Kind == "native" {
		s, err := m.installNative(tpl, req.Name, req.Version, p, req.Mirror)
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		status = s
		if tpl.Native != nil && tpl.Native.Tool {
			row.Tool = true
			row.Binary = tpl.Native.Binary
		}
	} else {
		mirror, mirrorNote := resolveMirror(req.Mirror, "compose")
		s, err := m.installCompose(tpl, req.Name, p, mirror)
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		status = s + "，镜像源: " + mirrorNote
	}
	if data, jerr := json.Marshal(req.Params); jerr == nil {
		row.Params = string(data)
	}
	row.Version = req.Version
	if err := m.db.Create(&row).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("应用已部署", "app", req.AppKey, "name", req.Name, "kind", tpl.Kind)
	core.OK(c, gin.H{"app": row, "status": status})
}

// ---- compose 安装 ----

func (m *Module) installCompose(tpl *Template, name string, p map[string]string, mirror MirrorDef) (string, error) {
	content, err := tpl.Gen(p)
	if err != nil {
		return "", err
	}
	if mirror.Docker != "" {
		content = applyDockerMirror(content, mirror.Docker)
	}
	dir := m.dir(name)
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, "www"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(m.composeFile(name), []byte(content), 0o644); err != nil {
		return "", err
	}
	out, code, err := execx.Run("appstore", 15*time.Minute,
		"docker", composeArgs(m.composeFile(name), "up", "-d")...)
	if err != nil {
		_ = os.RemoveAll(dir)
		msg := tailOf(out)
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("部署失败(exit %d): %s", code, msg)
	}
	return "容器已启动", nil
}

// ---- native 安装 ----

func (m *Module) installNative(tpl *Template, name, version string, p map[string]string, srcID string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("原生应用部署仅支持在 Linux 服务器上执行")
	}
	spec := tpl.Native
	arch := runtime.GOARCH
	var rawURL, sourceNote string
	if version != "" && tpl.NativeVersionURL != nil && (version != tpl.DefaultVersion || tpl.Key == "minio") {
		if !versionTokenRe.MatchString(version) {
			return "", fmt.Errorf("版本号非法: %s", version)
		}
		u, uerr := tpl.NativeVersionURL(version, arch)
		if uerr != nil {
			return "", uerr
		}
		versionParams := cloneParams(p)
		versionParams["ver"] = version
		paths := spec.MirrorPaths
		if version != "latest" && len(spec.VersionMirrorPaths) > 0 {
			paths = spec.VersionMirrorPaths
		}
		var rerr error
		rawURL, sourceNote, rerr = resolveNativeURL(map[string]string{arch: u}, paths, arch, srcID, versionParams)
		if rerr != nil {
			return "", rerr
		}
	} else {
		var rerr error
		rawURL, sourceNote, rerr = resolveNativeURL(spec.URLs, spec.MirrorPaths, arch, srcID, p)
		if rerr != nil {
			return "", rerr
		}
	}
	dir, _ := filepath.Abs(m.nativeDir(name))
	_ = os.MkdirAll(dir, 0o755)
	fail := func(err error) error {
		_ = os.RemoveAll(dir)
		return err
	}

	// 1. 下载
	local := filepath.Join(dir, "download.tmp")
	if _, code, err := execx.Run("appstore", 15*time.Minute, "curl", "-fSL", "--retry", "3", "--connect-timeout", "30", "-o", local, rawURL); err != nil {
		return "", fail(fmt.Errorf("下载失败(exit %d): %s", code, rawURL))
	}

	// 2. 解压或直接使用（{arch}=goarch，{vendor_arch}=厂商命名，如 Node.js 的 x64）
	alias := arch
	if spec.ArchAlias != nil {
		if a := spec.ArchAlias[arch]; a != "" {
			alias = a
		}
	}
	var dst string
	if spec.Archive {
		if err := archive.UnTarGz(local, dir); err != nil {
			return "", fail(fmt.Errorf("解压失败: %w", err))
		}
		_ = os.Remove(local)
		ver := version
		if ver == "" {
			ver = tpl.DefaultVersion
		}
		inArchive := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(spec.InArchive, "{arch}", arch), "{vendor_arch}", alias), "{ver}", ver)
		src := filepath.Join(dir, inArchive)
		if _, err := os.Stat(src); err != nil {
			return "", fail(fmt.Errorf("压缩包内未找到 %s", inArchive))
		}
		dst = filepath.Join(dir, spec.Binary)
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			return "", fail(err)
		}
	} else {
		dst = filepath.Join(dir, spec.Binary)
		if err := os.Rename(local, dst); err != nil {
			return "", fail(err)
		}
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", fail(err)
	}

	// 3. 附加目录与配置文件
	for _, d := range spec.Dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return "", fail(err)
		}
	}
	if spec.Files != nil {
		for rel, content := range spec.Files(name, p) {
			if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
				return "", fail(err)
			}
		}
	}

	// 3.5 工具类：软链到 /usr/local/bin 即完成，不创建 systemd 服务
	if spec.Tool {
		link := filepath.Join("/usr/local/bin", spec.Binary)
		_ = os.Remove(link)
		if err := os.Symlink(dst, link); err != nil {
			return "", fail(fmt.Errorf("创建软链失败: %w", err))
		}
		return fmt.Sprintf("工具已安装: %s → /usr/local/bin/%s（下载源: %s）", dst, spec.Binary, sourceNote), nil
	}

	// 4. systemd 单元
	unit := buildUnit(tpl, name, dir, dst, spec, p)
	unitPath := filepath.Join("/etc/systemd/system", m.unitName(name)+".service")
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return "", fail(fmt.Errorf("写入 systemd 单元失败: %w", err))
	}
	if _, _, err := execx.Run("appstore", 30*time.Second, "systemctl", "daemon-reload"); err != nil {
		return "", fail(fmt.Errorf("daemon-reload 失败: %w", err))
	}

	// 5. 启动
	if out, code, err := execx.Run("appstore", 60*time.Second, "systemctl", "enable", "--now", m.unitName(name)); err != nil {
		return "", fail(fmt.Errorf("服务启动失败(exit %d): %s", code, tailOf(out)))
	}
	time.Sleep(2 * time.Second)
	stateOut, _, _ := execx.Run("appstore", 10*time.Second, "systemctl", "is-active", m.unitName(name))
	state := strings.TrimSpace(stateOut)
	if state != "active" {
		logs, _, _ := execx.Run("appstore", 15*time.Second, "journalctl", "-u", m.unitName(name), "-n", "30", "--no-pager")
		return "", fail(fmt.Errorf("服务未进入 active 状态（当前 %s），最近日志:\n%s", state, tailOf(logs)))
	}
	return fmt.Sprintf("服务已启动（%s，目录 %s，下载源: %s）", m.unitName(name), dir, sourceNote), nil
}

func buildUnit(tpl *Template, name, dir, binary string, spec *NativeSpec, p map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=寒语馨面板应用:  %s (%s)\nAfter=network.target\n\n[Service]\nType=simple\n", tpl.Name, name)
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", dir)
	fmt.Fprintf(&b, "ExecStart=%s", binary)
	if spec.Args != nil {
		for _, a := range spec.Args(p) {
			fmt.Fprintf(&b, " \"%s\"", unitQuote(a))
		}
	}
	b.WriteString("\n")
	if spec.Env != nil {
		for k, v := range spec.Env(p) {
			fmt.Fprintf(&b, "Environment=\"%s=%s\"\n", k, unitQuote(v))
		}
	}
	b.WriteString("Restart=always\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n")
	return b.String()
}

// ---- 运维操作（按 kind 分发）----

func (m *Module) loadInst(c *gin.Context) (*AppInst, bool) {
	id, _ := strconv.Atoi(c.Param("id"))
	var r AppInst
	if err := m.db.First(&r, id).Error; err != nil {
		core.Err(c, 404, "实例不存在")
		return nil, false
	}
	return &r, true
}

func (m *Module) start(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var code int
	var err error
	if r.Kind == "package" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "start", r.Binary)
	} else if r.Kind == "native" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "start", m.unitName(r.Name))
	} else {
		out, code, err = execx.Run("appstore", 15*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "up", "-d")...)
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("启动失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) stop(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var code int
	var err error
	if r.Kind == "package" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "stop", r.Binary)
	} else if r.Kind == "native" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "stop", m.unitName(r.Name))
	} else {
		out, code, err = execx.Run("appstore", 5*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "stop")...)
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("停止失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) restart(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var code int
	var err error
	if r.Kind == "package" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "restart", r.Binary)
	} else if r.Kind == "native" {
		out, code, err = execx.Run("appstore", 60*time.Second, "systemctl", "restart", m.unitName(r.Name))
	} else {
		out, code, err = execx.Run("appstore", 5*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "restart")...)
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("重启失败(exit %d): %s", code, tailOf(out)))
		return
	}
	core.OK(c, nil)
}

func (m *Module) logs(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var out string
	var err error
	if r.Kind == "package" {
		out, _, err = execx.Run("appstore", 30*time.Second, "journalctl", "-u", r.Binary, "-n", "200", "--no-pager")
	} else if r.Kind == "native" {
		out, _, err = execx.Run("appstore", 30*time.Second, "journalctl", "-u", m.unitName(r.Name), "-n", "200", "--no-pager")
	} else if r.Kind == "script" {
		// 脚本类：安装/运行日志落盘在 scriptsDir/<实例名>.sh.log
		data, rerr := os.ReadFile(filepath.Join(m.scriptsDir(), r.Name+".sh.log"))
		if rerr != nil {
			core.OK(c, gin.H{"content": ""})
			return
		}
		core.OK(c, gin.H{"content": string(data)})
		return
	} else {
		out, _, err = execx.Run("appstore", 30*time.Second, "docker", composeArgs(m.composeFile(r.Name), "logs", "--tail=200")...)
	}
	if err != nil && out == "" {
		core.Err(c, 404, "日志不可用")
		return
	}
	core.OK(c, gin.H{"content": out})
}

// upgrade 已装应用升级到指定版本：native 按版本重装（覆盖二进制后重启服务）；
// package 经仓库安装指定版本；compose 应用提示用编排编辑改镜像版本。
func (m *Module) upgrade(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var row AppInst
	if err := m.db.First(&row, id).Error; err != nil {
		core.Err(c, 404, "实例不存在")
		return
	}
	var req struct {
		Version string `json:"version"`
		Mirror  string `json:"mirror"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if req.Version == "" {
		core.Err(c, 400, "请选择目标版本")
		return
	}
	tpl := m.findTemplate(row.AppKey)
	if tpl == nil {
		core.Err(c, 404, "模板不存在（远程源模板可能已下架）")
		return
	}
	p := map[string]string{}
	_ = json.Unmarshal([]byte(row.Params), &p)
	if row.Kind == "package" {
		if _, _, _, err := m.installPackage(tpl, row.Name, req.Version, req.Mirror, p); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
	} else if row.Kind == "native" {
		if _, err := m.installNative(tpl, row.Name, req.Version, p, req.Mirror); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		if !row.Tool {
			_, _, _ = execx.Run("appstore", 30*time.Second, "systemctl", "restart", m.unitName(row.Name))
		}
	} else {
		core.Err(c, 400, "Compose 应用请用编排编辑修改镜像版本后应用")
		return
	}
	m.db.Model(&AppInst{}).Where("id = ?", id).Update("version", req.Version)
	slog.Info("应用已升级", "name", row.Name, "app", row.AppKey, "version", req.Version)
	core.OK(c, gin.H{"ok": true, "version": req.Version})
}

func baseNames(files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, filepath.Base(f))
	}
	return out
}

func (m *Module) uninstall(c *gin.Context) {
	r, ok := m.loadInst(c)
	if !ok {
		return
	}
	var req struct {
		RemoveData bool `json:"remove_data"`
		Force      bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)

	// 面板依赖保护：卸载 nginx-pkg 会连带卸掉域名反代/网站托管所依赖的 Nginx
	if r.Kind == "package" && r.AppKey == "nginx-pkg" && !req.Force {
		if files, _ := filepath.Glob("/etc/nginx/conf.d/hanyuxin_*.conf"); len(files) > 0 {
			core.Err(c, 400, "面板正在使用 Nginx（域名反代/网站托管，配置: "+strings.Join(baseNames(files), ", ")+"）。卸载将导致面板域名与网站无法访问；确认不受影响可携带 force=1 强制卸载")
			return
		}
	}

	if r.Kind == "package" {
		_, _, _ = execx.Run("appstore", 60*time.Second, "systemctl", "disable", "--now", r.Binary)
		if r.Pkgs != "" {
			if detectPkgManager() == "apt" {
				_, _, _ = execx.RunShell("appstore:uninstall", 10*time.Minute, "DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 purge -y "+r.Pkgs)
			} else {
				_, _, _ = execx.RunShell("appstore:uninstall", 10*time.Minute, "dnf remove -y "+r.Pkgs)
			}
		}
	} else if r.Kind == "script" {
		// 脚本类：清理脚本与日志（脚本自身的清理由脚本作者负责，可在脚本内写 uninstall 逻辑）
		_ = os.Remove(filepath.Join(m.scriptsDir(), r.Name+".sh"))
		_ = os.Remove(filepath.Join(m.scriptsDir(), r.Name+".sh.log"))
	} else if r.Kind == "native" {
		if r.Tool {
			if r.Binary != "" {
				_ = os.Remove(filepath.Join("/usr/local/bin", r.Binary))
			}
		} else {
			_, _, _ = execx.Run("appstore", 60*time.Second, "systemctl", "disable", "--now", m.unitName(r.Name))
			_ = os.Remove(filepath.Join("/etc/systemd/system", m.unitName(r.Name)+".service"))
			_, _, _ = execx.Run("appstore", 30*time.Second, "systemctl", "daemon-reload")
		}
	} else {
		_, _, _ = execx.Run("appstore", 10*time.Minute, "docker", composeArgs(m.composeFile(r.Name), "down")...)
	}
	m.db.Delete(&r)
	if req.RemoveData {
		if r.Kind == "native" {
			_ = os.RemoveAll(m.nativeDir(r.Name))
		} else {
			_ = os.RemoveAll(m.dir(r.Name))
		}
	}
	core.OK(c, nil)
}

// ---- 工具 ----

func orParams(p map[string]string) map[string]string {
	if p == nil {
		return map[string]string{}
	}
	return p
}

func cloneParams(p map[string]string) map[string]string {
	out := make(map[string]string, len(p)+1)
	for k, v := range p {
		out[k] = v
	}
	return out
}

func tailOf(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1024 {
		return "..." + s[len(s)-1024:]
	}
	return s
}

// detectPkgManager 检测系统包管理器：apt | dnf | yum | 空。
func detectPkgManager() string {
	for _, m := range []string{"apt-get", "dnf", "yum"} {
		if _, err := os.Stat("/usr/bin/" + m); err == nil {
			return strings.TrimSuffix(m, "-get")
		}
	}
	return ""
}

// installPackage 系统包安装：包管理器安装 → systemctl enable --now。
func (m *Module) installPackage(tpl *Template, name, version, srcID string, p map[string]string) (string, []string, string, error) {
	aptInstallMu.Lock()
	defer aptInstallMu.Unlock()
	if goruntime.GOOS != "linux" {
		return "", nil, "", fmt.Errorf("系统包安装仅支持在 Linux 服务器上执行")
	}
	distro := detectPkgManager()
	if distro == "" {
		return "", nil, "", fmt.Errorf("未找到 apt-get/dnf/yum 包管理器")
	}
	if version != "" && version != "distro" && !versionTokenRe.MatchString(version) {
		return "", nil, "", fmt.Errorf("版本号非法: %s", version)
	}
	basePkgs, unit, err0 := tpl.Pkg(distro)
	if err0 != nil {
		return "", nil, "", err0
	}
	var pkgs []string
	var srcNote string
	useAptMirror := false
	if version != "" && version != "distro" {
		// 版本路由：系统镜像源内的版本 → pin 系统源；不在系统源 → 官方仓库版本走镜像站定向安装
		inSystem := false
		for _, v := range systemPkgVersions(basePkgs) {
			if v.Version == version {
				inSystem = true
				break
			}
		}
		if inSystem {
			pkgs = basePkgs
			pinned := make([]string, len(pkgs))
			for i, p0 := range pkgs {
				if distro == "apt" {
					pinned[i] = p0 + "=" + version
				} else {
					pinned[i] = p0 + "-" + version
				}
			}
			pkgs = pinned
			srcNote = "系统镜像源（指定版本 " + version + "）"
		} else if tpl.PkgInstallVer != nil {
			// 官方仓库版本：经镜像站（auto=阿里云公网；腾讯云内网机器可在系统设置选内网源时自动就近）定向安装
			pre, pk, post, u, note, err := tpl.PkgInstallVer(version, distro, "auto")
			if err != nil {
				return "", nil, "", err
			}
			for _, cmd := range pre {
				if out, code, err := execx.RunShell("appstore:pkg", 20*time.Minute, cmd); err != nil {
					return "", nil, "", fmt.Errorf("仓库准备失败(exit %d): %s", code, tailOf(out))
				}
			}
			pkgs, unit = pk, u
			srcNote = note
			useAptMirror = tpl.Key == "memcached-pkg" && distro == "apt"
			defer func() {
				for _, cmd := range post {
					_, _, _ = execx.RunShell("appstore:pkg", 10*time.Minute, cmd)
				}
			}()
		} else {
			return "", nil, "", fmt.Errorf("版本 %s 不在系统镜像源中，且该应用未提供官方仓库镜像安装", version)
		}
	} else {
		pkgs = basePkgs
	}
	if len(pkgs) == 0 || unit == "" {
		return "", nil, "", fmt.Errorf("模板包定义不完整")
	}
	var installCmd string
	switch distro {
	case "apt":
		aptBase := "systemctl stop unattended-upgrades || true; DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 update && DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=600 install -y " + strings.Join(pkgs, " ") + "; rc=$?; systemctl start unattended-upgrades || true; exit $rc"
		if useAptMirror {
			msrc, ok := aptMirrorSource("auto")
			if ok {
				installCmd = aptMirrorInstallShell(msrc, pkgs)
			} else {
				installCmd = aptBase
			}
		} else if strings.HasPrefix(srcNote, "mirror:") {
			msrc, _ := aptMirrorSource("auto")
			installCmd = aptMirrorInstallShell(msrc, pkgs)
		} else {
			installCmd = aptBase
		}
	case "dnf":
		installCmd = "dnf install -y " + strings.Join(pkgs, " ")
	default:
		installCmd = "yum install -y " + strings.Join(pkgs, " ")
	}
	if out, code, err := execx.RunShell("appstore:pkg", 20*time.Minute, installCmd); err != nil {
		return "", nil, "", fmt.Errorf("包安装失败(exit %d): %s", code, tailOf(out))
	}
	if _, code, err := execx.Run("appstore", 60*time.Second, "systemctl", "enable", "--now", unit); err != nil {
		return "", nil, "", fmt.Errorf("服务启动失败(exit %d): %s", code, unit)
	}
	time.Sleep(2 * time.Second)
	stateOut, _, _ := execx.Run("appstore", 10*time.Second, "systemctl", "is-active", unit)
	state := strings.TrimSpace(stateOut)
	if state != "active" {
		return "", nil, "", fmt.Errorf("服务未进入 active 状态（当前 %s），请检查 journalctl -u %s", state, unit)
	}
	msg := fmt.Sprintf("已安装并启动（%s，服务 %s）", strings.Join(pkgs, " "), unit)
	if srcNote != "" {
		msg += "，安装源: " + srcNote
	}
	return msg, pkgs, unit, nil
}

// systemPkgVersions 查询系统镜像源内指定包的可用版本（安装时可 pin）：
// apt 用 apt-cache madison（版本列），dnf/yum 用 --showduplicates（NEVRA 版本列，反转使最新在前）。
// 查询失败返回空切片（前端仅显示「系统默认」）。
func systemPkgVersions(pkgs []string) []AppVersion {
	distro := detectPkgManager()
	if distro == "" {
		return nil
	}
	out := []AppVersion{}
	seen := map[string]bool{}
	add := func(v, note string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, AppVersion{Version: v, Note: note})
	}
	switch distro {
	case "apt":
		out2, _, err := execx.Run("appstore", 60*time.Second, "apt-cache", append([]string{"madison"}, pkgs...)...)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(out2, "\n") {
			parts := strings.Split(line, "|")
			if len(parts) < 2 {
				continue
			}
			add(strings.TrimSpace(parts[1]), "系统镜像源")
		}
	default:
		out2, _, err := execx.Run("appstore", 120*time.Second, distro, "showduplicates", "list", "available")
		if err != nil {
			// yum 无 --showduplicates 时的兜底
			out2, _, err = execx.Run("appstore", 120*time.Second, distro, "list", "available")
			if err != nil {
				return nil
			}
		}
		for _, line := range strings.Split(out2, "\n") {
			f := strings.Fields(line)
			if len(f) < 2 || strings.HasSuffix(f[0], "Packages") || strings.HasSuffix(f[0], "内核") {
				continue
			}
			add(f[1], "系统镜像源")
		}
		// dnf/yum 输出为旧→新，反转让最新在前
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// repoVer 阿里云 MariaDB 仓库的一个版本目录。
var mariadbCache struct {
	mu   sync.Mutex
	vers []string
	at   time.Time
}

// mariadbVersions 阿里云 MariaDB 仓库可用版本（按当前发行版校验，缓存 10 分钟）。
func mariadbVersions() []string {
	mariadbCache.mu.Lock()
	defer mariadbCache.mu.Unlock()
	if mariadbCache.vers != nil && time.Since(mariadbCache.at) < 6*time.Hour {
		return mariadbCache.vers
	}
	base := "https://mirrors.aliyun.com/mariadb/repo/"
	data, err := httpGet(base, 20*time.Second, 4<<20)
	if err != nil {
		return nil
	}
	out := parseMariaDBRepoVersions(string(data))
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		return verLess(strings.Replace(out[j], "rolling", "999999", 1), strings.Replace(out[i], "rolling", "999999", 1))
	})
	mariadbCache.vers = out
	mariadbCache.at = time.Now()
	return out
}

func parseMariaDBRepoVersions(html string) []string {
	re := regexp.MustCompile(`href="(\d+(?:\.\d+)*(?:\.rolling)?)/?"`)
	seen := map[string]bool{}
	out := []string{}
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}

var redisCache struct {
	mu     sync.Mutex
	byDisp map[string]string // 上游版本号 → apt 完整版本
	order  []string          // 显示版本，降序
	at     time.Time
}

// redisVersions packages.redis.io 的 redis-server 全部历史版本（缓存 10 分钟）。
func redisVersions() (map[string]string, []string) {
	redisCache.mu.Lock()
	defer redisCache.mu.Unlock()
	if redisCache.byDisp != nil && time.Since(redisCache.at) < 6*time.Hour {
		return redisCache.byDisp, redisCache.order
	}
	_, cn := osRelease()
	if cn == "" || detectPkgManager() != "apt" {
		return nil, nil
	}
	data, err := httpGet("https://packages.redis.io/deb/dists/"+cn+"/main/binary-"+runtime.GOARCH+"/Packages", 25*time.Second, 8<<20)
	if err != nil {
		return nil, nil
	}
	byDisp := map[string]string{}
	order := []string{}
	isSrv := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "Package: ") {
			isSrv = strings.TrimSpace(strings.TrimPrefix(line, "Package: ")) == "redis-server"
			continue
		}
		if isSrv && strings.HasPrefix(line, "Version: ") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Version: "))
			disp := v
			if i := strings.Index(disp, ":"); i >= 0 {
				disp = disp[i+1:]
			}
			if i := strings.Index(disp, "-"); i >= 0 {
				disp = disp[:i]
			}
			if _, ok := byDisp[disp]; !ok {
				byDisp[disp] = v
				order = append(order, disp)
			} else {
				byDisp[disp] = v // Packages 升序排列，后者修订更高
			}
		}
	}
	if len(order) == 0 {
		return nil, nil
	}
	sort.Slice(order, func(i, j int) bool { return verLess(order[j], order[i]) })
	redisCache.byDisp = byDisp
	redisCache.order = order
	redisCache.at = time.Now()
	return byDisp, order
}

// osRelease 读取 /etc/os-release 的 ID 与 VERSION_CODENAME（进程内缓存）。
var osRelOnce struct {
	once sync.Once
	id   string
	cod  string
}

func osRelease() (id, codename string) {
	osRelOnce.once.Do(func() {
		data, err := os.ReadFile("/etc/os-release")
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "ID=") {
				osRelOnce.id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
			} else if strings.HasPrefix(line, "VERSION_CODENAME=") {
				osRelOnce.cod = strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), `"`)
			}
		}
	})
	return osRelOnce.id, osRelOnce.cod
}

// nginxDeb nginx.org pool 中的一个 deb：Ver 为上游版本号，File 为完整文件名。
type nginxDeb struct {
	Ver  string
	File string // "<branch>/<fname>"，branch = stable | mainline（决定 pool 下载路径）
}

var nginxPoolCache struct {
	mu      sync.Mutex
	byVer   map[string]string // 版本号 → 最高修订的 deb（<branch>/<fname>，stable 与 mainline pool 合并）
	pools   map[string]string // pool URL → Last-Modified（304 条件请求用）
	at      time.Time
	fetched bool
}

const nginxPoolTTL = 6 * time.Hour

// nginxPoolBaseCandidates 按分支返回 pool 目录基址候选：USTC 镜像优先（国内 0.5s），
// 官方 nginx.org 回退。branch = stable | mainline（主线版本独立发布在 packages/mainline/）。
func nginxPoolBaseCandidates(distroPath, branch string) []string {
	main := ""
	if branch == "mainline" {
		main = "mainline/"
	}
	return []string{
		"https://mirrors.ustc.edu.cn/nginx/" + main + distroPath + "/pool/nginx/n/nginx/",
		"https://nginx.org/packages/" + main + distroPath + "/pool/nginx/n/nginx/",
	}
}

// nginxPoolEntries 拉取 nginx.org packages 的 stable 与 mainline 两个 pool 目录，
// 按当前发行版代号与架构过滤（缓存 6 小时 + Last-Modified 条件请求：未变化时 304 零下载）。
func nginxPoolEntries() []nginxDeb {
	nginxPoolCache.mu.Lock()
	defer nginxPoolCache.mu.Unlock()
	if nginxPoolCache.fetched && time.Since(nginxPoolCache.at) < nginxPoolTTL {
		return poolList(nginxPoolCache.byVer)
	}
	_, cn := osRelease()
	if cn == "" || detectPkgManager() != "apt" {
		return nil
	}
	distroPath := "ubuntu"
	if id, _ := osRelease(); id == "debian" {
		distroPath = "debian"
	}
	re := regexp.MustCompile(`nginx_(\d+(?:\.\d+){2})-(\d+)~([a-z0-9]+)_(amd64|arm64)\.deb`)
	arch := runtime.GOARCH
	// 沿用缓存条目（对应 pool 304 未变化时零下载）
	byVer := map[string]string{}
	if nginxPoolCache.byVer != nil {
		for v, f := range nginxPoolCache.byVer {
			byVer[v] = f
		}
	}
	newPools := map[string]string{}
	for _, branch := range []string{"stable", "mainline"} {
		// 版本列表以官方 nginx.org 为唯一数据源（全量、最新）；USTC 镜像仅用于 deb 下载加速（同步可能滞后）
		for _, base := range nginxPoolBaseCandidates(distroPath, branch) {
			if !strings.Contains(base, "nginx.org") {
				continue
			}
			data, notModified, respLM, err := httpGetCond(base, 20*time.Second, 4<<20, nginxPoolCache.pools[base])
			if err != nil {
				continue
			}
			if notModified {
				// 保留该分支旧条目（按前缀过滤重建）
				for v, f := range nginxPoolCache.byVer {
					if strings.HasPrefix(f, branch+"/") {
						byVer[v] = f
					}
				}
				if lm, ok := nginxPoolCache.pools[base]; ok {
					newPools[base] = lm
				}
				break
			}
			newPools[base] = respLM
			// 该分支以新数据为准：清掉缓存中此分支旧条目
			for v, f := range byVer {
				if strings.HasPrefix(f, branch+"/") {
					delete(byVer, v)
				}
			}
			bestRev := map[string]int{} // branch|ver → 最高 deb 修订号
			for _, m := range re.FindAllStringSubmatch(string(data), -1) {
				if m[3] != cn || m[4] != arch {
					continue
				}
				rev, _ := strconv.Atoi(m[2])
				k := branch + "|" + m[1]
				if best, ok := bestRev[k]; ok && best >= rev {
					continue
				}
				bestRev[k] = rev
				byVer[m[1]] = branch + "/" + m[0]
			}
			break
		}
	}
	if len(byVer) == 0 {
		return nil
	}
	nginxPoolCache.byVer = byVer
	nginxPoolCache.pools = newPools
	nginxPoolCache.at = time.Now()
	nginxPoolCache.fetched = true
	return poolList(byVer)
}

// respLastModified 提取 Last-Modified 响应头（条件请求用；缺省回退当前 UTC 时间）。
func nginxDownloadVersions() []string {
	data, err := httpGet("https://nginx.org/download/", 20*time.Second, 4<<20)
	if err != nil {
		return nil
	}
	return parseNginxDownloadVersions(string(data))
}

func parseNginxDownloadVersions(html string) []string {
	re := regexp.MustCompile(`nginx-(\d+(?:\.\d+){2})\.tar\.gz`)
	seen := map[string]bool{}
	out := []string{}
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	sort.Slice(out, func(i, j int) bool { return verLess(out[j], out[i]) })
	return out
}

func poolList(byVer map[string]string) []nginxDeb {
	out := make([]nginxDeb, 0, len(byVer))
	for v, f := range byVer {
		out = append(out, nginxDeb{Ver: v, File: f})
	}
	sort.Slice(out, func(i, j int) bool { return verLess(out[j].Ver, out[i].Ver) })
	return out
}

// debRev 从 deb 文件名提取包修订号。
func debRev(file string) string {
	m := regexp.MustCompile(`-(\d+)~`).FindStringSubmatch(file)
	if m == nil {
		return "0"
	}
	return m[1]
}

// verLess 数值版语义比较 a < b（点分数字）。
func verLess(a, b string) bool {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		ai, bi := 0, 0
		if i < len(pa) {
			ai, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			bi, _ = strconv.Atoi(pb[i])
		}
		if ai != bi {
			return ai < bi
		}
	}
	return false
}

// cleanVerRe 合法版本号（过滤 GitHub tag 噪声）。
var cleanVerRe = regexp.MustCompile(`^\d+(\.\d+)*$`)

var nginxRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// versionTokenRe 允许上游包管理器/发行包使用的版本字符，拒绝路径和 shell 元字符。
var versionTokenRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+~:-]{0,79}$`)

func httpGet(url string, timeout time.Duration, limit int64) ([]byte, error) {
	data, _, _, err := httpGetCond(url, timeout, limit, "")
	return data, err
}

// httpGetCond 带条件请求的 GET：lastModified 非空时携带 If-Modified-Since，
// 服务端返回 304（内容未变化）时不下载正文，直接返回 notModified=true（调用方沿用缓存）。
// 返回的 respLM 为本次响应的 Last-Modified 头（304 时为请求传入值，调用方应保留）。
func httpGetCond(url string, timeout time.Duration, limit int64, lastModified string) (data []byte, notModified bool, respLM string, err error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, "", err
	}
	req.Header.Set("User-Agent", "hanyuxin-panel")
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, true, lastModified, nil
	}
	if resp.StatusCode != 200 {
		return nil, false, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, limit))
	return data, false, respLMOf(resp), err
}

// respLMOf 提取 Last-Modified 响应头（缺省回退当前 UTC 时间）。
func respLMOf(resp *http.Response) string {
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		return lm
	}
	return time.Now().UTC().Format(http.TimeFormat)
}

// parseAptMadisonVersions 提取 apt-cache madison 的完整包版本号。
func parseAptMadisonVersions(output string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		ver := strings.TrimSpace(parts[1])
		if ver == "" || !versionTokenRe.MatchString(ver) || seen[ver] {
			continue
		}
		seen[ver] = true
		out = append(out, ver)
	}
	return out
}

// aptVersionAvailable 检查完整 apt 包版本是否仍在本机索引中。
func aptVersionAvailable(pkg, version string) bool {
	if !versionTokenRe.MatchString(version) {
		return false
	}
	out, _, err := execx.Run("appstore", 20*time.Second, "apt-cache", "madison", pkg)
	if err != nil {
		return false
	}
	for _, v := range parseAptMadisonVersions(out) {
		if v == version {
			return true
		}
	}
	return false
}

// memcachedVersions 返回当前发行版 apt 索引中可锁定安装的 Memcached 版本。
func memcachedVersions() []AppVersion {
	out := []AppVersion{{Version: "distro", Note: "发行版最新"}}
	if detectPkgManager() == "apt" {
		raw, _, err := execx.Run("appstore", 20*time.Second, "apt-cache", "madison", "memcached")
		if err == nil {
			for _, v := range parseAptMadisonVersions(raw) {
				out = append(out, AppVersion{Version: v, Note: "发行版精确包版本"})
			}
		}
	}
	if len(out) == 1 {
		for _, v := range githubVersionList("memcached/memcached") {
			out = append(out, AppVersion{Version: v, Note: "Memcached 官方发布版本"})
		}
	}
	return out
}

func redisArchiveVersions() []string {
	data, err := httpGet("https://download.redis.io/releases/", 20*time.Second, 4<<20)
	if err != nil {
		return nil
	}
	return parseRedisArchiveVersions(string(data))
}

func parseRedisArchiveVersions(html string) []string {
	re := regexp.MustCompile(`redis-(\d+(?:\.\d+){2})\.tar\.gz`)
	seen := map[string]bool{}
	out := []string{}
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	sort.Slice(out, func(i, j int) bool { return verLess(out[j], out[i]) })
	return out
}

var minioReleaseRe = regexp.MustCompile(`minio\.(RELEASE\.\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}Z)`)

// minioVersions 从 MinIO 官方归档目录读取版本；官方目录失效时仍保留 latest 选项。
func minioVersions() []AppVersion {
	out := []AppVersion{{Version: "latest", Note: "官方最新"}}
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return out
	}
	base := "https://dl.min.io/server/minio/release/linux-" + arch + "/archive/"
	data, err := httpGet(base, 20*time.Second, 8<<20)
	if err != nil {
		base = "https://dl.minio.org.cn/server/minio/release/linux-" + arch + "/archive/"
		data, err = httpGet(base, 20*time.Second, 8<<20)
	}
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	for _, m := range minioReleaseRe.FindAllStringSubmatch(string(data), -1) {
		if len(m) < 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, AppVersion{Version: m[1], Note: "MinIO 官方归档"})
	}
	if len(out) > 1 {
		sort.Slice(out[1:], func(i, j int) bool { return out[i+1].Version > out[j+1].Version })
	}
	return out
}

func minioVersionURL(version, arch string) (string, error) {
	if version == "latest" {
		var err error
		version, err = minioLatestVersion()
		if err != nil {
			return "", err
		}
	}
	return "https://github.com/minio/minio/releases/download/" + version + "/minio.linux-" + arch + "." + version, nil
}

func minioLatestVersion() (string, error) {
	data, err := httpGet("https://api.github.com/repos/minio/minio/releases?per_page=1", 20*time.Second, 1<<20)
	if err != nil {
		return "", err
	}
	var releases []struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
		Pre     bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(data, &releases); err != nil || len(releases) == 0 || releases[0].TagName == "" {
		return "", fmt.Errorf("MinIO 官方版本清单解析失败")
	}
	return releases[0].TagName, nil
}

type ghTagEntry struct {
	tags []string
	at   time.Time
}

var ghTagCache sync.Map

// githubTags 拉取 GitHub 仓库 releases tag 列表（内存缓存 10 分钟）。
func githubTags(repo string) ([]string, error) {
	if v, ok := ghTagCache.Load(repo); ok {
		e := v.(ghTagEntry)
		if time.Since(e.at) < 10*time.Minute {
			return e.tags, nil
		}
	}
	data, err := httpGet("https://api.github.com/repos/"+repo+"/releases?per_page=30", 20*time.Second, 8<<20)
	if err != nil {
		return nil, err
	}
	var arr []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.Unmarshal(data, &arr) != nil {
		return nil, fmt.Errorf("GitHub releases 解析失败")
	}
	tags := []string{}
	for _, r := range arr {
		if r.TagName != "" && !r.Prerelease {
			tags = append(tags, r.TagName)
		}
	}
	ghTagCache.Store(repo, ghTagEntry{tags: tags, at: time.Now()})
	return tags, nil
}

// ghVerList 把 GitHub tag 列表转为版本清单（过滤噪声 tag）。
func ghVerList(repo string) []AppVersion {
	tags, err := githubTags(repo)
	if err != nil || len(tags) == 0 {
		return []AppVersion{}
	}
	out := []AppVersion{}
	for _, tg := range tags {
		v := strings.TrimPrefix(tg, "v")
		if !cleanVerRe.MatchString(v) {
			continue
		}
		out = append(out, AppVersion{Version: v, Note: "GitHub Release"})
	}
	return out
}

func githubVersionList(repo string) []string {
	data, err := httpGet("https://api.github.com/repos/"+repo+"/tags?per_page=100", 20*time.Second, 4<<20)
	if err != nil {
		return nil
	}
	return parseGitHubVersionTags(data)
}

func parseGitHubVersionTags(data []byte) []string {
	var tags []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &tags) != nil {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, tag := range tags {
		v := strings.TrimPrefix(tag.Name, "v")
		if !cleanVerRe.MatchString(v) || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return verLess(out[j], out[i]) })
	return out
}
