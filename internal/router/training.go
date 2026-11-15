package router

import "context"

func (r *Router) handleTrainingCommand(ctx context.Context, chatID int64, text string) {
	if r.intake == nil {
		r.reply(ctx, chatID, "Приём тренировок недоступен в этой сборке.")
		return
	}
	fields := splitCommandArgs(text)
	if len(fields) == 0 {
		r.reply(ctx, chatID, "Формат: /training силовая A: присед 60x5x5, жим 40x8, RPE 7")
		return
	}
	r.intake.SubmitTraining(ctx, chatID, joinCommandArgs(fields))
}
