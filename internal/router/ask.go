package router

import (
	"context"
	"strings"
	"time"

	"healthbot/internal/logging"
	"healthbot/internal/textutil"
	"healthbot/internal/typing"
)

// AskHandler — команда /ask: ответ по базе знаний (FR-4).
type AskHandler interface {
	Ask(ctx context.Context, text string) (string, error)
}

// WithAsk подключает обработчик /ask.
func WithAsk(handler AskHandler) Option {
	return func(r *Router) {
		r.ask = handler
	}
}

// askTimeout — отдельный увеличенный бюджет для RAG-ответа (модель + контекст).
const askTimeout = 60 * time.Second

func (r *Router) handleAskCommand(ctx context.Context, chatID int64, message string) {
	if r.ask == nil {
		r.reply(ctx, chatID, "Поиск по базе знаний недоступен в этой сборке.")
		return
	}
	fields := strings.Fields(message)
	question := ""
	if len(fields) > 1 {
		question = strings.Join(fields[1:], " ")
	}
	if question == "" {
		r.reply(ctx, chatID, "Напиши вопрос после команды: /ask что делать при плато веса?")
		return
	}
	if !textutil.LooksLikeQuestion(question) {
		r.reply(ctx, chatID, "Не понял вопрос. Напиши его словами — например: /ask какие плюсы и минусы креатинина?")
		return
	}

	askCtx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	if r.typing != nil {
		stopTyping := typing.Start(askCtx, chatID, r.typing)
		defer stopTyping()
	}

	answer, err := r.ask.Ask(askCtx, question)
	if err != nil {
		r.log.Error(logging.Msg("router:handleAskCommand", "ошибка поиска по базе знаний"),
			"chat_id", chatID, "error", err)
		r.reply(context.WithoutCancel(askCtx), chatID, "Не получилось найти ответ. Попробуй ещё раз чуть позже.")
		return
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		r.reply(context.WithoutCancel(askCtx), chatID, "По этому вопросу в базе знаний пока нет ответа.")
		return
	}
	r.reply(context.WithoutCancel(askCtx), chatID, answer)
}
