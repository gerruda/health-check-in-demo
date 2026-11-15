package router

import (
	"context"
	"strings"
	"time"

	"healthbot/internal/logging"
	"healthbot/internal/typing"
)

// Chatter — LLM-чат для свободного текста (FR-6.7). Интерфейс объявлен здесь,
// чтобы пакет router не зависел от реализации LLM (домен не импортирует сеть).
type Chatter interface {
	Ask(ctx context.Context, text string) (string, error)
}

// defaultChatTimeout — бюджет ответа LLM в чате (FR-6.6).
const defaultChatTimeout = 45 * time.Second

// Option настраивает роутер.
type Option func(*Router)

// WithChat подключает LLM-чат: свободный текст от разрешённого chat_id
// обрабатывается моделью и ответ уходит в чат (FR-6.7.1).
func WithChat(chat Chatter) Option {
	return func(r *Router) {
		r.chat = chat
	}
}

// WithTyping подключает индикатор «печатает» на время обработки (UX).
func WithTyping(action typing.ActionFunc) Option {
	return func(r *Router) {
		r.typing = action
	}
}

// WithChatTimeout меняет таймаут ответа LLM (по умолчанию 20 с).
func WithChatTimeout(timeout time.Duration) Option {
	return func(r *Router) {
		if timeout > 0 {
			r.chatTimeout = timeout
		}
	}
}

// answerWithChat получает ответ модели и отправляет его в чат. Ошибка LLM не
// роняет обработку: пользователю уходит понятный fallback-текст (NFR-2).
// likelyGibberish отсекает повторяющийся набор букв («ываыва», «аааа»),
// чтобы не отправлять мусор в LLM и не получать случайные ответы/источники.
func likelyGibberish(text string) bool {
	value := strings.ToLower(strings.TrimSpace(text))
	if value == "" || strings.ContainsAny(value, "?!.") {
		return false
	}
	if len(strings.Fields(value)) != 1 {
		return false
	}
	runes := []rune(value)
	for _, r := range runes {
		if r < 'а' || r > 'я' && r != 'ё' {
			return false
		}
	}
	if len(runes) < 4 || len(runes) > 16 {
		return false
	}
	for size := 1; size <= len(runes)/2; size++ {
		if len(runes)%size != 0 {
			continue
		}
		repeated := true
		for index := size; index < len(runes); index++ {
			if runes[index] != runes[index%size] {
				repeated = false
				break
			}
		}
		if repeated {
			return true
		}
	}
	return false
}

func (r *Router) answerWithChat(ctx context.Context, chatID int64, text string) {
	chatCtx, cancel := context.WithTimeout(ctx, r.chatTimeout)
	defer cancel()

	if r.typing != nil {
		stopTyping := typing.Start(chatCtx, chatID, r.typing)
		defer stopTyping()
	}

	answer, err := r.chat.Ask(chatCtx, text)
	if err != nil {
		r.log.Error(logging.Msg("router:HandleUpdate", "ошибка LLM-чата"),
			"chat_id", chatID, "error", err)
		r.reply(context.WithoutCancel(chatCtx), chatID,
			"Не получилось получить ответ от модели. Попробуй ещё раз чуть позже.")
		return
	}

	answer = strings.TrimSpace(answer)
	if answer == "" {
		r.log.Warn(logging.Msg("router:HandleUpdate", "модель вернула пустой ответ"), "chat_id", chatID)
		r.reply(context.WithoutCancel(chatCtx), chatID,
			"Модель вернула пустой ответ. Попробуй переформулировать вопрос.")
		return
	}

	r.reply(context.WithoutCancel(chatCtx), chatID, answer)
}
