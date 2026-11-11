# 寒语馨面板（Hanyuxin Panel）

**半成品的服务器管理面板** 

> 在线演示：http://panel.hanyuxin.cn （演示环境，数据定期重置，仅开放只读体验）
>
> 仓库：https://cnb.cool/hanyuxin/panel · 规划文档见 [PLAN.md](PLAN.md) · 待办清单见 [TODO.md](TODO.md) · 部署验收手册见 [DEPLOY.md](DEPLOY.md)

---

## 功能总览

### 🖥 基础能力

| 模块 | 说明 |
|---|---|
| 📊 监控仪表盘 | CPU/内存/负载/网络 3 秒采样、磁盘挂载点用量、进程占用 Top、CPU/内存/网络实时曲线 |
| 🌐 网站 | Nginx 站点管理：静态 / PHP / 反向代理，模板生成配置、`nginx -t` 校验失败自动回滚；访问/错误日志在线查看 |
| 🗄 数据库 | MySQL(MariaDB) / PostgreSQL 库表管理：建库、授权、账号管理，标识符防注入校验 |
| 📁 文件管理 | 浏览、上传、下载、编辑、压缩/解压、权限/属主修改 |
| ⌨️ Web 终端 | 浏览器内交互式 Shell（WebSocket），命令行记录进终端审计日志 |
| 🕒 计划任务 | 内置调度引擎（不写系统 crontab）：Shell / 备份网站 / 备份数据库 / 日志切割 / 系统更新等六类任务，执行历史、手动执行、终止运行实例、连续失败告警 |
| 🔥 防火墙 | ufw / firewalld 双适配：端口放行、规则增删 |
| 🔒 SSL 证书 | lego ACME 签发（Let's Encrypt HTTP-01）、证书上传、到期监控与自动续期 |

### 📦 软件商店（三大形态）

| 形态 | 说明 |
|---|---|
| 系统包（package） | Nginx / MySQL(MariaDB) / PostgreSQL / Redis / Memcached 走 apt/dnf，与二进制应用统一管理界面 |
| 二进制（native） | Caddy / MinIO / node_exporter / FRP 服务端：自动按架构下载官方发行包 → 生成 systemd 单元（`hanyuxin-app-*`）→ 启动健康检查，日志走 journalctl |
| Compose（docker） | Nginx / MySQL / PostgreSQL / Redis / RabbitMQ 等，Docker Compose 拉起 |

**版本选择**（全部可装到具体版本）：

- **Nginx**：nginx.org pool 全部历史稳定版（1.20.2→1.30.6），pool deb 直装
- **PostgreSQL**：PGDG 大版本 13-17（apt/dnf 自动建仓库 + initdb）
- **MariaDB**：阿里云 MariaDB 镜像仓库全版本（稳定分支 / 滚动分支 / 指定小版本）
- **Redis**：packages.redis.io 官方仓库全历史（6.0→最新），精确版本安装并锁定依赖
- **原生应用**：Caddy / node_exporter / FRP 拉 GitHub Releases 全版本清单（10 分钟缓存）

**下载源**（镜像站/官方可选）：

- 原生应用：自动（GitHub 加速）/ 官方直连 / ghfast / gh-proxy / 阿里云 / 腾讯云 / 清华 / 中科大
- 系统包：apt 经**临时 sources.list 定向安装**（不改动系统全局源，装完清理），Ubuntu/Debian 四家镜像站实测可达
- Docker Compose：镜像名自动改写为加速代理仓库

**运行时环境**（宝塔式多版本管理）：Node.js / Golang / Python / OpenJDK / PHP 全版本清单（官方 index.json + 阿里云/清华/搜狐/华为云镜像目录解析）、多版本共存、设默认、卸载；PHP 各版本独立 php-fpm 服务（Debian/Ubuntu 经 ondrej 仓库，RHEL 经 Remi）；Composer 安装与镜像源管理（阿里云/腾讯云/华为云/Packagist CN）、PECL 源配置

### 🐳 Docker 容器管理

容器列表与启停/重启/删除、容器日志在线查看、资源统计快照、镜像列表与未使用镜像清理、**Compose 编排在线编辑**（保存即 up -d）、**自定义 Compose 应用部署**；**Docker 镜像源配置**（daemon.json 读写、备份回滚）：DaoCloud / 毫秒镜像等加速源可视化配置（网络/卷管理见 TODO.md）

### 🛡 面板能力

