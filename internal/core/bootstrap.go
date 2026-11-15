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

// randomToken 生成 n 字节的十六进制随机串。
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// BootstrapAdmin 首次启动时创建管理员账号：用户名与密码均随机生成
// （防爆破/防社工猜测），凭据打印到控制台并写入 data/initial_admin.txt。
func BootstrapAdmin(db *gorm.DB, dataDir string) error {
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	user := "user_" + randomToken(4) // 如 user_3fa2b1c8
	pass := randomToken(8)
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := db.Create(&User{Username: user, Password: string(hash), Role: "admin"}).Error; err != nil {
		return err
	}
	msg := fmt.Sprintf("初始管理员账号: %s  密码: %s\n（本提示仅输出这一次；请在面板「系统设置」中尽快修改用户名与密码，修改后本文件自动删除）\n", user, pass)
	slog.Warn("首次启动已创建初始管理员账号 " + user + "，凭据已写入 " + filepath.Join(dataDir, "initial_admin.txt") + "（仅首次；修改密码后自动删除）")
	_ = os.WriteFile(filepath.Join(dataDir, "initial_admin.txt"), []byte(msg), 0o600)
	fmt.Print(msg)
	return nil
}

// RemoveInitialAdminFile 用户修改初始密码后删除凭据文件（避免明文残留）。
func RemoveInitialAdminFile(dataDir string) {
	p := filepath.Join(dataDir, "initial_admin.txt")
	if err := os.Remove(p); err == nil {
		slog.Info("初始管理员凭据文件已删除（用户已修改密码）")
	}
}
