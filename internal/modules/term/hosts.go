package term

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/core"
)

// SSHHost SSH 主机收藏（云端记忆）。凭据 AES-GCM 加密落库。
type SSHHost struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128" json:"name"`             // 备注名
	Host      string    `gorm:"size:256" json:"host"`             // IP 或域名
	Port      int       `gorm:"default:22" json:"port"`
	Username  string    `gorm:"size:128" json:"username"`
	AuthType  string    `gorm:"size:16" json:"auth_type"` // password | key
	Password  string    `gorm:"size:512" json:"password"` // 密文
	PrivateKey string   `gorm:"size:8192" json:"private_key"` // 密文（PEM）
	Passphrase string   `gorm:"size:512" json:"passphrase"`   // 私钥口令密文
	Sort      int       `gorm:"default:0" json:"sort"`       // 手动排序权重（小在前）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SSHHost) TableName() string { return "term_hosts" }

// HostView 列表返回视图：凭据一律不回传，只给 has_password/has_key 标记。
type HostView struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	AuthType  string    `json:"auth_type"`
	HasPassword bool    `json:"has_password"`
	HasKey      bool    `json:"has_key"`
	Sort      int       `json:"sort"`
	CreatedAt time.Time `json:"created_at"`
}

// ---- AES-GCM 加密（密钥来自 config.yaml server.secret，缺省自动生成持久化） ----

var cryptKey []byte

// initCrypt 初始化 32 字节 AES 密钥：优先配置 server.secret（hex），否则读/写 dataDir/.term.key。
func initCrypt(cfg *config.Config) {
	if cfg.Server.Secret != "" {
		if k, err := hex.DecodeString(cfg.Server.Secret); err == nil && len(k) >= 32 {
			cryptKey = k[:32]
			return
		}
	}
	keyPath := cfg.DataDir + "/.term.key"
	if b, err := os.ReadFile(keyPath); err == nil && len(b) >= 32 {
		cryptKey = b[:32]
		return
	}
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	_ = os.WriteFile(keyPath, k, 0o600)
	cryptKey = k
	slog.Info("已生成终端凭据加密密钥", "path", keyPath)
}

func encrypt(plain string) string {
	if plain == "" || len(cryptKey) == 0 {
		return ""
	}
	block, _ := aes.NewCipher(cryptKey)
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	nonce := make([]byte, gcm.NonceSize())
	_, _ = io.ReadFull(rand.Reader, nonce)
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return hex.EncodeToString(out)
}

func decrypt(cipherHex string) string {
	if cipherHex == "" || len(cryptKey) == 0 {
		return ""
	}
	b, err := hex.DecodeString(cipherHex)
	if err != nil || len(b) < 12 {
		return ""
	}
	block, _ := aes.NewCipher(cryptKey)
	gcm, _ := cipher.NewGCM(block)
	ns := gcm.NonceSize()
	plain, err := gcm.Open(nil, b[:ns], b[ns:], nil)
	if err != nil {
		return ""
	}
	return string(plain)
}

func toView(h *SSHHost) HostView {
	return HostView{
		ID: h.ID, Name: h.Name, Host: h.Host, Port: h.Port,
		Username: h.Username, AuthType: h.AuthType,
		HasPassword: h.Password != "", HasKey: h.PrivateKey != "",
		Sort: h.Sort, CreatedAt: h.CreatedAt,
	}
}

