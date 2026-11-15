# 寒语馨面板 · Android APP 设计文档

> 适用版本：v0.41.0+（面板端改造）+ APP v1.0（首发）
> 状态：设计定稿（M0），待开发
> 关联文档：[API.md](API.md)（开放 API）、[MCP.md](MCP.md)

---

## 1. 背景与目标

面板目前只有 Web 端（Vue3，桌面优先、移动端仅 CSS 适配）。本设计引入一个原生 Android APP，与面板 Web 端配合，实现：

1. **网站扫码登录**：Web 登录页展示二维码，已登录的 APP 扫码确认后，Web 免密登录。
2. **移动端服务器管理**：仪表盘监控、网站管理、软件商店（安装/卸载/升级）、文件管理等核心能力的随身入口。
3. **扫码即添加服务器**：扫码登录的二维码同时承载服务器地址与入口码，APP 首次扫码即可完成服务器添加，免去手动输入随机端口和入口码。

非目标（本期不做）：iOS 版、微信小程序、APP 自身的应用商店、面板安装脚本的移动端化。

## 2. 技术选型

已决策：**Kotlin + Jetpack Compose（原生 Android）**。决策依据与落选方案记录如下，避免后续反复：

| 方案 | 优势 | 劣势 | 结论 |
|---|---|---|---|
| **Kotlin + Jetpack Compose** | 系统集成最强（SAF 文件管理、后台任务、BiometricPrompt、厂商推送）；包体最小；无 GMS 依赖可分发到任何国产机 | 仅 Android；开发成本最高 | ✅ 采用 |
| Flutter | 跨平台、生态成熟 | 需学 Dart；文件管理与系统级能力走插件，深度定制弱于原生 | 落选 |
| uni-app (Vue3) | 复用团队 Vue 技能、可顺带出小程序 | 大文件 IO、WebSocket 稳定性、文件管理体验均弱；原生能力弱 | 落选 |

关键约束：面板用户大量使用无 GMS 的国产 ROM，APP 不得依赖 Google Play Services（扫码用 CameraX + zxing，不用 ML Kit；推送不依赖 FCM）。

## 3. 总体架构

```
┌──────────────┐   HTTPS(Bearer+entrance)   ┌─────────────────────┐
│  Android APP │ ──────────────────────────▶│      面板 (Go/gin)   │
│ Kotlin/Compose│ ◀──────────────────────────│  SQLite + gin /api  │
└──────────────┘    JSON / WebSocket        └─────────┬───────────┘
                                                      │ /api/ws/agent
                                            ┌─────────▼───────────┐
                                            │  节点 agent（反连）   │
                                            └─────────────────────┘
APP 可添加多个面板实例（多服务器），每个实例独立保存地址/入口码/会话令牌。
```

- **复用现有 API**：APP 直接调用现有 `/api/*` 业务接口（网站 `/api/sites`、软件商店 `/api/apps`、文件 `/api/files`、监控 `/api/dashboard` `/api/nodes`），不另起一套移动端 API。
- **新增 API**：仅鉴权相关（扫码登录、APP 会话、更新检查），见 §4–§5 与 §7。
- **实时性**：与 Web 端一致采用轮询（仪表盘 3s）；终端 WebSocket 为二期（见 §10 M6）。

## 4. 鉴权与会话设计

### 4.1 现状（调研结论）

- 登录 `POST /api/auth/login` 成功后下发 **HttpOnly Cookie `wm_session`**（随机 64 hex，24h 固定过期，无 refresh）；响应体不含 token。
- 所有 `/api` 请求先过 **入口中间件**（校验 `wm_entrance` Cookie；持有效开放 API Token 可豁免），再过 **会话中间件**（`internal/auth/auth.go` `Middleware()`）。
- 中间件已支持 `Authorization: Bearer` / `X-Api-Token`，但仅匹配**开放 API Token**（admin 等效，scope 限定，`internal/auth/apitoken.go`）。
- Session 表已含 IP / UA / 过期时间，且有现成会话列表/踢下线接口（`GET/DELETE /api/auth/sessions`、`POST /api/auth/sessions/revoke-others`）。

### 4.2 APP 会话（新增）

复用 Session 体系，扩展而非另建：

