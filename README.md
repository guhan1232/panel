# webmblx — Linux Web 管理面板

Go + Vue3 的全功能 Linux 服务器管理面板，编译为**单个二进制**（前端已嵌入），零运行时依赖，低内存占用。

> 规划文档见 [PLAN.md](PLAN.md)（技术选型、架构、模块设计、排期）。

## 功能

### P0 基础

| 模块 | 说明 |
|---|---|
| 🔐 认证安全 | 安全入口路径、登录防爆破（15 分钟 5 次锁定）、会话管理、HTTPS（自签/自定义证书） |
| 📊 仪表盘 | CPU/内存/网络实时曲线（3s 采样）、负载、磁盘用量、Top 进程、系统信息 |
| ⏰ 计划任务 | **内置调度引擎**（robfig/cron）：Shell/目录备份/URL 健康检查/数据库备份/日志切割/系统更新 六类任务，执行历史、手动执行、终止运行实例、连续失败告警、90 天历史自动清理；不写系统 crontab |
| 📁 文件管理 | 浏览、上传下载、在线编辑、新建/重命名/移动、权限修改、zip/tar.gz 压缩解压、回收站删除 |
| 🖥 Web 终端 | xterm.js + PTY，多会话、自适应尺寸（仅 Linux 部署时可用） |
| ⚙️ 进程管理 | 进程列表（CPU/内存排序）、结束/强杀进程、systemd 服务启停/自启管理 |

### P1 v0.2

| 模块 | 说明 |
|---|---|
| 🌐 网站 | Nginx 静态站/PHP 站/反向代理，配置模板生成，写入前 `nginx -t` 校验失败自动回滚，域名绑定、SSL 绑定、强制 HTTPS、站点日志查看、一键安装 Nginx |
| 🗄 数据库 | MySQL/PostgreSQL 建库、建用户、授权、删除；连接凭据面板托管；为计划任务提供备份凭据 |
| 🛡 防火墙 | ufw / firewalld 统一抽象：端口放行/移除、防火墙开关 |
| 🔒 SSL 证书 | 手动上传（含证书-私钥配对校验）、Let's Encrypt 自动签发（HTTP-01，lego 实现）、到期监控、ACME 证书剩余 30 天自动续期、到期告警 |
| 📣 通知 | 邮件 SMTP（含 465 隐式 TLS）+ 钉钉/企业微信/飞书/通用 Webhook；计划任务失败告警、证书到期告警接入 |
| ☁️ 对象存储 | S3 兼容存储配置与连通性测试，备份任务可同时归档到远端桶 |
| 🧾 审计 | 全部写操作 API 审计入库存查（操作人/路径/IP/耗时），支持筛选分页 |

### P2 v0.3

| 模块 | 说明 |
|---|---|
| 🛍 应用商店 | Docker Compose 模板化一键部署（Nginx/MySQL/Redis/WordPress），启停/重启/日志/卸载（可选删数据），未装 Docker 可一键安装 |
| 🚨 告警策略 | CPU/内存/磁盘/负载 阈值规则（支持持续时长与冷却去重），每 30s 评估，触发写入历史并经通知渠道推送 |

### v0.4 应用商店 · 原生部署

| 模块 | 说明 |
|---|---|
| 📦 原生应用 | 免 Docker 的 **Linux 原生二进制部署**：自动按架构（amd64/arm64）下载官方发行包 → 解压 → 生成 systemd 单元（`webmblx-app-*`，崩溃自动重启）→ 启动并健康检查；日志走 journalctl，卸载可选清数据。内置模板：**Caddy**（Web/自动HTTPS）、**MinIO**（对象存储）、**node_exporter**（指标采集）、**FRP 服务端**（内网穿透）；容器与原生模板统一管理界面 |

### v0.5 Docker 镜像源管理

| 模块 | 说明 |
|---|---|
| 🪞 镜像源配置 | 可视化管理 `/etc/docker/daemon.json` 的 `registry-mirrors` 与 `insecure-registries`：合并写入不动其它配置项、修改前自动备份、重启失败自动回滚并恢复 Docker；常用国内加速源一键填入；保存后回读 `docker info` 展示当前生效的镜像源 |

## 快速开始

### 安装（Linux 服务器）

```bash
# 1. 获取二进制（自行 make linux 构建或从 Release 下载）
tar -xzf webmblx-linux-amd64.tar.gz

# 2. 一键安装（systemd 服务 + 自动生成配置/入口/初始密码）
sudo bash scripts/install.sh ./webmblx

# 3. 按脚本输出的入口地址访问面板
sudo cat /opt/webmblx/data/initial_admin.txt
```

### 本地开发

```bash
# 后端（Windows/macOS/Linux 均可开发调试）
go run ./cmd/server          # 生成 config.yaml：随机端口(10000-30000) + 随机入口 + 初始密码

# 前端（热更新；代理目标 = 后端实际端口，见 config.yaml 的 listen）
cd web && WM_PANEL_URL=http://127.0.0.1:<端口> npm run dev
```

### 构建

```bash
make linux        # Linux amd64 单二进制 → bin/webmblx（先自动构建前端）
make linux-arm64  # arm64
make release      # 打包 tar.gz
```

> 纯 Go SQLite 驱动，`CGO_ENABLED=0` 交叉编译无障碍。

## 首次启动发生了什么

1. 生成 `config.yaml`：监听端口在 **10000–30000 中随机挑选**（自动探测空闲，重启保持不变；需固定端口可直接改 `listen`），随机 **安全入口**（如 `/s/a84e491e`）
2. 创建 SQLite 数据库 `data/panel.db`
3. 创建管理员 `admin`，随机密码写入 `data/initial_admin.txt` 并打印
4. 启动日志打印完整访问地址（端口 + 入口）
5. **不带入口串的任何请求一律 404**，只有访问过 `/s/<入口串>` 才能进入面板

## 安全说明

- 面板以 root 运行（与宝塔/1Panel 同），安全由以下机制控制：安全入口 404 防护、登录防爆破、会话 HttpOnly Cookie、全量写操作审计、统一命令执行层（固定二进制调用点 + 超时 + 日志）
- 建议生产环境：启用 HTTPS（`config.yaml` → `server.tls.enabled`，未配证书自动生成自签）、修改初始密码、防火墙限制面板端口来源 IP、或仅监听 `127.0.0.1` 走 SSH 隧道
- 升级前请备份 `data/` 目录

## 计划任务设计要点

- 调度引擎内置（5 段 cron / 带秒 6 段 / `@daily` 描述符），任务定义存 SQLite，重启自动恢复
- 同一任务默认防重叠（上次未结束则跳过），可配置并发
- 每次执行记录：状态/耗时/退出码/输出（截断 64KB），历史保留 90 天自动清理
- 连续失败达到阈值触发告警（通知渠道 P1 接入），成功自动复位

## 目录结构

```
├── cmd/server/          # 入口装配
├── internal/
│   ├── core/            # 配置/日志/SQLite/模型/初始化
│   ├── auth/            # 登录/会话/安全入口/防爆破
│   ├── audit/           # 写操作审计中间件
│   ├── execx/           # 统一命令执行层
│   ├── archive/         # zip/tar.gz（防路径穿越）
│   ├── platform/        # PTY 平台层（Windows 占位）
│   ├── selfcert/        # 自签证书
│   └── modules/         # dashboard / cron / filem / term / proc / setting
├── web/                 # Vue3 + Element Plus + xterm.js（dist 嵌入二进制）
├── scripts/             # install.sh / uninstall.sh
├── deploy/webmblx.service
└── Makefile
```

## License

MIT