// RegisterHosts 主机收藏 CRUD（挂在鉴权组上）。
func RegisterHosts(api gin.IRouter, db *gorm.DB, cfg *config.Config) {
	initCrypt(cfg)
	if err := db.AutoMigrate(&SSHHost{}); err != nil {
		slog.Warn("SSH 主机表迁移失败", "err", err)
	}
	g := api.Group("/term/hosts")
	g.GET("", func(c *gin.Context) {
		var list []SSHHost
		db.Order("sort ASC, id ASC").Find(&list)
		views := make([]HostView, 0, len(list))
		for i := range list {
			views = append(views, toView(&list[i]))
		}
		core.OK(c, views)
	})
	g.POST("", func(c *gin.Context) {
		var req struct {
			Name       string `json:"name"`
			Host       string `json:"host" binding:"required"`
			Port       int    `json:"port"`
			Username   string `json:"username" binding:"required"`
			AuthType   string `json:"auth_type" binding:"required,oneof=password key"`
			Password   string `json:"password"`
			PrivateKey string `json:"private_key"`
			Passphrase string `json:"passphrase"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			core.BindErr(c, err)
			return
		}
		if req.Port <= 0 || req.Port > 65535 {
			req.Port = 22
		}
		h := SSHHost{
			Name: req.Name, Host: strings.TrimSpace(req.Host), Port: req.Port,
			Username: req.Username, AuthType: req.AuthType,
			Password: encrypt(req.Password), PrivateKey: encrypt(req.PrivateKey), Passphrase: encrypt(req.Passphrase),
			Sort: 9999,
		}
		if err := db.Create(&h).Error; err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		core.OK(c, toView(&h))
	})
	g.PUT("/:id", func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var h SSHHost
		if err := db.First(&h, id).Error; err != nil {
			core.Err(c, 404, "主机不存在")
			return
		}
		var req struct {
			Name       *string `json:"name"`
			Host       *string `json:"host"`
			Port       *int    `json:"port"`
			Username   *string `json:"username"`
			AuthType   *string `json:"auth_type"`
			Password   *string `json:"password"`   // 空=不改
			PrivateKey *string `json:"private_key"`
			Passphrase *string `json:"passphrase"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			core.BindErr(c, err)
			return
		}
		if req.Name != nil {
			h.Name = *req.Name
		}
		if req.Host != nil {
			h.Host = strings.TrimSpace(*req.Host)
		}
		if req.Port != nil && *req.Port > 0 && *req.Port <= 65535 {
			h.Port = *req.Port
		}
		if req.Username != nil {
			h.Username = *req.Username
		}
		if req.AuthType != nil {
			h.AuthType = *req.AuthType
		}
		if req.Password != nil && *req.Password != "" {
			h.Password = encrypt(*req.Password)
		}
		if req.PrivateKey != nil && *req.PrivateKey != "" {
			h.PrivateKey = encrypt(*req.PrivateKey)
		}
		if req.Passphrase != nil && *req.Passphrase != "" {
			h.Passphrase = encrypt(*req.Passphrase)
		}
		db.Save(&h)
		core.OK(c, toView(&h))
	})
	g.DELETE("/:id", func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		db.Delete(&SSHHost{}, id)
		core.OK(c, gin.H{"ok": true})
	})
	// 手动排序：传全量 id 顺序
	g.POST("/reorder", func(c *gin.Context) {
		var req struct {
			IDs []uint `binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			core.BindErr(c, err)
			return
		}
		for i, id := range req.IDs {
			db.Model(&SSHHost{}).Where("id = ?", id).Update("sort", i)
		}
		core.OK(c, gin.H{"ok": true})
	})
}

// SSHSession 一个 SSH PTY 会话：Read/Write/Resize/Close。
type SSHSession struct {
	cli  *ssh.Client
	sess *ssh.Session
	in   io.WriteCloser
	out  io.Reader
}

func (s *SSHSession) Read(p []byte) (int, error)  { return s.out.Read(p) }
func (s *SSHSession) Write(b []byte) (int, error) { return s.in.Write(b) }
func (s *SSHSession) Close() {
	_ = s.sess.Close()
	_ = s.cli.Close()
}

// Resize 窗口尺寸变化。
func (s *SSHSession) Resize(cols, rows uint16) error {
	return s.sess.WindowChange(int(rows), int(cols))
}

// sshDial 建立 SSH 连接并申请 PTY，管道接好输出/输入。
func sshDial(h *SSHHost, cols, rows uint16) (*SSHSession, error) {
	var auth ssh.AuthMethod
	switch h.AuthType {
	case "key":
		pem := decrypt(h.PrivateKey)
		signer, err := makeSigner([]byte(pem), decrypt(h.Passphrase))
		if err != nil {
			return nil, fmt.Errorf("私钥解析失败: %w", err)
		}
		auth = ssh.PublicKeys(signer)
	default:
		p := decrypt(h.Password)
		if p == "" {
			return nil, errors.New("密码未配置")
		}
		auth = ssh.Password(p)
	}
	cfg := &ssh.ClientConfig{
		User:            h.Username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: known_hosts 校验
		Timeout:         12 * time.Second,
	}
	cli, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", h.Host, h.Port), cfg)
	if err != nil {
		return nil, err
	}
	sess, err := cli.NewSession()
	if err != nil {
		cli.Close()
		return nil, err
	}
	if err := sess.RequestPty("xterm-256color", int(rows), int(cols), ssh.TerminalModes{}); err != nil {
		sess.Close()
		cli.Close()
		return nil, err
	}
	out, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		cli.Close()
		return nil, err
	}
	in, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		cli.Close()
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		sess.Close()
		cli.Close()
		return nil, err
	}
	return &SSHSession{cli: cli, sess: sess, in: in, out: out}, nil
}

func makeSigner(pem []byte, pass string) (ssh.Signer, error) {
	if pass == "" {
		return ssh.ParsePrivateKey(pem)
	}
	return ssh.ParsePrivateKeyWithPassphrase(pem, []byte(pass))
}
