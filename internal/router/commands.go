package router

import (
	"context"
	"strings"

	"healthbot/internal/logging"
)

// SettingsHandler — команды /settings и /set (FR-7.1/7.2). Интерфейс объявлен
// в роутере, реализация — в app.
type SettingsHandler interface {
	Settings(chatID int64) (string, error)
	Set(chatID int64, key, value string) (string, error)
	Reset(chatID int64) (string, error)
}

// StatusHandler — команда /status (HRV_7, зона, цели, тренировка).
type StatusHandler interface {
	Status(ctx context.Context, chatID int64) (string, error)
}

// WeekHandler — команда /week (сводка последних 7 дней).
type WeekHandler interface {
	Week(ctx context.Context, chatID int64) (string, error)
}

// WithSettings подключает обработчик /settings и /set.
func WithSettings(handler SettingsHandler) Option {
	return func(r *Router) {
		r.settings = handler
	}
}

// WithWeek подключает обработчик /week.
func WithWeek(handler WeekHandler) Option {
	return func(r *Router) {
		r.week = handler
	}
}

// WithStatus подключает обработчик /status.
func WithStatus(handler StatusHandler) Option {
	return func(r *Router) {
		r.status = handler
	}
}

func (r *Router) handleSettingsCommand(ctx context.Context, chatID int64) {
	if r.settings == nil {
		r.reply(ctx, chatID, "Настройки недоступны в этой сборке.")
		return
	}
	text, err := r.settings.Settings(chatID)
	if err != nil {
		r.log.Error(logging.Msg("router:handleSettingsCommand", "ошибка настроек"),
			"chat_id", chatID, "error", err)
		r.reply(ctx, chatID, "Не удалось показать настройки: "+userFacingError(err))
		return
	}
	r.reply(ctx, chatID, text)
}

func (r *Router) handleSetCommand(ctx context.Context, chatID int64, message string) {
	if r.settings == nil {
		r.reply(ctx, chatID, "Настройки недоступны в этой сборке.")
		return
	}

	fields := strings.Fields(message)
	if len(fields) < 2 {
		r.reply(ctx, chatID, "Формат: /set <ключ> <значение>, например /set kcal 2400. Список ключей — /settings. Сброс — /set reset.")
		return
	}
	if fields[1] == "reset" {
		text, err := r.settings.Reset(chatID)
		if err != nil {
			r.log.Error(logging.Msg("router:handleSetCommand", "ошибка сброса настроек"),
				"chat_id", chatID, "error", err)
			r.reply(ctx, chatID, "Не удалось сбросить настройки: "+userFacingError(err))
			return
		}
		r.reply(ctx, chatID, text)
		return
	}
	if len(fields) < 3 {
		r.reply(ctx, chatID, "Формат: /set <ключ> <значение>, например /set kcal 2400.")
		return
	}

	key := fields[1]
	value := strings.Join(fields[2:], " ")
	// русские составные алиасы: «Воскресный разбор выкл»
	if len(fields) >= 4 {
		lowerSecond := strings.ToLower(fields[2])
		if (strings.EqualFold(fields[1], "Воскресный") && (lowerSecond == "разбор" || lowerSecond == "разбора")) ||
			(strings.EqualFold(fields[1], "Время") && (lowerSecond == "разбора" || lowerSecond == "разбор")) {
			key = fields[1] + " " + fields[2]
			value = strings.Join(fields[3:], " ")
		}
	}
	text, err := r.settings.Set(chatID, key, value)
	if err != nil {
		r.log.Warn(logging.Msg("router:handleSetCommand", "настройка отклонена"),
			"chat_id", chatID, "key", key, "error", err)
		r.reply(ctx, chatID, userFacingError(err))
		return
	}
	r.reply(ctx, chatID, text)
}

func (r *Router) handleWeekCommand(ctx context.Context, chatID int64) {
	if r.week == nil {
		r.reply(ctx, chatID, "Сводка недели недоступна в этой сборке.")
		return
	}
	text, err := r.week.Week(ctx, chatID)
	if err != nil {
		r.log.Error(logging.Msg("router:handleWeekCommand", "ошибка сводки недели"),
			"chat_id", chatID, "error", err)
		r.reply(ctx, chatID, "Не удалось собрать сводку недели: "+userFacingError(err))
		return
	}
	r.reply(ctx, chatID, text)
}

func (r *Router) handleStatusCommand(ctx context.Context, chatID int64) {
	if r.status == nil {
		r.reply(ctx, chatID, "Статус недоступен в этой сборке.")
		return
	}
	text, err := r.status.Status(ctx, chatID)
	if err != nil {
		r.log.Error(logging.Msg("router:handleStatusCommand", "ошибка статуса"),
			"chat_id", chatID, "error", err)
		r.reply(ctx, chatID, "Не удалось собрать статус: "+userFacingError(err))
		return
	}
	r.reply(ctx, chatID, text)
}

func splitCommandArgs(text string) []string {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return nil
	}
	return fields[1:]
}

func joinCommandArgs(fields []string) string {
	return strings.Join(fields, " ")
}

// userFacingError убирает служебный префикс пакета из текста ошибки.
func userFacingError(err error) string {
	message := err.Error()
	for _, prefix := range []string{"settings: ", "store: ", "app: ", "intake: "} {
		message = strings.TrimPrefix(message, prefix)
	}
	return message
}