| 项 | 设计 |
|---|---|
| 登录 | 复用 `POST /api/auth/login`，body 增加可选字段 `client`（缺省 `web`）。`client=app` 时：走同一 bcrypt + TOTP + 防爆破校验；会话 TTL **30 天**；响应体**额外返回** `{"token": "<64hex>", "expires_at": ...}`（APP 存本地，Cookie 照常下发但 APP 不依赖它） |
| 携带 | APP 每个请求带 `Authorization: Bearer <token>`；入口中间件对有效 Bearer **会话令牌**同样豁免（与现有 API Token 豁免一致，`wm_entrance` 不必再发） |
| 校验 | `Middleware()` 的 Bearer 分支扩为两级匹配：先查开放 API Token（现状），未命中再查 Session 表 `token` 字段（命中且 `client=app` 视为 APP 会话，按用户角色走 viewer/operator/admin 既有权限链，含 viewer 拦非 GET） |
| 续期 | APP 会话滑动续期：校验通过时若剩余寿命 < 15 天则自动续至 30 天（Web 会话保持 24h 固定不变，避免行为回归） |
| 设备标识 | `client=app` 登录时 body 可带 `device_name`（机型，如 "Pixel 8"）与 `device_model`；写入 Session 新增列 `client`、`device_name`。Web 登录写入 `client=web` |
| 设备管理 | 既有会话接口自然升级为设备列表：列表/踢下线/一键退出其他设备均按 Session 表操作，APP 与 Web 安全中心共用；APP 内「设置 → 登录设备」调用同一组接口 |
| 吊销 | 会话被删（任何端踢下线）后，APP 下次请求 401 → 清本地 token 回到登录页 |

### 4.3 生物识别锁（APP 本地）

APP 本地启用指纹/设备凭据锁（androidx.biometric）：令牌加密存于 EncryptedSharedPreferences/DataStore（androidx.security-crypto），每次冷启动需通过生物识别或设备锁屏凭据才放行。仅在 APP 设置中开关，默认关闭（首次登录后引导开启）。

## 5. 扫码登录协议（Web 端发起）

### 5.1 设计要点

- 二维码**双用途**：内容同时承载「服务器地址 + 入口码 + 登录密钥」，APP 已登录该服务器 → 扫码确认即完成 Web 登录；APP 未添加该服务器 → 扫码先落库服务器（含入口码），再走账密登录，登录成功后自动继续确认 Web 登录。一次扫码同时完成「添加服务器 + APP 登录 + Web 免密登录」。
- 登录密钥 `key` 单次有效、120s 过期、内存存储（面板重启即清空，Web 重新生成即可）。
- 二维码仅展示在入口校验（`wm_entrance`）之后的登录页，未过入口的访客看不到，不新增入口码泄漏面。
- 防钓鱼：APP 确认弹窗明确展示「目标服务器地址 + Web 侧轮询来源 IP」，由用户肉眼确认（自建面板场景够用；残余风险见 §9）。

### 5.2 端点规格（新增，前缀 `/api/auth/qr`）

QR 记录（内存 map + 过期清扫 goroutine）字段：`key`、`state`(pending/scanned/confirmed/used/expired)、`web_ip`（create 时记录）、`web_ua`、`ticket`、`created_at`、`expires_at`。

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | `/api/auth/qr/create` | 入口 Cookie（匿名可） | Web 登录页创建。body 可选 `{"ua": "..."}`（缺省取请求 UA）。响应 `{"key":"<uuid>","qr_text":"hanyuxin://login?...","expires_in":120}` |
| GET | `/api/auth/qr/state?key=` | 入口 Cookie（匿名可） | Web 每 2s 轮询。响应 `{"state":"pending|scanned|confirmed","ticket":"..."}`（仅 `confirmed` 时返回 ticket）。按 IP 限频复用登录防爆破工具（`internal/auth/auth.go:61-90` 模式） |
| POST | `/api/auth/qr/confirm` | **APP 会话**（Bearer） | APP 扫码后第一步「我扫了」。body `{"key":"..."}`。校验 key 有效且 APP 会话的 panel 与 key 所属 panel 一致；置 `state=scanned` |
| POST | `/api/auth/qr/approve` | **APP 会话**（Bearer） | APP 用户点「确认登录」。body `{"key":"..."}`。服务端据此为 `web_ip`/`web_ua` **创建一条 `client=web` 的 Session**，生成一次性 `ticket`（60s TTL）写回 QR 记录，置 `state=confirmed`。响应 `{"ok":true}` |
| POST | `/api/auth/qr/exchange` | 入口 Cookie（匿名可） | Web 拿 `{"key":"...","ticket":"..."}` 兑换：校验均单次有效 → 设置 `wm_session` Cookie（与现行登录一致，HttpOnly/SameSite=Lax/24h），QR 置 `used`。响应体与账密登录一致 `{"username":...,"role":...}` |

