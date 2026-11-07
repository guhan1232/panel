// Package cert SSL 证书管理：手动上传、Let's Encrypt 自动签发（HTTP-01）、
// 到期监控与自动续期（ACME 签发的证书剩余 <30 天自动续）。
package cert

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/notify"
)

var domainRe = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

type certMeta struct {
	Domains  []string  `json:"domains"`
	Source   string    `json:"source"` // upload | acme
	Email    string    `json:"email,omitempty"`
	Staging  bool      `json:"staging,omitempty"`
	IssuedAt time.Time `json:"issued_at"`
}

// Info 对外展示的证书信息。
type Info struct {
	Domain   string    `json:"domain"`
	Domains  []string  `json:"domains"`
	NotAfter time.Time `json:"not_after"`
	DaysLeft int       `json:"days_left"`
	Source   string    `json:"source"`
}

type Module struct {
	db      *gorm.DB
	dataDir string
	alerted map[string]time.Time
}

func New(db *gorm.DB, dataDir string) *Module {
	return &Module{db: db, dataDir: dataDir, alerted: map[string]time.Time{}}
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/certs")
	g.GET("", m.list)
	g.POST("/upload", m.upload)
	g.DELETE("/:domain", m.remove)
	g.POST("/issue", m.issue)
}

func (m *Module) Start() {
	go func() {
		time.Sleep(time.Minute)
		for {
			m.checkExpiry()
			time.Sleep(6 * time.Hour)
		}
	}()
}

func (m *Module) certsDir() string { return filepath.Join(m.dataDir, "certs-managed") }

func (m *Module) acmeDir() string { return filepath.Join(m.dataDir, "acme") }

// ---- 证书库 ----

func (m *Module) list(c *gin.Context) {
	items, _ := m.scan()
	sort.Slice(items, func(i, j int) bool { return items[i].NotAfter.Before(items[j].NotAfter) })
	core.OK(c, items)
}

func (m *Module) scan() ([]Info, []certMeta) {
	var items []Info
	var metas []certMeta
	entries, err := os.ReadDir(m.certsDir())
	if err != nil {
		return items, metas
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		certPath := filepath.Join(m.certsDir(), e.Name(), "fullchain.pem")
		data, err := os.ReadFile(certPath)
		if err != nil {
			continue
		}
		block, _ := pem.Decode(data)
		if block == nil {
			continue
		}
		leaf, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		meta := certMeta{Source: "upload"}
		if raw, err := os.ReadFile(filepath.Join(m.certsDir(), e.Name(), "meta.json")); err == nil {
			_ = json.Unmarshal(raw, &meta)
		}
		metas = append(metas, meta)
		days := int(time.Until(leaf.NotAfter).Hours() / 24)
		items = append(items, Info{
			Domain: e.Name(), Domains: leaf.DNSNames,
			NotAfter: leaf.NotAfter, DaysLeft: days, Source: meta.Source,
		})
	}
	return items, metas
}

func (m *Module) saveCert(domain string, certPEM, keyPEM []byte, meta certMeta) (string, error) {
	dir := filepath.Join(m.certsDir(), domain)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	meta.IssuedAt = time.Now()
	if metaJSON, err := json.Marshal(meta); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "meta.json"), metaJSON, 0o600)
	}
	certPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "privkey.pem")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

func validDomains(domains []string) error {
	if len(domains) == 0 || len(domains) > 20 {
		return fmt.Errorf("域名数量 1-20 个")
	}
	for _, d := range domains {
		if !domainRe.MatchString(strings.ToLower(d)) {
			return fmt.Errorf("域名非法: %s", d)
		}
	}
	return nil
}

func (m *Module) upload(c *gin.Context) {
	var req struct {
		CertPEM string `binding:"required"`
		KeyPEM  string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	block, _ := pem.Decode([]byte(req.CertPEM))
	if block == nil {
		core.Err(c, 400, "证书 PEM 解析失败")
		return
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		core.Err(c, 400, "证书解析失败: "+err.Error())
		return
	}
	keyBlock, _ := pem.Decode([]byte(req.KeyPEM))
	if keyBlock == nil {
		core.Err(c, 400, "私钥 PEM 解析失败")
		return
	}
	var priv crypto.PrivateKey
	switch {
	case strings.Contains(keyBlock.Type, "EC PRIVATE KEY"):
		priv, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	case strings.Contains(keyBlock.Type, "PRIVATE KEY"):
		priv, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err == nil {
			priv, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		}
	default: // RSA PRIVATE KEY
		priv, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	}
	if err != nil {
		core.Err(c, 400, "私钥解析失败: "+err.Error())
		return
	}
	// 证书与私钥配对校验
	var privPub crypto.PublicKey
	switch k := priv.(type) {
	case *ecdsa.PrivateKey:
		privPub = k.Public()
	case *rsa.PrivateKey:
		privPub = k.Public()
	case ed25519.PrivateKey:
		privPub = k.Public()
	default:
		core.Err(c, 400, "私钥类型不支持")
		return
	}
	privDER, err := x509.MarshalPKIXPublicKey(privPub)
	if err != nil {
		core.Err(c, 400, "私钥序列化失败: "+err.Error())
		return
	}
	leafDER, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		core.Err(c, 400, "证书公钥序列化失败: "+err.Error())
		return
	}
	if !bytes.Equal(privDER, leafDER) {
		core.Err(c, 400, "证书与私钥不匹配")
		return
	}
	if len(leaf.DNSNames) == 0 {
		core.Err(c, 400, "证书不含 DNS 域名")
		return
	}
	domain := leaf.DNSNames[0]
	dir, err := m.saveCert(domain, []byte(req.CertPEM), []byte(req.KeyPEM), certMeta{Domains: leaf.DNSNames, Source: "upload"})
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"domain": domain, "dir": dir, "not_after": leaf.NotAfter})
}

