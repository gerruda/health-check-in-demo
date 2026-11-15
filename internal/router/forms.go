package router

import (
	"context"
)

func (r *Router) handleFormCommand(ctx context.Context, chatID int64, form string) {
	if r.intake == nil {
		r.reply(ctx, chatID, "Формы недоступны в этой сборке. Пришли данные свободным текстом.")
		return
	}
	if !r.intake.StartForm(ctx, chatID, form) {
		r.reply(ctx, chatID, "Не знаю такую форму. Доступно: /checkin, /training, /food, /measure.")
	}
}

func (r *Router) handleCancelCommand(ctx context.Context, chatID int64) {
	if r.intake == nil || !r.intake.CancelForm(ctx, chatID) {
		r.reply(ctx, chatID, "Нет активной формы. Можно писать свободным текстом.")
	}
}
