// Package mcp 面板开放 API 与 MCP Server（Model Context Protocol over Streamable HTTP）。
// - 开放 API Token：Authorization: Bearer <token> 或 X-Api-Token，等效 admin 会话，外部脚本直调任意 /api。
// - MCP：AI 客户端（Trae/Claude/Cursor 等）经 JSON-RPC 2.0 调用面板工具；工具执行通过
//   内部回环 HTTP 调用面板自身 API（复用请求方 Token），权限与审计与会话操作完全一致。
package mcp

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/auth"
	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/core"
)

const protoVer = "2025-03-26"

// Module MCP 模块。
type Module struct {
	db      *gorm.DB
	cfg     *config.Config
	authSvc *auth.Service
	hc      *http.Client
	// MCP 调用统计（内存态，按天归零；面板重启后从 0 开始）
	statsMu    sync.Mutex
	statsDay   string
	statsCount int64
}

func New(db *gorm.DB, cfg *config.Config, authSvc *auth.Service) *Module {
	return &Module{
		db:      db,
		cfg:     cfg,
		authSvc: authSvc,
		hc: &http.Client{
			Timeout: 15 * time.Minute,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 回环自调用，面板证书为自签 CA
			},
		},
	}
}

// Register 注册 MCP 端点与 Token 管理（均仅 admin 组）。
func (m *Module) Register(protected gin.IRouter, admin gin.IRouter) {
	protected.POST("/mcp", m.handle)
	admin.GET("/mcp/token", m.getToken)
	admin.POST("/mcp/token", m.genToken)
	admin.PUT("/mcp/token", m.updateTokenPerm)
	admin.DELETE("/mcp/token", m.clearToken)
	admin.GET("/mcp/agent-guide", m.agentGuide)
}

// ---- 开放 API Token 管理 ----

func (m *Module) getToken(c *gin.Context) {
	rec := m.authSvc.MCPTokenRec()
	if rec == nil {
		core.OK(c, gin.H{"enabled": false, "token": "", "mode": "full", "scopes": []string{}, "readonly": false, "scopes_known": auth.ScopesKnown, "today_calls": m.todayCalls(), "mcp_url": m.publicURL() + "/api/mcp"})
		return
	}
	scopes := rec.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	core.OK(c, gin.H{"enabled": true, "token": rec.Token, "mode": rec.Mode, "scopes": scopes, "readonly": rec.ReadOnly, "scopes_known": auth.ScopesKnown, "today_calls": m.todayCalls(), "mcp_url": m.publicURL() + "/api/mcp"})
}

// genToken 生成/重置 MCP Token（可携带权限配置：mode/scopes/readonly；与开放 API Token 相互独立）。
func (m *Module) genToken(c *gin.Context) {
	var p auth.PermInput
	_ = c.ShouldBindJSON(&p) // 空请求体 = 默认 full
	rec, err := auth.BuildPerm(&p)
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		core.Err(c, 500, "生成 Token 失败")
		return
	}
	rec.Token = hex.EncodeToString(b)
	if err := auth.SaveMCPTokenRecord(m.db, rec); err != nil {
		core.Err(c, 500, "保存 Token 失败")
		return
	}
	slog.Info("MCP Token 已生成", "by", c.GetString("user"), "mode", rec.Mode, "readonly", rec.ReadOnly)
	core.OK(c, gin.H{"enabled": true, "token": rec.Token, "mode": rec.Mode, "scopes": rec.Scopes, "readonly": rec.ReadOnly, "scopes_known": auth.ScopesKnown, "today_calls": m.todayCalls(), "mcp_url": m.publicURL() + "/api/mcp"})
}

