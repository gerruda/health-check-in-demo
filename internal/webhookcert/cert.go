// Package webhookcert генерирует self-signed сертификат для webhook Telegram.
package webhookcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Options — параметры генерации сертификата.
type Options struct {
	CertPath string
	KeyPath  string
	Host     string
	Days     int
	Force    bool
}

// Info — сведения о созданном сертификате.
type Info struct {
	CertPath    string
	KeyPath     string
	CommonName  string
	NotAfter    time.Time
	Fingerprint string
}

// Generate создаёт self-signed сертификат для IP или DNS-имени.
// Для IP SAN обязателен: Telegram проверяет сертификат при загрузке.
func Generate(options Options) (Info, error) {
	if options.CertPath == "" || options.KeyPath == "" {
		return Info{}, fmt.Errorf("webhookcert: нужны пути cert и key")
	}
	host := options.Host
	if host == "" {
		return Info{}, fmt.Errorf("webhookcert: не задан host (IP или DNS)")
	}
	if options.Days <= 0 {
		options.Days = 3650
	}
	if !options.Force {
		for _, path := range []string{options.CertPath, options.KeyPath} {
			if _, err := os.Stat(path); err == nil {
				return Info{}, fmt.Errorf("webhookcert: %q уже существует (используй -force)", path)
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Info{}, fmt.Errorf("webhookcert: ключ: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Info{}, fmt.Errorf("webhookcert: serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, options.Days),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return Info{}, fmt.Errorf("webhookcert: сертификат: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return Info{}, fmt.Errorf("webhookcert: ключ: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err = os.MkdirAll(filepath.Dir(options.CertPath), 0o750); err != nil {
		return Info{}, fmt.Errorf("webhookcert: каталог cert: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(options.KeyPath), 0o750); err != nil {
		return Info{}, fmt.Errorf("webhookcert: каталог key: %w", err)
	}
	if err = os.WriteFile(options.CertPath, certPEM, 0o644); err != nil { // #nosec G306 -- сертификат публичный
		return Info{}, fmt.Errorf("webhookcert: записать cert: %w", err)
	}
	if err = os.WriteFile(options.KeyPath, keyPEM, 0o600); err != nil {
		return Info{}, fmt.Errorf("webhookcert: записать key: %w", err)
	}
	if err = os.Chmod(options.KeyPath, 0o600); err != nil {
		return Info{}, fmt.Errorf("webhookcert: права key: %w", err)
	}

	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return Info{}, fmt.Errorf("webhookcert: разобрать сертификат: %w", err)
	}
	sum := sha256.Sum256(parsed.Raw)
	return Info{
		CertPath:    options.CertPath,
		KeyPath:     options.KeyPath,
		CommonName:  host,
		NotAfter:    parsed.NotAfter,
		Fingerprint: hex.EncodeToString(sum[:]),
	}, nil
}
