package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/llm"
)

func newTestProvider(t *testing.T, url, model string, timeout time.Duration) llm.Provider {
	t.Helper()
	provider, err := llm.FromOptions(llm.Options{
		Mode:        llm.ModeOllama,
		OllamaURL:   url,
		OllamaModel: model,
		Timeout:     timeout,
	})
	require.NoError(t, err)
	return provider
}

func TestOllamaChat(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("читать тело: %v", err)
		}
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"Привет, я бот"},"done":true}`))
	}))
	defer srv.Close()

	provider := newTestProvider(t, srv.URL, "test-model", 2*time.Second)

	msg, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	})
	require.NoError(t, err)

	assert.Equal(t, "Привет, я бот", msg.Content)
	assert.Equal(t, "/api/chat", gotPath)
	assert.Contains(t, gotBody, `"model":"test-model"`)
	assert.Contains(t, gotBody, `"stream":false`)
	assert.Contains(t, gotBody, `"content":"привет"`)
}

func TestOllamaToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"read_last_checkin","arguments":{"person":"athlete"}}}]},"done":true}`))
	}))
	defer srv.Close()

	provider := newTestProvider(t, srv.URL, "test-model", 2*time.Second)
	msg, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "какой HRV?"}},
		Tools:    []llm.Tool{{Name: "read_last_checkin", Description: "читает чек-ин"}},
	})
	require.NoError(t, err)

	require.Len(t, msg.ToolCalls, 1)
	assert.Equal(t, "read_last_checkin", msg.ToolCalls[0].Name)
	assert.JSONEq(t, `{"person":"athlete"}`, string(msg.ToolCalls[0].Arguments))
}

func TestOllamaHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	provider := newTestProvider(t, srv.URL, "test-model", 2*time.Second)
	_, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama")
}

func TestOllamaTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"поздно"}}`))
	}))
	defer srv.Close()

	provider := newTestProvider(t, srv.URL, "test-model", 50*time.Millisecond)
	_, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	})
	require.Error(t, err)
}

func TestOllamaExtract(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"hrv\":34}"},"done":true}`))
	}))
	defer srv.Close()

	provider := newTestProvider(t, srv.URL, "test-model", 2*time.Second)
	schema := json.RawMessage(`{"type":"object","properties":{"hrv":{"type":"integer"}}}`)

	raw, err := provider.Extract(context.Background(), "HRV 34", schema)
	require.NoError(t, err)
	assert.JSONEq(t, `{"hrv":34}`, string(raw))
	assert.Contains(t, gotBody, `"format"`, "Extract должен передавать JSON-схему в format")
	assert.Contains(t, gotBody, `"stream":false`)
}
