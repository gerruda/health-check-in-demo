package intake_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/intake"
)

func TestServiceSendsTypingWhileProcessing(t *testing.T) {
	provider := &fakeProvider{textResponse: json.RawMessage(`{
		"kinds": ["checkin"],
		"checkin": {"sleep": "7:33", "hrv_ms": 38, "rhr": 58, "mood": 7}
	}`)}
	replies := make(chan string, 4)
	var typingCalls atomic.Int64

	extractor := intake.NewExtractor(provider, provider, discardLogger())
	send := func(_ context.Context, _ int64, text string) error {
		replies <- text
		return nil
	}
	svc := intake.NewService(extractor, &fakeDownloader{}, send, discardLogger(),
		intake.WithDebounce(10*time.Millisecond),
		intake.WithTyping(func(context.Context, int64) error {
			typingCalls.Add(1)
			return nil
		}),
	)

	// Текст без цифр → LLM-путь (правила не срабатывают), обработка дольше.
	svc.SubmitText(context.Background(), 1, "самочувствие странное, не выспался")
	waitReply(t, replies)

	assert.GreaterOrEqual(t, typingCalls.Load(), int64(1), "во время обработки должен идти typing")
}
