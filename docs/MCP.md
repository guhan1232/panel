# 寒语馨面板 · MCP 功能文档

> 适用版本：v0.39.0+
>
> 面板内置 MCP Server（Model Context Protocol over Streamable HTTP），AI 客户端（Trae、Claude Code、Cursor、Cherry Studio 等）可直连面板，用自然语言完成站点管理、运行时安装、节点运维等操作。Token 支持**按模块授权**与**只读模式**（见 [API 文档 · Token 权限体系](API.md)）。

## 一、概述

| 项目 | 值 |
|---|---|
| 端点 | `https://面板地址:端口/api/mcp` |
| 协议 | MCP Streamable HTTP（JSON-RPC 2.0），协议版本 `2025-03-26` |
| 鉴权 | `Authorization: Bearer <token>`（开放 API Token，与 REST API 同一枚） |
| 方法 | `initialize` / `notifications/initialized` / `tools/list` / `tools/call`（支持批量请求数组） |

- Token 与开放 API 共用：系统设置 →「开放 API 与 MCP」→ 生成 Token（可配置权限模式与只读）
- 工具执行通过面板内部回环调用自身 REST API 实现，**权限与审计与会话操作完全一致**（审计日志操作者为 `api`）
- **Token 需授权 `mcp` 模块（scope）才能调用 MCP 端点**：自定义权限未勾选「MCP 工具」时，`/api/mcp` 整体返回 403
- Token 自定义权限下 `tools/list` 只返回已授权模块对应的工具；越权调用返回 `isError: true`（内容为 403 说明）
- 只读 Token：工具可见但不限于查询类；写操作工具调用将被 403 拒绝
- 未启用 Token 时端点不可用；重置 Token 后旧客户端立即失效

## 二、客户端接入

### Trae / Claude Code / Cursor / 通用配置

系统设置页生成 Token 后，卡片会展示可直接复制的配置（面板地址与 Token 自动填充）：

```json
{
  "mcpServers": {
    "hanyuxin-panel": {
      "url": "https://面板地址:端口/api/mcp",
      "headers": {
        "Authorization": "Bearer <你的Token>"
      }
    }
  }
}
```

- Trae：设置 → MCP → 添加 → 粘贴上述 JSON（或手动填 URL + Header）
- Claude Code：写入 `.mcp.json` 或 `claude mcp add` 配置
- Cursor：设置 → MCP → Add new global MCP server

> HTTPS 自签证书说明：面板默认证书由面板 CA 签发，部分客户端校验证书失败时，请在客户端开启「跳过证书校验 / Insecure」选项，或使用已上传域名证书的反代地址。

## 三、工具清单（10 个）

> 自定义权限模式下，Token 仅能看到并调用已授权模块对应的工具（`panel_status`→status，站点四件套→sites，节点两件套→nodes，运行时两件套→runtimes；`api_request` 需至少授权一个模块）。

### 1. panel_status — 面板总览（scope: status）

无参数。返回系统信息、CPU / 内存 / 磁盘、站点数、节点数等仪表盘汇总。

### 2. list_sites — 站点列表（scope: sites）

无参数。返回全部站点（名称 / 类型 / 域名 / SSL / 启用状态 / 配置文件存在性）。

### 3. create_site — 创建站点（scope: sites）

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | ✅ | 站点标识，小写字母数字点短横线 |
| `type` | string | ✅ | `static` 静态站 / `php` PHP 站 / `proxy` 反向代理 |
| `domains` | string | | 空格分隔的域名 |
| `root` | string | | 网站根目录（static / php 必填） |
| `upstream` | string | | 反代目标（proxy 必填），如 `http://127.0.0.1:3000` |

创建时自动 `nginx -t` 校验并重载。

### 4. delete_site — 删除站点（scope: sites）

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | number | ✅ | 站点 ID（先 `list_sites` 获取） |

仅移除 Nginx 配置，不删除网站文件。

