package llm_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/llm"
)

func newAPIProvider(t *testing.T, url, model, key string) llm.Provider {
	t.Helper()
	provider, err := llm.FromOptions(llm.Options{
		Mode:    llm.ModeAPI,
		APIURL:  url,
		APIKey:  key,
		Model:   model,
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	return provider
}

func TestOpenAIChat(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Ответ API"}}]}`))
	}))
	defer srv.Close()

	provider := newAPIProvider(t, srv.URL, "test-model", "test-key")
	msg, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	})
	require.NoError(t, err)

	assert.Equal(t, "Ответ API", msg.Content)
	assert.Equal(t, "/chat/completions", gotPath)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Contains(t, gotBody, `"model":"test-model"`)
}

func TestOpenAIToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_2","type":"function","function":{"name":"read_last_checkin","arguments":"{\"person\":\"wife\"}"}}]}}]}`))
	}))
	defer srv.Close()

	provider := newAPIProvider(t, srv.URL, "test-model", "test-key")
	msg, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "чек-ин жены"}},
		Tools:    []llm.Tool{{Name: "read_last_checkin", Description: "читает чек-ин"}},
	})
	require.NoError(t, err)

	require.Len(t, msg.ToolCalls, 1)
	assert.Equal(t, "read_last_checkin", msg.ToolCalls[0].Name)
	assert.JSONEq(t, `{"person":"wife"}`, string(msg.ToolCalls[0].Arguments))
	assert.Equal(t, "call_2", msg.ToolCalls[0].ID)
}

func TestOpenAIHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer srv.Close()

	provider := newAPIProvider(t, srv.URL, "test-model", "bad-key")
	_, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api")
	assert.Contains(t, err.Error(), "401")
}
