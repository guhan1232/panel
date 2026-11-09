# 寒语馨面板（Hanyuxin Panel）— Linux Web 管理面板 开发规划

> 版本：v0.1（规划稿） 日期：2026-11-07
> 定位：Go + Vue3 的全功能 Linux 服务器管理面板，对标 1Panel / 宝塔，单二进制部署、低资源占用、内置调度引擎。

---

## 1. 项目概述

### 1.1 目标

做一个可通过浏览器管理 Linux 服务器的面板，覆盖：监控仪表盘、网站/反向代理、数据库、文件、终端、防火墙、**计划任务**、备份、日志审计、通知告警。

### 1.2 核心原则

| 原则 | 说明 |
|---|---|
| 零依赖部署 | 后端编译为单二进制，前端 `go:embed` 打包进二进制；面板自身数据用 SQLite |
| 低资源占用 | 空载内存目标 < 50MB，监控采集不做常驻子进程 |
| 安全默认值 | 强制 HTTPS、登录防爆破、安全入口路径、全量操作审计 |
| 不污染系统 | 面板自身装在 `/opt/hanyuxin`；计划任务不写入系统 crontab（见 §4.5） |
| 模块化 | 每个功能一个后端 module，注册进路由与权限表，方便裁剪与扩展 |

### 1.3 支持范围

- 发行版：Debian 11+ / Ubuntu 20.04+ / CentOS 8+ / Rocky / AlmaLinux（firewalld 与 ufw 双适配）
- 架构：x86_64、arm64
- 运行方式：systemd 服务，安装/卸载/升级均提供一键脚本

---

## 2. 技术选型

| 层 | 选型 | 理由 |
|---|---|---|
| 后端语言 | Go 1.22+ | 单二进制、内存低、并发好，1Panel 同路线 |
| Web 框架 | Gin | 生态成熟、中间件齐全 |
| WebSocket | gorilla/websocket | 终端、实时监控推送 |
| ORM / 存储 | GORM + SQLite (mattn/go-sqlite3) | 面板配置、任务、审计历史单文件存储 |
| 系统指标 | shirou/gopsutil | CPU/内存/磁盘/网络，纯 Go 无子进程 |
| 计划任务调度 | robfig/cron/v3 | 标准 5 段 + 秒级扩展，动态增删任务 |
| 终端 PTY | creack/pty | Web 终端核心 |
| 日志 | slog（结构化）+ 按天切割 | 标准库，少依赖 |
| 前端 | Vue3 + TypeScript + Vite + Pinia | 主流、类型安全 |
| UI 组件 | Element Plus | 后台面板组件全（表格/表单/弹窗） |
| 终端前端 | xterm.js | 标准 Web 终端方案 |
| 图表 | ECharts | 监控曲线 |
| 认证 | Session Token（HttpOnly Cookie）+ TOTP 可选 | 比 JWT 更易吊销 |

---

## 3. 总体架构

```
┌─────────────── 浏览器 (Vue3 SPA) ───────────────┐
│  仪表盘 │ 计划任务 │ 文件 │ 终端(xterm) │ 网站 │ … │
└────────── HTTPS : {8443, 安全入口路径} ──────────┘
                 │ REST API + WebSocket
┌─────────────── hanyuxin 二进制 (root) ───────────┐
│  中间件: 安全入口 → 认证 → 权限 → 审计 → 限流     │
│  core:  配置 / SQLite / 日志 / 通知 / 升级        │
│  modules: dashboard│cron│file│terminal│website … │
│  exec:   命令白名单 + sudo 策略 + 超时控制        │
└──────────────────────────────────────────────────┘
                 │
   Linux 主机：systemd / nginx / firewalld / crontab(只读)
```

要点：

- **单体模块化**：`cmd/server` 只做启动装配，所有功能在 `internal/modules/<name>` 内实现，统一接口 `Register(r, deps)` 注册路由、菜单项、权限点。
- **前端打包进二进制**：`web/dist` 通过 `go:embed` 嵌入，升级即换一个文件。
- **所有危险操作走统一 exec 层**：命令模板白名单、参数转义、超时、审计记录（谁、何时、执行了什么、退出码），禁止模块直接 `exec.Command`。

---

## 4. 功能模块规划

### 4.1 分期总览