`qr_text` 格式（APP 端解析契约，发版后不可变）：

```
hanyuxin://login?v=1&host=<面板对外地址>&entrance=<入口码>&key=<key>&v2=<2fa_enabled 0/1>
```

- `host`：`create` 时按 优先级推导——`X-Forwarded-Proto/Host`（域名反代场景）→ 请求 Host + 配置 TLS 状态；可用设置项 `app.public_url` 覆盖（设置页新增，管理员可改）。
- APP 收到非 `hanyuxin://login` scheme 的扫码结果一律忽略，防误扫外部二维码。

### 5.3 时序（三段式）

```
Web 登录页                面板                         APP（已登录该服务器）
   │ POST /auth/qr/create  │                              │
   │──────────────────────▶│ 生成 key，展示二维码           │
   │ GET /auth/qr/state ×N │                              │
   │──────────────────────▶│ state=pending                │
   │                       │        扫码（得 host/entrance/key）│
   │                       │ POST /auth/qr/confirm ◀──────│ state=scanned
   │                       │     弹窗：服务器地址 + Web侧IP   │
   │                       │ POST /auth/qr/approve  ◀──────│ 用户确认
   │                       │ 为 web_ip/ua 建 web 会话        │
   │ state=confirmed+ticket│                              │
   │◀──────────────────────│                              │
   │ POST /auth/qr/exchange│                              │
   │──────────────────────▶│ 设 wm_session Cookie，登录完成  │
```

APP 未登录该服务器时的分支：扫码 → 解析出 host/entrance → 保存服务器条目 → APP 登录页预填地址与入口码 → 用户账密登录（+2FA）→ 成功后 APP 自动继续 confirm/approve 流程。

## 6. 功能模块设计（APP 侧）

每模块先列**复用的现有接口**，再列 APP 交互要点。均受 §4.2 鉴权约束（Bearer + viewer 只读拦截同样生效）。

### 6.1 服务器管理（APP 特有）

- 本地多服务器列表（DataStore）：名称、host、入口码、令牌、添加时间、最后在线时间。
- 添加方式：① 扫码（§5.3 分支，推荐）；② 手动输入 `https://host:port` + 入口码 + 账密。
- 连接测试：`GET /api/setting/info`（版本号/主机名）+ `GET /api/dashboard/summary` 择一探测。
- 切换服务器：顶栏下拉，全局状态切换（会话各自独立持有）。

### 6.2 仪表盘

- 复用：`GET /api/dashboard/summary`、`GET /api/nodes`、`GET /api/nodes/:id/metrics`、`GET /api/nodes/:id/metrics/hourly`。
- 要点：CPU/内存/磁盘/负载/网络卡片 + 折线图（Vico，Compose 图表库）；3s 轮询（与 Web 端 `Dashboard.vue:283` 一致）；监控对象下拉过滤回环节点逻辑与 v0.40.5 行为对齐（`127.0.0.1/::1` 回连节点不重复展示）。

### 6.3 网站管理

- 复用：`GET/POST /api/sites`、`PUT /api/sites/:id`、`POST /api/sites/:id/toggle`、`GET /api/sites/nginx/status`、`POST /api/sites/backups/restore`。
- 要点：列表 + 启停开关 + 删除需二次确认（对齐 Web 端确认文案）；创建/编辑仅提供核心字段（域名、根目录、PHP/运行环境），完整编辑引导用 Web 端（「在 Web 端打开此页面」深链提示）。

### 6.4 软件商店

