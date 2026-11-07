# webmblx — Linux Web 管理面板

Go + Vue3 的全功能 Linux 服务器管理面板，编译为**单个二进制**（前端已嵌入），零运行时依赖，低内存占用。

> 规划文档见 [PLAN.md](PLAN.md)（技术选型、架构、模块设计、排期）。

## 功能（P0 MVP）

| 模块 | 说明 |
|---|---|
| 🔐 认证安全 | 安全入口路径、登录防爆破（15 分钟 5 次锁定）、会话管理、HTTPS（自签/自定义证书） |
| 📊 仪表盘 | CPU/内存/网络实时曲线（3s 采样）、负载、磁盘用量、Top 进程、系统信息 |
| ⏰ 计划任务 | **内置调度引擎**（robfig/cron），Shell 脚本/目录备份/访问 URL 三类任务，执行历史、手动执行、终止运行实例、连续失败告警、90 天历史自动清理；不写系统 crontab |
| 📁 文件管理 | 浏览、上传下载、在线编辑、新建/重命名/移动、权限修改、zip/tar.gz 压缩解压、回收站删除 |
| 🖥 Web 终端 | xterm.js + PTY，多会话、自适应尺寸（仅 Linux 部署时可用） |
| ⚙️ 进程管理 | 进程列表（CPU/内存排序）、结束/强杀进程、systemd 服务启停/自启管理 |
| 🧾 审计 | 全部写操作 API 审计入库存查，命令执行统一走白名单执行层并记录日志 |

P1 规划（网站/反代/SSL、防火墙、数据库、备份策略、通知渠道）见 PLAN.md。

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
go run ./cmd/server          # 生成 config.yaml（含随机入口）与初始密码

# 前端（热更新，代理到 :8443）
cd web && npm install && npm run dev
```

### 构建

```bash
make linux        # Linux amd64 单二进制 → bin/webmblx（先自动构建前端）
make linux-arm64  # arm64
make release      # 打包 tar.gz
```

> 纯 Go SQLite 驱动，`CGO_ENABLED=0` 交叉编译无障碍。

## 首次启动发生了什么

1. 生成 `config.yaml`：监听 `:8443`，随机 **安全入口**（如 `/s/a84e491e`）
2. 创建 SQLite 数据库 `data/panel.db`
3. 创建管理员 `admin`，随机密码写入 `data/initial_admin.txt` 并打印
4. **不带入口串的任何请求一律 404**，只有访问过 `/s/<入口串>` 才能进入面板

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