| 阶段 | 内容 | 出口标准 |
|---|---|---|
| **P0 MVP** | 框架 + 认证安全 + 监控仪表盘 + **计划任务** + 文件管理 + Web 终端 + 进程/服务管理 + 安装脚本 | 可在真实服务器上完成日常运维闭环 |
| **P1 v1.0** | 网站/Nginx 管理 + 反向代理 + 防火墙 + SSL 证书 + 数据库管理 + 备份 + 日志审计 + 通知 | 对齐 1Panel 核心能力 |
| **P2 v1.5** | 应用商店（容器化部署）、告警策略、定时自动备份策略化、多用户/多节点 | 差异化能力 |

### 4.2 P0 MVP 模块清单

| 模块 | 功能点 |
|---|---|
| 认证与安全 | 登录、防爆破锁定（15 分钟 5 次）、安全入口 `/s/<随机串>`、会话管理、可选 TOTP、修改密码 |
| 仪表盘 | CPU/内存/磁盘/网络实时曲线（5s 推送）、负载、系统信息（内核/发行版/开机时长）、Top 进程 |
| 计划任务 | 见 §4.5 完整设计 |
| 文件管理 | 浏览、上传/下载、新建/重命名/移动/删除、权限(chmod/chown)、编辑文本、压缩(zip/tar.gz)/解压、回收站 |
| Web 终端 | xterm.js + PTY、多标签会话、断线重连、会话空闲超时 |
| 进程/服务 | 进程列表/终止、systemd 服务启停/开机自启 |
| 系统设置 | 面板端口/入口、HTTPS 证书、通知渠道、备份面板数据 |

### 4.3 P1 v1.0 模块清单

| 模块 | 功能点 |
|---|---|
| 网站 | 静态站/PHP 站创建、Nginx 配置模板生成、域名绑定、日志查看、伪静态 |
| 反向代理 | 域名 → 上游(端口/Unix socket)、路径转发规则、强制 HTTPS |
| SSL | Let's Encrypt 自动签发/续期（HTTP-01）、自签证书、证书到期提醒 |
| 防火墙 | ufw/firewalld 统一抽象：端口放行/封禁、IP 黑白名单、ping 开关 |
| 数据库 | MySQL/PostgreSQL：建库建用户、授权、备份恢复入口、phpMyAdmin 可选 |
| 备份 | 网站/数据库/目录备份到本地或 S3 兼容对象存储，与计划任务联动 |
| 日志审计 | 登录日志、操作审计查询、网站访问日志检索 |
| 通知 | 邮件 SMTP + 企业微信/钉钉/飞书 Webhook，任务失败、证书到期、磁盘水位告警 |

### 4.4 P2 规划（方向性）

- 应用商店：Docker 化的一键部署（Nginx/MySQL/Redis/WordPress…），compose 模板管理
- 告警策略引擎：指标阈值 + 持续时间 + 通知分级
- 多用户与 RBAC；多节点 Agent（后续架构演进，MVP 阶段明确不做）

### 4.5 计划任务模块详细设计（内置调度引擎）★

用户重点关注模块，设计标准如下：

**任务模型**

```go
type Task struct {
    ID          uint
    Name        string        // 任务名
    Type        string        // shell|backup_site|backup_db|log_rotate|sys_update|visit_url
    Spec        string        // cron 表达式（5 段标准 + 可选 6 段秒级）
    Command     string        // shell 内容 / 任务参数(JSON)
    Timeout     int           // 秒，0=不限
    Concurrent  bool          // 是否允许并发（默认同一任务上次未结束则跳过本次）
    NotifyFail  int           // 连续失败 N 次后通知，0=不通知
    Enabled     bool
    // 运行态由调度器内存持有，不落库
}
```

**任务类型**（P0 支持 `shell`、`backup_dir`、`visit_url`；其余 P1 随备份/网站模块补齐）

| 类型 | 说明 |
|---|---|
| shell | 任意 Shell 脚本，走统一 exec 层（审计+超时） |
| backup_dir | 目录打包备份到备份目录/对象存储，保留 N 份自动清理 |
| backup_site / backup_db | 调用网站/数据库模块（P1） |
| log_rotate | 对指定日志按大小/日期切割压缩 |
| sys_update | 执行系统安全更新（apt upgrade -y / dnf upgrade -y --security） |
| visit_url | HTTP 健康检查，非 2xx/3xx 记为失败（可做站心跳） |

**调度引擎行为**

