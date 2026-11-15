package llm

import (
	"context"
	"encoding/json"
	"time"

	"healthbot/internal/metrics"
)

// observedProvider оборачивает Provider метриками Prometheus.
type observedProvider struct {
	next         Provider
	providerName string
	model        string
}

// WithMetrics добавляет счётчики/латентность/токены к провайдеру.
func WithMetrics(provider Provider, providerName, model string) Provider {
	if provider == nil {
		return nil
	}
	return &observedProvider{next: provider, providerName: providerName, model: model}
}

func (p *observedProvider) Chat(ctx context.Context, req Request) (Message, error) {
	start := time.Now()
	msg, err := p.next.Chat(ctx, req)
	status := "ok"
	if err != nil {
		status = "error"
	}
	var usage *metrics.TokenUsage
	if msg.Usage != nil {
		usage = &metrics.TokenUsage{Prompt: msg.Usage.Prompt, Completion: msg.Usage.Completion, Total: msg.Usage.Total}
	}
	metrics.Default().ObserveLLM(p.providerName, p.model, "chat", status, time.Since(start), usage)
	return msg, err
}

func (p *observedProvider) Extract(ctx context.Context, text string, schema json.RawMessage) (json.RawMessage, error) {
	start := time.Now()
	raw, err := p.next.Extract(ctx, text, schema)
	status := "ok"
	if err != nil {
		status = "error"
		metrics.Default().IncError("llm_extract")
	}
	metrics.Default().ObserveLLM(p.providerName, p.model, "extract", status, time.Since(start), nil)
	return raw, err
}

func (p *observedProvider) ExtractVision(ctx context.Context, prompt string, schema json.RawMessage, images [][]byte) (json.RawMessage, error) {
	start := time.Now()
	raw, err := p.next.ExtractVision(ctx, prompt, schema, images)
	status := "ok"
	if err != nil {
		status = "error"
		metrics.Default().IncError("llm_vision")
	}
	metrics.Default().ObserveLLM(p.providerName, p.model, "vision", status, time.Since(start), nil)
	return raw, err
}
