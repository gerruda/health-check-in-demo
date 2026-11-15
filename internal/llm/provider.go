// Package llm реализует LLM-провайдеры (FR-6) и агентский цикл с
// инструментами: Ollama (локально) и OpenAI-совместимый API пользователя.
// Провайдеры не знают ни про Telegram, ни про файловую базу — только сеть LLM.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"healthbot/internal/logging"
	"healthbot/internal/metrics"
)

// Режимы активного провайдера (FR-6).
const (
	ModeOllama = "ollama"
	ModeAPI    = "api"
)

// defaultTimeout — бюджет вызова LLM (FR-6.6).
const defaultTimeout = 20 * time.Second

// Role — роль сообщения в диалоге.
type Role string

// Роли сообщений.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall — запрос модели на вызов инструмента. Arguments — JSON-объект
// аргументов (у OpenAI в проводе это строка JSON, у Ollama — объект).
type ToolCall struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Usage — расход токенов LLM-вызова (если провайдер его сообщает).
type Usage struct {
	Prompt     int
	Completion int
	Total      int
}

// Message — сообщение диалога (единый формат для всех провайдеров).
type Message struct {
	Role      Role       `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolName заполняется для результата инструмента (Ollama).
	ToolName string `json:"tool_name,omitempty"`
	// ToolCallID заполняется для результата инструмента (OpenAI).
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Usage заполнен для ответов чата, если провайдер вернул статистику токенов.
	Usage *Usage `json:"-"`
}

// Tool — описание инструмента, доступного модели (JSON-schema аргументов).
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// Request — запрос к модели.
type Request struct {
	Messages []Message
	Tools    []Tool
}

// Provider — единый интерфейс LLM-провайдера (FR-6.1).
type Provider interface {
	// Chat выполняет диалог с моделью (с инструментами, если заданы).
	Chat(ctx context.Context, req Request) (Message, error)
	// Extract извлекает структурированные данные по JSON-schema (FR-6.1).
	Extract(ctx context.Context, text string, schema json.RawMessage) (json.RawMessage, error)
	// ExtractVision извлекает структурированные данные по изображениям (FR-6.8.2).
	ExtractVision(ctx context.Context, prompt string, schema json.RawMessage, images [][]byte) (json.RawMessage, error)
}

// Options — настройки провайдера из конфигурации бота.
type Options struct {
	Mode        string // ollama | api
	OllamaURL   string
	OllamaModel string
	APIURL      string
	APIKey      string
	Model       string
	Fallback    string // второй провайдер при сбое активного (FR-6.4)
	Timeout     time.Duration
	Logger      *slog.Logger

	// Vision — отдельный провайдер для скриншотов (FR-6.8.1).
	VisionMode   string
	VisionModel  string
	VisionAPIURL string
	VisionAPIKey string
}

// FromOptions создаёт провайдер по настройкам (с fallback-цепочкой FR-6.4).
func FromOptions(opts Options) (Provider, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}

	primary, err := newProvider(opts.Mode, opts)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.Fallback) == "" {
		return primary, nil
	}
	if opts.Fallback == opts.Mode {
		return nil, fmt.Errorf("LLM_FALLBACK совпадает с активным режимом %q", opts.Mode)
	}

	secondary, err := newProvider(opts.Fallback, opts)
	if err != nil {
		return nil, fmt.Errorf("резервный провайдер (LLM_FALLBACK): %w", err)
	}
	return &fallbackProvider{
		primary:       primary,
		secondary:     secondary,
		primaryName:   opts.Mode,
		secondaryName: opts.Fallback,
		log:           opts.Logger,
	}, nil
}

// VisionFromOptions создаёт провайдер для скриншотов (FR-6.8.1): режим и
// модель могут отличаться от чата (чат ollama + vision api и наоборот).
func VisionFromOptions(opts Options) (Provider, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}

	mode := strings.TrimSpace(opts.VisionMode)
	if mode == "" {
		mode = opts.Mode
	}

	switch mode {
	case ModeOllama:
		model := strings.TrimSpace(opts.VisionModel)
		if model == "" {
			model = strings.TrimSpace(opts.OllamaModel)
		}
		if model == "" {
			model = strings.TrimSpace(opts.Model)
		}
		if strings.TrimSpace(opts.OllamaURL) == "" {
			return nil, errors.New("vision: не задан OLLAMA_URL")
		}
		if model == "" {
			return nil, errors.New("vision: не задана модель (LLM_VISION_MODEL)")
		}
		return newOllamaProvider(opts.OllamaURL, model, opts.Timeout, opts.Logger), nil
	case ModeAPI:
		apiURL := strings.TrimSpace(opts.VisionAPIURL)
		if apiURL == "" {
			apiURL = opts.APIURL
		}
		apiKey := strings.TrimSpace(opts.VisionAPIKey)
		if apiKey == "" {
			apiKey = opts.APIKey
		}
		model := strings.TrimSpace(opts.VisionModel)
		if model == "" {
			model = strings.TrimSpace(opts.Model)
		}
		if apiURL == "" {
			return nil, errors.New("vision: не задан LLM_VISION_API_URL (или LLM_API_URL)")
		}
		if model == "" {
			return nil, errors.New("vision: не задана LLM_VISION_MODEL")
		}
		return newOpenAIProvider(apiURL, model, apiKey, opts.Timeout, opts.Logger), nil
	default:
		return nil, fmt.Errorf("vision: неизвестный режим %q (ожидается %q или %q)", mode, ModeOllama, ModeAPI)
	}
}

func newProvider(mode string, opts Options) (Provider, error) {
	switch mode {
	case ModeOllama:
		if strings.TrimSpace(opts.OllamaURL) == "" {
			return nil, errors.New("не задан OLLAMA_URL")
		}
		model := strings.TrimSpace(opts.OllamaModel)
		if model == "" {
			model = strings.TrimSpace(opts.Model)
		}
		if model == "" {
			return nil, errors.New("не задана модель Ollama (OLLAMA_MODEL)")
		}
		return newOllamaProvider(opts.OllamaURL, model, opts.Timeout, opts.Logger), nil
	case ModeAPI:
		if strings.TrimSpace(opts.APIURL) == "" {
			return nil, errors.New("не задан LLM_API_URL")
		}
		if strings.TrimSpace(opts.Model) == "" {
			return nil, errors.New("не задана модель API (LLM_MODEL)")
		}
		return newOpenAIProvider(opts.APIURL, opts.Model, opts.APIKey, opts.Timeout, opts.Logger), nil
	default:
		return nil, fmt.Errorf("неизвестный LLM_MODE %q (ожидается %q или %q)", mode, ModeOllama, ModeAPI)
	}
}

// fallbackProvider пробует активного провайдера, при ошибке — резервного (FR-6.4).
type fallbackProvider struct {
	primary       Provider
	secondary     Provider
	primaryName   string
	secondaryName string
	log           *slog.Logger
}

// Chat реализует Provider.
func (f *fallbackProvider) Chat(ctx context.Context, req Request) (Message, error) {
	msg, err := f.primary.Chat(ctx, req)
	if err == nil {
		return msg, nil
	}
	f.log.Warn(logging.Msg("llm:fallback", "активный провайдер недоступен, пробую резервный"),
		"provider", f.primaryName, "fallback", f.secondaryName, "error", err)
	metrics.Default().ObserveFallback(f.primaryName, f.secondaryName)
	return f.secondary.Chat(ctx, req)
}

// ExtractVision реализует Provider.
func (f *fallbackProvider) ExtractVision(ctx context.Context, prompt string, schema json.RawMessage, images [][]byte) (json.RawMessage, error) {
	raw, err := f.primary.ExtractVision(ctx, prompt, schema, images)
	if err == nil {
		return raw, nil
	}
	f.log.Warn(logging.Msg("llm:fallback", "vision не удался, пробую резервный провайдер"),
		"provider", f.primaryName, "fallback", f.secondaryName, "error", err)
	metrics.Default().ObserveFallback(f.primaryName, f.secondaryName)
	return f.secondary.ExtractVision(ctx, prompt, schema, images)
}

// Extract реализует Provider.
func (f *fallbackProvider) Extract(ctx context.Context, text string, schema json.RawMessage) (json.RawMessage, error) {
	raw, err := f.primary.Extract(ctx, text, schema)
	if err == nil {
		return raw, nil
	}
	f.log.Warn(logging.Msg("llm:fallback", "извлечение не удалось, пробую резервный провайдер"),
		"provider", f.primaryName, "fallback", f.secondaryName, "error", err)
	metrics.Default().ObserveFallback(f.primaryName, f.secondaryName)
	return f.secondary.Extract(ctx, text, schema)
}
