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
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/acmedns"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/azuredns"
	"github.com/go-acme/lego/v4/providers/dns/baiducloud"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/desec"
	"github.com/go-acme/lego/v4/providers/dns/duckdns"
	"github.com/go-acme/lego/v4/providers/dns/dnspod"
	"github.com/go-acme/lego/v4/providers/dns/gandiv5"
	"github.com/go-acme/lego/v4/providers/dns/gcloud"
	"github.com/go-acme/lego/v4/providers/dns/godaddy"
	"github.com/go-acme/lego/v4/providers/dns/hetzner"
	"github.com/go-acme/lego/v4/providers/dns/huaweicloud"
	"github.com/go-acme/lego/v4/providers/dns/namecheap"
	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/go-acme/lego/v4/providers/dns/porkbun"
	"github.com/go-acme/lego/v4/providers/dns/route53"
	"github.com/go-acme/lego/v4/providers/dns/tencentcloud"
	"github.com/go-acme/lego/v4/providers/dns/westcn"
	"github.com/go-acme/lego/v4/registration"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/notify"
)

var domainRe = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

type certMeta struct {
	Domains      []string  `json:"domains"`
	Source       string    `json:"source"` // upload | acme
	Email        string    `json:"email,omitempty"`
	Staging      bool      `json:"staging,omitempty"`
	DNSProvider  string             `json:"dns_provider,omitempty"`
	DNSAPIKey    string             `json:"dns_api_key,omitempty"`
	DNSAPISecret string             `json:"dns_api_secret,omitempty"`
	DNSFields    map[string]string  `json:"dns_fields,omitempty"`
	IssuedAt     time.Time          `json:"issued_at"`
}

// Info 对外展示的证书信息。
type Info struct {
	Domain   string    `json:"domain"`
	Domains  []string  `json:"domains"`
	NotAfter time.Time `json:"not_after"`
	DaysLeft int       `json:"days_left"`
	Source   string    `json:"source"`
	CertPath string    `json:"cert_path"` // 绝对路径（nginx 反代引用）
	KeyPath  string    `json:"key_path"`
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
	g.GET("/dns-providers", dnsProvidersMeta)
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
			CertPath: absPath(filepath.Join(m.certsDir(), e.Name(), "fullchain.pem")),
			KeyPath:  absPath(filepath.Join(m.certsDir(), e.Name(), "privkey.pem")),
		})
	}
	return items, metas
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
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

// dnsCfg DNS-01 挑战的 DNS 服务商凭据。
type dnsCfg struct {
	Provider  string            `json:"provider"` // alidns | dnspod | cloudflare | tencentcloud | ... 见 DNS_PROVIDERS
	APIKey    string            `json:"api_key"`  // 旧字段（兼容），并入 Fields
	APISecret string            `json:"api_secret"`
	Fields    map[string]string `json:"fields"`   // 各服务商凭据（key 见 DNS_PROVIDERS 元数据）
}

// fieldsMap 合并旧字段与新 Fields（旧字段优先级低）。
func (c *dnsCfg) fieldsMap() map[string]string {
	m := map[string]string{}
	for k, v := range c.Fields {
		m[k] = v
	}
	if c.APIKey != "" {
		m["api_key"] = c.APIKey
	}
	if c.APISecret != "" {
		m["api_secret"] = c.APISecret
	}
	return m
}

// ---- DNS 服务商元数据（spec 驱动：前端动态表单 + 后端 env 注入共用一份定义） ----