func (m *Module) remove(c *gin.Context) {
	domain := c.Param("domain")
	dir := filepath.Join(m.certsDir(), filepath.Clean("/"+domain))
	if dir == m.certsDir() || !strings.HasPrefix(dir, m.certsDir()) {
		core.Err(c, 400, "非法证书名")
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

// ---- Let's Encrypt (lego, HTTP-01) ----

type acmeUser struct {
	email string
	reg   *registration.Resource
	key   crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.reg }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

func (m *Module) ensureAccount(email string) (*acmeUser, error) {
	if err := os.MkdirAll(m.acmeDir(), 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(m.acmeDir(), "account.key")
	var key *ecdsa.PrivateKey
	if data, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("ACME 账户密钥损坏")
		}
		key, err = x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		der, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return nil, err
		}
	}
	u := &acmeUser{email: email, key: key}
	if raw, err := os.ReadFile(filepath.Join(m.acmeDir(), "account.json")); err == nil {
		var reg registration.Resource
		if json.Unmarshal(raw, &reg) == nil {
			u.reg = &reg
		}
	}
	return u, nil
}

func (m *Module) saveAccount(u *acmeUser) {
	if raw, err := json.Marshal(u.reg); err == nil {
		_ = os.WriteFile(filepath.Join(m.acmeDir(), "account.json"), raw, 0o600)
	}
}

// issueDomains 申请证书（阻塞，HTTP-01 需 80 端口可达面板所在服务器）。
func (m *Module) issueDomains(domains []string, email string, staging bool) (string, error) {
	u, err := m.ensureAccount(email)
	if err != nil {
		return "", err
	}
	cfg := lego.NewConfig(u)
	if staging {
		cfg.CADirURL = lego.LEDirectoryStaging
	} else {
		cfg.CADirURL = lego.LEDirectoryProduction
	}
	cfg.Certificate.KeyType = certcrypto.EC256
	client, err := lego.NewClient(cfg)
	if err != nil {
		return "", err
	}
	// 面板以 root 运行，可绑定 80 端口完成 HTTP-01 校验
	if err := client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "80")); err != nil {
		return "", err
	}
	if u.reg == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			// 密钥已注册过账户时改用解析
			if strings.Contains(strings.ToLower(err.Error()), "already exist") {
				if reg2, err2 := client.Registration.ResolveAccountByKey(); err2 == nil {
					reg = reg2
					err = nil
				}
			}
			if err != nil {
				return "", fmt.Errorf("ACME 注册失败: %w", err)
			}
		}
		u.reg = reg
		m.saveAccount(u)
	}
	res, err := client.Certificate.Obtain(certificate.ObtainRequest{Domains: domains, Bundle: true})
	if err != nil {
		return "", fmt.Errorf("签发失败: %w", err)
	}
	primary := domains[0]
	dir, err := m.saveCert(primary, res.Certificate, res.PrivateKey, certMeta{
		Domains: domains, Source: "acme", Email: email, Staging: staging,
	})
	if err != nil {
		return "", err
	}
	slog.Info("证书签发成功", "domains", strings.Join(domains, ","), "staging", staging)
	return dir, nil
}

func (m *Module) issue(c *gin.Context) {
	var req struct {
		Domains []string `binding:"required"`
		Email   string   `binding:"required,email"`
		Staging bool     `json:"staging"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	for i := range req.Domains {
		req.Domains[i] = strings.ToLower(strings.TrimSpace(req.Domains[i]))
	}
	if err := validDomains(req.Domains); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	done := make(chan struct{})
	var dir string
	var issueErr error
	go func() {
		defer close(done)
		dir, issueErr = m.issueDomains(req.Domains, req.Email, req.Staging)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		core.Err(c, 500, "签发超时")
		return
	}
	if issueErr != nil {
		core.Err(c, 500, issueErr.Error())
		return
	}
	core.OK(c, gin.H{"domain": req.Domains[0], "dir": dir})
}

// ---- 到期监控与自动续期 ----

func (m *Module) checkExpiry() {
	infos, metas := m.scan()
	for i, info := range infos {
		meta := metas[i]
		switch {
		case meta.Source == "acme" && info.DaysLeft < 30 && info.DaysLeft > -30:
			slog.Info("证书即将到期，尝试自动续期", "domain", info.Domain, "days_left", info.DaysLeft)
			if _, err := m.issueDomains(meta.Domains, meta.Email, meta.Staging); err != nil {
				slog.Warn("证书自动续期失败", "domain", info.Domain, "err", err)
				notify.Send(m.db, "证书续期失败: "+info.Domain,
					fmt.Sprintf("域名 %s 证书剩余 %d 天，自动续期失败: %v", info.Domain, info.DaysLeft, err))
			}
		case info.DaysLeft < 15 && info.DaysLeft > -15:
			if last, ok := m.alerted[info.Domain]; ok && time.Since(last) < 7*24*time.Hour {
				continue
			}
			m.alerted[info.Domain] = time.Now()
			notify.Send(m.db, "证书即将到期: "+info.Domain,
				fmt.Sprintf("域名 %s 证书将于 %s 到期（剩余 %d 天），请及时更新", info.Domain,
					info.NotAfter.Format("2006-01-02"), info.DaysLeft))
		}
	}
}
