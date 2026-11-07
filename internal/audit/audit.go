// Package audit 操作审计中间件：记录所有非 GET 的 API 调用。
package audit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
)

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Middleware 写操作审计中间件（须放在鉴权之后以获取操作人）。
func Middleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()

		log := core.AuditLog{
			Username: c.GetString("user"),
			Method:   c.Request.Method,
			Path:     truncate(c.Request.URL.Path, 256),
			IP:       c.ClientIP(),
			UA:       truncate(c.Request.UserAgent(), 256),
			Status:   c.Writer.Status(),
			CostMS:   time.Since(start).Milliseconds(),
		}
		if err := db.Create(&log).Error; err != nil {
			c.Error(err)
		}
	}
}

// Module 审计查询 API。
type Module struct{ db *gorm.DB }

func New(db *gorm.DB) *Module { return &Module{db: db} }

func (m *Module) Register(api gin.IRouter) {
	api.GET("/audit/list", m.list)
}

func (m *Module) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	qry := m.db.Model(&core.AuditLog{})
	if method := c.Query("method"); method != "" {
		qry = qry.Where("method = ?", method)
	}
	if q := c.Query("q"); q != "" {
		qry = qry.Where("path LIKE ?", "%"+q+"%")
	}
	if u := c.Query("username"); u != "" {
		qry = qry.Where("username = ?", u)
	}
	var total int64
	qry.Count(&total)
	var items []core.AuditLog
	qry.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items)
	core.OK(c, gin.H{"total": total, "items": items})
}