// updateTokenPerm 仅更新 MCP Token 权限（Token 值不变，客户端无需重新配置）。
func (m *Module) updateTokenPerm(c *gin.Context) {
	rec := m.authSvc.MCPTokenRec()
	if rec == nil {
		core.Err(c, 400, "尚未启用 MCP Token")
		return
	}
	var p auth.PermInput
	if err := c.ShouldBindJSON(&p); err != nil {
		core.BindErr(c, err)
		return
	}
	perm, err := auth.BuildPerm(&p)
	if err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	rec.Mode, rec.Scopes, rec.ReadOnly = perm.Mode, perm.Scopes, perm.ReadOnly
	if err := auth.SaveMCPTokenRecord(m.db, rec); err != nil {
		core.Err(c, 500, "保存权限失败")
		return
	}
	slog.Info("MCP Token 权限已更新", "by", c.GetString("user"), "mode", rec.Mode, "readonly", rec.ReadOnly)
	core.OK(c, gin.H{"enabled": true, "token": rec.Token, "mode": rec.Mode, "scopes": rec.Scopes, "readonly": rec.ReadOnly, "scopes_known": auth.ScopesKnown, "today_calls": m.todayCalls(), "mcp_url": m.publicURL() + "/api/mcp"})
}


// clearToken 禁用 MCP Token（清空记录，已配置的 MCP 客户端立即失效）。
func (m *Module) clearToken(c *gin.Context) {
	if err := core.SetSetting(m.db, core.KMcpToken, ""); err != nil {
		core.Err(c, 500, "禁用失败")
		return
	}
	slog.Info("MCP Token 已禁用", "by", c.GetString("user"))
	core.OK(c, gin.H{"enabled": false})
}

// publicURL 面板对外地址（配置的监听地址，供 MCP 客户端配置参考）。
// todayCalls 当前今日调用次数（内存统计）。
func (m *Module) todayCalls() int64 {
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	if m.statsDay != time.Now().Format("2006-01-02") {
		return 0
	}
	return m.statsCount
}

// agentGuide 生成《AI Agent 自配置 MCP 指令》Markdown（admin 专用）。
// Agent 读取提示词后自行完成接入：读 mcp_info → 处理证书（如需）→ 写配置 → 验证；
// 交互形态参照宝塔 MCP 安装提示词。Token/证书链等敏感信息按需生成，不常驻页面。
func (m *Module) agentGuide(c *gin.Context) {
	rec := m.authSvc.MCPTokenRec()
	if rec == nil {
		core.Err(c, 400, "尚未启用 MCP Token，请先在系统设置生成")
		return
	}
	scheme := "http"
	if m.cfg.Server.TLS.Enabled {
		scheme = "https"
	}
	// 以请求 Host 生成访问地址（管理员当前可达地址），比占位符实用
	base := scheme + "://" + c.Request.Host
	url := base + "/api/mcp"
	tlsRequired := m.cfg.Server.TLS.Enabled
	certPEM := ""
	if tlsRequired && m.cfg.Server.TLS.Cert != "" {
		cp := m.cfg.Server.TLS.Cert
		if b, err := os.ReadFile(cp); err == nil {
			certPEM = string(b)
		} else if !filepath.IsAbs(cp) {
			for _, base2 := range []string{filepath.Join("..", m.cfg.DataDir), m.cfg.DataDir, "."} {
				if b2, err2 := os.ReadFile(filepath.Join(base2, cp)); err2 == nil {
					certPEM = string(b2)
					break
				}
			}
		}
	}
	if tlsRequired && certPEM == "" {
		certPEM = "（证书文件读取失败，请手动从面板安装目录 " + m.cfg.Server.TLS.Cert + " 导出证书链）"
	}
	core.OK(c, gin.H{"markdown": buildAgentGuide(rec.Token, url, tlsRequired, certPEM), "mcp_url": url})
}

