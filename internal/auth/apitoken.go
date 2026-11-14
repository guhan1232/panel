// apitoken.go —— 开放 API Token 记录与权限（scope）校验。
// KApiToken 存 JSON 记录：{"token":"<48hex>","mode":"full|custom","scopes":[...],"readonly":bool}
// 旧格式（纯 hex 字符串）自动兼容为 full 权限。
package auth

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
)

// APITokenRec Token 记录。
type APITokenRec struct {
	Token    string   `json:"token"`
	Mode     string   `json:"mode"`          // full（全部权限）| custom（按 scopes）
	Scopes   []string `json:"scopes"`        // mode=custom 时生效的模块清单
	ReadOnly bool     `json:"readonly"`      // 仅允许 GET/HEAD（对所有已授权模块）
}

// scopePrefixes 模块 scope → API 路径前缀（path 以 /api/ 开头）。
var scopePrefixes = []struct {
	Scope string
	List  []string
}{
	{"status", []string{"/api/dashboard"}},
	{"sites", []string{"/api/sites"}},
	{"runtimes", []string{"/api/runtimes"}},
	{"nodes", []string{"/api/nodes"}},
	{"certs", []string{"/api/certs"}},
	{"firewall", []string{"/api/firewall"}},
	{"dbmgr", []string{"/api/dbmgr"}},
	{"apps", []string{"/api/apps"}},
	{"tasks", []string{"/api/tasks"}},
	{"files", []string{"/api/files"}},
	{"term", []string{"/api/ws/terminal", "/api/ws/ssh", "/api/term", "/api/terminal-logs"}},
	{"proc", []string{"/api/proc"}},
	{"notify", []string{"/api/notify"}},
	{"alerts", []string{"/api/alerts"}},
	{"audit", []string{"/api/audit"}},
	{"s3", []string{"/api/s3"}},
	{"setting", []string{"/api/setting", "/api/users"}},
	{"auth", []string{"/api/auth/me", "/api/auth/logout", "/api/auth/sessions", "/api/sessions"}},
	{"mcp", []string{"/api/mcp"}},
}

// ScopesKnown 全部可用 scope（前端权限编辑用）。
var ScopesKnown = func() []string {
	out := []string{}
	for _, s := range scopePrefixes {
		out = append(out, s.Scope)
	}
	return out
}()

// parseTokenRecord 解析 KApiToken 存储值（JSON 记录或旧格式纯 hex）。
func parseTokenRecord(raw string) *APITokenRec {
	if raw == "" {
		return nil
	}
	rec := &APITokenRec{}
	if err := json.Unmarshal([]byte(raw), rec); err == nil && rec.Token != "" {
		if rec.Mode != "custom" {
			rec.Mode = "full"
			rec.Scopes = nil
		}
		return rec
	}
	return &APITokenRec{Token: raw, Mode: "full"} // 旧格式兼容
}

// SaveTokenRecord 写回 Token 记录。
func SaveTokenRecord(db *gorm.DB, rec *APITokenRec) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return core.SetSetting(db, core.KApiToken, string(b))
}

// APITokenRec 读取当前 Token 记录（无 Token 返回 nil）。
func (s *Service) APITokenRec() *APITokenRec {
	return parseTokenRecord(core.GetSetting(s.db, core.KApiToken))
}

// apiTokenMatch 请求携带的 Token 是否与记录一致（恒定时间比较）。
func (s *Service) apiTokenMatch(c *gin.Context, rec *APITokenRec) bool {
	t := apiTokenFrom(c)
	return t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(rec.Token)) == 1
}

// APITokenValid 请求携带有效 Token（入口豁免判断用，不含 scope）。
func (s *Service) APITokenValid(c *gin.Context) bool {
	if rec := s.APITokenRec(); rec != nil && s.apiTokenMatch(c, rec) {
		return true
	}
	if rec := s.MCPTokenRec(); rec != nil && s.apiTokenMatch(c, rec) {
		return true
	}
	return false
}

// apiTokenDenied 判断该请求是否被 Token 权限拒绝（rec 已验证一致）。
// apiTokenDenied 开放 API Token 的权限判断（rec 已验证一致）。
// 分家规则：不可调用 MCP 端点（MCP 专属 Token），也不可管理自身。
func (s *Service) apiTokenDenied(c *gin.Context, rec *APITokenRec) bool {
	path := c.Request.URL.Path
	if path == "/api/setting/api-token" || path == "/api/mcp" || strings.HasPrefix(path, "/api/mcp/") {
		return true
	}
	return tokenPermDenied(c, rec)
}

