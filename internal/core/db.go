package core

import (
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Version 面板版本号。
const Version = "0.1.0"

type User struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Username   string    `gorm:"uniqueIndex;size:64" json:"username"`
	Password   string    `json:"-"` // bcrypt 哈希
	TOTPSecret string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Session struct {
	Token     string    `gorm:"primaryKey;size:64" json:"token"`
	UserID    uint      `json:"user_id"`
	IP        string    `gorm:"size:64" json:"ip"`
	UA        string    `gorm:"size:512" json:"ua"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `json:"username"`
	Method    string    `gorm:"size:8" json:"method"`
	Path      string    `gorm:"size:256" json:"path"`
	IP        string    `gorm:"size:64" json:"ip"`
	UA        string    `gorm:"size:256" json:"ua"`
	Status    int       `json:"status"`
	CostMS    int64     `json:"cost_ms"`
	CreatedAt time.Time `json:"created_at"`
}

type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `json:"value"`
}

// OpenDB 打开面板 SQLite 数据库（纯 Go 驱动，CGO_ENABLED=0 可编译），并自动迁移核心表。
func OpenDB(dataDir string) (*gorm.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dataDir, "panel.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&User{}, &Session{}, &AuditLog{}, &Setting{}); err != nil {
		return nil, err
	}
	return db, nil
}
