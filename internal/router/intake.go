package router

import "context"

// Intake — приём фото (FR-6.8) и текстовых чек-инов. Интерфейс объявлен в
// роутере, чтобы не зависеть от реализации (intake.Service).
type Intake interface {
	SubmitPhoto(ctx context.Context, chatID int64, mediaGroupID, fileID, caption string)
	SubmitText(ctx context.Context, chatID int64, text string)
	SubmitTraining(ctx context.Context, chatID int64, text string)
	LooksLikeCheckin(text string) bool
	LooksLikeNutrition(text string) bool
	LooksLikeTraining(text string) bool
	TryAnswerPending(ctx context.Context, chatID int64, text string) bool
	StartForm(ctx context.Context, chatID int64, form string) bool
	HandleFormAnswer(ctx context.Context, chatID int64, text string) bool
	CancelForm(ctx context.Context, chatID int64) bool
	HasActiveForm(chatID int64) bool
}

// WithIntake подключает приём скриншотов и текстовых чек-инов.
func WithIntake(intake Intake) Option {
	return func(r *Router) {
		r.intake = intake
	}
}
