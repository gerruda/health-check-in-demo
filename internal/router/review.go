package router

import (
	"context"

	"healthbot/internal/logging"
)

// ReviewHandler — команда /review (воскресный разбор).
type ReviewHandler interface {
	Review(ctx context.Context, chatID int64) (string, error)
}

// WithReview подключает обработчик /review.
func WithReview(handler ReviewHandler) Option {
	return func(r *Router) {
		r.review = handler
	}
}

func (r *Router) handleReviewCommand(ctx context.Context, chatID int64) {
	if r.review == nil {
		r.reply(ctx, chatID, "Воскресный разбор недоступен в этой сборке.")
		return
	}
	text, err := r.review.Review(ctx, chatID)
	if err != nil {
		r.log.Error(logging.Msg("router:handleReviewCommand", "ошибка разбора"), "chat_id", chatID, "error", err)
		r.reply(ctx, chatID, "Не удалось собрать разбор: "+userFacingError(err))
		return
	}
	r.reply(ctx, chatID, text)
}