| 模块 | 说明 |
|---|---|
| 👥 多用户 RBAC | admin / operator / viewer 三级角色，用户启停管理；初始管理员账号随机生成 |
| 📜 审计日志 | 全部写操作中间件级审计（请求体敏感键脱敏）、60+ 路由中文描述、终端命令逐行记录、分页查询 |
| 🔔 告警策略 | 阈值规则引擎（CPU/内存/磁盘，持续时间+冷却），触发写入通知渠道 |
| 📮 通知与存储 | Webhook/自定义渠道通知测试；对象存储对接 |
| 🖧 多节点 Agent | 独立 `hanyuxin-agent` 二进制经 **WebSocket 反连**（token 鉴权，穿透 NAT 无需开放被控端口），15s 上报指标；接入命令一键复制、token 重置、指标图表、远程命令执行（operator+，全程审计）；指标 SQLite 入库 3 天保留，仪表盘支持本机/远程节点切换 |
| 🔐 安全 | 随机安全入口路径（`/s/<随机>`）、登录防爆破、HTTPS 自签证书可选、随机端口（10000-30000） |
| 🏪 应用源 | 远程应用目录（JSON 清单）拉取校验合并，与内置模板去重展示 |

---

## 快速开始

### 在线演示

演示地址：<http://panel.hanyuxin.cn>（预留演示环境，如遇重置或维护请稍后再试）

### 安装（Linux 服务器）

```bash
# 下载发布包（或从源码构建）
tar -xzf hanyuxin-linux-amd64.tar.gz
sudo bash scripts/install.sh ./hanyuxin

# 查看初始管理员账号
sudo cat /opt/hanyuxin/data/initial_admin.txt

# 浏览器访问（端口与入口路径随机生成）
# http://<服务器IP>:<端口>/s/<安全入口>
```

服务管理：`systemctl {status|restart|stop} hanyuxin`；卸载：`sudo bash scripts/uninstall.sh [--keep-data]`

### 本地开发

```bash
# 后端（Go 1.26+，工具链在 go.mod 固定）
go build ./...

# 前端（Vue3 + Ant Design 兼容层）
cd web && npm install && npm run dev
```

### 构建

```bash
make linux        # Linux amd64 单二进制 → bin/hanyuxin（先自动构建前端）
make linux-arm64  # Linux arm64
make agent        # hanyuxin-agent（多节点 Agent）
make release      # 全平台打包
```

---

## 架构

```
┌─────────────── hanyuxin 二进制 (root) ─────────────┐
│  Gin HTTP / WebSocket                              │
│  ├─ dashboard  3s 采样器 + 节点指标                  │
│  ├─ website    Nginx 站点模板 + 校验回滚             │
│  ├─ dbmgr      MySQL / PostgreSQL 管理              │
│  ├─ filem      文件管理                              │
│  ├─ term       Web 终端（WS + 命令审计）             │
│  ├─ cron       内置调度引擎（不写 crontab）          │
│  ├─ firewall   ufw / firewalld 抽象                 │
│  ├─ cert       lego ACME + 到期监控                 │
│  ├─ appstore   软件商店（package/native/compose）    │
│  ├─ runtime    多版本运行时管理器                    │
│  ├─ docker     容器管理 + 镜像源配置                 │
│  ├─ alert      告警策略引擎                          │
│  ├─ nodes      多节点 Agent 反连接入                 │
│  ├─ audit      操作审计 + 终端命令日志               │
│  └─ auth       用户/会话/防爆破/RBAC                 │
│  SQLite（GORM，单文件存储）                          │
│  前端 Vue3 + Ant Design 兼容层（go:embed 嵌入）      │
└────────────────────────────────────────────────────┘
```

## 首次启动发生了什么

1. 自动生成随机管理员账号（写入 `data/initial_admin.txt`，请立即改密）
2. 随机监听端口（10000-30000，重启保持不变）+ 随机安全入口路径
3. 初始化 SQLite 数据库与默认配置

## 安全说明

- 面板以 root 运行（需管理系统资源），安全依赖：入口路径防护、会话超时、登录防爆破、全量操作审计
- 建议尽快启用 HTTPS（config.yaml 中 `server.tls`）并修改初始密码
- 所有写操作（安装、配置变更、命令执行）均记录审计日志

## 目录结构

```
hanyuxin/
├── cmd/server/          面板主程序
├── cmd/agent/           多节点 Agent
├── internal/core/       数据库模型/启动引导/响应封装
├── internal/modules/    dashboard/website/dbmgr/filem/term/cron/firewall/cert/appstore/runtime/docker/alert/nodes
├── internal/audit/      审计中间件
├── internal/execx/      统一命令执行层（白名单+超时+审计）
├── web/                 Vue3 前端（构建产物 go:embed）
├── scripts/             install.sh / uninstall.sh
└── deploy/hanyuxin.service
```

## License

[MIT](LICENSE)
