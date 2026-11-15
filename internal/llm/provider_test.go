package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/llm"
)

func TestFromOptionsValidation(t *testing.T) {
	tests := []struct {
		name string
		opts llm.Options
	}{
		{name: "неизвестный режим", opts: llm.Options{Mode: "unknown"}},
		{name: "ollama без URL", opts: llm.Options{Mode: llm.ModeOllama, OllamaModel: "m"}},
		{name: "api без URL", opts: llm.Options{Mode: llm.ModeAPI, Model: "m", APIKey: "k"}},
		{name: "api без модели", opts: llm.Options{Mode: llm.ModeAPI, APIURL: "http://x", APIKey: "k"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := llm.FromOptions(tt.opts)
			require.Error(t, err)
		})
	}
}

func TestFallbackUsesSecondaryProvider(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "primary down", http.StatusInternalServerError)
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ответ из второго провайдера"}}]}`))
	}))
	defer secondary.Close()

	provider, err := llm.FromOptions(llm.Options{
		Mode:        llm.ModeOllama,
		OllamaURL:   primary.URL,
		OllamaModel: "m",
		Fallback:    llm.ModeAPI,
		APIURL:      secondary.URL,
		APIKey:      "k",
		Model:       "m",
		Timeout:     2 * time.Second,
	})
	require.NoError(t, err)

	msg, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "ответ из второго провайдера", msg.Content)
}

func TestFallbackBothFail(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer failing.Close()

	provider, err := llm.FromOptions(llm.Options{
		Mode:        llm.ModeOllama,
		OllamaURL:   failing.URL,
		OllamaModel: "m",
		Fallback:    llm.ModeAPI,
		APIURL:      failing.URL,
		APIKey:      "k",
		Model:       "m",
		Timeout:     2 * time.Second,
	})
	require.NoError(t, err)

	_, err = provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "привет"}},
	})
	require.Error(t, err)
}
