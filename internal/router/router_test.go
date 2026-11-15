package router_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"healthbot/internal/router"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSender struct {
	mu      sync.Mutex
	texts   []string
	sendErr error
}

func (f *fakeSender) SendMessage(_ context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.texts = append(f.texts, params.Text)
	return &telego.Message{}, nil
}

func (f *fakeSender) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.texts...)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func helpUpdate(chatID int64) telego.Update {
	return telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			Chat: telego.Chat{ID: chatID},
			Text: "/help",
		},
	}
}

func TestHelpResponds(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	r.HandleUpdate(context.Background(), helpUpdate(42))

	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "/help")
}

func TestHelpWithBotUsername(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	update := helpUpdate(42)
	update.Message.Text = "/help@health_check_in_bot"
	r.HandleUpdate(context.Background(), update)

	assert.Len(t, sender.sent(), 1, "команда с @username должна обрабатываться")
}

func TestForeignChatIgnored(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{Athlete: 1, Wife: 2})

	r.HandleUpdate(context.Background(), helpUpdate(99))

	assert.Empty(t, sender.sent(), "чужой chat_id не должен получать ответ (ТЗ §2)")
}

func TestEmptyAllowlistAllowsEveryone(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	r.HandleUpdate(context.Background(), helpUpdate(123456))

	assert.Len(t, sender.sent(), 1)
}

func TestFreeTextIgnoredForNow(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	update := helpUpdate(1)
	update.Message.Text = "сон 7:33, HRV 38, пульс 58"
	r.HandleUpdate(context.Background(), update)

	assert.Empty(t, sender.sent(), "свободный текст обрабатывает intake на M1")
}

func TestNonMessageUpdateIgnored(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	assert.NotPanics(t, func() {
		r.HandleUpdate(context.Background(), telego.Update{UpdateID: 1})
	})
	assert.Empty(t, sender.sent())
}

func TestSendErrorDoesNotPanic(t *testing.T) {
	sender := &fakeSender{sendErr: errors.New("telegram недоступен")}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	assert.NotPanics(t, func() {
		r.HandleUpdate(context.Background(), helpUpdate(1))
	})
}

func TestCommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "простая команда", in: "/help", want: "help"},
		{name: "с username бота", in: "/help@health_check_in_bot", want: "help"},
		{name: "верхний регистр", in: "  /HELP  ", want: "help"},
		{name: "с аргументом", in: "/set kcal 2400", want: "set"},
		{name: "не команда", in: "сон 7:33", want: ""},
		{name: "пустая строка", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, router.Command(tt.in))
		})
	}
}

func TestUnknownCommandGetsHint(t *testing.T) {
	sender := &fakeSender{}
	r := router.New(sender, discardLogger(), router.Allowlist{})

	update := helpUpdate(1)
	update.Message.Text = "/bogus"
	r.HandleUpdate(context.Background(), update)

	sent := sender.sent()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0], "/help", "неизвестная команда должна подсказывать /help")
}

func TestLogsUsePackageMethodFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := router.New(&fakeSender{}, logger, router.Allowlist{Athlete: 1, Wife: 2})

	r.HandleUpdate(context.Background(), helpUpdate(99)) // чужой chat_id

	out := buf.String()
	assert.Contains(t, out, "[router:HandleUpdate]", "лог должен начинаться с [пакет:метод]")
	assert.Contains(t, out, "постороннего chat_id")
}