### 5. toggle_site — 启停站点（scope: sites）

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | number | ✅ | 站点 ID |

### 6. list_nodes — 节点列表（scope: nodes）

无参数。返回节点（名称 / 系统架构 / IP / 在线状态 / 最近心跳）。

### 7. node_exec — 节点远程命令（scope: nodes）

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `node_id` | number | ✅ | 节点 ID（先 `list_nodes` 获取，仅在线节点） |
| `script` | string | ✅ | bash 命令 |
| `timeout_sec` | number | | 超时秒数，默认 120 |

命令经面板审计记录。

### 8. list_runtimes — 运行时状态（scope: runtimes）

无参数。返回 PHP / Node.js / Go / Python / OpenJDK 已安装版本。

### 9. install_runtime — 安装运行时（scope: runtimes）

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `lang` | string | ✅ | `php` / `nodejs` / `golang` / `python` / `openjdk` |
| `version` | string | ✅ | 版本号，如 `8.5`、`v22.11.0` |

PHP 经发行版仓库安装（含源配置与依赖包），耗时可达数分钟。

### 10. api_request — 万能透传（scope: 按目标路径判定）

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `method` | string | | `GET` / `POST` / `PUT` / `DELETE`，默认 GET |
| `path` | string | ✅ | API 路径，必须以 `/api/` 开头，如 `/api/certs` |
| `body` | object | | JSON 请求体（GET 留空） |

适合上述工具未覆盖的操作（证书、防火墙、数据库、计划任务、备份等）。

## 四、调用示例（原始 JSON-RPC）

```bash
TOKEN=你的token
BASE=https://面板地址:端口

# 1. 初始化握手
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/mcp \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"demo","version":"1"}}}'
# → {"jsonrpc":"2025-03-26","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"hanyuxin-panel","version":"0.38.9"}}}

# 2. 列出工具
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/mcp -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

# 3. 调用工具：查看站点列表
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/mcp \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_sites","arguments":{}}}'
# → {"jsonrpc":"2025-03-26","id":3,"result":{"content":[{"type":"text","text":"{\"code\":0,...}"}]}}

# 4. 调用工具：在节点执行命令
curl -sk -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -X POST $BASE/api/mcp \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"node_exec","arguments":{"node_id":1,"script":"uptime"}}}'
```

工具执行结果统一封装为 `content: [{type:"text", text: 面板API原始JSON}]`；执行失败时 `isError: true`，错误信息在 text 中。

## 五、错误处理

| 场景 | 表现 |
|---|---|
| Token 未启用 / 错误 | HTTP 404（入口防护语义，不暴露端点） |
| 未知方法 | JSON-RPC error `-32601` |
| 参数缺失 / 非法 | `tools/call` 返回 `isError: true` 与原因；JSON-RPC 层错误为 `-32602` |
| 面板 API 业务错误 | text 中包含面板原始响应（`code != 0` 与 `msg`） |
| 通知类请求（如 `notifications/initialized`） | HTTP 202，无响应体 |

## 六、典型用法

- **对话式运维**：「帮我在 node-1 上看下磁盘占用」→ AI 自动调用 `list_nodes` + `node_exec`
- **建站流水线**：「创建一个 PHP 站 blog.example.com 用 PHP 8.5」→ `install_runtime`(若无) + `create_site`
- **巡检报告**：「汇总所有节点状态和站点启用情况」→ `panel_status` + `list_nodes` + `list_sites`
- **任意扩展**：面板后续新增接口无需升级 MCP Server，直接用 `api_request` 调用

## 七、安全说明

- Token 等效管理员：泄露后立即在设置页重置；仅经 HTTPS 调用
- 所有 MCP 工具调用进入面板审计日志（方法为 `MCP 工具调用`，远程命令另有命令级审计）
- `node_exec` 建议仅对可信节点使用；命令会以 Agent 运行身份（通常 root）执行