- 复用：`GET /api/apps/templates`、`GET /api/apps`、`POST /api/apps`（安装）、`POST /api/apps/:id/uninstall`、`POST /api/apps/:id/upgrade`、`GET/POST /api/apps/scripts`、Docker 子树 `/api/apps/docker/*`。
- 要点：安装/卸载为长任务，APP 展示任务进度（复用面板任务查询接口）+ 通知栏进度；**卸载强确认**：涉及面板依赖保护的场景（v0.40.7 引入的 nginx-pkg 卸载拒绝 + force 二次确认）在 APP 端同样弹「影响说明 + 输入确认」两段式，禁止一键强卸。

### 6.5 文件管理

- 复用：`GET /api/files/list?path=`、`POST /api/files/mkdir|rename|move|delete|save|chmod|compress|decompress`、`GET /api/files/read|download`、`POST /api/files/upload|upload-chunk|upload-finish`。
- 要点：
  - 下载走 OkHttp 流式写 `getExternalFilesDir`/SAF 目标，>100MB 用系统下载通知；
  - 上传走既有分块接口 `upload-chunk` + `upload-finish`（后台 WorkManager 队列，断点续传按现有分块语义）；
  - 文本编辑：`read`/`save`，仅编辑 <2MB 文本文件，编辑器页面带行号与「保存前差异确认」；
  - 删除/覆盖/解压均有确认弹窗，路径展示完整绝对路径防误操作。

### 6.6 消息与告警（简化版）

- 复用：`GET /api/alerts`（规则与事件）、`GET /api/notify`。
- 要点：APP 内「消息中心」页轮询未读告警事件；**系统级推送为二期**（无 GMS 环境需厂商推送通道，见 §10 M6）。

### 6.7 终端（二期，M6）

- 复用：`GET /api/ws/terminal`、`GET /api/ws/ssh`、`/api/term/hosts`。
- 前置改造：WS 握手经 `protected` 组的会话中间件，§4.2 的 Bearer 会话扩展落地后 OkHttp WebSocket 带 `Authorization` 头即可连通，面板无需专门改 WS；入口豁免同样由中间件统一处理。
- APP 端用 xterm.js 等价物（Termux 风格自绘或 `connectbot` 内核方案）单独排期，键盘布局/字体缩放工作量不小。

## 7. 面板端改造清单（M1，随 v0.41.0 发布）

### 7.1 后端（Go）

| # | 改造 | 位置 | 说明 |
|---|---|---|---|
| B1 | Session 模型加列 `client`、`device_name`，AutoMigrate 自动建列 | `internal/core/db.go`（模型）、启动迁移已覆盖 | 兼容旧数据：存量记录 client 缺省 `web` |
| B2 | 登录 handler 支持 `client=app`：30 天会话 + 响应体返回 token + 记录设备名 | `internal/auth/auth.go:111-163` | web 路径行为完全不变 |
| B3 | Bearer 会话校验：中间件 Bearer 分支两级匹配（API Token → app session），并纳入入口豁免 | `internal/auth/auth.go:181-186,205-256`、`internal/auth/apitoken.go` | 保持 API Token 现有 scope 语义不变 |
| B4 | APP 会话滑动续期（剩 <15 天续到 30 天，仅 client=app） | 同上中间件 | 写 Session 需考虑 WAL 并发，沿用现有写法 |
| B5 | QR 五端点 + 内存记录 + 过期清扫 + 轮询限频 | 新文件 `internal/auth/qrlogin.go`，注册进 `auth.go:96` 附近 | 见 §5.2；exchange 写 Cookie 复用 `auth.go:157-160` 的下发逻辑 |
| B6 | `app.public_url` 设置项 + QR host 推导 | `internal/auth/qrlogin.go` + 设置模块 | 优先 X-Forwarded-*，其次 Host，最后设置项 |
| B7 | `GET /api/app/check-update?platform=android&v=` | 新增（设置模块或 auth 均可） | 读设置项 `app.apk_url` / `app.latest_version` / `app.release_notes`，匿名（入口后）可访问；供 APP 更新检查 |
| B8 | 冒烟测试增项 | `scripts/smoke.sh` | 新增约 8 项：qr create/state 循环、confirm 需登录 401、approve/exchange 全流程（参数化风格与现有 34 项一致） |

### 7.2 前端（Vue3）