// buildAgentGuide 组装自配置指令 Markdown。
func buildAgentGuide(token, url string, tlsRequired bool, certPEM string) string {
	var b strings.Builder
	b.WriteString("# 寒语馨面板 MCP · AI Agent 自配置指令\n\n")
	b.WriteString("> 你是 AI Agent。你的任务：根据下方 mcp_info，为**自身**配置好寒语馨面板的 MCP 连接并验证可用。\n")
	b.WriteString("> 你只做：读取信息 → 处理证书（如需）→ 把凭证写入自身配置 → 验证可用。\n\n")
	b.WriteString("---\n\n## 输入：mcp_info\n\n| 字段 | 值 |\n|---|---|\n")
	b.WriteString("| api_token | `" + token + "`（Bearer 凭证，按配置的权限模式生效，仅限本人使用） |\n")
	b.WriteString("| url | `" + url + "`（MCP Streamable HTTP，JSON-RPC 2.0） |\n")
	tlsVal := "false"
	if tlsRequired {
		tlsVal = "true（自签/私有 CA，需处理证书链）"
	}
	b.WriteString("| tls_required | " + tlsVal + " |\n")
	if tlsRequired {
		b.WriteString("\ntls_cert（证书链 PEM 全文，**原样**保存为 hanyuxin_panel.crt——含叶子+中间+根，勿只取第一张）：\n\n```text\n" + certPEM + "\n```\n")
	}
	b.WriteString("\n---\n\n## 第一步：确认自身身份（配置方式索引）\n\n")
	b.WriteString("| Agent | 配置方式 |\n|---|---|\n")
	b.WriteString("| Claude Code | 推荐 CLI：`claude mcp add --transport http hanyuxin-panel " + url + " --header \"Authorization: Bearer " + token + "\"`；或写项目 .mcp.json |\n")
	b.WriteString("| Codex | ~/.codex/config.toml → [mcp_servers.hanyuxin-panel]：type=\"http\"、url、headers={Authorization=\"Bearer <token>\"} |\n")
	b.WriteString("| Cursor | ~/.cursor/mcp.json（或项目 .cursor/mcp.json）→ mcpServers.hanyuxin-panel：{ url, headers } |\n")
	b.WriteString("| Trae | 须在客户端 UI 添加（官方禁 AI 直写配置文件） |\n")
	b.WriteString("| 其余 Agent | 查阅自身官方最新文档：写入 url + Authorization Bearer 头即可，**不要猜测**路径与格式 |\n")
	if tlsRequired {
		b.WriteString("\n## 第二步：证书处理\n\n将上方 tls_cert 原样保存为 hanyuxin_panel.crt（完整证书链），然后按下表选择信任方式（平铺无优先级，按客户端支持选择）：\n\n")
		b.WriteString("| 方式 | 做法 | 适用 |\n|---|---|---|\n")
		b.WriteString("| A 系统信任库 | Windows 管理员 PowerShell：Import-Certificate -FilePath hanyuxin_panel.crt -CertStoreLocation Cert:\\LocalMachine\\Root；Debian/Ubuntu：cp 到 /usr/local/share/ca-certificates/ 并 update-ca-certificates；macOS：security add-trusted-cert | 走系统信任链的客户端 |\n")
		b.WriteString("| B 系统环境变量 | setx NODE_EXTRA_CA_CERTS <crt 绝对路径>（用户级，新进程生效） | Node 系客户端（实测可靠，免管理员） |\n")
		b.WriteString("| C 配置内 env | MCP 配置的 env 中加 NODE_EXTRA_CA_CERTS 指向 crt | 支持 env 的 stdio 类客户端（http 类型通常无 env 字段） |\n")
		b.WriteString("| D 跟随系统库 | env 中加 NODE_OPTIONS=--use-system-ca（Node ≥22） | 配合方式 A 使用 |\n")
		b.WriteString("| E 跳过验证 | env 中加 NODE_TLS_REJECT_UNAUTHORIZED=0 | 仅临时测试，不安全 |\n")
	}
	stepNo := "二"
	if tlsRequired {
		stepNo = "三"
	}
	b.WriteString("\n## 第" + stepNo + "步：写入自身配置后验证\n\n- [ ] 配置中的 url 与 api_token 与本文一致\n")
	if tlsRequired {
		b.WriteString("- [ ] 证书信任方式已生效（无 self-signed / unable to verify 报错）\n")
	}
	b.WriteString("- [ ] 客户端能成功 tools/list 列出面板工具，无 401\n")
	b.WriteString("\n**常见失败排查**：401 = Token 填错或失效；self-signed = 证书未信任（换一种信任方式）；hostname/IP mismatch = 地址不在证书 SAN 内（本文 url 以当前访问地址生成，一般不会出现）；ECONNREFUSED = 面板未运行或端口未放行。\n\n")
	b.WriteString("---\n\n> **安全提醒**：本提示词包含 Token（等效管理员权限，按配置的权限模式生效），请勿转发或泄露；泄露后请在面板重置 MCP Token。\n")
	return b.String()
}

