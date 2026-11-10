// Package selfcert 面板 HTTPS 证书：自签生成（含公网 IP SAN，10 年）与中间 CA 交叉签发。
package selfcert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultYears 自签兜底证书的固定有效期（年）。正常路径用 CA 签发（有效期跟随 CA），仅 CA 不可用时才自签。
const DefaultYears = 10

// outboundIP 探测默认路由的出口 IP（UDP 连接不实际发包）。云服务器 NAT 环境下是内网 IP。
func outboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP
}

// publicIPv4 经外部服务获取公网出口 IP（NAT/云环境网卡上没有公网地址时补 SAN 用）。
func publicIPv4() net.IP {
	for _, u := range []string{"https://api.ipify.org", "https://ifconfig.me/ip"} {
		client := &http.Client{Timeout: 4 * time.Second}
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(string(b)))
		if ip != nil && ip.To4() != nil {
			return ip
		}
	}
	return nil
}

// PublicIPs 汇总服务器 IP：出口 IP + 全部网卡地址 + 127.0.0.1/::1。
func PublicIPs() []net.IP {
	var out []net.IP
	seen := map[string]bool{}
	add := func(ip net.IP) {
		if ip == nil || seen[ip.String()] {
			return
		}
		seen[ip.String()] = true
		out = append(out, ip)
	}
	add(outboundIP())
	add(publicIPv4())
	if ifaces, err := net.Interfaces(); err == nil {
		for _, f := range ifaces {
			addrs, err := f.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if ipn, ok := a.(*net.IPNet); ok {
					add(ipn.IP)
				} else {
					if ip, _, err := net.ParseCIDR(a.String()); err == nil {
						add(ip)
					}
				}
			}
		}
	}
	add(net.ParseIP("127.0.0.1"))
	add(net.ParseIP("::1"))
	return out
}

func selfTmpl(dns []string, ips []net.IP, years int) x509.Certificate {
	return x509.Certificate{
		SerialNumber:          bigSerial(),
		Subject:               pkix.Name{CommonName: "hanyuxin"},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().AddDate(years, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           ips,
		DNSNames:              dns,
	}
}

func bigSerial() *big.Int {
	s, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return s
}

// Ensure 确保目录内存在自签证书（10 年，SAN 含全部本机 IP）；force=true 时重签。
func Ensure(dir string) (string, string, error) { return ensure(dir, false, nil, nil) }

// EnsureForce 强制重签自签证书（比如旧证书只有 localhost SAN）。
func EnsureForce(dir string) (string, string, error) { return ensure(dir, true, nil, nil) }

// IssueSelfSigned 生成自签证书 PEM（10 年，SAN 由调用方给定，落盘由调用方负责）。
func IssueSelfSigned(dns []string, ips []net.IP) (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := selfTmpl(dns, ips, DefaultYears)
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	key := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return cert, key, nil
}

func ensure(dir string, force bool, extraDNS []string, extraIPs []net.IP) (string, string, error) {
	certPath := filepath.Join(dir, "panel.crt")
	keyPath := filepath.Join(dir, "panel.key")
	if !force {
		if _, err := os.Stat(certPath); err == nil {
			if _, err2 := os.Stat(keyPath); err2 == nil {
				return certPath, keyPath, nil
			}
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	dns := append([]string{"localhost"}, extraDNS...)
	ips := append(PublicIPs(), extraIPs...)
	certPEM, keyPEM, err := IssueSelfSigned(dns, ips)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

// parseKeyPair 解析 CA 证书与私钥（PKCS1/PKCS8/EC 均支持）。
func parseKeyPair(caCertPEM, caKeyPEM []byte) (*x509.Certificate, crypto.PrivateKey, error) {
	block, _ := pem.Decode(caCertPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, errors.New("CA 证书 PEM 无效")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("CA 证书解析失败: %w", err)
	}
	if !ca.IsCA {
		return nil, nil, errors.New("所给证书不是 CA（BasicConstraints CA!=TRUE）")
	}
	kb, _ := pem.Decode(caKeyPEM)
	if kb == nil {
		return nil, nil, errors.New("CA 私钥 PEM 无效")
	}
	var key crypto.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(kb.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParsePKCS1PrivateKey(kb.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParseECPrivateKey(kb.Bytes); err == nil {
		key = k
	} else {
		return nil, nil, errors.New("CA 私钥格式不支持（PKCS1/PKCS8/EC）")
	}
	return ca, key, nil
}

// IssueFromCA 用中间 CA 给指定 IP/域名签发叶子证书。
// 有效期策略：CA 能签多久就签多久（取 CA 剩余有效期 - 24h）；仅当 CA 不可用时才回退自签 10 年。
// 返回值 certPEM 已拼好证书链（叶子 + CA）。
func IssueFromCA(caCertPEM, caKeyPEM []byte, ips []net.IP, dns []string) (certPEM, keyPEM []byte, err error) {
	ca, caKey, err := parseKeyPair(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	cn := ""
	if len(ips) > 0 {
		// 面板以 IP 访问为主，CN 优先取第一个 IP
		cn = ips[0].String()
	} else if len(dns) > 0 {
		cn = dns[0]
	} else {
		cn = "hanyuxin-panel"
	}
	// 正常 CA 签发：有效期跟随 CA（不设固定上限），留 24h 余量避免链校验边界问题
	notAfter := ca.NotAfter.Add(-24 * time.Hour)
	tmpl := x509.Certificate{
		SerialNumber:          bigSerial(),
		Subject:               pkix.Name{CommonName: cn, Organization: []string{"mianban"}},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           ips,
		DNSNames:              dns,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("CA 签发失败: %w", err)
	}
	leaf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	chain := append(append([]byte{}, leaf...), caCertPEM...)
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return nil, nil, err
	}
	return chain, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

// Inspect 读取证书 PEM 的主题与有效期（用于状态展示）。
func Inspect(certPEM []byte) (subject string, notAfter time.Time, err error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", time.Time{}, errors.New("证书 PEM 无效")
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", time.Time{}, err
	}
	return c.Subject.String(), c.NotAfter, nil
}
