# 寒语馨面板 · Hanyuxin Panel

> 单二进制 Go + Vue3 的 Linux 服务器管理面板。默认 HTTPS、安全入口保护、全量写操作审计。

![Version](https://img.shields.io/badge/version-0.39.8-blue) ![Go](https://img.shields.io/badge/Go-1.26-00ADD8)

仓库：https://cnb.cool/hanyuxin/panel · 规划文档 [PLAN.md](PLAN.md) · 部署手册 [DEPLOY.md](DEPLOY.md) · 脚本开发 [docs/SCRIPT-REPO.md](docs/SCRIPT-REPO.md)

## 快速开始

\
安装完成输出访问地址（https://<IP>:<端口>/s/<安全入口>）与初始凭据位置。默认 HTTPS（自签 10 年含公网 IP SAN）；可选 --ca-cert/--ca-key 用自有中间 CA 签发（有效期跟随 CA）。

**系统要求**：Linux x86_64 / arm64；Debian 10+ / Ubuntu 20.04+ / RHEL 系。

## 功能总览

### 站点与服务

| 模块 | 说明 |
|---|---|
| 🌐 网站 | Nginx 站点管理，按项目类型分类（HTML/PHP/反向代理/Java/Node/Go/Python/.Net/其他 Tab 区分）；域名绑定、SSL 托管证书、强制 HTTPS、访问与错误日志、nginx -t 写前校验失败自动回滚 |
| 🗄 数据库 | MySQL/PostgreSQL 建库建用户授权；MySQL 一键安装（Ubuntu 官方源 / Debian 官方 APT 仓库）与卸载；root 免密认证（Error 1698）自动修复；未安装优雅降级引导；dump 备份/恢复/下载 |
| 🛒 软件商店 | 容器编排模板一键部署；原生二进制与系统包（多镜像源）；脚本应用（仓库托管一键安装脚本 + 自定义上传，执行留痕） |
| 🐳 Docker 容器管理 | 11 页签：应用商店/总览/网站/容器/线上镜像/本地镜像/容器编排/网络/存储卷/仓库/设置；Docker 一键安装（官方脚本+阿里云回退）；镜像搜索拉取（Hub API+内置清单回退） |
| 🖥 终端 | 多标签 SSH/本机 Shell；主机收藏（AES-GCM 加密凭据）、快速临时连接、手动排序、密码/私钥认证；移动端可自定义快捷键条；五主题（暗/亮/Dracula/Nord/Solarized）；空闲提醒自动断开；选中即复制/右键粘贴 |

### 系统运维

| 模块 | 说明 |
|---|---|
| ⏰ 计划任务 | 内置调度引擎，数据库备份/脚本/URL 探测等任务类型与执行历史 |
| 🔔 告警策略 | CPU/内存/负载阈值规则与触发历史 |
| 📁 文件管理 | 浏览、上传下载、编辑、压缩解压、权限属主修改 |
| ⚙ 进程管理 | 进程与 systemd 服务实时状态与控制 |
| 🖧 节点管理 | Agent 反连接入、远程指标与命令执行 |
| 🧱 防火墙 | 一键安装 ufw/firewalld（防自锁放行 SSH/面板端口）；宝塔式端口规则（协议/端口段/来源/策略/方向/备注）；导入导出；端口防扫描；同步重放 |
| 🔒 SSL 证书 | 手动上传；Let's Encrypt HTTP-01/DNS-01 签发与自动续期；19 家 DNS 服务商（阿里云/腾讯云/华为云/百度云/西部数码/Cloudflare/Route53/GoogleCloud/Azure/GoDaddy/Namecheap/OVH/Gandi/Hetzner/Porkbun/DuckDNS/deSEC/ACME-DNS）；一键部署到网站 |

### 面板

| 模块 | 说明 |
|---|---|
| 🔐 HTTPS 加密连接 | 中间 CA 交叉签发（有效期跟随 CA）/ Let's Encrypt 公共免费 CA（域名，自动续期）/ 纯自签 10 年兜底；SAN 仅公网 IP+显式访问地址 |
| 🔗 域名反代 | 一键安装 NGINX，域名反代面板并挂证书（独立于网站站点管理） |
| 💾 备份与恢复 | 面板数据导出/导入恢复（数据库快照+配置+托管证书，按项勾选，恢复前自动留存回滚点，一键回滚）；站点数据备份 tar.gz 导出/导入 |
| 📜 审计日志 | 全部写操作留痕，筛选与导出，终端命令敏感参数自动掩码 |
| 🔓 开放 API 与 MCP | API Token + MCP Server（AI 客户端管理面板），按模块授权 |
| 📱 两因素验证 | TOTP（RFC 6238） |
| 💻 会话管理 | 在线设备列表、单设备踢出、一键退出其他设备 |
| 📨 通知 | SMTP / Webhook / Server酱 / Bark / Telegram |
| 🌍 国际化 | 中/英界面，亮暗主题 |

## 安全设计

- **安全入口**：所有页面/接口需经入口 Cookie，未携带时 302 引导（防扫描爆破）
- **登录锁定**：连续失败触发锁定；两因素验证 TOTP
- **审计**：全部写操作留痕；终端命令敏感参数自动掩码
- **会话**：在线设备管理、单设备踢出、一键退出其他设备
- **凭据加密**：SSH 主机凭据 AES-GCM 加密落库（密钥独立于数据库）
- **防自锁**：防火墙安装/站点启用自动放行 SSH 与面板端口

## 从源码构建

\
> webmblx-web@0.1.0 build
> vite build

[36mvite v5.4.21 [32mbuilding for production...[36m[39m
transforming...
[32m✓[39m 3909 modules transformed.
rendering chunks...
computing gzip size...
[2mdist/[22m[32mindex.html                                [39m[1m[2m    0.79 kB[22m[1m[22m[2m │ gzip:   0.52 kB[22m
[2mdist/[22m[35massets/Website-C7nnjgUP.css               [39m[1m[2m    0.21 kB[22m[1m[22m[2m │ gzip:   0.17 kB[22m
[2mdist/[22m[35massets/SoftStore-DiQgQpLR.css             [39m[1m[2m    0.28 kB[22m[1m[22m[2m │ gzip:   0.20 kB[22m
[2mdist/[22m[35massets/Firewall-BPDdOWDh.css              [39m[1m[2m    0.31 kB[22m[1m[22m[2m │ gzip:   0.22 kB[22m
[2mdist/[22m[35massets/DockerStore-uDuEUFwE.css           [39m[1m[2m    2.34 kB[22m[1m[22m[2m │ gzip:   0.73 kB[22m
[2mdist/[22m[35massets/Terminal-BgwtIXtV.css              [39m[1m[2m    2.83 kB[22m[1m[22m[2m │ gzip:   0.85 kB[22m
[2mdist/[22m[35massets/xterm-DYP7pi_n.css                 [39m[1m[2m    4.15 kB[22m[1m[22m[2m │ gzip:   1.68 kB[22m
[2mdist/[22m[35massets/index-Du2o5W81.css                 [39m[1m[2m   32.37 kB[22m[1m[22m[2m │ gzip:   7.21 kB[22m
[2mdist/[22m[36massets/ThunderboltOutlined-BTqM5vK1.js    [39m[1m[2m    1.05 kB[22m[1m[22m[2m │ gzip:   0.67 kB[22m
[2mdist/[22m[36massets/utils-DrMMF4bf.js                  [39m[1m[2m    1.10 kB[22m[1m[22m[2m │ gzip:   0.65 kB[22m
[2mdist/[22m[36massets/addon-fit-MFGWFLOW.js              [39m[1m[2m    1.78 kB[22m[1m[22m[2m │ gzip:   0.81 kB[22m
[2mdist/[22m[36massets/i18n-Ct_AVZOV.js                   [39m[1m[2m    3.50 kB[22m[1m[22m[2m │ gzip:   2.01 kB[22m
[2mdist/[22m[36massets/Login-CY5LM8sK.js                  [39m[1m[2m    4.21 kB[22m[1m[22m[2m │ gzip:   2.35 kB[22m
[2mdist/[22m[36massets/Users-B5MIiWNC.js                  [39m[1m[2m    4.52 kB[22m[1m[22m[2m │ gzip:   2.15 kB[22m
[2mdist/[22m[36massets/Proc-Cb72k-Lr.js                   [39m[1m[2m    4.55 kB[22m[1m[22m[2m │ gzip:   1.85 kB[22m
[2mdist/[22m[36massets/Audit-BRIphbif.js                  [39m[1m[2m    5.74 kB[22m[1m[22m[2m │ gzip:   2.27 kB[22m
[2mdist/[22m[36massets/Notify-v65tcL-O.js                 [39m[1m[2m    5.92 kB[22m[1m[22m[2m │ gzip:   2.23 kB[22m
[2mdist/[22m[36massets/Alerts-CAMV4nQy.js                 [39m[1m[2m    6.00 kB[22m[1m[22m[2m │ gzip:   2.69 kB[22m
[2mdist/[22m[36massets/Files-CDU6RPjs.js                  [39m[1m[2m    7.05 kB[22m[1m[22m[2m │ gzip:   3.10 kB[22m
[2mdist/[22m[36massets/Certs-DNkIyGD0.js                  [39m[1m[2m    8.10 kB[22m[1m[22m[2m │ gzip:   3.35 kB[22m
[2mdist/[22m[36massets/Firewall-wi2jh0rt.js               [39m[1m[2m    9.10 kB[22m[1m[22m[2m │ gzip:   4.00 kB[22m
[2mdist/[22m[36massets/Cron-F1HJwQGZ.js                   [39m[1m[2m   11.95 kB[22m[1m[22m[2m │ gzip:   4.80 kB[22m
[2mdist/[22m[36massets/Databases-CPFNULKB.js              [39m[1m[2m   13.05 kB[22m[1m[22m[2m │ gzip:   4.39 kB[22m
[2mdist/[22m[36massets/Website-BK3jC2jm.js                [39m[1m[2m   16.15 kB[22m[1m[22m[2m │ gzip:   6.16 kB[22m
[2mdist/[22m[36massets/Nodes-CHpdI6-8.js                  [39m[1m[2m   16.97 kB[22m[1m[22m[2m │ gzip:   6.77 kB[22m
[2mdist/[22m[36massets/Terminal-DHvIqdMr.js               [39m[1m[2m   21.28 kB[22m[1m[22m[2m │ gzip:   8.94 kB[22m
[2mdist/[22m[36massets/Setting--zXdEq9L.js                [39m[1m[2m   25.43 kB[22m[1m[22m[2m │ gzip:  10.26 kB[22m
[2mdist/[22m[36massets/SoftStore-FDnKTKGL.js              [39m[1m[2m   26.98 kB[22m[1m[22m[2m │ gzip:   8.94 kB[22m
[2mdist/[22m[36massets/MainLayout-Cvd36HJ2.js             [39m[1m[2m   27.99 kB[22m[1m[22m[2m │ gzip:   8.12 kB[22m
[2mdist/[22m[36massets/DockerStore-CSLBB5U0.js            [39m[1m[2m   29.97 kB[22m[1m[22m[2m │ gzip:   9.43 kB[22m
[2mdist/[22m[36massets/Dashboard-Cas8xalo.js              [39m[1m[2m   87.99 kB[22m[1m[22m[2m │ gzip:  31.18 kB[22m
[2mdist/[22m[36massets/xterm-BG7mAkCJ.js                  [39m[1m[2m  290.38 kB[22m[1m[22m[2m │ gzip:  72.55 kB[22m
[2mdist/[22m[36massets/installCanvasRenderer-Bo4Z0Mi6.js  [39m[1m[2m  519.70 kB[22m[1m[22m[2m │ gzip: 178.05 kB[22m
[2mdist/[22m[36massets/index-BfWRVqm3.js                  [39m[1m[2m1,626.17 kB[22m[1m[22m[2m │ gzip: 510.87 kB[22m
[32m✓ built in 13.55s[39m
## 仓库结构

| 路径 | 说明 |
|---|---|
| cmd/server | 面板主程序 |
| cmd/agent | 多节点 Agent |
| web/ | Vue3 前端（go:embed 嵌入） |
| scripts/install.sh | 一键安装脚本 |
| scripts/smoke.sh | 全功能冒烟测试（34 项核心 API 打点） |
| jiaoben/ | 脚本应用仓库（规范见 jiaoben/README.md 与 docs/SCRIPT-REPO.md） |
| docs/ | 开发文档 |
| .cnb.yml | CI 流水线（go vet/test/build + 前端 build） |

## 脚本应用（jiaoben 分支）

脚本与清单托管于 jiaoben 分支：面板自动拉取清单 → 软件商店浏览 → 填参数一键安装（bash 执行、输出留痕、30 分钟限时）。编写规范见 [docs/SCRIPT-REPO.md](docs/SCRIPT-REPO.md)。开发流程：jiaoben 分支开发 → 合并 main → 面板刷新生效。

## 冒烟测试

\
## License

[MIT](LICENSE)
