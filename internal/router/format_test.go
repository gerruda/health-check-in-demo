package router_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

func TestRouterFormatsMarkdownAnswer(t *testing.T) {
	sender := &fakeSender{}
	chatter := &fakeChatter{answer: "**Итог**\n| Практика | Эффект |\n|---|---|\n| Сауна | давление |"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithChat(chatter))
	update := helpUpdate(1)
	update.Message.Text = "что помогает?"
	r.HandleUpdate(context.Background(), update)
	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.NotContains(t, sent[0], "|")
	assert.Contains(t, sent[0], "<b>Итог</b>")
	assert.Contains(t, sent[0], "• <b>Сауна</b>")
}
