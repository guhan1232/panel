// Package setting 系统设置：面板信息、修改管理员密码。
package setting

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/core"
)

type Module struct {
	db  *gorm.DB
	cfg *config.Config
}

func New(db *gorm.DB, cfg *config.Config) *Module {
	return &Module{db: db, cfg: cfg}
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/setting")
	g.GET("/info", m.info)
	g.POST("/password", m.changePassword)
}

func (m *Module) info(c *gin.Context) {
	core.OK(c, gin.H{
		"version":     core.Version,
		"listen":      m.cfg.Server.Listen,
		"entrance_on": m.cfg.Server.Entrance != "",
		"tls":         m.cfg.Server.TLS.Enabled,
		"data_dir":    m.cfg.DataDir,
		"go_version":  runtime.Version(),
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	})
}

func (m *Module) changePassword(c *gin.Context) {
	var req struct {
		Old string `binding:"required"`
		New string `binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	uid := c.GetUint("uid")
	var u core.User
	if err := m.db.First(&u, uid).Error; err != nil {
		core.Err(c, 404, "用户不存在")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Old)) != nil {
		core.Err(c, 400, "原密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := m.db.Model(&u).Update("password", string(hash)).Error; err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	// 注销除当前会话外的所有会话
	cur := c.GetString("session")
	m.db.Where("user_id = ? AND token <> ?", uid, cur).Delete(&core.Session{})
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "密码已修改，其它会话已注销"})
}
