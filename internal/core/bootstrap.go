package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// BootstrapAdmin 首次启动时创建管理员账号；随机密码打印到控制台并写入 data/initial_admin.txt。
func BootstrapAdmin(db *gorm.DB, dataDir string) error {
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	pass := hex.EncodeToString(b)
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := db.Create(&User{Username: "admin", Password: string(hash), Role: "admin"}).Error; err != nil {
		return err
	}
	msg := fmt.Sprintf("初始管理员账号: admin  密码: %s\n", pass)
	slog.Warn("首次启动已创建初始管理员账号 admin，密码已写入 " + filepath.Join(dataDir, "initial_admin.txt"))
	_ = os.WriteFile(filepath.Join(dataDir, "initial_admin.txt"), []byte(msg), 0o600)
	fmt.Print(msg)
	return nil
}
