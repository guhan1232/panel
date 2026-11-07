// 用户管理（仅 admin）：CRUD、角色调整、禁用、重置密码。
// 自锁保护：不能修改自己的角色/禁用自己/删除自己；最后一个可用 admin 不可降级或禁用。
package auth

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"cnb.cool/hanyuxin/panel/internal/core"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{2,32}$`)

// RequireRole 指定角色放行中间件。
func (s *Service) RequireRole(roles ...Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := Role(c.GetString("role"))
		for _, want := range roles {
			if r == want {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "当前角色无权访问"})
	}
}

func (s *Service) RequireAdmin() gin.HandlerFunc { return s.RequireRole(RoleAdmin) }

// RegisterAdmin 注册用户管理路由（挂载于鉴权组，内部再校验 admin）。
func (s *Service) RegisterAdmin(api gin.IRouter) {
	g := api.Group("/users", s.RequireAdmin())
	g.GET("", s.listUsers)
	g.POST("", s.createUser)
	g.PUT("/:id", s.updateUser)
	g.DELETE("/:id", s.removeUser)
}

func (s *Service) listUsers(c *gin.Context) {
	var users []core.User
	s.db.Select("id, username, role, disabled, created_at, updated_at").Order("id").Find(&users)
	core.OK(c, users)
}

type userReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Disabled *bool  `json:"disabled"`
}

// enabledAdminCount 统计除 excludeID 外可用的 admin 数量。
func (s *Service) enabledAdminCount(excludeID uint) int64 {
	var n int64
	q := s.db.Model(&core.User{}).Where("role = ? AND disabled = ?", string(RoleAdmin), false)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	q.Count(&n)
	return n
}

func (s *Service) createUser(c *gin.Context) {
	var req userReq
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !usernameRe.MatchString(req.Username) {
		core.Err(c, 400, "用户名需为 2-32 位字母/数字/下划线")
		return
	}
	if len(req.Password) < 8 {
		core.Err(c, 400, "密码至少 8 位")
		return
	}
	if req.Role == "" {
		req.Role = string(RoleOperator)
	}
	if !ValidRole(req.Role) {
		core.Err(c, 400, "角色非法（admin/operator/viewer）")
		return
	}
	var count int64
	s.db.Model(&core.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		core.Err(c, 400, "用户名已存在")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	u := core.User{Username: req.Username, Password: string(hash), Role: req.Role}
	if err := s.db.Create(&u).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	slog.Info("创建用户", "username", req.Username, "role", req.Role, "by", c.GetString("user"))
	core.OK(c, gin.H{"id": u.ID, "username": u.Username, "role": u.Role})
}

func (s *Service) updateUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var target core.User
	if err := s.db.First(&target, id).Error; err != nil {
		core.Err(c, 404, "用户不存在")
		return
	}
	var req userReq
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	selfID := c.GetUint("uid")

	// 角色调整校验
	roleChanged := req.Role != "" && req.Role != target.Role
	if roleChanged && !ValidRole(req.Role) {
		core.Err(c, 400, "角色非法（admin/operator/viewer）")
		return
	}
	// 自锁保护：不能改自己的角色、不能禁用自己
	if target.ID == selfID {
		if roleChanged {
			core.Err(c, 400, "不能修改自己的角色")
			return
		}
		if req.Disabled != nil && *req.Disabled {
			core.Err(c, 400, "不能禁用自己")
			return
		}
	}
	// 最后一个可用 admin 保护（降级或禁用 admin 时）
	if target.Role == string(RoleAdmin) && !target.Disabled &&
		((roleChanged && req.Role != string(RoleAdmin)) || (req.Disabled != nil && *req.Disabled)) {
		if s.enabledAdminCount(target.ID) == 0 {
			core.Err(c, 400, "至少保留一个可用的 admin 账号")
			return
		}
	}
	if req.Password != "" && len(req.Password) < 8 {
		core.Err(c, 400, "密码至少 8 位")
		return
	}

	changed := false
	if roleChanged {
		target.Role = req.Role
		changed = true
	}
	if req.Disabled != nil && *req.Disabled != target.Disabled {
		target.Disabled = *req.Disabled
		changed = true
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		target.Password = string(hash)
		changed = true
	}
	if !changed {
		core.OK(c, target)
		return
	}
	if err := s.db.Save(&target).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	// 角色/禁用/密码变更后注销该用户全部会话
	s.db.Where("user_id = ?", target.ID).Delete(&core.Session{})
	slog.Info("更新用户", "username", target.Username, "by", c.GetString("user"))
	core.OK(c, gin.H{"id": target.ID, "username": target.Username, "role": target.Role, "disabled": target.Disabled})
}

func (s *Service) removeUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var target core.User
	if err := s.db.First(&target, id).Error; err != nil {
		core.Err(c, 404, "用户不存在")
		return
	}
	if target.ID == c.GetUint("uid") {
		core.Err(c, 400, "不能删除自己")
		return
	}
	if target.Role == string(RoleAdmin) && !target.Disabled && s.enabledAdminCount(target.ID) == 0 {
		core.Err(c, 400, "至少保留一个可用的 admin 账号")
		return
	}
	s.db.Delete(&target)
	s.db.Where("user_id = ?", target.ID).Delete(&core.Session{})
	slog.Info("删除用户", "username", target.Username, "by", c.GetString("user"))
	core.OK(c, nil)
}
