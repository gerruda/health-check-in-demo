package app

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTelegoLoggerMasksToken(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	logger := newTelegoLogger(log, "123456:secret-token")

	logger.Errorf(`Execution error getUpdates: Post "https://api.telegram.org/bot123456:secret-token/getUpdates": boom`)

	out := buf.String()
	assert.NotContains(t, out, "123456:secret-token", "токен не должен попадать в лог")
	assert.Contains(t, out, "BOT_TOKEN", "токен должен маскироваться как BOT_TOKEN")
	assert.Contains(t, out, "[telego:api]", "сообщение должно иметь префикс пакета")
	assert.Contains(t, out, "ошибка Telegram API")
}

func TestTelegoLoggerContextCanceledIsNotError(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger := newTelegoLogger(log, "123456:secret-token")

	logger.Errorf("Getting updates: getUpdates: context canceled")

	assert.Empty(t, buf.String(), "штатная отмена long polling не должна писаться как ошибка")
}

func TestTelegoLoggerEmptyTokenDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	logger := newTelegoLogger(log, "")

	assert.NotPanics(t, func() {
		logger.Debugf("debug message")
		logger.Errorf("ошибка")
	})
}