func (m *Module) publicURL() string {
	scheme := "http"
	if m.cfg.Server.TLS.Enabled {
		scheme = "https"
	}
	return scheme + "://<面板IP>" + m.cfg.Server.Listen
}

// ---- JSON-RPC 2.0 ----

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolDef MCP 工具定义；执行统一按 name 分发到 execTool，经内部回环调用面板 API
// （转发请求方 Token/Cookie，权限与审计与会话操作完全一致）。Scope 为所需模块权限，
// Token 模式下 tools/list 按 scope 过滤、tools/call 预检查。
type toolDef struct {
	Name        string
	Scope       string
	Description string
	Schema      map[string]any
}

var tools = []toolDef{
	{
		Name: "panel_status", Scope: "status",
		Description: "面板总览：系统信息、CPU/内存/磁盘、站点数、节点数等仪表盘汇总",
		Schema:      obj(map[string]any{}),
	},
	{
		Name: "list_sites", Scope: "sites",
		Description: "网站站点列表（名称/类型/域名/SSL/启用状态）",
		Schema:      obj(map[string]any{}),
	},
	{
		Name: "create_site", Scope: "sites",
		Description: "创建网站站点：type=static 静态站 / php PHP 站 / proxy 反向代理",
		Schema: obj(map[string]any{
			"name":    map[string]any{"type": "string", "description": "站点标识，小写字母数字点短横线"},
			"type":    map[string]any{"type": "string", "enum": []string{"static", "php", "proxy"}},
			"domains": map[string]any{"type": "string", "description": "空格分隔的域名"},
			"root":    map[string]any{"type": "string", "description": "网站根目录（static/php 必填）"},
			"upstream": map[string]any{"type": "string", "description": "反代目标（proxy 必填），如 http://127.0.0.1:3000"},
		}),
	},
	{
		Name: "delete_site", Scope: "sites",
		Description: "删除站点（仅移除 Nginx 配置，不删除网站文件）",
		Schema: obj(map[string]any{
			"id": map[string]any{"type": "number", "description": "站点 ID（先 list_sites 获取）"},
		}),
	},
	{
		Name: "toggle_site", Scope: "sites",
		Description: "启用/停用站点",
		Schema: obj(map[string]any{
			"id": map[string]any{"type": "number", "description": "站点 ID"},
		}),
	},
	{
		Name: "list_nodes", Scope: "nodes",
		Description: "多节点列表（名称/系统架构/IP/在线状态/最近心跳）",
		Schema:      obj(map[string]any{}),
	},
	{
		Name: "node_exec", Scope: "nodes",
		Description: "在指定节点执行 shell 命令并返回输出（经审计）",
		Schema: obj(map[string]any{
			"node_id":     map[string]any{"type": "number", "description": "节点 ID（先 list_nodes 获取）"},
			"script":      map[string]any{"type": "string", "description": "bash 命令"},
			"timeout_sec": map[string]any{"type": "number", "description": "超时秒数，默认 120"},
		}),
	},
	{
		Name: "list_runtimes", Scope: "runtimes",
		Description: "运行时（PHP/Node.js/Go/Python/OpenJDK）已安装版本与状态",
		Schema:      obj(map[string]any{}),
	},
	{
		Name: "install_runtime", Scope: "runtimes",
		Description: "安装运行时版本（如 php 8.5 / nodejs v22 等，耗时操作）",
		Schema: obj(map[string]any{
			"lang":    map[string]any{"type": "string", "enum": []string{"php", "nodejs", "golang", "python", "openjdk"}},
			"version": map[string]any{"type": "string", "description": "版本号，如 8.5"},
		}),
	},
	{
		Name: "api_request", Scope: "", // 按实际 method/path 判定
		Description: "万能工具：直接调用面板任意 REST API（路径以 /api 开头），适合上述工具未覆盖的操作",
		Schema: obj(map[string]any{
			"method": map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "DELETE"}},
			"path":   map[string]any{"type": "string", "description": "API 路径，如 /api/certs"},
			"body":   map[string]any{"type": "object", "description": "JSON 请求体（GET 留空）"},
		}),
	},
}

