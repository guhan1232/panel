# 寒语馨面板 · 开放 API 文档

> 适用版本：v0.39.0+
>
> 开放 API 面向外部脚本与程序化调用（自动化运维、CI、监控联动等）。所有接口与面板 Web 页面使用同一套 REST 端点，凭 API Token 即可直调，无需浏览器会话与安全入口 Cookie。

## 一、启用与获取 Token

1. 登录面板 → **系统设置** → **开放 API 与 MCP** 卡片
2. 点击 **生成 Token 启用**（已有 Token 时按钮为「重置 Token」，重置后旧客户端立即失效）
3. 复制显示的 Token（48 位十六进制字符串）

也可用会话 API 管理 Token：

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET /api/mcp/token` | 查看状态 / Token / 权限 | `{"enabled","token","mode","scopes","readonly","scopes_known","mcp_url"}` |
| `POST /api/mcp/token` | 生成 / 重置 Token | 请求体可携带权限：`{"mode":"full\|custom","scopes":[...],"readonly":bool}` |
| `PUT /api/mcp/token` | **仅更新权限**（Token 值不变，客户端无需重新配置） | 请求体同上 |
| `DELETE /api/mcp/token` | 禁用开放 API | 所有 API/MCP 客户端立即失效 |

> Token 管理端点**仅限面板会话调用**：Token 自身无权查看或修改自己的权限（防止越权提权）。

## 二、Token 权限体系（v0.39.0+）

Token 不再是单一全权凭证，支持三种粒度控制，生成与保存时均可配置：

### 权限模式

- **完整权限（mode=full）**：等效管理员，可调用全部模块的全部方法（默认）
- **按模块授权（mode=custom）**：仅放行勾选模块的路径，共 19 个 scope：

| scope | 覆盖路径 | scope | 覆盖路径 |
|---|---|---|---|
| `status` | /api/dashboard | `tasks` | /api/tasks |
| `sites` | /api/sites | `files` | /api/files |
| `runtimes` | /api/runtimes | `term` | /api/ws/terminal、/api/ws/ssh、/api/term、/api/terminal-logs |
| `nodes` | /api/nodes | `proc` | /api/proc |
| `certs` | /api/certs | `notify` | /api/notify |
| `firewall` | /api/firewall | `alerts` | /api/alerts |
| `dbmgr` | /api/dbmgr | `audit` | /api/audit |
| `apps` | /api/apps | `s3` | /api/s3 |
| `mcp` | /api/mcp | `setting` | /api/setting、/api/users |
| | | `auth` | /api/auth/me、/api/sessions |

### 只读模式

勾选后**所有已授权模块的写方法（POST/PUT/DELETE）一律拒绝**（403），仅放行 GET/HEAD——适合给监控/报表类外部系统发只读凭证。

### 拒绝行为

- 权限不足：`HTTP 403 {"code":403,"msg":"API Token 权限不足（模块未授权或只读模式）"}`
- Token 无效/未启用：`HTTP 401`
- 请求路径不在任何已授权 scope 内：403

### 示例

```bash
# 生成只读 Token（仅网站+总览模块）
curl -sk -b "wm_session=<会话>" -H "Cookie: wm_entrance=<入口>" \
  -X POST https://面板/api/mcp/token -H 'Content-Type: application/json' \
  -d '{"mode":"custom","scopes":["sites","status"],"readonly":true}'

# 之后：GET /api/sites 放行（200）；POST /api/sites 拒绝（403）

# 仅调整权限（Token 不变）
curl -sk -X PUT https://面板/api/mcp/token -H 'Content-Type: application/json' \
  -d '{"mode":"full"}'