// dnsFieldSpec 一个凭据字段。
type dnsFieldSpec struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Env         string `json:"-"`
	Secret      bool   `json:"secret,omitempty"`
	Textarea    bool   `json:"textarea,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// dnsSpec 一个服务商：id 与 lego 包名一致（持久化记录沿用）。
type dnsSpec struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Fields []dnsFieldSpec `json:"fields"`
}

// DNS_PROVIDERS 主流 DNS 服务商（国内 + 国际）。
var DNS_PROVIDERS = []dnsSpec{
	{ID: "alidns", Name: "阿里云 DNS", Fields: []dnsFieldSpec{
		{Key: "api_key", Label: "AccessKey ID", Env: "ALIYUN_ACCESS_KEY_ID", Required: true},
		{Key: "api_secret", Label: "AccessKey Secret", Env: "ALIYUN_ACCESS_KEY_SECRET", Secret: true, Required: true},
	}},
	{ID: "dnspod", Name: "腾讯云 DNSPod", Fields: []dnsFieldSpec{
		{Key: "api_key", Label: "API Token ID", Env: "DNSPOD_ID", Required: true, Placeholder: "ID,Token 的前半段"},
		{Key: "api_secret", Label: "API Token", Env: "DNSPOD_TOKEN", Secret: true, Required: true},
	}},
	{ID: "tencentcloud", Name: "腾讯云（API 密钥）", Fields: []dnsFieldSpec{
		{Key: "secret_id", Label: "SecretId", Env: "TENCENTCLOUD_SECRET_ID", Required: true},
		{Key: "secret_key", Label: "SecretKey", Env: "TENCENTCLOUD_SECRET_KEY", Secret: true, Required: true},
	}},
	{ID: "huaweicloud", Name: "华为云 DNS", Fields: []dnsFieldSpec{
		{Key: "access_key", Label: "Access Key Id", Env: "HUAWEICLOUD_ACCESS_KEY_ID", Required: true},
		{Key: "secret_key", Label: "Secret Access Key", Env: "HUAWEICLOUD_SECRET_ACCESS_KEY", Secret: true, Required: true},
		{Key: "region", Label: "Region（可选，如 cn-north-4）", Env: "HUAWEICLOUD_REGION"},
	}},
	{ID: "baiducloud", Name: "百度智能云 DNS", Fields: []dnsFieldSpec{
		{Key: "access_key", Label: "Access Key ID", Env: "BAIDUCLOUD_ACCESS_KEY_ID", Required: true},
		{Key: "secret_key", Label: "Secret Access Key", Env: "BAIDUCLOUD_SECRET_ACCESS_KEY", Secret: true, Required: true},
	}},
	{ID: "cloudflare", Name: "Cloudflare", Fields: []dnsFieldSpec{
		{Key: "api_token", Label: "API Token", Env: "CF_DNS_API_TOKEN", Secret: true, Required: true},
	}},
	{ID: "westcn", Name: "西部数码", Fields: []dnsFieldSpec{
		{Key: "username", Label: "用户名", Env: "WESTCN_USERNAME", Required: true},
		{Key: "password", Label: "密码", Env: "WESTCN_PASSWORD", Secret: true, Required: true},
	}},
	{ID: "route53", Name: "AWS Route 53", Fields: []dnsFieldSpec{
		{Key: "access_key", Label: "AWS Access Key ID", Env: "AWS_ACCESS_KEY_ID", Required: true},
		{Key: "secret_key", Label: "AWS Secret Access Key", Env: "AWS_SECRET_ACCESS_KEY", Secret: true, Required: true},
		{Key: "region", Label: "Region（可选，如 us-east-1）", Env: "AWS_REGION"},
	}},
	{ID: "gcloud", Name: "Google Cloud DNS", Fields: []dnsFieldSpec{
		{Key: "project", Label: "项目 ID（可选，SA 内含时可留空）", Env: "GCE_PROJECT"},
		{Key: "sa_json", Label: "服务账号 JSON", Env: "", Textarea: true, Required: true, Placeholder: `{"type": "service_account", ...}`},
	}},
	{ID: "azuredns", Name: "Azure DNS", Fields: []dnsFieldSpec{
		{Key: "client_id", Label: "Client ID", Env: "AZURE_CLIENT_ID", Required: true},
		{Key: "client_secret", Label: "Client Secret", Env: "AZURE_CLIENT_SECRET", Secret: true, Required: true},
		{Key: "tenant_id", Label: "Tenant ID", Env: "AZURE_TENANT_ID", Required: true},
		{Key: "subscription_id", Label: "Subscription ID", Env: "AZURE_SUBSCRIPTION_ID", Required: true},
		{Key: "resource_group", Label: "Resource Group（可选）", Env: "AZURE_RESOURCE_GROUP"},
	}},
	{ID: "godaddy", Name: "GoDaddy", Fields: []dnsFieldSpec{
		{Key: "api_key", Label: "API Key", Env: "GODADDY_API_KEY", Required: true},
		{Key: "api_secret", Label: "API Secret", Env: "GODADDY_API_SECRET", Secret: true, Required: true},
	}},
	{ID: "namecheap", Name: "Namecheap", Fields: []dnsFieldSpec{
		{Key: "api_user", Label: "API User", Env: "NAMECHEAP_API_USER", Required: true},
		{Key: "api_key", Label: "API Key", Env: "NAMECHEAP_API_KEY", Secret: true, Required: true},
	}},
	{ID: "ovh", Name: "OVH", Fields: []dnsFieldSpec{
		{Key: "endpoint", Label: "Endpoint（可选，默认 ovh-eu）", Env: "OVH_ENDPOINT", Placeholder: "ovh-eu / ovh-ca / kimsufi-eu"},
		{Key: "app_key", Label: "Application Key", Env: "OVH_APPLICATION_KEY", Required: true},
		{Key: "app_secret", Label: "Application Secret", Env: "OVH_APPLICATION_SECRET", Secret: true, Required: true},
		{Key: "consumer_key", Label: "Consumer Key", Env: "OVH_CONSUMER_KEY", Secret: true, Required: true},
	}},
	{ID: "gandiv5", Name: "Gandi", Fields: []dnsFieldSpec{
		{Key: "api_token", Label: "Personal Access Token", Env: "GANDIV5_PERSONAL_ACCESS_TOKEN", Secret: true, Required: true},
	}},
	{ID: "hetzner", Name: "Hetzner", Fields: []dnsFieldSpec{
		{Key: "api_token", Label: "API Token", Env: "HETZNER_API_TOKEN", Secret: true, Required: true},
	}},
	{ID: "porkbun", Name: "Porkbun", Fields: []dnsFieldSpec{
		{Key: "api_key", Label: "API Key", Env: "PORKBUN_API_KEY", Required: true},
		{Key: "secret_key", Label: "Secret API Key", Env: "PORKBUN_SECRET_API_KEY", Secret: true, Required: true},
	}},
	{ID: "duckdns", Name: "DuckDNS（免费动态域名）", Fields: []dnsFieldSpec{
		{Key: "token", Label: "Token", Env: "DUCKDNS_TOKEN", Secret: true, Required: true},
	}},
	{ID: "desec", Name: "deSEC（免费安全 DNS）", Fields: []dnsFieldSpec{
		{Key: "token", Label: "Token", Env: "DESEC_TOKEN", Secret: true, Required: true},
	}},
	{ID: "acmedns", Name: "ACME-DNS（CNAME 委托中转）", Fields: []dnsFieldSpec{
		{Key: "api_base", Label: "API 地址", Env: "ACME_DNS_API_BASE", Required: true, Placeholder: "https://auth.example.acme-dns.io"},
		{Key: "storage_path", Label: "凭据存储文件路径", Env: "ACME_DNS_STORAGE_PATH", Required: true, Placeholder: "/opt/hanyuxin/data/acme-dns.json"},
	}},
}

// dnsEnvMu 串行化 env 注入式签发（os.Setenv 是进程级全局，并发会串凭据）。
var dnsEnvMu sync.Mutex

func dnsProvider(cfg *dnsCfg) (challenge.Provider, func(), error) {
	id := cfg.Provider
	if id == "aliyun" { // 旧记录兼容
		id = "alidns"
	}
	var spec *dnsSpec
	for i := range DNS_PROVIDERS {
		if DNS_PROVIDERS[i].ID == id {
			spec = &DNS_PROVIDERS[i]
			break
		}
	}
	if spec == nil {
		return nil, nil, fmt.Errorf("不支持的 DNS 服务商: %s", cfg.Provider)
	}
	vals := cfg.fieldsMap()

	dnsEnvMu.Lock()
	sets := map[string]string{}
	var missing []string
	for _, f := range spec.Fields {
		v := strings.TrimSpace(vals[f.Key])
		if f.Required && v == "" {
			missing = append(missing, f.Label)
			continue
		}
		if v != "" && f.Env != "" {
			sets[f.Env] = v
		}
	}
	if len(missing) > 0 {
		dnsEnvMu.Unlock()
		return nil, nil, fmt.Errorf("%s 需填写: %s", spec.Name, strings.Join(missing, "、"))
	}
	for k, v := range sets {
		os.Setenv(k, v)
	}
	cleanup := func() {
		for k := range sets {
			os.Unsetenv(k)
		}
		dnsEnvMu.Unlock()
	}
	p, err := buildDNSProvider(id, cfg, vals)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return p, cleanup, nil
}

// buildDNSProvider 各服务商构造（默认读 env；特例走显式构造）。
func buildDNSProvider(id string, cfg *dnsCfg, vals map[string]string) (challenge.Provider, error) {
	switch id {
	case "gcloud":
		if sa := strings.TrimSpace(vals["sa_json"]); sa != "" {
			return gcloud.NewDNSProviderServiceAccountKey([]byte(sa))
		}
		return gcloud.NewDNSProvider()
	case "dnspod":
		// lego dnspod env 为完整 token："id,token"
		if idTok := vals["api_key"] + "," + vals["api_secret"]; vals["api_key"] != "" && vals["api_secret"] != "" {
			os.Setenv("DNSPOD_API_TOKEN", idTok)
			os.Unsetenv("DNSPOD_ID")
			os.Unsetenv("DNSPOD_TOKEN")
		}
		return dnspod.NewDNSProvider()
	case "route53":
		return route53.NewDNSProvider()
	case "gandiv5":
		// 新版 lego 用 PAT env；同时兼容旧 API_KEY
		if tok := vals["api_token"]; tok != "" {
			os.Setenv("GANDIV5_PERSONAL_ACCESS_TOKEN", tok)
		}
		return gandiv5.NewDNSProvider()
	default:
		switch id {
		case "alidns":
			return alidns.NewDNSProvider()
		case "cloudflare":
			return cloudflare.NewDNSProvider()
		case "tencentcloud":
			return tencentcloud.NewDNSProvider()
		case "huaweicloud":
			return huaweicloud.NewDNSProvider()
		case "baiducloud":
			return baiducloud.NewDNSProvider()
		case "westcn":
			return westcn.NewDNSProvider()
		case "azuredns":
			return azuredns.NewDNSProvider()
		case "godaddy":
			return godaddy.NewDNSProvider()
		case "namecheap":
			return namecheap.NewDNSProvider()
		case "ovh":
			return ovh.NewDNSProvider()
		case "hetzner":
			return hetzner.NewDNSProvider()
		case "porkbun":
			return porkbun.NewDNSProvider()
		case "duckdns":
			return duckdns.NewDNSProvider()
		case "desec":
			return desec.NewDNSProvider()
		case "acmedns":
			return acmedns.NewDNSProvider()
		}
		return nil, fmt.Errorf("不支持的 DNS 服务商: %s", id)
	}
}

// dnsProvidersMeta 前端动态表单数据。
func dnsProvidersMeta(c *gin.Context) {
	core.OK(c, DNS_PROVIDERS)
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
// IssuePublicCert 面板 HTTPS 用：HTTP-01 给域名签发公共免费 CA 证书（Let's Encrypt），返回证书/私钥目录。
// 独立于「SSL 证书」页的常规签发（那边支持 DNS-01/泛域名/测试环境）。
func (m *Module) IssuePublicCert(domain, email string) (string, error) {
	return m.issueDomains([]string{domain}, email, false, nil)
}

func (m *Module) issueDomains(domains []string, email string, staging bool, dns *dnsCfg) (string, error) {
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
	// 挑战类型：dns01（支持泛域名，经 DNS 服务商 API）或 http01（面板 root 绑定 80）
	if dns != nil && dns.Provider != "" {
		p, cleanup, err := dnsProvider(dns)
		if err != nil {
			return "", err
		}
		if err := client.Challenge.SetDNS01Provider(p, dns01.AddRecursiveNameservers([]string{"119.29.29.29:53", "223.5.5.5:53"})); err != nil {
			cleanup()
			return "", err
		}
		defer cleanup()
	} else {
		if err := client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "80")); err != nil {
			return "", err
		}
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
	meta := certMeta{Domains: domains, Source: "acme", Email: email, Staging: staging}
	if dns != nil {
		meta.DNSProvider = dns.Provider
		meta.DNSAPIKey = dns.APIKey
		meta.DNSAPISecret = dns.APISecret
		meta.DNSFields = dns.Fields
	}
	dir, err := m.saveCert(primary, res.Certificate, res.PrivateKey, meta)
	if err != nil {
		return "", err
	}
	slog.Info("证书签发成功", "domains", strings.Join(domains, ","), "staging", staging)
	return dir, nil
}

func (m *Module) issue(c *gin.Context) {
	var req struct {
		Domains      []string `binding:"required"`
		Email        string   `binding:"required,email"`
		Staging      bool     `json:"staging"`
		Challenge    string   `json:"challenge"` // http01（默认）| dns01
		DNSProvider  string            `json:"dns_provider"`
		DNSAPIKey    string            `json:"dns_api_key"`
		DNSAPISecret string            `json:"dns_api_secret"`
		DNSFields    map[string]string `json:"dns_fields"`
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
		var dns *dnsCfg
		if req.Challenge == `dns01` {
			dns = &dnsCfg{Provider: req.DNSProvider, APIKey: req.DNSAPIKey, APISecret: req.DNSAPISecret, Fields: req.DNSFields}
		}
		dir, issueErr = m.issueDomains(req.Domains, req.Email, req.Staging, dns)
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
			// 续期沿用签发时的 DNS 凭据（若签发时保存于 meta）
			var dns *dnsCfg
			if meta.DNSProvider != "" {
				dns = &dnsCfg{Provider: meta.DNSProvider, APIKey: meta.DNSAPIKey, APISecret: meta.DNSAPISecret, Fields: meta.DNSFields}
			}
			if _, err := m.issueDomains(meta.Domains, meta.Email, meta.Staging, dns); err != nil {
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
