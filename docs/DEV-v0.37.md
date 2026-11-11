# 寒语馨面板 v0.37 开发文档

> 2026-11-11 立项。来源：用户需求（应用商店脚本安装 / 默认 HTTPS / 防火墙 Web 安装 / Docker 重构 / 数据库优雅降级）。
> 状态标记：[x] 待开发 [x] 已完成（含验证）

## 1. 默认 HTTPS 加密连接（用户截图「明文连接」黄点 → 目标绿点）

### 需求
- 面板默认为加密连接，打开即 HTTPS。
- 优先尝试给服务器公网 IP 申请证书（ACME，RFC 8738 IP identifier）。
- 不可行时用用户提供的中间 CA（`mianban.crt` + `mianban.key`，CN=mianban，由根 CA `ca.crt` CN=hanyuxin 签发，CA:TRUE pathlen:0，有效期至 2076）交叉签发叶子证书；浏览器不信任可接受。
- 兜底：自签证书（IP SAN 必须含公网 IP）。

### 实现
- [x] `config.go`：`defaultConfig()` 新装默认 `server.tls.enabled: true`（已有配置不强制变更，提供一键开启）。
- [x] `selfcert.Ensure()`：自签证书 SAN 加入服务器公网 IP（探测出口 IP + 枚举网卡），不再只有 127.0.0.1；**有效期默认 10 年**。
- [x] `selfcert.IssueFromCA()`：用中间 CA 签发叶子证书（SAN=公网 IP+localhost，链=叶子+中间 CA）。**有效期策略：CA 能签多久签多久（跟随 CA 剩余有效期）；仅当 CA 不可用时才回退自签（固定 10 年）**。
- [x] **仓库一键安装脚本 `scripts/install.sh`**：安装完成后即 HTTPS——未提供 CA 时面板自动自签（10 年含 IP SAN）；提供 `--ca-cert/--ca-key` 参数时用中间 CA 签发 10 年叶子证书。
- [x] `setting.go` 新增接口：
  - `GET /setting/tls/status`：当前 TLS 状态/证书路径/有效期/是否已有 CA。
  - `POST /setting/tls/apply`：`mode=self|ca`；`ca` 需上传 CA 证书+私钥（存 `dataDir/certs/ca/`，后续续签复用）；签发后写 config.yaml 并重启面板。
  - `POST /setting/tls/acme-ip`：尝试 ACME IP 证书（失败返回明确原因，自动提示改用 CA 交叉）。
- [x] `Setting.vue` 新增「HTTPS 加密连接」卡片：状态展示 + 自签/CA 交叉/ACME 尝试三个操作。
- [x] 顶部连接状态胶囊：HTTPS 下显示「加密连接」（绿）。

## 2. 防火墙 Web 安装（用户截图「未检测到防火墙」）

### 需求
- 未安装 ufw/firewalld 时可在 Web 上一键安装（默认安装）；安装后可开关防火墙、放行/移除端口（已有能力）。

### 实现
- [x] `firewall.go`：`Register(api, panelPort)`；`POST /firewall/install` 按发行版安装（apt→ufw，dnf/yum→firewalld）。
- [x] 防自锁：启用 ufw 前先 `allow 22/tcp` + `allow <面板端口>/tcp` 再 `--force enable`；firewalld 启动后立即放行面板端口。
- [x] `Firewall.vue`：未检测到时显示安装引导卡片（含防自锁说明），安装完成自动刷新。

## 3. 应用商店一键安装脚本

### 需求
- 应用条目支持「一键安装脚本」：脚本托管在仓库（URL），点击安装=拉取执行。
- 支持自行上传脚本安装。

### 实现
- [x] `catalog.go`：`remoteApp` 增加 `kind:"script"`（字段 `script`：URL 或内嵌 bash；`args` 参数声明）。
- [x] `Template` 增加 `Script` 字段；`install()` 增加 `case "script"`：下载/渲染占位符 → 落盘 `dataDir/appstore/scripts/` → bash 执行 → 记录 App（kind=script）与日志。
- [x] 自定义脚本接口：`GET/POST /appstore/scripts`、`DELETE /appstore/scripts/:name`、`POST /appstore/scripts/:name/run`（上传的脚本同样走安装登记）。
- [x] `SoftStore.vue` 新增「脚本应用」区块：仓库清单脚本列表（一键安装）+ 自定义上传 + 运行日志查看。

## 4. Docker 模块重构（用户截图导航：应用商店/总览/网站/容器/线上镜像/本地镜像/容器编排/网络/存储卷/仓库/设置）

### 实现（后端 `docker.go` 补齐）
- [x] `GET /docker/overview`：docker version+info 汇总（版本/运行中容器/镜像数/存储驱动/资源）。
- [x] `GET /docker/sites`：发布了端口映射的容器（网站视图）。
- [x] 网络：`GET/POST /docker/networks`、`DELETE /docker/networks/:name`（禁删 bridge/host/none）。
- [x] 存储卷：`GET /docker/volumes`、`DELETE /docker/volumes/:name`。
- [x] 线上镜像：`GET /docker/search?term=`、`POST /docker/pull {image}`（同步限时执行，输出日志）。
- [x] 仓库：`POST /docker/login {registry,username,password}`（docker login 校验，仓库地址存 settings）。
- [x] 前端 `DockerStore.vue` 整页重写为 11 个标签。

## 5. 数据库优雅降级（用户报错 `dial tcp 127.0.0.1:3306: connect: connection refused`）

### 实现
- [x] `dbmgr.go`：`GET /db/status`（net.DialTimeout 1.5s 探测 MySQL/PG 端口，返回 `{mysql:{installed,addr},pg:{...}}`，绝不 500）。
- [x] 列表/用户等接口：连接失败（connection refused/dial 类错误）时返回 `{installed:false}` 而非报错弹窗。
- [x] `Databases.vue`：先查 status；未安装显示引导卡片 +「一键安装 MariaDB」（apt/dnf 装包并启动服务），装完引导保存连接配置。

## 6. 验收
- [x] HTTPS：`https://<公网IP>:<port>/s/<entrance>` 可打开（证书不受信任属预期），明文连接胶囊变加密。
- [x] 防火墙：一键安装 ufw → 自动放行 SSH/面板端口 → 端口增删正常。
- [x] 应用商店：仓库脚本一键安装 + 自定义上传安装。
- [x] Docker：11 标签全部可用（无 Docker 时显示安装引导）。
- [x] 数据库：未安装时页面无报错弹窗，显示引导与一键安装。
