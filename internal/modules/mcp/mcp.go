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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/core"
)

const protoVer = "2025-03-26"

// Module MCP 模块。
type Module struct {
	db  *gorm.DB
	cfg *config.Config
	hc  *http.Client
}

func New(db *gorm.DB, cfg *config.Config) *Module {
	return &Module{
		db:  db,
		cfg: cfg,
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
	admin.DELETE("/mcp/token", m.clearToken)
}

// ---- 开放 API Token 管理 ----

func (m *Module) getToken(c *gin.Context) {
	t := core.GetSetting(m.db, core.KApiToken)
	core.OK(c, gin.H{"enabled": t != "", "token": t, "mcp_url": m.publicURL() + "/api/mcp"})
}

// genToken 生成/重置开放 API Token（启用开放 API）。
func (m *Module) genToken(c *gin.Context) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		core.Err(c, 500, "生成 Token 失败")
		return
	}
	t := hex.EncodeToString(b)
	if err := core.SetSetting(m.db, core.KApiToken, t); err != nil {
		core.Err(c, 500, "保存 Token 失败")
		return
	}
	slog.Info("开放 API Token 已生成", "by", c.GetString("user"))
	core.OK(c, gin.H{"enabled": true, "token": t, "mcp_url": m.publicURL() + "/api/mcp"})
}

// clearToken 禁用开放 API（清空 Token，已有客户端立即失效）。
func (m *Module) clearToken(c *gin.Context) {
	if err := core.SetSetting(m.db, core.KApiToken, ""); err != nil {
		core.Err(c, 500, "禁用失败")
		return
	}
	slog.Info("开放 API Token 已禁用", "by", c.GetString("user"))
	core.OK(c, gin.H{"enabled": false})
}

// publicURL 面板对外地址（配置的监听地址，供 MCP 客户端配置参考）。
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
// （转发请求方 Token/Cookie，权限与审计与会话操作完全一致）。
type toolDef struct {
	Name        string
	Description string
	Schema      map[string]any
}

var tools = []toolDef{
	{
		Name:        "panel_status",
		Description: "面板总览：系统信息、CPU/内存/磁盘、站点数、节点数等仪表盘汇总",
		Schema:      obj(map[string]any{}),
	},
	{
		Name:        "list_sites",
		Description: "网站站点列表（名称/类型/域名/SSL/启用状态）",
		Schema:      obj(map[string]any{}),
	},
	{
		Name:        "create_site",
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
		Name:        "delete_site",
		Description: "删除站点（仅移除 Nginx 配置，不删除网站文件）",
		Schema: obj(map[string]any{
			"id": map[string]any{"type": "number", "description": "站点 ID（先 list_sites 获取）"},
		}),
	},
	{
		Name:        "toggle_site",
		Description: "启用/停用站点",
		Schema: obj(map[string]any{
			"id": map[string]any{"type": "number", "description": "站点 ID"},
		}),
	},
	{
		Name:        "list_nodes",
		Description: "多节点列表（名称/系统架构/IP/在线状态/最近心跳）",
		Schema:      obj(map[string]any{}),
	},
	{
		Name:        "node_exec",
		Description: "在指定节点执行 shell 命令并返回输出（经审计）",
		Schema: obj(map[string]any{
			"node_id":     map[string]any{"type": "number", "description": "节点 ID（先 list_nodes 获取）"},
			"script":      map[string]any{"type": "string", "description": "bash 命令"},
			"timeout_sec": map[string]any{"type": "number", "description": "超时秒数，默认 120"},
		}),
	},
	{
		Name:        "list_runtimes",
		Description: "运行时（PHP/Node.js/Go/Python/OpenJDK）已安装版本与状态",
		Schema:      obj(map[string]any{}),
	},
	{
		Name:        "install_runtime",
		Description: "安装运行时版本（如 php 8.5 / nodejs v22 等，耗时操作）",
		Schema: obj(map[string]any{
			"lang":    map[string]any{"type": "string", "enum": []string{"php", "nodejs", "golang", "python", "openjdk"}},
			"version": map[string]any{"type": "string", "description": "版本号，如 8.5"},
		}),
	},
	{
		Name:        "api_request",
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
			if res, isNotice := m.dispatch(auth, cookie, r); !isNotice {
				out = append(out, res)
			}
		}
		c.JSON(http.StatusOK, out)
		return
	}
	res, isNotice := m.dispatch(auth, cookie, req)
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
func (m *Module) dispatch(auth, cookie string, req rpcReq) (rpcResp, bool) {
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
		text, err := m.execTool(auth, cookie, p.Name, p.Arguments)
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

// execTool 执行工具：统一走内部回环 HTTP（除 api_request 外按工具映射）。
func (m *Module) execTool(auth, cookie, name string, raw json.RawMessage) (string, error) {
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
	code, out, err := m.loop(auth, cookie, method, path, body)
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
