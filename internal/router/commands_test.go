package router_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/router"
)

type fakeSettingsHandler struct {
	settingsReply string
	setReply      string
	resetReply    string
	err           error

	setChatID int64
	setKey    string
	setValue  string
	resetCall bool
}

func (f *fakeSettingsHandler) Settings(int64) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.settingsReply, nil
}

func (f *fakeSettingsHandler) Set(chatID int64, key, value string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.setChatID, f.setKey, f.setValue = chatID, key, value
	return f.setReply, nil
}

func (f *fakeSettingsHandler) Reset(int64) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.resetCall = true
	return f.resetReply, nil
}

type fakeStatusHandler struct {
	reply string
	err   error
	calls int
}

func (f *fakeStatusHandler) Status(context.Context, int64) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

func commandUpdate(text string) telego.Update {
	update := helpUpdate(1)
	update.Message.Text = text
	return update
}

func TestRouterSettingsCommand(t *testing.T) {
	sender := &fakeSender{}
	settingsHandler := &fakeSettingsHandler{settingsReply: "Настройки: ккал 2400*"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithSettings(settingsHandler))

	r.HandleUpdate(context.Background(), commandUpdate("/settings"))

	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "Настройки: ккал 2400*", sent[0])
}

func TestRouterSetCommand(t *testing.T) {
	sender := &fakeSender{}
	settingsHandler := &fakeSettingsHandler{setReply: "Отметил: kcal = 2400"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithSettings(settingsHandler))

	r.HandleUpdate(context.Background(), commandUpdate("/set kcal 2400"))

	assert.Equal(t, "kcal", settingsHandler.setKey)
	assert.Equal(t, "2400", settingsHandler.setValue)
	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "Отметил")
}

func TestRouterSetResetCommand(t *testing.T) {
	sender := &fakeSender{}
	settingsHandler := &fakeSettingsHandler{resetReply: "Сброшено"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithSettings(settingsHandler))

	r.HandleUpdate(context.Background(), commandUpdate("/set reset"))

	assert.True(t, settingsHandler.resetCall)
	require.Len(t, sender.sent(), 1)
}

func TestRouterSetWithoutArguments(t *testing.T) {
	sender := &fakeSender{}
	settingsHandler := &fakeSettingsHandler{}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithSettings(settingsHandler))

	r.HandleUpdate(context.Background(), commandUpdate("/set"))

	assert.Empty(t, settingsHandler.setKey, "хендлер не должен вызываться без аргументов")
	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "/set")
}

func TestRouterStatusCommand(t *testing.T) {
	sender := &fakeSender{}
	statusHandler := &fakeStatusHandler{reply: "HRV_7: 33.0 мс"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithStatus(statusHandler))

	r.HandleUpdate(context.Background(), commandUpdate("/status"))

	assert.Equal(t, 1, statusHandler.calls)
	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "HRV_7")
}

func TestRouterCommandHandlerError(t *testing.T) {
	sender := &fakeSender{}
	settingsHandler := &fakeSettingsHandler{err: errors.New("settings: kcal=900 вне диапазона 1200..4000")}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithSettings(settingsHandler))

	r.HandleUpdate(context.Background(), commandUpdate("/set kcal 900"))

	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "диапазона")
	assert.NotContains(t, sender.sent()[0], "settings:")
}

func TestRouterHelpListsCommands(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	r.HandleUpdate(context.Background(), commandUpdate("/help"))

	require.Len(t, sender.sent(), 1)
	assert.Contains(t, sender.sent()[0], "/set")
	assert.Contains(t, sender.sent()[0], "/status")
}

func TestRouterSetRussianWeeklyAliases(t *testing.T) {
	sender := &fakeSender{}
	handler := &fakeSettingsHandler{setReply: "Отметил"}
	r := router.New(sender, discardLogger(), router.Allowlist{}, router.WithSettings(handler))

	r.HandleUpdate(context.Background(), commandUpdate("/set Воскресный разбор выкл"))
	assert.Equal(t, "Воскресный разбор", handler.setKey)
	assert.Equal(t, "выкл", handler.setValue)

	r.HandleUpdate(context.Background(), commandUpdate("/set Время разбора 10:30"))
	assert.Equal(t, "Время разбора", handler.setKey)
	assert.Equal(t, "10:30", handler.setValue)
}
