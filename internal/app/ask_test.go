package app_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/app"
)

func setAskEnv(t *testing.T, ollamaURL string) {
	t.Helper()
	unsetEnv(t,
		"BOT_TOKEN", "BOT_PROXY_URL", "TELEGRAM_CHAT_ID_ATHLETE", "TELEGRAM_CHAT_ID_WIFE",
		"DATA_ROOT", "LOG_DIR", "LLM_MODE", "OLLAMA_URL", "OLLAMA_MODEL",
		"LLM_API_URL", "LLM_API_KEY", "LLM_MODEL", "LLM_FALLBACK",
		"EMBEDDING_ENABLED", "EMBEDDING_URL", "EMBEDDING_MODEL",
	)
	t.Setenv("LLM_MODE", "ollama")
	t.Setenv("OLLAMA_URL", ollamaURL)
	t.Setenv("OLLAMA_MODEL", "test-model")
	t.Setenv("LOG_DIR", t.TempDir())
	t.Setenv("EMBEDDING_ENABLED", "false")
}

func TestAskReturnsModelAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Ответ модели"},"done":true}`))
	}))
	defer srv.Close()

	setAskEnv(t, srv.URL)

	answer, err := app.Ask(context.Background(), "привет")
	require.NoError(t, err)
	assert.Equal(t, "Ответ модели", answer)
}

func TestAskExecutesLastCheckinTool(t *testing.T) {
	dataRoot := t.TempDir()
	checkinDir := filepath.Join(dataRoot, "данные_пользователя", "ежедневные_чекины")
	require.NoError(t, os.MkdirAll(checkinDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(checkinDir, "2026-11-03.md"),
		[]byte("# Чек-ин 2026-11-03\nHRV: 34 мс\nПульс покоя: 58"),
		0o600,
	))

	var (
		mu       sync.Mutex
		requests int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		n := requests
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"read_last_checkin","arguments":{"person":"athlete"}}}]},"done":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"В последнем чек-ине HRV 34 мс."},"done":true}`))
	}))
	defer srv.Close()

	setAskEnv(t, srv.URL)
	t.Setenv("DATA_ROOT", dataRoot)

	answer, err := app.Ask(context.Background(), "какой HRV в последнем чек-ине?")
	require.NoError(t, err)
	assert.Contains(t, answer, "34")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, requests, "ожидается tool call и финальный ответ модели")
}

func TestAskProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	setAskEnv(t, srv.URL)

	_, err := app.Ask(context.Background(), "привет")
	require.Error(t, err)
}

func TestAskExecutesKnowledgeSearch(t *testing.T) {
	dataRoot := t.TempDir()
	kbDir := filepath.Join(dataRoot, "база_знаний", "исследования")
	require.NoError(t, os.MkdirAll(kbDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(kbDir, "сон.md"),
		[]byte("# Сон\n\n## HRV\n\nСон и вариабельность ЧСС. PMID: 12345678.\n"),
		0o600,
	))

	var (
		mu       sync.Mutex
		requests int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		n := requests
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"search_knowledge","arguments":{"query":"сон и HRV","person":"athlete"}}}]},"done":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Сон и HRV связаны; источник: сон.md, PMID 12345678."},"done":true}`))
	}))
	defer srv.Close()

	setAskEnv(t, srv.URL)
	t.Setenv("DATA_ROOT", dataRoot)

	answer, err := app.Ask(context.Background(), "что с сном и HRV?")
	require.NoError(t, err)
	assert.Contains(t, answer, "12345678")
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, requests, "ожидается tool call search_knowledge")
}

func TestAskInjectsKnowledgeContextBeforeLLM(t *testing.T) {
	dataRoot := t.TempDir()
	kbDir := filepath.Join(dataRoot, "база_знаний", "исследования")
	require.NoError(t, os.MkdirAll(kbDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(kbDir, "сон.md"),
		[]byte("# Сон\n\n## HRV\n\nСон и вариабельность ЧСС. PMID: 12345678.\n"),
		0o600,
	))

	var (
		mu      sync.Mutex
		body    string
		request int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		request++
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Сон важен для восстановления."},"done":true}`))
	}))
	defer srv.Close()

	setAskEnv(t, srv.URL)
	t.Setenv("DATA_ROOT", dataRoot)
	answer, err := app.Ask(context.Background(), "что с сном и HRV?")
	require.NoError(t, err)
	assert.Contains(t, answer, "Сон важен")
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, request)
	assert.Contains(t, body, "12345678", "контекст базы знаний должен попасть в промпт")
}

func TestAskSkipsKnowledgeContextForGibberish(t *testing.T) {
	dataRoot := t.TempDir()
	kbDir := filepath.Join(dataRoot, "база_знаний", "исследования")
	require.NoError(t, os.MkdirAll(kbDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(kbDir, "сон.md"), []byte("# Сон\n\nPMID: 12345678\n"), 0o600))
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Не понял."},"done":true}`))
	}))
	defer srv.Close()
	setAskEnv(t, srv.URL)
	t.Setenv("DATA_ROOT", dataRoot)

	answer, err := app.Ask(context.Background(), "ываыва")
	require.NoError(t, err)
	assert.Contains(t, answer, "Не понял")
	assert.NotContains(t, body, "12345678", "для мусора база не должна подмешиваться")
}
