package webhookcert_test

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/webhookcert"
)

func TestGenerateIPCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	info, err := webhookcert.Generate(webhookcert.Options{
		CertPath: certPath, KeyPath: keyPath, Host: "203.0.113.10", Days: 30,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, info.Fingerprint)
	assert.True(t, info.NotAfter.After(time.Now()))

	data, err := os.ReadFile(certPath)
	require.NoError(t, err)
	block, _ := pem.Decode(data)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Len(t, cert.IPAddresses, 1)
	assert.Equal(t, "203.0.113.10", cert.IPAddresses[0].String())

	keyInfo, err := os.Stat(keyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm())
}

func TestGenerateRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	opts := webhookcert.Options{CertPath: filepath.Join(dir, "c"), KeyPath: filepath.Join(dir, "k"), Host: "test.example"}
	_, err := webhookcert.Generate(opts)
	require.NoError(t, err)
	_, err = webhookcert.Generate(opts)
	require.Error(t, err)
	opts.Force = true
	_, err = webhookcert.Generate(opts)
	require.NoError(t, err)
}
