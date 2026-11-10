# 寒语馨面板 · 功能补齐与优化清单

> 基于全代码审计（2026-11-09，Go 约 10.4K 行 + 18 个前端视图），对照 `PLAN.md` 规划、`README.md` 宣称与各模块实际实现逐项核对得出。
> 优先级：**P0** = 立即处理（缺陷/一致性）；**P1** = 功能补齐（高价值）；**P2** = 优化（质量/体验）。

---

> **命名已拍板（2026-11-09）：寒语馨面板（Hanyuxin Panel）**。改名已由后续操作基本完成，收尾：TODO/文档标题同步、web/dist 重建、服务器二进制与单元迁移见各提交。

---

## P0 · 立即处理

### P0-1 文档宣称与实现不符（影响可信度）

| 宣称 | 实际 | 处理建议 |
|---|---|---|
| README：「Docker 容器管理：容器/镜像/网络/卷全生命周期管理」 | `DockerStore.vue` 仅包含：Docker 安装、daemon.json 镜像源配置、**Compose 应用**（商店模板）的安装/启停/日志/卸载。无 `docker ps / images / network / volume` 任何容器操作 | 补齐功能（见 P1-1），或先改文档为「Docker 环境 + Compose 应用管理」 |
| README / PLAN：「仪表盘支持本机/远程节点切换」 | `dashboard.go` 无任何 node 维度代码；`node_metrics` 表有数据但仪表盘不读 | 补齐（见 P1-6），或改文档 |

### P0-2 改名残留清查（命名拍板后执行）

- `web/dist/` 构建产物当前处于「旧产物已删、新产物未构建」的中间态，命名拍板后需 `npm run build` 重新嵌入。
- 全仓 `grep -i hanyuxin|digong` 逐文件核对（Go / Vue / sh / service / Makefile / PLAN / README），确保只剩一个命名。
- 自签证书 CN、`User-Agent`、`daemon.json` 备份后缀、apt 临时源文件名等隐蔽处易漏（上轮改名即漏了 `hanyuxin-mirror.list`、`hanyuxin-nginx-*`、`hanyuxin.bak` 三类）。

---

## P1 · 功能补齐（按价值排序）

### P1-1 Docker 容器管理补全（缺口最大）

现状只有 Docker 安装 + 镜像源 + Compose 商店应用。补齐为真正的容器管理页：

- [ ] 容器列表（`docker ps -a`）：状态/端口/镜像/资源占用，启停/重启/删除
- [x] 容器日志在线查看（v0.26）（`docker logs --tail`，复用 Compose 应用日志的交互）
- [x] 容器资源统计（v0.26）（`docker stats --no-stream`）
- [ ] 镜像列表（`docker images`）与清理（`docker image prune` / `system prune`，危险操作需确认 + 审计）
- [ ] 网络/卷列表（只读展示起步）
- [x] Compose 应用 YAML **在线编辑** + 自定义部署（v0.26） + `docker compose up -d` 应用变更
- [x] **自定义 Compose 应用**（v0.26）：用户粘贴 YAML 部署（当前只能装商店模板，无法部署自己的编排）

### P1-2 已安装应用的版本升级 / 回滚

版本选择目前只在**安装时**生效；装完即固定。补齐：

- [ ] 应用详情页展示当前版本（`nginx -v` / `redis-server --version` / systemd 单元版本探测）
- [x] 「升级到指定版本」动作（v0.26）：native 换 GitHub Release 下载重装；package 走对应仓库换版本（复用 PkgInstallVer 分发）；升级前提示影响（nginx 替换式升级会短暂停服）
- [ ] 版本历史记录（app_insts 增 version 字段 + 升级流水）

### P1-3 SSL 证书与网站联动（证书最常用的闭环）

- [x] **DNS-01 挑战**（v0.27：阿里云/DNSPod/Cloudflare，凭据写入 meta 支持自动续期，泛域名可用）
- [x] **证书一键部署到站点**（v0.27：域名匹配校验 + 强制 HTTPS + nginx reload）：证书列表「部署到网站」→ 自动写入站点 `ssl_cert/ssl_key` + `force_https` + `nginx -s reload`（当前 Website.vue 仅手填路径，路径提示还是旧前缀）
- [ ] 证书续期后自动 reload nginx（当前只续期不重载）

### P1-4 网站模块增强

- [ ] **反代 WebSocket 升级头**：模板缺 `proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade";`——`proxy_http_version 1.1` 已有但无升级头，反代 WS 应用（含反代本面板）必失败；子 location 还缺 `X-Forwarded-Proto`
- [ ] PHP 站点按站点选择 PHP 版本（当前固定 `/run/php/php-fpm.sock`，多版本 runtime 装好后应可选 `php8.2-fpm.sock` / `php8.3-fpm.sock`）
- [ ] 进阶：自定义 nginx 配置片段（每站点追加 location / 头），模板生成 + 片段拼接，保留 `nginx -t` 校验回滚机制

### P1-5 数据库管理增强

- [ ] **即时备份/恢复**：库/实例级一键 dump（复用 cron `backup_db` 的逻辑）+ 备份文件列表 + 下载/删除
- [ ] 备份文件上传对象存储（S3 通道 cron 里已有，UI 补入口）
- [ ] Redis 键管理：Redis 可装但无任何管理界面（键浏览/TTL/删除，起步只做只读 + 危险操作确认）

### P1-6 多节点能力下沉（PLAN「P2 收官后展望」三项）

