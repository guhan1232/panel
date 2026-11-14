# 寒语馨面板 · Hanyuxin Panel

> 单二进制 Go + Vue3 的 Linux 服务器管理面板：一套面板覆盖网站、数据库、Docker、终端、证书、防火墙与多节点运维，配套 Android APP 随身管理。默认 HTTPS、安全入口保护、全量写操作审计，小内存服务器开箱即轻量。

![Version](https://img.shields.io/badge/version-0.41.0-blue) ![Go](https://img.shields.io/badge/Go-1.26-00ADD8) ![License](https://img.shields.io/badge/license-MIT-green)

**仓库**：<https://cnb.cool/hanyuxin/panel> · 文档：[开放 API](docs/API.md) · [MCP](docs/MCP.md) · [脚本应用](docs/SCRIPT-REPO.md) · [APP 设计](docs/APP.md)

---

## 一键安装

```bash
curl -fsSL https://cnb.cool/hanyuxin/panel/-/raw/main/scripts/install.sh | bash
```

安装完成输出访问地址（`https://<IP>:<端口>/s/<安全入口>`）与初始凭据位置。默认 HTTPS（自签 10 年含公网 IP SAN）；可选 `--ca-cert/--ca-key` 用自有中间 CA 签发（有效期跟随 CA）。

| 命令 | 说明 |
|---|---|
| `bash install.sh --upgrade` | 升级（自动备份旧二进制，配置与数据不动，可一键回滚） |
| `bash install.sh --force` | 跳过已安装检查强制重装 |
| `sudo bash scripts/uninstall.sh` | 卸载（`--keep-data` 保留数据） |

**系统要求**：Linux x86_64 / arm64；Debian 10+ / Ubuntu 20.04+ / RHEL 系。

## 轻量化部署（开箱即省内存）

针对小内存服务器（≤4GB）自动生效，**核心功能优先：全部失败不阻断安装、独立配置文件、可随时回滚**：

| 项目 | 做法 |
|---|---|
| 面板进程 | systemd 注入 `GOGC=50` + `GOMEMLIMIT`，常驻内存约 44MB |
| journald | 日志上限 30M（磁盘）/ 15M（内存），功能不变 |
| Swap | 无 swap 的 ≤4GB 机器自动创建 2G（swappiness=10，OOM 兜底） |
| MySQL/MariaDB | 面板内安装后自适应：≤2GB → buffer_pool 32M + performance_schema 关；2-4GB → 128M；>4GB 机器不套用 |
| PHP-FPM | 面板内安装后自动 `pm=ondemand`（worker 零常驻，空闲 10s 回收） |

实测 1.9GB VPS：整机内存占用从 55% 降至 ~41%，面板本体常驻 44MB。所有轻量配置写入独立文件（如 `zz-hanyuxin-light.cnf`），删除并重启对应服务即恢复发行版默认。

## 功能总览

### 站点与服务

| 模块 | 说明 |
|---|---|
| 🌐 网站 | Nginx 站点管理，按项目类型分类（HTML/PHP/反向代理/Java/Node/Go/Python/.Net/其他）；域名绑定、SSL 托管、强制 HTTPS；nginx -t 写前校验失败自动回滚 |
| 🗄 数据库 | MySQL/PostgreSQL 建库建用户授权；MySQL 一键安装/卸载与 root 认证自动修复；dump 备份/恢复/下载 |
| 🛒 软件商店 | 容器编排模板一键部署；原生二进制与系统包（多镜像源）；脚本应用仓库（一键安装脚本 + 执行留痕） |
| 🐳 Docker | 总览/容器/镜像/编排/网络/存储卷/仓库/设置等 11 页签；Docker 一键安装；镜像搜索拉取 |
| 🖥 终端 | 多标签 SSH/本机 Shell；主机收藏（AES-GCM 加密凭据）；五主题；空闲自动断开；选中即复制 |

### 系统运维

| 模块 | 说明 |
|---|---|
| ⏰ 计划任务 | 内置调度引擎，脚本/备份/探测任务与执行历史 |
| 🔔 告警策略 | CPU/内存/负载阈值规则（新建/编辑/启停/删除）与触发历史 |
| 📁 文件管理 | 浏览、上传下载、在线编辑、压缩解压、权限属主修改 |
| ⚙ 进程管理 | 进程与 systemd 服务实时状态与控制 |
| 🖧 节点管理 | Agent 反连接入、远程指标与命令执行 |
| 🧱 防火墙 | ufw/firewalld 一键安装（防自锁）；端口规则、防扫描、导入导出、同步重放 |
| 🔒 SSL 证书 | 上传 / Let's Encrypt HTTP-01·DNS-01 签发与自动续期 / 19 家 DNS 服务商；一键部署到网站 |
| 📊 通知与存储 | SMTP / IM Webhook（钉钉/企微/飞书/Server酱/Bark/Telegram）测试发送；S3 兼容对象存储备份 |
| 🧾 审计日志 | 全部写操作与终端命令留痕，筛选/导出/分页，敏感参数自动掩码 |
| 👥 用户管理 | 三角角色 RBAC（管理员/操作员/只读），禁用登录，禁止改自身 |

### 面板

| 模块 | 说明 |
|---|---|
| 🔐 HTTPS 与域名 | 中间 CA / Let's Encrypt（自动续期）/ 自签 10 年兜底；一键 NGINX 域名反代挂证书 |
| 🏷 面板名称 | 自定义品牌名，登录页/侧栏/APP 全端联动 |
| 💾 备份与恢复 | 面板数据导出/导入（恢复前自动留存回滚点）；站点数据 tar.gz 备份 |
| 🔓 开放 API 与 MCP | API Token + MCP Server（AI 客户端管理面板），按模块授权 |
| 📱 两因素验证 | TOTP（RFC 6238）；登录锁定防爆破 |
| 💻 会话管理 | 在线设备列表、单设备踢出、一键退出其他设备 |
| ⚙ 系统设置 | 面板名称 / 改密 / 开放 API Token（生成/禁用） |
| 🌍 界面 | 中/英双语、亮暗主题、青绿主色设计体系（1Panel 风格卡片质感） |

## 移动 APP（Android）

原生 Kotlin + Compose（无 GMS 依赖），与 Web 端**功能全量对齐**：

- 五大导航（概览/服务/运维/文件/我的）+ 全功能页：审计日志、通知与存储、用户管理、系统设置、告警规则 CRUD、计划任务、防火墙、SSL、节点管理、Docker、终端（WS 行模式）等
- 登录方式：扫码绑定（Web 登录页二维码）/ `hanyuxin://` 深链 / 账号密码；30 天滑动续期
- 安全：生物识别锁、自签证书 TOFU 指纹锁定、401 统一处理
- **升级继承数据**：正式签名覆盖安装自动继承服务器列表/令牌/绑定关系；APP 内置检查更新（版本元数据由面板下发）

构建：`app-android/` 目录（Gradle 8.10 + AGP 8.7 + Compose BOM 2024.10），`assembleRelease` 需根目录 `keystore.properties` 指向签名密钥。设计文档见 [docs/APP.md](docs/APP.md)。

## 安全设计

- **安全入口**：所有页面/接口需经入口 Cookie，未携带时 302 引导（防扫描爆破）
- **登录锁定 + TOTP 两因素**
- **全量审计**：写操作留痕、终端命令敏感参数自动掩码
- **凭据加密**：SSH 主机凭据 AES-GCM 加密落库（密钥独立于数据库）
- **防自锁**：防火墙安装/站点启用自动放行 SSH 与面板端口

## 从源码构建

前端产物经 `go:embed` 嵌入二进制，先构建前端再编译 Go：

```bash
make build-web       # 前端（npm install + vite build → web/dist）
make build           # 本机构建 bin/hanyuxin(.exe)
make linux           # Linux amd64（前端 + 交叉编译一步完成）
make linux-arm64     # Linux arm64
make agent           # 多节点 Agent（amd64）
make release         # 全平台构建并打 tar.gz 到 bin/
make vet             # go vet 全量检查
```

## 仓库结构

| 路径 | 说明 |
|---|---|
| cmd/server | 面板主程序 |
| cmd/agent | 多节点 Agent |
| internal/modules | 后端业务模块（网站/数据库/Docker/终端/防火墙/证书/审计/通知/用户…） |
| web/ | Vue3 前端（go:embed 嵌入） |
| app-android/ | Android APP（Kotlin + Compose） |
| scripts/ | install.sh（一键安装+轻量化）/ uninstall.sh / smoke.sh（冒烟） |
| jiaoben/ | 脚本应用仓库（规范见 docs/SCRIPT-REPO.md） |
| docs/ | 开发文档（API / MCP / SCRIPT-REPO / APP） |

## 冒烟测试

```bash
PANEL_BASE=https://<IP>:<端口> PANEL_ENTRANCE=<安全入口> \
PANEL_USER=<用户> PANEL_PASS=<密码> bash scripts/smoke.sh
```

51 项打点（核心 API + 扫码登录与 APP 会话全流程），判定标准：HTTP 200 且业务 `code==0`。

## License

[MIT](LICENSE)
