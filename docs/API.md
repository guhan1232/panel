# 寒语馨面板 · 开放 API 文档

> 适用版本：v0.38.9+
>
> 开放 API 面向外部脚本与程序化调用（自动化运维、CI、监控联动等）。所有接口与面板 Web 页面使用同一套 REST 端点，凭 API Token 即可直调，无需浏览器会话与安全入口 Cookie。

## 一、启用与获取 Token

1. 登录面板 → **系统设置** → **开放 API 与 MCP** 卡片
2. 点击 **生成 Token 启用**（已有 Token 时按钮为「重置 Token」，重置后旧客户端立即失效）
3. 复制显示的 Token（48 位十六进制字符串）

也可用会话 API 管理 Token：

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET /api/mcp/token` | 查看状态与当前 Token | `{"enabled":bool,"token":"...","mcp_url":"..."}` |
| `POST /api/mcp/token` | 生成 / 重置 Token | 返回同上 |
| `DELETE /api/mcp/token` | 禁用开放 API | 所有 API/MCP 客户端立即失效 |

## 二、鉴权

所有 `/api/*` 请求（除登录外）携带以下任一请求头：

```text
Authorization: Bearer <token>
X-Api-Token: <token>
```

- Token **等效 admin 会话**：可调用全部面板接口（含写操作与节点远程命令）
- **豁免安全入口**：无需先访问 `/s/<入口串>` 获取入口 Cookie，直接请求 `https://面板地址:端口/api/...`
- Token 未启用或错误时返回 404（与入口防护语义一致，不暴露端点存在性）；未登录会话返回 401
- 凭据即用即弃：面板不记录 Token 关联的会话，泄露后立即重置即可

## 三、响应约定

```json
{ "code": 0, "msg": "ok", "data": { } }
```

- HTTP 状态码通常为 200；`code != 0` 表示业务错误（见 `msg`）
- 少数鉴权错误直接返回 401 / 403 / 404
- 列表类接口 `data` 为数组；分页类接口见具体端点说明

## 四、快速开始

```bash
TOKEN=你的token
BASE=https://面板地址:端口

# 面板总览（CPU/内存/磁盘/站点数/节点数）
curl -sk -H "Authorization: Bearer $TOKEN" $BASE/api/dashboard/summary

# 站点列表
curl -sk -H "Authorization: Bearer $TOKEN" $BASE/api/sites

# 创建 PHP 站点
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/sites \
  -d '{"name":"myblog","type":"php","domains":"blog.example.com","root":"/www/wwwroot/myblog","php_socket":"/run/php/php8.5-fpm.sock"}'

# 在节点 1 上执行命令（经审计）
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/nodes/1/exec -d '{"script":"df -h","timeout_sec":120}'
```

## 五、常用端点速查

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

## 六、安全注意事项

- Token 等效管理员：**务必仅经 HTTPS 调用**，HTTP 明文会泄露 Token
- 不要把 Token 写入公开仓库 / 日志 / 截图；泄露后立即在设置页重置
- 开放 API 的所有写操作同样进入面板审计日志（操作者显示为 `api`）
- 长时间操作（运行时安装、SSH 接入）注意客户端超时设置，建议 ≥ 15 分钟