func obj(props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props}
}

// handle MCP Streamable HTTP 端点（JSON-RPC 2.0）。
func (m *Module) handle(c *gin.Context) {
	// 会话中间件已放行（API Token 或浏览器会话）；工具回环调用转发请求方凭证
	auth := c.GetHeader("Authorization")
	if auth == "" {
		if t := c.GetHeader("X-Api-Token"); t != "" {
			auth = "Bearer " + t
		}
	}
	cookie := c.GetHeader("Cookie")
	authHdr := auth
	// 今日调用计数（每次 MCP 端点请求 +1，按天归零）
	m.statsMu.Lock()
	today := time.Now().Format("2006-01-02")
	if m.statsDay != today {
		m.statsDay, m.statsCount = today, 0
	}
	m.statsCount++
	m.statsMu.Unlock()
	permRec, isToken := m.authSvc.MCPTokenPerm(c)
	var req rpcReq
	body, err := c.GetRawData()
	if err != nil || json.Unmarshal(body, &req) != nil {
		writeRPC(c, rpcResp{JSONRPC: protoVer, Error: &rpcErr{Code: -32600, Message: "invalid request"}})
		return
	}
	// 批量请求（数组）
	if len(body) > 0 && body[0] == '[' {
		var arr []rpcReq
		_ = json.Unmarshal(body, &arr)
		out := make([]rpcResp, 0, len(arr))
		for _, r := range arr {
			if res, isNotice := m.dispatch(permRec, isToken, authHdr, cookie, r); !isNotice {
				out = append(out, res)
			}
		}
		c.JSON(http.StatusOK, out)
		return
	}
	res, isNotice := m.dispatch(permRec, isToken, authHdr, cookie, req)
	if isNotice {
		c.Status(http.StatusAccepted)
		return
	}
	writeRPC(c, res)
}

func writeRPC(c *gin.Context, res rpcResp) {
	c.JSON(http.StatusOK, res)
}

// dispatch 处理单条 JSON-RPC；通知类请求返回 isNotice=true（无响应体）。
// permRec 非 nil 表示 Token 调用（按 scope 过滤工具）；authHdr/cookie 用于工具回环调用。
func (m *Module) dispatch(permRec *auth.APITokenRec, isToken bool, authHdr, cookie string, req rpcReq) (rpcResp, bool) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		ver := p.ProtocolVersion
		if ver == "" {
			ver = protoVer
		}
		return rpcResp{JSONRPC: protoVer, ID: req.ID, Result: gin.H{
			"protocolVersion": ver,
			"capabilities":    gin.H{"tools": gin.H{}},
			"serverInfo":      gin.H{"name": "hanyuxin-panel", "version": core.Version},
		}}, false
	case "notifications/initialized", "":
		return rpcResp{}, true
	case "tools/list":
		items := make([]gin.H, 0, len(tools))
		for _, t := range tools {
			// Token 自定义权限模式：过滤无权模块的工具（full 模式/会话调用全量）
			if permRec != nil && isToken && permRec.Mode == "custom" && !m.toolAllowed(permRec, t.Scope) {
				continue
			}
			items = append(items, gin.H{"name": t.Name, "description": t.Description, "inputSchema": t.Schema})
		}
		return rpcResp{JSONRPC: protoVer, ID: req.ID, Result: gin.H{"tools": items}}, false
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return rpcErrResp(req.ID, -32602, "参数错误"), false
		}
		text, err := m.execTool(authHdr, cookie, p.Name, p.Arguments)
		if err != nil {
			return rpcResp{JSONRPC: protoVer, ID: req.ID, Result: gin.H{
				"content": []gin.H{{"type": "text", "text": err.Error()}}, "isError": true,
			}}, false
		}
		return rpcResp{JSONRPC: protoVer, ID: req.ID, Result: gin.H{
			"content": []gin.H{{"type": "text", "text": text}},
		}}, false
	default:
		return rpcErrResp(req.ID, -32601, "未知方法: "+req.Method), false
	}
}

