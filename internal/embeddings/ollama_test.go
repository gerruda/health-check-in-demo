package embeddings_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/embeddings"
)

func TestOllamaEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embed", r.URL.Path)
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		assert.Equal(t, "nomic-embed-text:latest", request.Model)
		vectors := make([][]float64, len(request.Input))
		for index := range request.Input {
			vectors[index] = []float64{float64(index), 1}
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := embeddings.NewOllama(server.URL, "nomic-embed-text:latest", 5*time.Second)
	vectors, err := client.Embed(context.Background(), []string{"сон и HRV", "белок"})
	require.NoError(t, err)
	require.Len(t, vectors, 2)
	assert.Equal(t, []float64{0, 1}, vectors[0])
	assert.Equal(t, []float64{1, 1}, vectors[1])
}

func TestOllamaEmbedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer server.Close()
	client := embeddings.NewOllama(server.URL, "missing", time.Second)
	_, err := client.Embed(context.Background(), []string{"x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