| # | 改造 | 位置 | 说明 |
|---|---|---|---|
| F1 | 登录页加「扫码登录」Tab：调 `qr/create` 渲染二维码 + 2s 轮询状态 + exchange 落地 | `web/src/views/Login.vue` | 依赖 `vue-qrcode`（仓库当前无任何 QR 依赖，需新增）；轮询 120s 后停止并刷新二维码 |
| F2 | 安全中心/用户页会话列表显示「客户端」列（Web/APP + 设备名） | 会话列表组件（`/api/auth/sessions` 消费处） | 数据来自 B1 |
| F3 | 设置页新增：面板对外地址 `app.public_url`、APP 下载地址 `app.apk_url`、最新版本 `app.latest_version`、更新说明 `app.release_notes` | 设置模块 | 管理员可改；APP 下载页/更新检查由此驱动 |

### 7.3 文档与版本

- `docs/API.md` 增补「扫码登录 & APP 会话」章节（端点表格 + qr_text 契约）。
- `README.md` 功能表增加「移动端 APP（Android）」行，徽章版本随发版更新。
- 版本号递增点：`internal/core/db.go:14` `const Version`。

## 8. APP 端工程设计

### 8.1 工具链

| 工具 | 版本要求 | 用途 |
|---|---|---|
| Android Studio | Ladybug（2024.2）或更新 | IDE、模拟器、Profiler |
| JDK | 17（AS 自带 JBR 即可） | Gradle 编译 |
| Android SDK | Platform 35 / Build-Tools 35 | 编译目标 |
| 真机 | Android 8.0+（minSdk 26）一台 + adb | 调试主力（模拟器无指纹/弱网真实感） |
| 密钥库 | `keytool -genkeypair` 自建 keystore | release 签名，**务必离线备份**，丢失即无法自更新 |

### 8.2 工程结构（单 module 起步，包名 `cn.hanyuxin.panel`）

```
app/
  src/main/
    java/cn/hanyuxin/panel/
      MainActivity.kt          # 单 Activity + NavHost
      navigation/              # 路由表（登录/首页/各模块）
      data/
        api/                   # OkHttp + kotlinx.serialization；拦截器注入 Bearer + 多服务器路由
        model/                 # 与面板 /api 响应结构对应的 data class
        repo/                  # 各模块仓库层（网络 → 本地缓存）
        local/                 # DataStore：服务器列表、偏好；EncryptedDataStore：令牌
      security/                # 生物识别锁、令牌加解密、自签证书 TOFU pinning
      ui/
        servers/  login/  dashboard/  sites/  appstore/  files/
        alerts/   settings/    # 每模块 Screen + ViewModel
    res/                       # 图标、主题字符串（i18n 中英，对齐 web/src/i18n.ts 键）
```

### 8.3 关键依赖（全部无 GMS 依赖）

| 库 | 用途 |
|---|---|
| OkHttp 4 + kotlinx.serialization | 网络层（WS 二期直接用 OkHttp WebSocket） |
| CameraX + zxing core | 扫码（§5.2 qr_text 解析） |
| androidx.biometric | 生物识别锁 |
| androidx.security-crypto + DataStore | 令牌加密存储与偏好 |
| WorkManager | 上传队列、后台轮询 |
| Vico | Compose 折线图（仪表盘） |
| Coil | 网站图标/头像类图片加载 |

### 8.4 自签证书处理（TOFU + 指纹锁定）

面板默认自签 TLS（`internal/selfcert/`，含公网 IP SAN）。APP 策略：

1. 首次连接某服务器时展示证书 SHA-256 指纹，用户确认「信任此服务器」后固化到该服务器的本地记录（Trust On First Use）。
2. 后续每次连接用 OkHttp `CertificatePinner` 校验指纹一致，不一致立即断连并强提示（防中间人替换证书）。
3. 服务器若换证书（面板重装/换正式证书），APP 提示重新确认指纹；用户在服务器详情页可主动重置信任。
4. 文档与 APP 内文案同时建议：生产环境给面板配正式域名证书（域名反代场景 `panel.hanyuxin.cn` 已具备此条件）。

### 8.5 打包、分发与自更新

