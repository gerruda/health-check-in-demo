package llm_test

import (
	"context"
	"encoding/base64"
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

func TestOllamaExtractVision(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"kinds\":[\"checkin\"],\"checkin\":{\"hrv_ms\":28}}"},"done":true}`))
	}))
	defer srv.Close()

	provider, err := llm.VisionFromOptions(llm.Options{
		VisionMode:  llm.ModeOllama,
		OllamaURL:   srv.URL,
		VisionModel: "vision-model",
		Timeout:     2 * time.Second,
	})
	require.NoError(t, err)

	schema := json.RawMessage(`{"type":"object"}`)
	raw, err := provider.ExtractVision(context.Background(), "прочитай скриншот", schema, [][]byte{[]byte("img-bytes")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"kinds":["checkin"],"checkin":{"hrv_ms":28}}`, string(raw))

	assert.Contains(t, gotBody, `"format"`)
	assert.Contains(t, gotBody, `"images"`)
	assert.Contains(t, gotBody, base64.StdEncoding.EncodeToString([]byte("img-bytes")))
	assert.Contains(t, gotBody, `"model":"vision-model"`)
}

func TestOpenAIExtractVision(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"kinds\":[\"nutrition\"],\"nutrition\":{\"kcal\":2480}}"}}]}`))
	}))
	defer srv.Close()

	provider, err := llm.VisionFromOptions(llm.Options{
		VisionMode:   llm.ModeAPI,
		VisionAPIURL: srv.URL,
		VisionAPIKey: "vision-key",
		VisionModel:  "vision-model",
		Timeout:      2 * time.Second,
	})
	require.NoError(t, err)

	raw, err := provider.ExtractVision(context.Background(), "прочитай скриншот", json.RawMessage(`{"type":"object"}`), [][]byte{[]byte("jpeg-bytes")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"kinds":["nutrition"],"nutrition":{"kcal":2480}}`, string(raw))

	assert.Equal(t, "Bearer vision-key", gotAuth)
	assert.Contains(t, gotBody, `"image_url"`)
	assert.Contains(t, gotBody, `"data:image/`)
	assert.Contains(t, gotBody, `"response_format"`)
	assert.Contains(t, gotBody, `"json_object"`)
}

func TestVisionFromOptionsSeparateFromChat(t *testing.T) {
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"{\"kinds\":[\"checkin\"]}"},"done":true}`))
	}))
	defer ollamaSrv.Close()

	provider, err := llm.VisionFromOptions(llm.Options{
		Mode:        llm.ModeAPI, // чат через API
		APIURL:      "http://chat-api.invalid",
		Model:       "chat-model",
		VisionMode:  llm.ModeOllama,
		OllamaURL:   ollamaSrv.URL,
		VisionModel: "vision-model",
		Timeout:     2 * time.Second,
	})
	require.NoError(t, err)

	_, err = provider.ExtractVision(context.Background(), "p", json.RawMessage(`{}`), [][]byte{[]byte("x")})
	require.NoError(t, err)
}

func TestVisionFromOptionsFallbackToChatModel(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"{\"kinds\":[\"other\"]}"},"done":true}`))
	}))
	defer srv.Close()

	provider, err := llm.VisionFromOptions(llm.Options{
		Mode:        llm.ModeOllama,
		OllamaURL:   srv.URL,
		OllamaModel: "chat-model",
		Timeout:     2 * time.Second,
	})
	require.NoError(t, err)

	_, err = provider.ExtractVision(context.Background(), "p", json.RawMessage(`{}`), [][]byte{[]byte("x")})
	require.NoError(t, err)
	assert.Contains(t, gotBody, `"model":"chat-model"`)
}

func TestVisionFromOptionsInvalidMode(t *testing.T) {
	_, err := llm.VisionFromOptions(llm.Options{VisionMode: "bogus"})
	require.Error(t, err)
}
