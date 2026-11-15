package config_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"healthbot/internal/config"
	"healthbot/internal/webhookcert"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allEnvKeys = []string{
	"BOT_TOKEN",
	"BOT_PROXY_URL",
	"TELEGRAM_CHAT_ID_ATHLETE",
	"TELEGRAM_CHAT_ID_WIFE",
	"DATA_ROOT",
	"LOG_DIR",
	"SETTINGS_DIR",
	"LLM_MODE",
	"OLLAMA_URL",
	"OLLAMA_MODEL",
	"LLM_API_URL",
	"LLM_API_KEY",
	"LLM_MODEL",
	"LLM_FALLBACK",
	"LLM_VISION_MODE",
	"LLM_VISION_MODEL",
	"LLM_VISION_API_URL",
	"LLM_VISION_API_KEY",
	"WEBHOOK_ENABLED",
	"WEBHOOK_URL",
	"WEBHOOK_LISTEN",
	"WEBHOOK_SECRET",
	"WEBHOOK_MAX_CONNECTIONS",
	"WEBHOOK_DROP_PENDING",
	"WEBHOOK_TLS_CERT",
	"WEBHOOK_TLS_KEY",
	"METRICS_ENABLED",
	"METRICS_LISTEN",
	"METRICS_PATH",
}

// unsetEnv снимает переменные на время теста; t.Cleanup восстановит исходные
// значения. Нужен, чтобы дефолты из envDefault действительно применялись.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("снять %s: %v", key, err)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "../..", cfg.DataRoot)
	assert.Equal(t, "logs", cfg.LogDir)
	assert.Equal(t, "настройки", cfg.SettingsDir)
	assert.Equal(t, config.ModeOllama, cfg.LLMMode)
	assert.Equal(t, "http://127.0.0.1:11434", cfg.OllamaURL)
	assert.Equal(t, "qwen2.5:3b", cfg.OllamaModel)
	assert.False(t, cfg.VisionEnabled(), "vision выключен, пока не задан (FR-6.8.1)")
	assert.Equal(t, config.ModeOllama, cfg.VisionMode())
	assert.Equal(t, "qwen2.5:3b", cfg.VisionModel())
	assert.Equal(t, "123456:test-token", cfg.BotToken)
	assert.Equal(t, int64(0), cfg.AthleteChatID)
	assert.False(t, cfg.MetricsEnabled, "метрики по умолчанию выключены")
}

func TestLoadChatIDs(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("TELEGRAM_CHAT_ID_ATHLETE", "111111111")
	t.Setenv("TELEGRAM_CHAT_ID_WIFE", "222222222")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, int64(111111111), cfg.AthleteChatID)
	assert.Equal(t, int64(222222222), cfg.WifeChatID)
}

func TestInvalidLLMMode(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("LLM_MODE", "bogus")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM_MODE")
}

func TestAPIModeRequiresCredentials(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("LLM_MODE", config.ModeAPI)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM_API_URL")

	t.Setenv("LLM_API_URL", "https://api.example.com/v1")
	t.Setenv("LLM_MODEL", "example-chat")
	t.Setenv("LLM_API_KEY", "sk-test")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, config.ModeAPI, cfg.LLMMode)
	assert.Equal(t, "https://api.example.com/v1", cfg.LLMAPIURL)
}

func TestFallbackMustDifferFromActiveMode(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("LLM_MODE", config.ModeOllama)
	t.Setenv("LLM_FALLBACK", config.ModeOllama)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM_FALLBACK")
}

func TestRequireBotToken(t *testing.T) {
	unsetEnv(t, "BOT_TOKEN")

	cfg := config.Config{}
	require.Error(t, cfg.RequireBotToken())

	cfg.BotToken = "123456:test-token"
	require.NoError(t, cfg.RequireBotToken())
}

func TestLogValueMasksSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	cfg := config.Config{ //nolint:gosec // G101: тестовые фикстуры, не реальные секреты
		BotToken:    "SECRET-BOT-TOKEN",
		LLMAPIKey:   "SECRET-API-KEY",
		LLMMode:     config.ModeOllama,
		OllamaURL:   "http://localhost:11434",
		OllamaModel: "qwen3",
		DataRoot:    "../..",
		LogDir:      "logs",
	}
	logger.Info("config", "config", cfg)

	out := buf.String()
	assert.NotContains(t, out, "SECRET-BOT-TOKEN")
	assert.NotContains(t, out, "SECRET-API-KEY")
	assert.Contains(t, out, "bot_token_set")
}

