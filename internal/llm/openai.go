package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"healthbot/internal/logging"
)

// openAIProvider — произвольный OpenAI-совместимый chat/completions (FR-6.3).
type openAIProvider struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
	log     *slog.Logger
}

func newOpenAIProvider(baseURL, model, apiKey string, timeout time.Duration, log *slog.Logger) *openAIProvider {
	return &openAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function openAIToolCallFunction `json:"function"`
}

type openAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIRequest struct {
	Model          string            `json:"model"`
	Messages       []openAIMessage   `json:"messages"`
	Tools          []openAITool      `json:"tools,omitempty"`
	Stream         bool              `json:"stream"`
	ResponseFormat *openAIRespFormat `json:"response_format,omitempty"`
}

type openAIRespFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Vision-запрос: content — массив текста и картинок (data-URL).
type openAIVisionRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIVisionMessage `json:"messages"`
	Stream         bool                  `json:"stream"`
	ResponseFormat *openAIRespFormat     `json:"response_format,omitempty"`
}

type openAIVisionMessage struct {
	Role    string                `json:"role"`
	Content []openAIVisionContent `json:"content"`
}

type openAIVisionContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

// Chat реализует Provider.
func (p *openAIProvider) Chat(ctx context.Context, req Request) (Message, error) {
	start := time.Now()

	resp, err := p.do(ctx, openAIRequest{
		Model:    p.model,
		Messages: toOpenAIMessages(req.Messages),
		Tools:    toOpenAITools(req.Tools),
	})
	if err != nil {
		return Message{}, err
	}
	if len(resp.Choices) == 0 {
		return Message{}, fmt.Errorf("api: пустой ответ модели")
	}

	msg := fromOpenAIMessage(resp.Choices[0].Message)
	if resp.Usage.TotalTokens > 0 {
		msg.Usage = &Usage{Prompt: resp.Usage.PromptTokens, Completion: resp.Usage.CompletionTokens, Total: resp.Usage.TotalTokens}
	}
	p.log.Info(logging.Msg("llm:api", "ответ модели"),
		"model", p.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"tool_calls", len(msg.ToolCalls),
		"content_chars", len(msg.Content))
	return msg, nil
}

// Extract реализует Provider (response_format=json_object + инструкция).
func (p *openAIProvider) Extract(ctx context.Context, text string, schema json.RawMessage) (json.RawMessage, error) {
	start := time.Now()

	prompt := fmt.Sprintf("Верни только валидный JSON по схеме, без пояснений:\n%s\n\nДанные:\n%s", schema, text)
	resp, err := p.do(ctx, openAIRequest{
		Model:          p.model,
		Messages:       []openAIMessage{{Role: string(RoleUser), Content: prompt}},
		ResponseFormat: &openAIRespFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("api: пустой ответ Extract")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if !json.Valid([]byte(content)) {
		return nil, fmt.Errorf("api: ответ Extract не является JSON")
	}

	p.log.Info(logging.Msg("llm:api", "JSON извлечён"),
		"model", p.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"content_chars", len(content))
	return json.RawMessage(content), nil
}

func (p *openAIProvider) do(ctx context.Context, req openAIRequest) (*openAIResponse, error) {
	req.Stream = false
	data, err := p.post(ctx, req)
	if err != nil {
		return nil, err
	}

	var out openAIResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("api: разобрать ответ: %w", err)
	}
	return &out, nil
}

// ExtractVision реализует Provider (FR-6.8.2): data-URL + response_format.
func (p *openAIProvider) ExtractVision(ctx context.Context, prompt string, _ json.RawMessage, images [][]byte) (json.RawMessage, error) {
	start := time.Now()

	content := make([]openAIVisionContent, 0, len(images)+1)
	content = append(content, openAIVisionContent{Type: "text", Text: prompt})
	for _, image := range images {
		mime := http.DetectContentType(image)
		if !strings.HasPrefix(mime, "image/") {
			mime = "image/jpeg"
		}
		content = append(content, openAIVisionContent{
			Type: "image_url",
			ImageURL: &openAIImageURL{
				URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image),
			},
		})
	}

	data, err := p.post(ctx, openAIVisionRequest{
		Model:          p.model,
		Messages:       []openAIVisionMessage{{Role: string(RoleUser), Content: content}},
		ResponseFormat: &openAIRespFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, err
	}

	var out openAIResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("api: разобрать vision-ответ: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("api: пустой vision-ответ")
	}

	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("api: vision-ответ не является JSON")
	}

	p.log.Info(logging.Msg("llm:api", "vision-ответ"),
		"model", p.model,
		"duration_ms", time.Since(start).Milliseconds(),
		"images", len(images),
		"content_chars", len(text))
	return json.RawMessage(text), nil
}

func (p *openAIProvider) post(ctx context.Context, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("api: сериализовать запрос: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("api: создать запрос: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		p.log.Error(logging.Msg("llm:api", "ошибка запроса к модели"), "model", p.model, "error", err)
		return nil, fmt.Errorf("api: запрос /chat/completions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("api: прочитать ответ: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: HTTP %d: %s", resp.StatusCode, trimBody(data))
	}
	return data, nil
}

func toOpenAIMessages(messages []Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages))
	for _, msg := range messages {
		item := openAIMessage{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ToolCalls) > 0 {
			item.ToolCalls = make([]openAIToolCall, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				args := "{}"
				if len(call.Arguments) > 0 {
					args = string(call.Arguments)
				}
				item.ToolCalls = append(item.ToolCalls, openAIToolCall{
					ID:   call.ID,
					Type: "function",
					Function: openAIToolCallFunction{
						Name:      call.Name,
						Arguments: args,
					},
				})
			}
		}
		out = append(out, item)
	}
	return out
}

func toOpenAITools(tools []Tool) []openAITool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, openAITool{
			Type:     "function",
			Function: openAIToolFunction(tool),
		})
	}
	return out
}

func fromOpenAIMessage(msg openAIMessage) Message {
	out := Message{
		Role:    Role(msg.Role),
		Content: msg.Content,
	}
	if len(msg.ToolCalls) > 0 {
		out.ToolCalls = make([]ToolCall, 0, len(msg.ToolCalls))
		for _, call := range msg.ToolCalls {
			args := call.Function.Arguments
			if !json.Valid([]byte(args)) {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: json.RawMessage(args),
			})
		}
	}
	return out
}
