// Package audit 操作审计中间件：记录所有非 GET 的 API 调用。
package audit

import (
	"net/http"
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