func TestLoadDotEnv(t *testing.T) {
	unsetEnv(t, "HC_TEST_TOKEN", "HC_TEST_MODE", "HC_TEST_QUOTED")

	path := filepath.Join(t.TempDir(), ".env")
	content := "# комментарий\n\nHC_TEST_TOKEN=abc:123\nexport HC_TEST_MODE=api\nHC_TEST_QUOTED=\"x y\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	require.NoError(t, config.LoadDotEnv(path))

	assert.Equal(t, "abc:123", os.Getenv("HC_TEST_TOKEN"))
	assert.Equal(t, "api", os.Getenv("HC_TEST_MODE"))
	assert.Equal(t, "x y", os.Getenv("HC_TEST_QUOTED"))
}

func TestLoadDotEnvDoesNotOverride(t *testing.T) {
	t.Setenv("HC_TEST_EXISTING", "from-env")

	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte("HC_TEST_EXISTING=from-file\n"), 0o600))

	require.NoError(t, config.LoadDotEnv(path))
	assert.Equal(t, "from-env", os.Getenv("HC_TEST_EXISTING"))
}

func TestLoadDotEnvMissingFileIsOK(t *testing.T) {
	require.NoError(t, config.LoadDotEnv(filepath.Join(t.TempDir(), "нет-файла.env")))
}

func TestProxyDefaultsEmpty(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Empty(t, cfg.BotProxyURL)
	proxy, err := cfg.TelegramProxy()
	require.NoError(t, err)
	assert.Nil(t, proxy, "без BOT_PROXY_URL подключение прямое")
}

func TestProxyValid(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("BOT_PROXY_URL", "http://127.0.0.1:12334")

	cfg, err := config.Load()
	require.NoError(t, err)

	proxy, err := cfg.TelegramProxy()
	require.NoError(t, err)
	require.NotNil(t, proxy)
	assert.Equal(t, "http", proxy.Scheme)
	assert.Equal(t, "127.0.0.1:12334", proxy.Host)
}

func TestProxySOCKS5(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("BOT_PROXY_URL", "socks5://127.0.0.1:1080")

	cfg, err := config.Load()
	require.NoError(t, err)

	proxy, err := cfg.TelegramProxy()
	require.NoError(t, err)
	require.NotNil(t, proxy)
	assert.Equal(t, "socks5", proxy.Scheme)
}

func TestProxyInvalid(t *testing.T) {
	tests := []struct {
		name  string
		proxy string
	}{
		{name: "без схемы", proxy: "127.0.0.1:12334"},
		{name: "неподдерживаемая схема", proxy: "ftp://127.0.0.1:12334"},
		{name: "без хоста", proxy: "http://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, allEnvKeys...)
			t.Setenv("BOT_TOKEN", "123456:test-token")
			t.Setenv("BOT_PROXY_URL", tt.proxy)

			_, err := config.Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "BOT_PROXY_URL")
		})
	}
}

func TestLogValueHidesProxyCredentials(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	cfg := config.Config{ //nolint:gosec // G101: тестовые фикстуры, не реальные секреты
		BotToken:    "SECRET-BOT-TOKEN",
		BotProxyURL: "http://user:secret-pass@127.0.0.1:12334",
		LLMMode:     config.ModeOllama,
		DataRoot:    "../..",
	}
	logger.Info("config", "config", cfg)

	out := buf.String()
	assert.NotContains(t, out, "secret-pass", "пароль прокси не должен попадать в логи")
	assert.Contains(t, out, "127.0.0.1:12334", "хост прокси полезен для диагностики")
}

func TestVisionSeparateFromChat(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("LLM_VISION_MODE", config.ModeAPI)
	t.Setenv("LLM_VISION_MODEL", "vision-model")
	t.Setenv("LLM_VISION_API_URL", "https://vision.example.com/v1")
	t.Setenv("LLM_VISION_API_KEY", "vision-key")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.True(t, cfg.VisionEnabled())
	assert.Equal(t, config.ModeAPI, cfg.VisionMode())
	assert.Equal(t, "vision-model", cfg.VisionModel())
	assert.Equal(t, "https://vision.example.com/v1", cfg.VisionAPIURL())
	assert.Equal(t, "vision-key", cfg.VisionAPIKey())
	assert.Equal(t, config.ModeOllama, cfg.LLMMode, "чат-провайдер не меняется")
}

