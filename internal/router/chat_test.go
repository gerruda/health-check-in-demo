package router_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

type fakeChatter struct {
	answer string
	err    error
	texts  []string
}

func (f *fakeChatter) Ask(_ context.Context, text string) (string, error) {
	f.texts = append(f.texts, text)
	return f.answer, f.err
}

func TestRouterFreeTextUsesChat(t *testing.T) {
	sender := &fakeSender{}
	chatter := &fakeChatter{answer: "Ответ модели"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithChat(chatter))

	update := helpUpdate(1)
	update.Message.Text = "привет, как дела?"
	r.HandleUpdate(context.Background(), update)

	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "Ответ модели", sent[0])
	assert.Equal(t, []string{"привет, как дела?"}, chatter.texts)
}

func TestRouterChatErrorSendsFallback(t *testing.T) {
	sender := &fakeSender{}
	chatter := &fakeChatter{err: errors.New("модель недоступна")}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithChat(chatter))

	update := helpUpdate(1)
	update.Message.Text = "привет"
	r.HandleUpdate(context.Background(), update)

	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "модел", "при ошибке LLM должен уйти понятный fallback-текст")
	assert.NotContains(t, sent[0], "недоступна", "детали ошибки — только в логи")
}

func TestRouterEmptyChatAnswerSendsFallback(t *testing.T) {
	sender := &fakeSender{}
	chatter := &fakeChatter{answer: "   "}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithChat(chatter))

	update := helpUpdate(1)
	update.Message.Text = "привет"
	r.HandleUpdate(context.Background(), update)

	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.NotEmpty(t, sent[0])
}

func TestRouterGibberishDoesNotReachLLM(t *testing.T) {
	sender := &fakeSender{}
	chatter := &fakeChatter{answer: "не должно быть"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithChat(chatter))
	update := helpUpdate(1)
	update.Message.Text = "ываыва"
	r.HandleUpdate(context.Background(), update)
	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "Не понял")
}
