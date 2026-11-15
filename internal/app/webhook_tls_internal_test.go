package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/config"
	"healthbot/internal/webhookcert"
)

func TestConfigureWebhookUploadsSelfSignedCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	_, err := webhookcert.Generate(webhookcert.Options{
		CertPath: certPath, KeyPath: keyPath, Host: "203.0.113.10", Days: 30,
	})
	require.NoError(t, err)

	var gotCertificate []byte
	var gotURL string
	telegram := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.True(t, strings.HasSuffix(request.URL.Path, "/setWebhook"))
		if parseErr := request.ParseMultipartForm(1 << 20); parseErr != nil { // #nosec G120 -- тест, лимит 1 МиБ
			http.Error(writer, parseErr.Error(), http.StatusBadRequest)
			return
		}
		gotURL = request.FormValue("url")
		file, _, formErr := request.FormFile("certificate")
		if formErr == nil {
			defer func() { _ = file.Close() }()
			gotCertificate, _ = io.ReadAll(file)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer telegram.Close()

	bot, err := telego.NewBot("123456:AAHaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", telego.WithAPIServer(telegram.URL))
	require.NoError(t, err)
	cfg := config.Config{
		WebhookEnabled:        true,
		WebhookURL:            "https://203.0.113.10:8443/telegram/webhook",
		WebhookTLSCert:        certPath,
		WebhookTLSKey:         keyPath,
		WebhookSecret:         "secret",
		WebhookMaxConnections: 1,
	}
	require.NoError(t, configureWebhook(context.Background(), bot, cfg))
	assert.Equal(t, cfg.WebhookURL, gotURL)
	certBytes, err := os.ReadFile(certPath)
	require.NoError(t, err)
	assert.Equal(t, string(certBytes), string(gotCertificate), "в Telegram должен уйти публичный сертификат")
}
