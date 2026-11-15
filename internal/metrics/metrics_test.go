package metrics_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/metrics"
)

func TestMetricsExposition(t *testing.T) {
	m := metrics.New()
	m.ObserveUpdate("status", "ok", 12*time.Millisecond)
	m.ObserveWebhookRequest("200")
	m.SetWebhookQueue(3)
	m.ObserveLLM("api", "qwen3", "chat", "ok", time.Second, &metrics.TokenUsage{Prompt: 10, Completion: 5, Total: 15})
	m.ObserveSave("checkin", "ok")
	m.IncError("llm")
	m.SetLastUpdate(time.Unix(100, 0))

	recorder := httptest.NewRecorder()
	m.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	require.Equal(t, 200, recorder.Code)
	for _, name := range []string{
		"healthbot_updates_total", "healthbot_update_duration_seconds",
		"healthbot_webhook_queue_length", "healthbot_webhook_requests_total",
		"healthbot_llm_requests_total", "healthbot_llm_tokens_total",
		"healthbot_saves_total", "healthbot_errors_total",
		"healthbot_last_update_timestamp_seconds", "go_goroutines",
	} {
		assert.Contains(t, body, name)
	}
	assert.Contains(t, body, `direction="total"`)
	assert.Contains(t, body, `command="status"`)
}
