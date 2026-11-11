// apitoken.go —— 开放 API Token 记录与权限（scope）校验。
// KApiToken 存 JSON 记录：{"token":"<48hex>","mode":"full|custom","scopes":[...],"readonly":bool}
// 旧格式（纯 hex 字符串）自动兼容为 full 权限。
package auth

import (
	"crypto/subtle"
	"encoding/json"
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
	rec := s.APITokenRec()
	return rec != nil && s.apiTokenMatch(c, rec)
}

// apiTokenDenied 判断该请求是否被 Token 权限拒绝（rec 已验证一致）。
func (s *Service) apiTokenDenied(c *gin.Context, rec *APITokenRec) bool {
	path := c.Request.URL.Path
	// Token 管理端点仅限面板会话：Token 不能查看/修改自身权限
	if path == "/api/mcp/token" {
		return true
	}
	if rec.Mode == "full" {
		return false
	}
	// 只读模式：仅放行查询方法
	if rec.ReadOnly {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			return true
		}
	}
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
