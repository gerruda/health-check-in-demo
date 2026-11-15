package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"healthbot/internal/logging"
)

// maxResponseBytes — предохранитель от гигантских ответов модели.
const maxResponseBytes = 1 << 20 // 1 MiB

// ollamaProvider — провайдер локального Ollama (/api/chat, native tools).
type ollamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
	log     *slog.Logger
}

func newOllamaProvider(baseURL, model string, timeout time.Duration, log *slog.Logger) *ollamaProvider {
	return &ollamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
	Images    []string         `json:"images,omitempty"`
}

type ollamaToolCall struct {
	ID       string                 `json:"id,omitempty"`
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	Format   json.RawMessage `json:"format,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaChatResponse struct {
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

// Chat реализует Provider.
func (p *ollamaProvider) Chat(ctx context.Context, req Request) (Message, error) {
	start := time.Now()

	resp, err := p.do(ctx, ollamaChatRequest{
		Model:    p.model,
		Messages: toOllamaMessages(req.Messages),
		Tools:    toOllamaTools(req.Tools),
	})
	if err != nil {
		return Message{}, err
	}

	msg := fromOllamaMessage(resp.Message)
	if resp.PromptEvalCount > 0 || resp.EvalCount > 0 {
		msg.Usage = &Usage{Prompt: resp.PromptEvalCount, Completion: resp.EvalCount, Total: resp.PromptEvalCount + resp.EvalCount}
	}
	p.log.Info(logging.Msg("llm:ollama", "ответ модели"),
		"model", p.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"tool_calls", len(msg.ToolCalls),
		"content_chars", len(msg.Content))
	return msg, nil
}

// Extract реализует Provider: structured output через поле format.
func (p *ollamaProvider) Extract(ctx context.Context, text string, schema json.RawMessage) (json.RawMessage, error) {
	start := time.Now()

	resp, err := p.do(ctx, ollamaChatRequest{
		Model:    p.model,
		Messages: []ollamaMessage{{Role: string(RoleUser), Content: text}},
		Format:   schema,
	})
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(resp.Message.Content)
	if !json.Valid([]byte(content)) {
		return nil, errors.New("ollama: ответ Extract не является JSON")
	}

	p.log.Info(logging.Msg("llm:ollama", "JSON извлечён"),
		"model", p.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"content_chars", len(content))
	return json.RawMessage(content), nil
}

// ExtractVision реализует Provider (FR-6.8.2): изображения base64 + format.
func (p *ollamaProvider) ExtractVision(ctx context.Context, prompt string, schema json.RawMessage, images [][]byte) (json.RawMessage, error) {
	start := time.Now()

	encoded := make([]string, 0, len(images))
	for _, image := range images {
		encoded = append(encoded, base64.StdEncoding.EncodeToString(image))
	}

	resp, err := p.do(ctx, ollamaChatRequest{
		Model: p.model,
		Messages: []ollamaMessage{{
			Role:    string(RoleUser),
			Content: prompt,
			Images:  encoded,
		}},
		Format: schema,
	})
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(resp.Message.Content)
	if !json.Valid([]byte(content)) {
		return nil, fmt.Errorf("ollama: ответ vision не является JSON: %q", trimBody([]byte(content)))
	}

	p.log.Info(logging.Msg("llm:ollama", "vision-ответ"),
		"model", p.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"images", len(images),
		"content_chars", len(content))
	return json.RawMessage(content), nil
}

func (p *ollamaProvider) do(ctx context.Context, req ollamaChatRequest) (*ollamaChatResponse, error) {
	req.Stream = false
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: сериализовать запрос: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama: создать запрос: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		p.log.Error(logging.Msg("llm:ollama", "ошибка запроса к модели"), "model", p.model, "error", err)
		return nil, fmt.Errorf("ollama: запрос /api/chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama: прочитать ответ: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, trimBody(data))
	}

	var out ollamaChatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("ollama: разобрать ответ: %w", err)
	}
	return &out, nil
}

func toOllamaMessages(messages []Message) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, ollamaMessage{
			Role:      string(msg.Role),
			Content:   msg.Content,
			ToolCalls: toOllamaToolCalls(msg.ToolCalls),
			ToolName:  msg.ToolName,
		})
	}
	return out
}

func toOllamaToolCalls(calls []ToolCall) []ollamaToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ollamaToolCall, 0, len(calls))
	for _, call := range calls {
		args := call.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		out = append(out, ollamaToolCall{
			ID: call.ID,
			Function: ollamaToolCallFunction{
				Name:      call.Name,
				Arguments: args,
			},
		})
	}
	return out
}

func toOllamaTools(tools []Tool) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ollamaTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, ollamaTool{
			Type:     "function",
			Function: ollamaToolFunction(tool),
		})
	}
	return out
}

func fromOllamaMessage(msg ollamaMessage) Message {
	out := Message{
		Role:    Role(msg.Role),
		Content: msg.Content,
	}
	if len(msg.ToolCalls) > 0 {
		out.ToolCalls = make([]ToolCall, 0, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			})
		}
	}
	return out
}

func trimBody(data []byte) string {
	const limit = 300
	text := strings.TrimSpace(string(data))
	if len(text) > limit {
		return text[:limit] + "…"
	}
	return text
}