- 构建：`./gradlew assembleRelease`（签名配置走 `key.properties`，不入库）；debug 包供联调。
- 分发：APK 上传到面板自身托管（静态托管/文件管理对外目录或对象存储），设置项 `app.apk_url` 指向之；Web 端设置页展示下载入口。
- 自更新：APP 启动及每 6h 调 `GET /api/app/check-update`（B7），有新版则弹窗 + 下载 APK + 拉起安装（`REQUEST_INSTALL_PACKAGES` 权限，引导用户授权「安装未知应用」）。
- CI：`.cnb.yml` 本期不构建 Android（本地打包上传）；二期可加 `./gradlew assembleRelease` job + 产物推送。

## 9. 安全设计汇总

| 威胁 | 对策 |
|---|---|
| APP 令牌泄漏（Root/备份/截图） | EncryptedDataStore + 生物识别锁；会话列表可随时踢下线；敏感页面（文件删除、强卸载）二次确认 |
| 扫码钓鱼（诱导扫他人面板二维码） | 确认弹窗明示「服务器地址 + Web 侧来源 IP」；qr_text 校验 scheme 与 host 一致性；key 单次 + 120s 过期；confirm 需 APP 已登录同一服务器 |
| QR state 枚举/爆破 | key 为 UUIDv4 + 120s 短命；轮询接口按 IP 限频（复用登录防爆破实现模式） |
| 中间人（自签证书） | §8.4 TOFU 指纹锁定 + 换证书强提示；文档引导正式证书 |
| viewer/低权用户越权 | APP 复用同一中间件角色链（viewer 拦非 GET 照旧生效），APP 端不实现本地权限旁路 |
| 入口码泄漏 | 二维码仅在过入口校验后的登录页展示；Bearer 会话豁免入口与现有 API Token 豁免语义一致，未放宽匿名面 |
| 审计缺失 | QR confirm/approve/exchange 及 APP 登录全部落在既有审计中间件（`audit_logs`），设备名随请求 UA 记录 |

## 10. 里程碑

| 阶段 | 内容 | 验收 |
|---|---|---|
| **M0** 本文档 | 设计定稿 | 评审通过 |
| **M1** 面板端 v0.41.0 | §7 全部改造（B1–B8、F1–F3）+ docs/API.md 增章 | 冒烟测试含 QR 全流程通过；Web 扫码登录可用 |
| **M2** APP 骨架 | 工程搭建、多服务器添加（扫码 + 手动）、账密登录（含 2FA）、仪表盘、生物识别锁 | 真机连测试机 154.8.165.155 完成登录与仪表盘展示 |
| **M3** 网站管理 + 软件商店 | §6.3、§6.4 全量交互 + 任务进度 | 网站启停、应用安装/卸载/升级真机验收，强卸载两段式生效 |
| **M4** 文件管理 | §6.5 全量交互（分块上传、流式下载、编辑器） | 100MB 上传下载真机通过；断点续传按分块语义恢复 |
| **M5** 发布 | 自更新、签名 keystore、APK 分发页、设置项打通 | 从面板下载 APK 安装→检查更新→覆盖安装成功 |
| **M6** 二期 | 终端 WS、系统级推送（厂商通道调研）、节点管理、iOS 评估 | 另立文档 |

## 11. 风险与开放问题

1. **滑动续期的并发写**：高频轮询下 Session 行更新频率上升（WAL 模式下可接受，实现时验证；必要时节流为「每次响应仅当跨过阈值才写」）。
2. **`app.public_url` 推导在多级反代下的正确性**：以 X-Forwarded-* 为主、设置项兜底，M1 冒烟需覆盖域名反代（panel.hanyuxin.cn）与 IP 直连（154.8.165.155）两种通道。
3. **分块上传断点语义**：APP 端 WorkManager 重试与 `upload-chunk` 既有语义的对齐需在 M4 实测确认（面板当前分块接口的续传能力以实现为准，文档先行约定，必要时补一个 `upload-progress` 查询端点）。
4. **推送通道**：无 GMS 环境的系统推送需接厂商通道（小米/华为/OPPO…）或自建长连接，成本高，故置于 M6 并先调研。
5. **包名与签名**：`cn.hanyuxin.panel` 与 keystore 一经发布不可更改，M5 前需最终确认。
