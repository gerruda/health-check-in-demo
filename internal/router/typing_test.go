package router_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/router"
)

func TestRouterChatSendsTyping(t *testing.T) {
	sender := &fakeSender{}
	chatter := &fakeChatter{answer: "ответ модели"}
	var typingCalls atomic.Int64

	r := router.New(sender, discardLogger(), router.Allowlist{},
		router.WithChat(chatter),
		router.WithTyping(func(context.Context, int64) error {
			typingCalls.Add(1)
			return nil
		}),
	)

	update := helpUpdate(1)
	update.Message.Text = "привет"
	r.HandleUpdate(context.Background(), update)

	assert.GreaterOrEqual(t, typingCalls.Load(), int64(1), "перед ответом LLM должен уходить typing")
	assert.Len(t, sender.sent(), 1)
}