// mcpTokenDenied MCP Token 的权限判断（scope 规则与 API Token 一致；管理端点恒拒）。
func (s *Service) mcpTokenDenied(c *gin.Context, rec *APITokenRec) bool {
	if c.Request.URL.Path == "/api/mcp/token" {
		return true
	}
	return tokenPermDenied(c, rec)
}

// tokenPermDenied scope/readonly 通用校验（mode=full 全放行；custom 按 scopePrefixes 前缀匹配）。
func tokenPermDenied(c *gin.Context, rec *APITokenRec) bool {
	if rec.Mode == "full" {
		return false
	}
	if rec.ReadOnly {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			return true
		}
	}
	path := c.Request.URL.Path
	for _, sp := range scopePrefixes {
		if !containsScope(rec.Scopes, sp.Scope) {
			continue
		}
		for _, p := range sp.List {
			if path == p || strings.HasPrefix(path, p+"/") || strings.HasPrefix(path, p+"?") {
				return false
			}
		}
	}
	return true
}

func containsScope(scopes []string, s string) bool {
	for _, v := range scopes {
		if v == s {
			return true
		}
	}
	return false
}

// APIPerm 返回该请求的 Token 权限记录；第二个值 false 表示非 Token 调用
// （浏览器会话，MCP 工具全量放行）。
func (s *Service) APIPerm(c *gin.Context) (*APITokenRec, bool) {
	rec := s.APITokenRec()
	if rec == nil || !s.apiTokenMatch(c, rec) {
		return nil, false
	}
	return rec, true
}

// MCPTokenPerm 返回该请求的 MCP Token 权限记录；第二个值 false 表示非 MCP Token 调用。
func (s *Service) MCPTokenPerm(c *gin.Context) (*APITokenRec, bool) {
	rec := s.MCPTokenRec()
	if rec == nil || !s.apiTokenMatch(c, rec) {
		return nil, false
	}
	return rec, true
}

// isLoopback 请求是否来自本机回环（MCP 工具执行经进程内回环 HTTP 调用面板 API）。
func isLoopback(c *gin.Context) bool {
	addr := c.Request.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		addr = strings.Trim(addr[:i], "[]")
	}
	return addr == "127.0.0.1" || addr == "::1"
}

// MCPTokenRec 读取 MCP Server 专用 Token 记录（与开放 API Token 相互独立）。
func (s *Service) MCPTokenRec() *APITokenRec {
	return parseTokenRecord(core.GetSetting(s.db, core.KMcpToken))
}

// SaveMCPTokenRecord 写回 MCP Token 记录。
func SaveMCPTokenRecord(db *gorm.DB, rec *APITokenRec) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return core.SetSetting(db, core.KMcpToken, string(b))
}

// MCPTokenRecOf 读取 MCP Token 记录（包级，供无 authSvc 的模块使用）。
func MCPTokenRecOf(db *gorm.DB) *APITokenRec {
	return parseTokenRecord(core.GetSetting(db, core.KMcpToken))
}

// APITokenRecOf 读取开放 API Token 记录（包级，供无 authSvc 的模块使用）。
func APITokenRecOf(db *gorm.DB) *APITokenRec {
	return parseTokenRecord(core.GetSetting(db, core.KApiToken))
}

// TokenMatch 请求携带的 Token 是否与记录一致（包级，恒定时间比较）。
func TokenMatch(c *gin.Context, rec *APITokenRec) bool {
	t := apiTokenFrom(c)
	return t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(rec.Token)) == 1
}

// PermInput 权限配置输入（开放 API Token 与 MCP Token 共用校验规则）。
type PermInput struct {
	Mode     string   `json:"mode"`
	Scopes   []string `json:"scopes"`
	ReadOnly bool     `json:"readonly"`
}

// BuildPerm 校验并归一化权限配置。
func BuildPerm(p *PermInput) (*APITokenRec, error) {
	rec := &APITokenRec{Mode: p.Mode, Scopes: p.Scopes, ReadOnly: p.ReadOnly}
	if rec.Mode == "" {
		rec.Mode = "full"
	}
	if rec.Mode != "full" && rec.Mode != "custom" {
		return nil, fmt.Errorf("mode 仅支持 full / custom")
	}
	if rec.Mode == "full" {
		rec.Scopes = nil
		rec.ReadOnly = false
		return rec, nil
	}
	known := map[string]bool{}
	for _, s := range ScopesKnown {
		known[s] = true
	}
	for _, s := range rec.Scopes {
		if !known[s] {
			return nil, fmt.Errorf("未知模块 scope: %s", s)
		}
	}
	return rec, nil
}
