package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"healthbot/internal/app"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("снять %s: %v", key, err)
		}
	}
}

func TestRunDryRunWithoutToken(t *testing.T) {
	unsetEnv(t,
		"BOT_TOKEN", "BOT_PROXY_URL", "TELEGRAM_CHAT_ID_ATHLETE", "TELEGRAM_CHAT_ID_WIFE",
		"DATA_ROOT", "LLM_MODE", "OLLAMA_URL", "OLLAMA_MODEL",
		"LLM_API_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_FALLBACK",
	)
	logDir := t.TempDir()
	t.Setenv("LOG_DIR", logDir)

	require.NoError(t, app.Run(context.Background(), true),
		"dry-run должен работать без токена: конфиг + /help")

	data, err := os.ReadFile(filepath.Join(logDir, "healthbot.log"))
	require.NoError(t, err)
	logs := string(data)
	assert.Contains(t, logs, "[app:Run] запуск healthbot", "лог должен иметь формат [пакет:метод]")
	assert.Contains(t, logs, "[app:Run] dry-run:", "dry-run должен отмечаться в логе")
}

func TestRunWithoutTokenFails(t *testing.T) {
	unsetEnv(t,
		"BOT_TOKEN", "BOT_PROXY_URL", "LLM_MODE", "LLM_API_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_FALLBACK",
	)
	t.Setenv("LOG_DIR", t.TempDir())

	err := app.Run(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BOT_TOKEN")
}
