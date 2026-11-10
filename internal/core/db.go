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
const Version = "0.35.7"

// Setting 表的保留键（各模块共享的 JSON 配置）。
const (
	KNotifyEmail    = "notify_email_json"
	KNotifyWebhook  = "notify_webhook_json"
	KMysqlCfg       = "dbmgr_mysql_json"
	KPgCfg          = "dbmgr_pg_json"
	KS3Cfg          = "objectstore_s3_json"
	KNodeToken      = "nodes_token"
	KAppSourceURL   = "appstore_source_url"
	KAppSourceCache = "appstore_source_cache"
	KUpgradeURL     = "upgrade_manifest_url" // 面板在线自升级清单地址（JSON）
)

// GetSetting 读取一个 KV 配置（不存在返回空串）。
func GetSetting(db *gorm.DB, key string) string {
	var s Setting
	if err := db.Where("`key` = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

// SetSetting 写入一个 KV 配置。
func SetSetting(db *gorm.DB, key, value string) error {
	return db.Save(&Setting{Key: key, Value: value}).Error
}

type User struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Username   string    `gorm:"uniqueIndex;size:64" json:"username"`
	Password   string    `json:"-"`                                 // bcrypt 哈希
	Role       string    `gorm:"size:16;default:admin" json:"role"` // admin | operator | viewer
	Disabled   bool      `json:"disabled"`
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
	Role      string    `gorm:"size:16" json:"role"`
	Action    string    `gorm:"size:128" json:"action"`
	Body      string    `gorm:"type:text" json:"body"`
	Method    string    `gorm:"size:8" json:"method"`
	Path      string    `gorm:"size:256" json:"path"`
	IP        string    `gorm:"size:64" json:"ip"`
	UA        string    `gorm:"size:256" json:"ua"`
	Status    int       `json:"status"`
	CostMS    int64     `json:"cost_ms"`
	CreatedAt time.Time `json:"created_at"`
}

// TerminalLog Web 终端命令行记录（按回车逐条落库）。
type TerminalLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:64;index" json:"username"`
	Line      string    `gorm:"size:512" json:"line"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
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
	if err := db.AutoMigrate(&User{}, &Session{}, &AuditLog{}, &Setting{}, &TerminalLog{}); err != nil {
		return nil, err
	}
	return db, nil
}
