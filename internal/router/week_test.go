package router_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

type fakeWeekHandler struct {
	reply string
	err   error
	calls int
}

func (f *fakeWeekHandler) Week(context.Context, int64) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

func TestRouterWeekCommand(t *testing.T) {
	sender := &fakeSender{}
	week := &fakeWeekHandler{reply: "🌿 Неделя 06.11–12.11.2026"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithWeek(week))

	update := helpUpdate(1)
	update.Message.Text = "/week"
	r.HandleUpdate(context.Background(), update)

	assert.Equal(t, 1, week.calls)
	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "Неделя")
}

func TestRouterWeekCommandError(t *testing.T) {
	sender := &fakeSender{}
	week := &fakeWeekHandler{err: errors.New("store: файлы недоступны")}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithWeek(week))

	update := helpUpdate(1)
	update.Message.Text = "/week"
	r.HandleUpdate(context.Background(), update)

	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "Не удалось")
	assert.NotContains(t, sender.sent()[0], "store:")
}

func TestRouterHelpListsWeek(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	update := helpUpdate(1)
	update.Message.Text = "/help"
	r.HandleUpdate(context.Background(), update)

	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "/week")
}