func rpcErrResp(id json.RawMessage, code int, msg string) rpcResp {
	return rpcResp{JSONRPC: protoVer, ID: id, Error: &rpcErr{Code: code, Message: msg}}
}

// toolAllowed Token 自定义权限下工具是否可见/可调：工具 scope 已授权；
// api_request（无固定 scope）需至少授权一个模块。readonly 不影响可见性（仅拦截写方法）。
func (m *Module) toolAllowed(rec *auth.APITokenRec, scope string) bool {
	if scope == "" {
		return len(rec.Scopes) > 0
	}
	for _, s := range rec.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// execTool 执行工具：统一走内部回环 HTTP（除 api_request 外按工具映射）。
func (m *Module) execTool(authHdr, cookie, name string, raw json.RawMessage) (string, error) {
	var args map[string]any
	_ = json.Unmarshal(raw, &args)
	num := func(k string) (int64, error) {
		v, _ := args[k].(float64)
		if v == 0 {
			return 0, fmt.Errorf("缺少参数 %s", k)
		}
		return int64(v), nil
	}
	str := func(k string) string { s, _ := args[k].(string); return strings.TrimSpace(s) }

	var method, path string
	var body any
	switch name {
	case "panel_status":
		method, path = "GET", "/api/dashboard/summary"
	case "list_sites":
		method, path = "GET", "/api/sites"
	case "create_site":
		method, path = "POST", "/api/sites"
		body = args
	case "delete_site":
		id, err := num("id")
		if err != nil {
			return "", err
		}
		method, path = "DELETE", fmt.Sprintf("/api/sites/%d", id)
	case "toggle_site":
		id, err := num("id")
		if err != nil {
			return "", err
		}
		method, path = "POST", fmt.Sprintf("/api/sites/%d/toggle", id)
	case "list_nodes":
		method, path = "GET", "/api/nodes"
	case "node_exec":
		id, err := num("node_id")
		if err != nil {
			return "", err
		}
		script := str("script")
		if script == "" {
			return "", fmt.Errorf("缺少参数 script")
		}
		method, path = "POST", fmt.Sprintf("/api/nodes/%d/exec", id)
		body = gin.H{"script": script, "timeout_sec": int(num2(args, "timeout_sec", 120))}
	case "list_runtimes":
		method, path = "GET", "/api/runtimes"
	case "install_runtime":
		lang, ver := str("lang"), str("version")
		if lang == "" || ver == "" {
			return "", fmt.Errorf("缺少参数 lang / version")
		}
		method, path = "POST", "/api/runtimes/"+lang+"/install"
		body = gin.H{"version": ver}
	case "api_request":
		method, path = str("method"), str("path")
		if method == "" {
			method = "GET"
		}
		if !strings.HasPrefix(path, "/api/") {
			return "", fmt.Errorf("path 需以 /api/ 开头")
		}
		if b, ok := args["body"]; ok && b != nil && method != "GET" {
			body = b
		}
	default:
		return "", fmt.Errorf("未知工具: %s", name)
	}
	code, out, err := m.loop(authHdr, cookie, method, path, body)
	if err != nil {
		return "", fmt.Errorf("面板调用失败: %w", err)
	}
	if code != 0 && code != 200 {
		return fmt.Sprintf("HTTP %d\n%s", code, out), nil
	}
	return out, nil
}

func num2(args map[string]any, k string, def float64) float64 {
	if v, ok := args[k].(float64); ok && v > 0 {
		return v
	}
	return def
}

// loop 内部回环调用面板自身 API（携带请求方 Token/Cookie）。
func (m *Module) loop(auth, cookie, method, path string, body any) (int, string, error) {
	scheme := "http"
	if m.cfg.Server.TLS.Enabled {
		scheme = "https"
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, "", err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, scheme+"://127.0.0.1"+m.cfg.Server.Listen+path, rdr)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", auth)
	// 标记 MCP 工具内部调用：鉴权中间件仅对本机回环 + 此标记放行 MCP Token 直调面板 API
	req.Header.Set("X-MCP-Loopback", "1")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, string(data), nil
}
