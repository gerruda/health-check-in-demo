// Package logging создаёт slog-логгер по NFR-3: JSON в файл <dir>/healthbot.log
// с дублированием в stderr. Сообщения — в формате «[пакет:метод] суть на
// русском» (см. Msg), секреты (токен бота, ключи API) маскируются во всех
// записях, включая вложенные атрибуты и текст ошибок.
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// secretMask — маска для секретов в логах (NFR-1).
const secretMask = "***"

// Msg собирает сообщение лога в формате «[пакет:метод] суть на русском»
// (NFR-3): по префиксу сразу видно пакет и метод-источник записи.
func Msg(pkgMethod, text string) string {
	return "[" + pkgMethod + "] " + text
}

// New создаёт логгер, пишущий JSON в <dir>/healthbot.log и в stderr.
// secrets — значения, которые нельзя писать в логи (токен бота, ключи API):
// заменяются маской во всех сообщениях и атрибутах, включая вложенные группы.
// Возвращает логгер и io.Closer для закрытия файла.
func New(dir string, secrets ...string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, nil, fmt.Errorf("создать каталог логов %q: %w", dir, err)
	}

	path := filepath.Join(dir, "healthbot.log")
	//nolint:gosec // путь задаётся оператором через LOG_DIR
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("открыть лог-файл %q: %w", path, err)
	}

	jsonHandler := slog.NewJSONHandler(io.MultiWriter(file, os.Stderr), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(newRedactHandler(jsonHandler, secrets)), file, nil
}

// Sanitize заменяет секреты в тексте маской. Нужен для ошибок, которые уходят
// в stderr или чат: ошибки HTTP-клиента telego содержат URL с токеном (NFR-1).
func Sanitize(text string, secrets ...string) string {
	pairs := make([]string, 0, 2*len(secrets))
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		pairs = append(pairs, secret, secretMask)
	}
	if len(pairs) == 0 {
		return text
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// SanitizeError возвращает ошибку с замаскированными секретами в тексте.
func SanitizeError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	return errors.New(Sanitize(err.Error(), secrets...))
}

// redactHandler — обёртка slog.Handler, заменяющая секреты во всех записях.
type redactHandler struct {
	inner    slog.Handler
	replacer *strings.Replacer
}

func newRedactHandler(inner slog.Handler, secrets []string) slog.Handler {
	pairs := make([]string, 0, 2*len(secrets))
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		pairs = append(pairs, secret, secretMask)
	}
	if len(pairs) == 0 {
		return inner
	}
	return &redactHandler{inner: inner, replacer: strings.NewReplacer(pairs...)}
}

// Enabled реализует slog.Handler.
func (h *redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle реализует slog.Handler: маскирует сообщение и атрибуты записи.
func (h *redactHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, h.redact(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(h.redactAttr(attr))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

// WithAttrs реализует slog.Handler.
func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		clean = append(clean, h.redactAttr(attr))
	}
	return &redactHandler{inner: h.inner.WithAttrs(clean), replacer: h.replacer}
}

// WithGroup реализует slog.Handler.
func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: h.inner.WithGroup(name), replacer: h.replacer}
}

func (h *redactHandler) redact(text string) string {
	return h.replacer.Replace(text)
}

func (h *redactHandler) redactAttr(attr slog.Attr) slog.Attr {
	switch attr.Value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(h.redact(attr.Value.String()))
	case slog.KindGroup:
		group := attr.Value.Group()
		clean := make([]slog.Attr, 0, len(group))
		for _, nested := range group {
			clean = append(clean, h.redactAttr(nested))
		}
		attr.Value = slog.GroupValue(clean...)
	case slog.KindAny:
		if err, ok := attr.Value.Any().(error); ok {
			attr.Value = slog.StringValue(h.redact(err.Error()))
		}
	}
	return attr
}