1. 任务定义持久化在 SQLite；面板启动时加载全部 `enabled` 任务注册到 `robfig/cron`。
2. 新建/修改/删除/启停 → 内存调度器即时增删（`cron.Remove(id)`），无需重启。
3. 执行历史表 `task_log`：任务ID、触发方式(定时/手动)、开始/结束时间、耗时、退出码、输出**截断存前 64KB**、超时标记；历史保留可配置（默认 90 天，超期清理任务自动滚动）。
4. 失败告警：连续失败达到 `NotifyFail` 推送通知渠道，成功后自动复位计数。
5. 执行隔离：任务输出落 `/opt/hanyuxin/logs/tasks/<task_id>/<date>.log`；面板升级/重启不丢任务；运行中任务在面板上可见并可手动终止。
6. 与系统 crontab 隔离：**面板任务全部走内置引擎，不写 `/etc/crontab`**；系统设置页提供系统 crontab **只读视图**，避免与手工任务互相踩踏。

**API 草案**

```
GET    /api/tasks                 列表(含下次执行时间)
POST   /api/tasks                 创建
PUT    /api/tasks/:id             修改
DELETE /api/tasks/:id             删除
POST   /api/tasks/:id/toggle      启用/停用
POST   /api/tasks/:id/run         手动立即执行
DELETE /api/tasks/:id/running     终止运行中实例
GET    /api/tasks/:id/logs?page=  执行历史
GET    /api/task-logs/:logId      单次执行详情(含输出)
```

---

## 5. 安全设计（面板本身是攻击面）

| 层面 | 措施 |
|---|---|
| 入口 | 面板仅监听指定端口；安全入口路径（未带随机串访问一律 404）；可选仅监听 127.0.0.1 走 SSH 隧道 |
| 传输 | 自带自签证书强制 HTTPS；P1 支持 Let's Encrypt |
| 登录 | 密码强度校验、15 分钟内失败 5 次锁定、可选 TOTP 两步验证、登录地/设备提醒 |
| 会话 | HttpOnly + Secure Cookie、空闲 30 分钟过期、支持一键下线全部会话 |
| 权限 | API 权限点化（模块注册权限），P0 单管理员，P2 扩 RBAC |
| 危险操作 | 统一 exec 层：命令模板白名单 + 参数校验 + 超时 + 全量审计（操作、IP、UA、结果） |
| 终端 | 会话超时、并发会话数限制、（可选）命令录屏存档 |
| 升级 | 版本号 + 校验和，升级前自动备份面板数据 |

---

## 6. 代码结构规划

```
hanyuxin/
├── cmd/server/main.go            # 启动装配
├── internal/
│   ├── core/                     # 配置/日志/SQLite/通知/审计中间件
│   ├── auth/                     # 登录、会话、TOTP、防爆破
│   ├── execx/                    # 统一命令执行层(白名单/超时/审计)
│   ├── modules/
│   │   ├── dashboard/            # 系统监控
│   │   ├── cron/                 # ★ 计划任务(调度引擎+任务类型)
│   │   ├── file/                 # 文件管理
│   │   ├── terminal/             # Web 终端(WS)
│   │   ├── process/              # 进程/systemd 服务
│   │   ├── website/  database/  firewall/  backup/  notify/   # P1
│   └── embed.go                  # go:embed 前端产物
├── web/                          # Vue3 前端
│   └── src/{views,api,stores,components}
├── scripts/install.sh            # 一键安装(自动装 systemd unit)
├── deploy/hanyuxin.service
└── Makefile                      # build / build-web / release
```

---

## 7. 开发排期与计划任务（WBS）

> 按 1 人全职估算，双人可近线性压缩；每周含 0.5 天测试缓冲。

### 阶段一：P0 MVP（第 1–5 周）

| 周 | 任务 | 交付/验收 |
|---|---|---|
| W1 | 仓库/Makefile/CI 搭建；Gin 骨架、SQLite、配置、日志；登录+会话+安全入口+防爆破；前端脚手架(Vite+Vue3+路由+布局) | 能登录，安全入口生效 |
| W2 | gopsutil 采集 + WS 实时推送；仪表盘前端（ECharts 曲线、系统信息卡）；审计中间件 | 仪表盘实时刷新 |
| W3 | **计划任务模块**：任务 CRUD + robfig/cron 动态调度 + 执行历史 + 手动执行/终止 + 失败通知 + shell/backup_dir/visit_url 三类任务 | 建一个定时备份任务并跑通历史记录 |
| W4 | 文件管理（浏览/上传/下载/编辑/压缩/回收站）；Web 终端（PTY+WS+xterm 多标签） | 在面板里完成一次完整文件操作 |
| W5 | 进程/服务管理；系统设置页；install.sh + systemd + 打包发布流程；安全加固走查；**MVP 在真实 VPS 上验收** | MVP 里程碑 v0.1.0 发布 |

