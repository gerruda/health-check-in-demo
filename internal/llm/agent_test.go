package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/llm"
)

type fakeProvider struct {
	responses []llm.Message
	requests  []llm.Request
	err       error
}

func (f *fakeProvider) Chat(_ context.Context, req llm.Request) (llm.Message, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return llm.Message{}, f.err
	}
	if len(f.responses) == 0 {
		return llm.Message{Role: llm.RoleAssistant, Content: "пусто"}, nil
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

func (f *fakeProvider) Extract(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("не используется в тесте")
}

func (f *fakeProvider) ExtractVision(context.Context, string, json.RawMessage, [][]byte) (json.RawMessage, error) {
	return nil, errors.New("не используется в тесте")
}

type fakeExecutor struct {
	calls []string
	args  []json.RawMessage
}

func (f *fakeExecutor) Execute(_ context.Context, name string, args json.RawMessage) (string, error) {
	f.calls = append(f.calls, name)
	f.args = append(f.args, args)
	return "HRV: 34 мс", nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAgentExecutesTool(t *testing.T) {
	provider := &fakeProvider{responses: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:        "call_1",
				Name:      "read_last_checkin",
				Arguments: json.RawMessage(`{"person":"athlete"}`),
			}},
		},
		{Role: llm.RoleAssistant, Content: "В последнем чек-ине HRV 34 мс."},
	}}
	executor := &fakeExecutor{}

	agent := llm.NewAgent(provider, []llm.Tool{{Name: "read_last_checkin"}}, executor, discardLogger())
	answer, err := agent.Ask(context.Background(), "какой HRV в последнем чек-ине?")
	require.NoError(t, err)
	assert.Equal(t, "В последнем чек-ине HRV 34 мс.", answer)

	require.Len(t, executor.calls, 1)
	assert.Equal(t, "read_last_checkin", executor.calls[0])

	require.Len(t, provider.requests, 2, "агент должен сделать повторный запрос с результатом инструмента")
	second := provider.requests[1].Messages
	require.NotEmpty(t, second)
	last := second[len(second)-1]
	assert.Equal(t, llm.RoleTool, last.Role)
	assert.Equal(t, "HRV: 34 мс", last.Content)
}

func TestAgentMaxSteps(t *testing.T) {
	toolCall := llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{
			Name:      "read_last_checkin",
			Arguments: json.RawMessage(`{}`),
		}},
	}
	provider := &fakeProvider{responses: []llm.Message{toolCall, toolCall, toolCall, toolCall, toolCall, toolCall}}

	agent := llm.NewAgent(provider, nil, &fakeExecutor{}, discardLogger())
	_, err := agent.Ask(context.Background(), "вопрос")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "шаг")
	assert.Len(t, provider.requests, 4, "лимит tool-loop — 4 шага")
}

func TestAgentProviderError(t *testing.T) {
	provider := &fakeProvider{err: errors.New("модель недоступна")}

	agent := llm.NewAgent(provider, nil, &fakeExecutor{}, discardLogger())
	_, err := agent.Ask(context.Background(), "вопрос")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "модель недоступна")
}
