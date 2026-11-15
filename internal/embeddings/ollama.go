// Package embeddings — локальные embeddings через Ollama (M3 vector search).
package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Ollama — клиент /api/embed (nomic-embed-text и совместимые модели).
type Ollama struct {
	url    string
	model  string
	client *http.Client
}

// NewOllama создаёт клиент; пустой URL — ошибка на уровне вызова Embed.
func NewOllama(url, model string, timeout time.Duration) *Ollama {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Ollama{
		url:    strings.TrimRight(strings.TrimSpace(url), "/"),
		model:  strings.TrimSpace(model),
		client: &http.Client{Timeout: timeout},
	}
}

// Name — идентификатор модели для кэша.
func (o *Ollama) Name() string { return "ollama/" + o.model }

// Embed возвращает вектора для текстов (батчами, Ollama /api/embed).
func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if o.url == "" || o.model == "" {
		return nil, fmt.Errorf("embeddings: не заданы URL или модель")
	}
	all := make([][]float64, 0, len(texts))
	const batchSize = 64
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vectors, err := o.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		all = append(all, vectors...)
	}
	if len(all) != len(texts) {
		return nil, fmt.Errorf("embeddings: получено %d векторов вместо %d", len(all), len(texts))
	}
	return all, nil
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

func (o *Ollama) embedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	payload, err := json.Marshal(embedRequest{Model: o.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("embeddings: сериализовать запрос: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("embeddings: запрос: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := o.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("embeddings: Ollama: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return nil, fmt.Errorf("embeddings: Ollama вернул %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded embedResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("embeddings: разобрать ответ: %w", err)
	}
	if len(decoded.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embeddings: Ollama вернул %d векторов на %d текстов", len(decoded.Embeddings), len(texts))
	}
	return decoded.Embeddings, nil
}
