package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/mymmrac/telego"

	"healthbot/internal/config"
	"healthbot/internal/logging"
	"healthbot/internal/scheduler"
	"healthbot/internal/telegramfmt"
)

// startSundayScheduler запускает проактивный воскресный разбор.
func startSundayScheduler(ctx context.Context, cfg config.Config, commands *commandHandlers, bot *telego.Bot, logger *slog.Logger) {
	if !cfg.SundayReviewEnabled {
		return
	}
	target, err := time.Parse("15:04", cfg.SundayReviewTime)
	if err != nil {
		logger.Warn(logging.Msg("app:startSundayScheduler", "не разобрать SUNDAY_REVIEW_TIME"), "error", err)
		return
	}
	location, err := time.LoadLocation(cfg.SundayReviewTZ)
	if err != nil {
		logger.Warn(logging.Msg("app:startSundayScheduler", "не загрузить SUNDAY_REVIEW_TZ"), "error", err)
		return
	}
	chats := make([]int64, 0, 2)
	if cfg.AthleteChatID != 0 {
		chats = append(chats, cfg.AthleteChatID)
	}
	if cfg.WifeChatID != 0 {
		chats = append(chats, cfg.WifeChatID)
	}
	if len(chats) == 0 {
		logger.Info(logging.Msg("app:startSundayScheduler", "воскресный разбор не запущен: нет chat_id"))
		return
	}
	statePath := filepath.Join(cfg.DataRoot, ".healthbot", "sunday_review.json")
	options := scheduler.Options{
		Chats:  chats,
		Target: target,
		Now:    func() time.Time { return time.Now().In(location) },
		Review: commands.Review,
		Notify: func(ctx context.Context, chatID int64, text string) error {
			_, sendErr := bot.SendMessage(ctx, &telego.SendMessageParams{
				ChatID:    telego.ChatID{ID: chatID},
				Text:      telegramfmt.Format(text),
				ParseMode: telegramfmt.ParseMode,
			})
			if sendErr != nil {
				return fmt.Errorf("отправить воскресный разбор: %w", sendErr)
			}
			return nil
		},
		StatePath: statePath,
		Logger:    logger,
	}
	logger.Info(logging.Msg("app:startSundayScheduler", "воскресный разбор включён"),
		"time", cfg.SundayReviewTime, "tz", cfg.SundayReviewTZ, "chats", len(chats))
	go scheduler.Run(ctx, options)
}