func TestVisionFallsBackToCommonAPICredentials(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("LLM_MODE", config.ModeAPI)
	t.Setenv("LLM_API_URL", "https://api.example.com/v1")
	t.Setenv("LLM_API_KEY", "common-key")
	t.Setenv("LLM_MODEL", "common-model")
	t.Setenv("LLM_VISION_MODE", config.ModeAPI)
	t.Setenv("LLM_VISION_MODEL", "vision-model")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "https://api.example.com/v1", cfg.VisionAPIURL())
	assert.Equal(t, "common-key", cfg.VisionAPIKey())
}

func TestVisionInvalidMode(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("LLM_VISION_MODE", "bogus")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM_VISION_MODE")
}

func TestVisionAPIModeRequiresURL(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("LLM_VISION_MODE", config.ModeAPI)
	t.Setenv("LLM_VISION_MODEL", "vision-model")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM_VISION")
}

func TestWebhookConfig(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("WEBHOOK_ENABLED", "true")
	t.Setenv("WEBHOOK_URL", "https://bot.example.com/telegram/webhook")
	t.Setenv("WEBHOOK_LISTEN", "127.0.0.1:8080")
	t.Setenv("WEBHOOK_SECRET", "secret-123")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.WebhookEnabled)
	assert.Equal(t, "/telegram/webhook", cfg.WebhookPath())
	assert.Equal(t, 1, cfg.WebhookMaxConnections)

	t.Setenv("WEBHOOK_URL", "http://bot.example.com/telegram/webhook")
	_, err = config.Load()
	require.Error(t, err, "Telegram webhook требует https")

	t.Setenv("WEBHOOK_URL", "https://bot.example.com/telegram/webhook")
	t.Setenv("WEBHOOK_SECRET", "bad secret!")
	_, err = config.Load()
	require.Error(t, err, "недопустимые символы секрета")
}

func TestWebhookTLSConfig(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("WEBHOOK_ENABLED", "true")
	t.Setenv("WEBHOOK_URL", "https://203.0.113.10:8443/telegram/webhook")
	t.Setenv("WEBHOOK_LISTEN", "0.0.0.0:8443")
	t.Setenv("WEBHOOK_SECRET", "secret-123")

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	_, err := webhookcert.Generate(webhookcert.Options{
		CertPath: certPath, KeyPath: keyPath, Host: "203.0.113.10", Days: 30,
	})
	require.NoError(t, err)
	t.Setenv("WEBHOOK_TLS_CERT", certPath)
	t.Setenv("WEBHOOK_TLS_KEY", keyPath)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.WebhookTLSEnabled())
	assert.Equal(t, 8443, mustWebhookPort(t, cfg))

	// порт URL и listener должны совпадать
	t.Setenv("WEBHOOK_LISTEN", "0.0.0.0:8080")
	_, err = config.Load()
	require.Error(t, err)

	t.Setenv("WEBHOOK_LISTEN", "0.0.0.0:8443")
	t.Setenv("WEBHOOK_TLS_KEY", "")
	_, err = config.Load()
	require.Error(t, err, "cert без key запрещён")
}

func mustWebhookPort(t *testing.T, cfg config.Config) int {
	t.Helper()
	port, err := cfg.WebhookURLPort()
	require.NoError(t, err)
	return port
}

func TestMetricsConfig(t *testing.T) {
	unsetEnv(t, allEnvKeys...)
	t.Setenv("BOT_TOKEN", "123456:test-token")
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_LISTEN", "127.0.0.1:9091")
	t.Setenv("METRICS_PATH", "/metrics")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.MetricsEnabled)
	assert.Equal(t, "127.0.0.1:9091", cfg.MetricsListen)
	assert.Equal(t, "/metrics", cfg.MetricsPath)

	t.Setenv("METRICS_LISTEN", "bad")
	_, err = config.Load()
	require.Error(t, err)
}