```

> 兼容性：v0.38.x 生成的旧 Token（纯字符串存储）自动兼容为完整权限；在设置页保存一次权限后即迁移为记录格式。

## 三、鉴权

所有 `/api/*` 请求（除登录外）携带以下任一请求头：

```text
Authorization: Bearer <token>
X-Api-Token: <token>
```

- 有效 Token **豁免安全入口**：无需先访问 `/s/<入口串>` 获取入口 Cookie，直接请求 `https://面板地址:端口/api/...`
- 未登录会话返回 401；权限不足返回 403
- 凭据即用即弃：面板不记录 Token 关联的会话，泄露后立即重置即可

## 四、响应约定

```json
{ "code": 0, "msg": "ok", "data": { } }
```

- HTTP 状态码通常为 200；`code != 0` 表示业务错误（见 `msg`）
- 鉴权类错误直接返回 401 / 403 / 404
- 列表类接口 `data` 为数组；分页类接口见具体端点说明

## 五、快速开始

```bash
TOKEN=你的token
BASE=https://面板地址:端口

# 面板总览（CPU/内存/磁盘/站点数/节点数）
curl -sk -H "Authorization: Bearer $TOKEN" $BASE/api/dashboard/summary

# 站点列表
curl -sk -H "Authorization: Bearer $TOKEN" $BASE/api/sites

# 创建 PHP 站点（需 Token 有 sites 模块写权限）
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/sites \
  -d '{"name":"myblog","type":"php","domains":"blog.example.com","root":"/www/wwwroot/myblog","php_socket":"/run/php/php8.5-fpm.sock"}'

# 在节点 1 上执行命令（经审计，需 nodes 模块权限）
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/nodes/1/exec -d '{"script":"df -h","timeout_sec":120}'
```

## 六、常用端点速查

### 面板与设置

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/setting/info` | 版本、监听地址、HTTPS 状态、运行环境 |
| `GET` | `/api/dashboard/summary` | 仪表盘汇总（当前负载/磁盘/站点与节点统计） |
| `GET/POST/DELETE` | `/api/mcp/token` | 开放 API Token 管理（见第一节） |
| `GET` | `/api/setting/tls/status` | HTTPS 证书状态 |
| `GET` | `/api/setting/panel-backup` | 下载面板数据备份 ZIP |

### 网站（Nginx）

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/sites` | 站点列表（含配置文件存在性与在线状态） |
| `POST` | `/api/sites` | 创建站点：`{name, type: static\|php\|proxy, domains, root, php_socket, upstream, ssl_cert, ssl_key, force_https}`；写入前自动 `nginx -t` 校验 |
| `PUT` | `/api/sites/:id` | 修改站点 |
| `DELETE` | `/api/sites/:id` | 删除站点（仅移除 Nginx 配置，不删网站文件） |
| `POST` | `/api/sites/:id/toggle` | 启用 / 停用 |
| `GET` | `/api/sites/:id/log?type=access\|error&lines=200` | 读取站点日志 |
| `GET` | `/api/sites/nginx/status` | Nginx 安装与运行状态 |
| `POST` | `/api/sites/nginx/reload` | 重载 Nginx |

PHP 站点 `php_socket` 取值：面板已安装的 PHP 为 `/run/php/php<版本>-fpm.sock`（如 `/run/php/php8.5-fpm.sock`）；RHEL/Remi 为 `/run/php-fpm/www.sock`；也支持自定义 unix socket 或 `127.0.0.1:9000` TCP 形式。

### 运行时（PHP / Node.js / Go / Python / OpenJDK）

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/runtimes` | 各语言已安装版本列表 |
| `GET` | `/api/runtimes/langs` | 支持的语言与下载源 |
| `GET` | `/api/runtimes/:lang/versions?source=<源>` | 可安装版本清单 |
| `POST` | `/api/runtimes/:lang/install` | 安装版本 `{version, source}`（PHP 经发行版仓库，耗时可达数分钟） |
| `POST` | `/api/runtimes/:lang/set-default` | 设默认版本 `{instance_id}`（PHP 无默认版本概念） |
| `DELETE` | `/api/runtimes/:id` | 卸载某版本 |

### 多节点

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/nodes` | 节点列表（在线状态依据最近心跳） |
| `POST` | `/api/nodes/:id/exec` | 远程执行命令 `{script, timeout_sec}`（经审计） |
| `GET` | `/api/nodes/:id/metrics?limit=240` | 历史指标（15s 采样，保留 3 天） |
| `GET` | `/api/nodes/:id/metrics/hourly?days=30` | 小时聚合指标（长期保留） |
| `GET` | `/api/nodes/:id/files?path=/root` | 远程目录浏览 |
| `GET` | `/api/nodes/:id/file?path=...` | 读取远程文件（base64，≤8MB） |
| `POST` | `/api/nodes/:id/file` | 写远程文件 `{path, data(base64)}` |
| `DELETE` | `/api/nodes/:id/file?path=...&force=1` | 删除远程文件/目录（递归） |
| `POST` | `/api/nodes/ssh-install` | SSH 一键接入：自动推送 Agent 并注册 systemd 服务 `{host, port, user, auth_type: password\|key, password, private_key, passphrase, name, server}` |

### 其他模块

证书（`/api/certs`）、防火墙（`/api/firewall`）、数据库（`/api/dbmgr`）、应用商店（`/api/apps`）、计划任务（`/api/tasks`）、告警、通知、对象存储、审计日志、文件管理、进程管理、终端等模块端点结构与面板页面一一对应——可在浏览器开发者工具的 Network 面板观察具体参数，或直接使用 MCP 的 `api_request` 工具试调。

## 七、安全注意事项

- Token 等效管理员：**务必仅经 HTTPS 调用**，HTTP 明文会泄露 Token
- 不要把 Token 写入公开仓库 / 日志 / 截图；泄露后立即在设置页重置
- 开放 API 的所有写操作同样进入面板审计日志（操作者显示为 `api`）
- 长时间操作（运行时安装、SSH 接入）注意客户端超时设置，建议 ≥ 15 分钟

## 八、扫码登录与 APP 会话（v0.41.0+，docs/APP.md）

移动 APP（Android）与 Web 端共用的鉴权扩展。二维码由 Web 登录页生成，内容同时承载服务器地址与入口码：

```
hanyuxin://login?v=1&host=<面板对外地址>&entrance=<入口码>&key=<key>&v2=<2fa 0/1>
```

### 扫码登录端点（前缀 /api/auth/qr）

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| `POST /api/auth/qr/create` | 入口 Cookie（匿名可） | Web 登录页创建，返回 `{key, qr_text, expires_in:120}` |
| `GET /api/auth/qr/state?key=` | 入口 Cookie（匿名可） | Web 每 2s 轮询：`{state: pending/scanned/confirmed/expired, ticket?}` |
| `POST /api/auth/qr/confirm` | APP 会话（Bearer） | APP 扫码后标记已扫，body `{"key"}` |
| `POST /api/auth/qr/approve` | APP 会话（Bearer） | 用户确认登录，body `{"key"}`；生成一次性票据 |
| `POST /api/auth/qr/exchange` | 入口 Cookie（匿名可） | Web 兑换 `{"key","ticket"}` → 下发 `wm_session` Cookie |

key 单次有效、120s 过期；票据 60s 一次性；confirm/approve/exchange 全部进入审计日志。

### APP 会话

- 登录：`POST /api/auth/login` 带 `"client":"app"`（可选 `device_name`）→ 30 天会话，响应体返回 `{"token","expires_at"}`（Web 登录行为不变）
- 携带：`Authorization: Bearer <token>`，且与开放 API Token 一样豁免入口 Cookie
- 续期：剩余寿命 < 15 天自动续至 30 天；会话列表（`GET /api/auth/sessions`）新增 `client`/`device_name` 字段
- 管理：踢下线/一键退出其他设备接口不变，APP 与 Web 安全中心共用

### APP 配置与自更新

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET/POST /api/setting/app-config` | 读任意会话 / 写仅 admin：`{public_url, apk_url, latest_version, release_notes}` |
| `GET /api/app/check-update?platform=android&v=<当前版本>` | 匿名可（入口后）：`{has_update, current, latest, apk_url, notes}` |
