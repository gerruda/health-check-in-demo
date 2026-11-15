package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"healthbot/internal/logging"
	"healthbot/internal/persona"
)

// defaultMaxSteps — лимит шагов tool-loop (FR-6.7.2).
const defaultMaxSteps = 4

// SystemPrompt — голос и границы «Ритма» (FR-8); текст живёт в пакете persona.
var SystemPrompt = persona.SystemPrompt

// ToolExecutor выполняет инструмент по имени и аргументам (JSON).
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args json.RawMessage) (string, error)
}

// ContextProvider отдаёт компактный контекст из базы знаний по вопросу.
// Вызывается до LLM, чтобы поиск не зависел от того, захочет ли модель tool.
type ContextProvider func(ctx context.Context, userText string) string

// Agent — диалог с LLM с поддержкой инструментов (tool-loop).
type Agent struct {
	provider        Provider
	tools           []Tool
	executor        ToolExecutor
	log             *slog.Logger
	maxSteps        int
	contextProvider ContextProvider
}

// NewAgent создаёт агента. nil-логгер заменяется на slog.Default.
func NewAgent(provider Provider, tools []Tool, executor ToolExecutor, log *slog.Logger) *Agent {
	if log == nil {
		log = slog.Default()
	}
	return &Agent{
		provider: provider,
		tools:    tools,
		executor: executor,
		log:      log,
		maxSteps: defaultMaxSteps,
	}
}

var (
	doiPattern  = regexp.MustCompile(`(?i)\b10\.\d{4,9}/[^\s)\]]+`)
	linkPattern = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	urlPattern  = regexp.MustCompile(`https?://\S+`)
	pmidPattern = regexp.MustCompile(`(?i)PMID\s*[:#]?\s*(\d{7,9})`)
)

// sanitizeKnowledgeAnswer убирает выдуманные ссылки/DOI и оставляет PMID,
// которые реально были в найденном контексте. Затем добавляет источники.
func sanitizeKnowledgeAnswer(answer, context string) string {
	allowedPMIDs := make(map[string]struct{})
	for _, match := range pmidPattern.FindAllStringSubmatch(context, -1) {
		allowedPMIDs[match[1]] = struct{}{}
	}

	lines := strings.Split(answer, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = linkPattern.ReplaceAllString(line, "$1")
		line = urlPattern.ReplaceAllString(line, "")
		line = doiPattern.ReplaceAllString(line, "")
		line = pmidPattern.ReplaceAllStringFunc(line, func(match string) string {
			for pmid := range allowedPMIDs {
				if strings.Contains(match, pmid) {
					return "PMID: " + pmid
				}
			}
			return ""
		})
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" && (len(cleaned) == 0 || cleaned[len(cleaned)-1] == "") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	answer = strings.TrimSpace(strings.Join(cleaned, "\n"))

	sources := make([]string, 0, 2)
	for _, line := range strings.Split(context, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		sources = append(sources, strings.TrimPrefix(line, "- "))
		if len(sources) == 2 {
			break
		}
	}
	if len(sources) > 0 && !strings.Contains(answer, "Источники:") {
		answer += "\n\nИсточники: " + strings.Join(sources, "; ")
	}
	return strings.TrimSpace(answer)
}

func withoutTool(items []Tool, name string) []Tool {
	result := make([]Tool, 0, len(items))
	for _, item := range items {
		if item.Name == name {
			continue
		}
		result = append(result, item)
	}
	return result
}

// WithContextProvider подключает предварительный поиск по базе знаний.
func (a *Agent) WithContextProvider(provider ContextProvider) *Agent {
	a.contextProvider = provider
	return a
}

// Ask отправляет вопрос модели, выполняет запрошенные инструменты и возвращает
// финальный текстовый ответ (FR-6.7).
func (a *Agent) Ask(ctx context.Context, userText string) (string, error) {
	systemPrompt := SystemPrompt
	contextUsed := false
	extraContext := ""
	if a.contextProvider != nil {
		if extra := strings.TrimSpace(a.contextProvider(ctx, userText)); extra != "" {
			contextUsed = true
			extraContext = extra
			systemPrompt += "\n\nКОНТЕКСТ ИЗ БАЗЫ ЗНАНИЙ (найден автоматически, используй его как основу ответа; не выдумывай PMID):\n" + extra
		}
	}
	messages := []Message{
		{Role: RoleSystem, Content: systemPrompt},
		{Role: RoleUser, Content: userText},
	}
	tools := a.tools
	if contextUsed {
		// контекст уже найден — search_knowledge не нужен; остальные инструменты
		// (история) оставляем, но при пустом ответе повторим без инструментов
		tools = withoutTool(tools, "search_knowledge")
	}

	for step := 1; step <= a.maxSteps; step++ {
		msg, err := a.provider.Chat(ctx, Request{Messages: messages, Tools: tools})
		if err != nil {
			return "", fmt.Errorf("вызов модели: %w", err)
		}

		if len(msg.ToolCalls) == 0 {
			answer := strings.TrimSpace(msg.Content)
			if answer == "" && contextUsed && len(tools) > 0 {
				// 3B иногда отдаёт пустой content при наличии tools — повторяем
				// без инструментов, опираясь на уже найденный контекст
				retry, retryErr := a.provider.Chat(ctx, Request{Messages: messages})
				if retryErr == nil {
					answer = strings.TrimSpace(retry.Content)
				}
			}
			if answer == "" {
				return "", errors.New("модель вернула пустой ответ")
			}
			if contextUsed {
				answer = sanitizeKnowledgeAnswer(answer, extraContext)
			}
			return answer, nil
		}

		messages = append(messages, msg)
		for _, call := range msg.ToolCalls {
			result, execErr := a.executor.Execute(ctx, call.Name, call.Arguments)
			if execErr != nil {
				a.log.Warn(logging.Msg("llm:agent", "инструмент вернул ошибку"),
					"tool", call.Name, "error", execErr)
				result = "ошибка инструмента: " + execErr.Error()
			}
			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    result,
				ToolName:   call.Name,
				ToolCallID: call.ID,
			})
		}
	}

	return "", fmt.Errorf("превышен лимит шагов инструментов (%d)", a.maxSteps)
}