### 阶段二：P1 v1.0（第 6–11 周）

| 周 | 任务 | 交付 |
|---|---|---|
| W6–7 | 网站/静态与 PHP 站、Nginx 模板生成、域名绑定、反代与路径规则、站点日志 | 全流程建站可验收 |
| W8 | 防火墙抽象层(ufw/firewalld)、SSL：Let's Encrypt 签发+自动续期+到期提醒 | 证书自动续期跑通 |
| W9 | MySQL/PostgreSQL 管理：建库/建用户/授权/备份入口 | 数据库日常操作面板化 |
| W10 | 备份模块（本地+S3 兼容存储，联动计划任务）；通知渠道(邮件+IM webhook)；日志审计查询页 | 备份策略+告警闭环 |
| W11 | 整体联调、异常场景测试（Nginx 配置错误回滚、磁盘满、任务超时）、性能与安全复查、文档与安装体验优化 | **v1.0.0 发布** |

### 阶段三：P2（第 12 周起，滚动）——已全部交付

应用商店（容器化 + 原生二进制）✅ → 告警策略引擎 ✅ → Docker 镜像源管理 ✅ → Web UI 升级 ✅ → 端口随机化 ✅ → 多用户 RBAC ✅ → 多节点 Agent ✅ → **商店拆分（v0.10，软件商店走大镜像站源）✅**。

> v0.3 已交付：应用商店（Docker Compose 模板：Nginx/MySQL/Redis/WordPress）与
> 告警策略引擎（阈值+持续+冷却，事件历史+通知推送）。
> v0.4 已交付：应用商店扩展 **Linux 原生二进制部署**（下载/解压/systemd 托管，
> 内置 Caddy/MinIO/node_exporter/FRPS 模板，amd64/arm64 自适配）。
> v0.5 已交付：Docker 镜像源可视化管理（daemon.json 合并写入/备份回滚/docker info 回读）。
> v0.6 已交付：面板端口默认在 **10000–30000 随机**（首启生成时探测空闲，重启保持不变），
> 启动日志打印完整访问地址。
> v0.7 已交付：多用户 RBAC——三角色（admin/operator/viewer）+ 权限拦截
> （viewer 只读、终端 operator+、用户管理 admin），用户管理页（增删改/禁用/
> 重置密码），自锁与最后管理员保护，禁用即踢会话，审计记录操作人角色。
> v0.8 已交付：**多节点 Agent**——独立 agent 二进制（WSS 反连注册/token 鉴权、
> 15s 指标采样上报、远程命令执行、断线自动重连指数退避）；面板节点管理页
> （接入命令一键复制、token 重置、在线状态、指标图表抽屉、远程命令）；指标
> SQLite 入库（3 天保留自动清理）；仪表盘支持本机/远程节点切换查看。

> v0.9 已交付：安装镜像源选择 + 内置模板扩至 17 个 + 应用源（远程目录）。
> v0.10 已交付：商店拆分——软件商店（原生，下载源可选阿里云/腾讯云/清华/中科大镜像站）与 Docker 容器管理。
> v0.11 已交付：运行时版本管理器（宝塔式）——Node.js/Golang/Python/OpenJDK 全版本清单（镜像站拉取）、多版本共存、设默认、卸载。

> v0.14 已交付：Python pypi 源管理（清华/阿里云/中科大/华为云/腾讯云/豆瓣/官方，经 pip3 config 写入）。
> v0.15 已交付：OpenJDK 多源（清华 Temurin + 华为云通用构建，均实测目录可达），源架构按发行版可扩展。

> v0.16 已交付：依赖漏洞修复（go-jose/quic-go/echarts 升级 + Go 工具链 1.26.10，govulncheck 清零）。
> v0.17 已交付：软件商店新增「系统包」安装形态——Nginx/MySQL(MariaDB)/PHP-FPM/Redis/PostgreSQL/Memcached 走 apt/dnf，与二进制应用统一管理界面。

> v0.20 已交付：PHP 全历史版本（5.6→8.5，17 分支）+ Composer 安装/源管理 + PECL 源配置。

> v0.24 已交付：软件商店全量版本选择——所有原生应用（Caddy/node_exporter/FRP）拉取 GitHub Releases 全版本清单（10 分钟缓存），Nginx 支持 nginx.org stable/mainline 源，PostgreSQL 支持 PGDG 13-17，安装请求携带 version 字段按版本下载/建仓库；原生压缩包路径支持 {ver} 占位；版本化下载同样走 GitHub 加速通道。真实服务器验收：PG16（PGDG）与 node_exporter 1.12.1（非默认版本）均安装成功且 systemd active。

