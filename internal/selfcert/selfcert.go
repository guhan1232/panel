// Package selfcert 面板 HTTPS 自签证书生成（未配置证书时自动生成）。
package selfcert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Ensure 确保目录内存在自签证书，返回 cert/key 路径。
func Ensure(dir string) (string, string, error) {
	certPath := filepath.Join(dir, "panel.crt")
	keyPath := filepath.Join(dir, "panel.key")
	if _, err := os.Stat(certPath); err == nil {
		if _, err2 := os.Stat(keyPath); err2 == nil {
			return certPath, keyPath, nil
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "hanyuxin"},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}
