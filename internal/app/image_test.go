package app_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/app"
)

func TestScanImageReturnsExtracted(t *testing.T) {
	var calls int
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"{\"kinds\":[\"checkin\"],\"checkin\":{\"hrv_ms\":28,\"rhr\":62}}"},"done":true}`))
	}))
	defer srv.Close()

	unsetEnv(t,
		"BOT_TOKEN", "BOT_PROXY_URL", "TELEGRAM_CHAT_ID_ATHLETE", "TELEGRAM_CHAT_ID_WIFE",
		"DATA_ROOT", "LOG_DIR", "LLM_MODE", "OLLAMA_URL", "OLLAMA_MODEL",
		"LLM_API_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_FALLBACK",
		"LLM_VISION_MODE", "LLM_VISION_MODEL", "LLM_VISION_API_URL", "LLM_VISION_API_KEY",
	)
	t.Setenv("LLM_MODE", "ollama")
	t.Setenv("OLLAMA_URL", srv.URL)
	t.Setenv("OLLAMA_MODEL", "chat-model")
	t.Setenv("LLM_VISION_MODE", "ollama")
	t.Setenv("LLM_VISION_MODEL", "vision-model")
	t.Setenv("LOG_DIR", t.TempDir())

	dir := t.TempDir()
	path := filepath.Join(dir, "shot.jpg")
	require.NoError(t, os.WriteFile(path, []byte("fake-jpeg-bytes"), 0o600))

	ex, err := app.ScanImage(context.Background(), []string{path})
	require.NoError(t, err)

	require.NotNil(t, ex.Checkin)
	assert.Equal(t, 28, *ex.Checkin.HRV)
	assert.Equal(t, 62, *ex.Checkin.RHR)
	assert.Equal(t, 1, calls)
	assert.Contains(t, body, `"images"`)
	assert.Contains(t, body, `"vision-model"`)
}

func TestScanImageWithoutVisionConfig(t *testing.T) {
	unsetEnv(t,
		"BOT_TOKEN", "BOT_PROXY_URL", "DATA_ROOT", "LOG_DIR", "LLM_MODE", "OLLAMA_URL", "OLLAMA_MODEL",
		"LLM_API_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_FALLBACK",
		"LLM_VISION_MODE", "LLM_VISION_MODEL", "LLM_VISION_API_URL", "LLM_VISION_API_KEY",
	)
	t.Setenv("LOG_DIR", t.TempDir())

	_, err := app.ScanImage(context.Background(), []string{"нет-файла.jpg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vision")
}
