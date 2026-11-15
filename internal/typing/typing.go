// Package typing отправляет в чат индикатор «печатает» (sendChatAction), пока
// бот обрабатывает запрос: Telegram держит статус ~5 секунд, поэтому action
// повторяется до завершения работы.
package typing

import (
	"context"
	"time"
)

// ActionFunc отправляет chat action (обычно typing) в чат.
type ActionFunc func(ctx context.Context, chatID int64) error

// defaultInterval меньше 5 с, чтобы индикатор не «мигал».
const defaultInterval = 4 * time.Second

type options struct {
	interval time.Duration
}

// Option настраивает индикатор (интервал для тестов).
type Option func(*options)

// WithInterval переопределяет интервал повторной отправки action.
func WithInterval(interval time.Duration) Option {
	return func(o *options) {
		if interval > 0 {
			o.interval = interval
		}
	}
}

// Start запускает отправку action: сразу и каждые interval до отмены ctx или
// вызова stop. nil-send — безопасная заглушка. Возвращает функцию остановки.
func Start(ctx context.Context, chatID int64, send ActionFunc, opts ...Option) func() {
	if send == nil {
		return func() {}
	}

	cfg := options{interval: defaultInterval}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, cancel := context.WithCancel(ctx)

	// Первый action — сразу (Telegram держит статус ~5 с), чтобы пользователь
	// увидел «печатает» до долгой обработки; далее — по тикеру.
	_ = send(ctx, chatID)

	go func() {
		ticker := time.NewTicker(cfg.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = send(ctx, chatID)
			}
		}
	}()
	return cancel
}
