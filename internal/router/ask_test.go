package router_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

type fakeAskHandler struct {
	answer string
	err    error
	text   string
	calls  int
}

func (f *fakeAskHandler) Ask(_ context.Context, text string) (string, error) {
	f.calls++
	f.text = text
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

func TestRouterAskCommand(t *testing.T) {
	sender := &fakeSender{}
	ask := &fakeAskHandler{answer: "Плато — это нормально. Источник: методики/тренды."}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithAsk(ask))

	update := helpUpdate(1)
	update.Message.Text = "/ask почему вес встал?"
	r.HandleUpdate(context.Background(), update)

	require.Equal(t, 1, ask.calls)
	assert.Equal(t, "почему вес встал?", ask.text)
	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "Плато")
}

func TestRouterAskUsageAndError(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithAsk(&fakeAskHandler{}))
	update := helpUpdate(1)
	update.Message.Text = "/ask"
	r.HandleUpdate(context.Background(), update)
	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "Напиши вопрос")

	sender2 := &fakeSender{}
	r2 := router.New(sender2, discardLogger(), router.Allowlist{}, router.WithAsk(&fakeAskHandler{err: errors.New("down")}))
	update.Message.Text = "/ask что случилось?"
	r2.HandleUpdate(context.Background(), update)
	require.Len(t, sender2.sent(), 1)
	assert.Contains(t, sender2.sent()[0], "Не получилось")

	sender3 := &fakeSender{}
	ask3 := &fakeAskHandler{answer: "не должно быть"}
	r3 := router.New(sender3, discardLogger(), router.Allowlist{}, router.WithAsk(ask3))
	update.Message.Text = "/ask ываыва"
	r3.HandleUpdate(context.Background(), update)
	assert.Zero(t, ask3.calls)
	assert.Contains(t, sender3.sent()[0], "Не понял вопрос")
}