> v0.24.5 已交付：MariaDB/Redis 具体版本安装（阿里云镜像仓库/packages.redis.io 官方仓库全历史版本）；修复并发安装 apt 锁竞争与失败退出码被吞；真实服务器验收 Redis 7.4.12（redis-server v=7.4.12）与 MariaDB 11.8（11.8.9，阿里云镜像）安装成功。

> v0.24.4 已交付：Nginx 安装到具体版本——nginx.org pool 目录解析（发行版代号+架构过滤、版本降序、修订去重），指定版本经 pool deb 直装（apt 本地包），真实服务器验收 1.26.3 安装成功且 nginx -v 确认。

> v0.24.3 已交付：系统包应用镜像站/官方源选择——apt 临时 sources.list 定向安装（不污染系统源），Ubuntu/Debian 阿里云/腾讯云/清华/中科大四镜像实测；PGDG 仓库支持阿里云/腾讯云镜像（其余回退官方并提示），nginx.org 仓库无镜像收录自动回退。

> v0.26 已交付：Docker 容器管理补全（容器/镜像/统计/Compose 编辑/自定义部署）、应用版本升级（含实例版本记录）、数据库即时备份、面板数据一键备份、反代 WebSocket 头修复、仪表盘远程节点支持（summary ?node=）、CNB CI 流水线。

> v0.27 已交付：证书 DNS-01（阿里云/DNSPod/Cloudflare，泛域名 + 凭据随续期）、证书一键部署到站点（域名匹配校验）、计划任务按节点下发（shell 类经 Agent 通道）、告警规则按节点判定。

> v0.28 已交付：节点指标小时聚合（月视图 API）、数据库备份恢复（整库覆盖 force 确认）、终端命令敏感参数掩码、审计导出 CSV、密码强度策略、通知渠道 Server酱/Bark。

> v0.29 已交付：登录 2FA（TOTP RFC 6238 手写实现，免新依赖）——设置页绑定/解除（解除需密码），登录第二阶段验证码（need_2fa 流程）。

> v0.30 已交付：Telegram Bot 通知（Bot Token + Chat ID）、节点指标月视图前端（小时聚合图表切换）。

> v0.31 已交付：节点文件管理与终端（agent 协议下沉——file 消息四操作 + bash 流式终端；面板 file API 与 /api/ws/nodeterm WS 桥；Nodes 页文件/终端抽屉）。

> v0.32 已交付：在线会话管理（列表/踢出其他设备）、echarts 按需加载（渲染器/charts/components 分离注册）。

> v0.33 已交付：文件分片上传（5MB/片，失败重传单片）、软件商店新增 Uptime-Kuma/Alist/n8n/Gitea 模板。

### P2 收官后展望（未排期）

- 节点指标月级视图前端（v0.30 已交付图表切换，月级导出可扩展）

### 关键里程碑

- **M1（W1 末）**：可登录的面板骨架
- **M2（W3 末）**：计划任务模块完整可用（本规划重点）
- **M3（W5 末）**：**MVP v0.1.0 上线真实服务器**
- **M4（W11 末）**：**v1.0.0 全功能发布**

---

## 8. 风险与对策

| 风险 | 对策 |
|---|---|
| 发行版差异（包管理器/防火墙/路径） | 抽象 `distro` 检测层，命令走 execx 白名单模板；CI 矩阵跑 Debian/Ubuntu/Rocky 容器 |
| Nginx 配置生成写坏导致站点不可用 | 每次写入前 `nginx -t` 校验，失败不落盘并保留上一版可一键回滚 |
| Web 终端/文件管理的安全暴露 | 终端会话强鉴权+超时+审计；文件操作限制在白名单根目录，防路径穿越 |
| 面板以 root 运行的风险 | execx 全量审计、权限点化、安全入口、防爆破；文档提供 127.0.0.1+SSH 隧道模式 |
| 单人开发进度风险 | MVP 范围严格冻结（P0 清单外一律进 P1/P2）；每周里程碑可交付 |
| SQLite 并发写 | 面板自身写入频率低，开 WAL；执行历史异步批量写 |

---

## 9. 下一步（立即可做）

1. `git init` + Go module 初始化 + Makefile/CI
2. 按 §6 结构搭骨架，先跑通「登录 → 仪表盘空页」
3. 直接进入 W1 任务清单

---

*本文档为 P0/P1 阶段执行依据；模块设计在开发前如有变更，先改本文档再动代码。*