Agent 已有 `metric` / `exec_result` 双消息通道，扩展消息类型即可：

- [x] **计划任务按节点下发**（v0.27：shell 类任务可选节点，经 ExecOnNode 通道）
- [x] **告警规则按节点**（v0.27：规则绑定节点，5 分钟内最新指标判定，通知带节点名）
- [x] **节点文件管理与终端**（v0.31：agent file 消息 ls/read/write/delete ≤8MB + bash 会话流式转发；面板文件 API、终端 WS 桥 /api/ws/nodeterm/:id、Nodes 页文件抽屉与 xterm 终端抽屉）
- [x] **节点指标小时聚合**（v0.28：NodeMetricHourly 表随清理任务增量聚合，月视图 API /nodes/:id/metrics/hourly）

### P1-7 面板自身运维

- [x] **面板数据一键备份**（v0.26；恢复流程见下载包说明，自动恢复待做）：panel.db + config.yaml + certs-managed 打包下载 / 上传恢复（恢复后自动重启）
- [ ] **在线自升级**：PLAN 里程碑已列（版本号 + 校验和 + 升级前自动备份面板数据）；下载新二进制 → 校验 → 替换 → 重启服务，失败自动回滚
- [x] 审计日志导出 CSV（v0.28：按筛选条件全量导出，UTF-8 BOM）

---

## P2 · 优化项

| # | 项 | 说明 |
|---|---|---|
| 1 | CI/CD | CNB 流水线：`go test ./...` + `go vet` + `govulncheck` + 前端 build + Linux 双架构 release 产物；当前无任何 CI |
| 2 | 测试覆盖 | 仅 `appstore` / `runtime` 两个 `_test.go`；优先补纯逻辑：`auth`（防爆破窗口）、`audit`（脱敏/保留清理）、`cron`（参数解析）、`archive`、`execx` |
| 3 | 通知渠道扩展 | 现有 SMTP + IM Webhook（钉钉/企微/飞书/通用）；补 Server酱 / Bark / Telegram Bot |
| 4 | 登录 2FA | TOTP（兼容 Google Authenticator）+ 在线会话列表（查看/踢出其他会话） |
| 5 | 文件管理 | 大文件分片上传（当前 `SaveUploadedFile` 一次读全量）、回收站、目录内容搜索 |
| 6 | 终端审计脱敏 | 终端命令记录含明文密码的命令（`mysql -p***`、`export TOKEN=...`）应掩码，与操作日志 `sanitizeBody` 对齐 |
| 7 | 前端体积 | `index-*.js` 约 1.6MB（echarts 全量）；按需引入/分包，路由级懒加载已具备 |
| 8 | 审计大表体验 | 操作日志 180 天保留数据量大时，表格加时间范围筛选默认值 + 虚拟滚动 |
| 9 | i18n | 中文界面骨架已定，抽英文语言包（面向海外发布时） |
| 10 | 移动端 | 侧栏抽屉已有，表格/弹窗在小屏的适配细节 |
| 11 | 软件商店模板扩展 | 常用应用继续扩充：Uptime-Kuma、Alist、Halo、n8n、Gitea（compose/native 混合） |
| 12 | 密码强度策略 | 修改管理员密码时校验长度/复杂度，防弱口令 |

---

> v0.35.4 修复：SPA 页面（刷新/直达路径）在无入口 Cookie 时 302 引导至安全入口（替代 404 断裂）；显式 WriteHeader(200) 根治 SPA fallback 返回 404 头的问题。

> v0.35 已交付：SSH 初始凭据一次性输出——首次启动打印/写入凭据文件仅此一次，用户在面板修改密码后凭据文件自动删除；系统设置新增「账号信息」卡片支持修改登录用户名（唯一性校验）。

## 已确认交付能力（审计对照基线）

| 模块 | 已实现 |
|---|---|
| 仪表盘 | 本机 3s 采样、磁盘、Top 进程、实时曲线 |
| 网站 | 静态/PHP/反代模板、nginx -t 校验回滚、日志查看、HTTPS 站点字段 |
| 数据库 | MySQL/PG 建库授权、标识符校验 |
| 文件 | 浏览/上传/下载/编辑/压缩/权限 |
| 终端 | WS 交互 + 命令行审计记录 |
| 计划任务 | 6 类任务（shell/backup_dir/visit_url/backup_db/log_rotate/sys_update）+ S3 上传 + 失败告警 |
| 防火墙 | ufw/firewalld 端口规则 |
| 证书 | HTTP-01 签发、上传、到期监控自动续期 |
| 软件商店 | 3 形态安装、全应用版本选择（nginx pool/PGDG/MariaDB 阿里云/Redis 官方仓库/GitHub Releases）、镜像站定向安装、应用源目录 |
| 运行时 | Node/Go/Python/OpenJDK/PHP 多版本（宝塔式） |
| Docker | 安装 + daemon.json 镜像源 + Compose 商店应用管理 |
| 告警 | 阈值规则（持续+冷却）→ 通知渠道（SMTP + IM Webhook） |
| 多节点 | Agent 反连、指标入库 3 天、远程命令、图表 |
| 审计 | 写操作全量审计（脱敏）、终端命令日志、180 天保留 |
| 认证 | 入口路径、防爆破（15min/5 次）、RBAC 三级角色、随机初始管理员 |

> 维护提示：P1 各项落地后请同步更新本清单勾选状态与 PLAN.md 的「已交付」记录。
