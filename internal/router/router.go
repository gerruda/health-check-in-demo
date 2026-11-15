// Package router маршрутизирует Telegram-обновления.
//
// На этапе M0 реализованы: allowlist по chat_id (ТЗ §2) и команда /help.
// Пакет не зависит от сети: общение с Telegram идёт через интерфейс Sender,
// поэтому обработчики тестируются фейковым отправителем без Telegram (ТЗ §12).
package router

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mymmrac/telego"

	"healthbot/internal/logging"
	"healthbot/internal/telegramfmt"
	"healthbot/internal/typing"
)

// Sender — минимальная часть telego.Bot, нужная роутеру.
// Позволяет тестировать обработчики без сети.
type Sender interface {
	SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error)
}

// Allowlist — chat_id атлета и жены (ТЗ §2).
// Пустой список (нули) выключает фильтр: удобно для локальной отладки.
type Allowlist struct {
	Athlete int64
	Wife    int64
}

// Allows сообщает, разрешён ли chatID.
func (a Allowlist) Allows(chatID int64) bool {
	if a.Athlete == 0 && a.Wife == 0 {
		return true
	}
	return chatID == a.Athlete || chatID == a.Wife
}

// Router обрабатывает обновления Telegram.
type Router struct {
	sender      Sender
	log         *slog.Logger
	allow       Allowlist
	chat        Chatter
	ask         AskHandler
	chatTimeout time.Duration
	intake      Intake
	typing      typing.ActionFunc
	settings    SettingsHandler
	status      StatusHandler
	week        WeekHandler
	review      ReviewHandler
}

// New создаёт роутер. nil-логгер заменяется на slog.Default.
func New(sender Sender, log *slog.Logger, allow Allowlist, opts ...Option) *Router {
	if log == nil {
		log = slog.Default()
	}
	router := &Router{
		sender:      sender,
		log:         log,
		allow:       allow,
		chatTimeout: defaultChatTimeout,
	}
	for _, opt := range opts {
		opt(router)
	}
	return router
}

// HandleUpdate обрабатывает одно обновление Telegram. Не паникует на чужих
// chat_id, не-командных сообщениях и пустых апдейтах (NFR-2).
func (r *Router) HandleUpdate(ctx context.Context, update telego.Update) {
	msg := update.Message
	if msg == nil {
		return
	}
	if !r.allow.Allows(msg.Chat.ID) {
		r.log.Warn(logging.Msg("router:HandleUpdate", "сообщение от постороннего chat_id проигнорировано"),
			"chat_id", msg.Chat.ID)
		return
	}

	if len(msg.Photo) > 0 {
		r.handlePhoto(ctx, msg, msg.Photo[len(msg.Photo)-1].FileID)
		return
	}
	if msg.Document != nil && strings.HasPrefix(msg.Document.MimeType, "image/") {
		r.handlePhoto(ctx, msg, msg.Document.FileID)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	if !strings.HasPrefix(text, "/") {
		if r.intake != nil && r.intake.HandleFormAnswer(ctx, msg.Chat.ID, text) {
			return
		}
		if r.intake != nil && r.intake.TryAnswerPending(ctx, msg.Chat.ID, text) {
			return
		}
		if r.intake != nil && r.intake.LooksLikeTraining(text) {
			r.intake.SubmitTraining(ctx, msg.Chat.ID, text)
			return
		}
		if r.intake != nil && r.intake.LooksLikeCheckin(text) {
			r.intake.SubmitText(ctx, msg.Chat.ID, text)
			return
		}
		if r.intake != nil && r.intake.LooksLikeNutrition(text) {
			r.intake.SubmitText(ctx, msg.Chat.ID, text)
			return
		}
		if likelyGibberish(text) {
			r.reply(ctx, msg.Chat.ID, "Не понял сообщение. Напиши словами, что нужно сделать.")
			return
		}
		if r.chat != nil {
			r.answerWithChat(ctx, msg.Chat.ID, text)
			return
		}
		// TODO(M1, FR-1): без LLM свободный текст пойдёт в intake на этапе M1.
		r.log.Info(logging.Msg("router:HandleUpdate", "сообщение без команды проигнорировано (intake появится на этапе M1)"),
			"chat_id", msg.Chat.ID)
		return
	}

	switch Command(text) {
	case "start", "help":
		r.reply(ctx, msg.Chat.ID, HelpText)
	case "settings":
		r.handleSettingsCommand(ctx, msg.Chat.ID)
	case "set":
		r.handleSetCommand(ctx, msg.Chat.ID, text)
	case "status":
		r.handleStatusCommand(ctx, msg.Chat.ID)
	case "week":
		r.handleWeekCommand(ctx, msg.Chat.ID)
	case "review":
		r.handleReviewCommand(ctx, msg.Chat.ID)
	case "ask":
		r.handleAskCommand(ctx, msg.Chat.ID, text)
	case "checkin":
		r.handleFormCommand(ctx, msg.Chat.ID, "checkin")
	case "food":
		r.handleFormCommand(ctx, msg.Chat.ID, "food")
	case "measure":
		r.handleFormCommand(ctx, msg.Chat.ID, "measure")
	case "cancel":
		r.handleCancelCommand(ctx, msg.Chat.ID)
	case "training":
		if r.intake != nil && len(strings.Fields(text)) == 1 {
			r.handleFormCommand(ctx, msg.Chat.ID, "training")
			return
		}
		r.handleTrainingCommand(ctx, msg.Chat.ID, text)
	default:
		r.log.Info(logging.Msg("router:HandleUpdate", "неизвестная команда"),
			"chat_id", msg.Chat.ID, "command", Command(text))
		r.reply(ctx, msg.Chat.ID, fmt.Sprintf("Пока не умею /%s. Доступно — /help.", Command(text)))
	}
}

// handlePhoto передаёт скриншот в приём данных (FR-6.8.3).
func (r *Router) handlePhoto(ctx context.Context, msg *telego.Message, fileID string) {
	if r.intake == nil {
		r.reply(ctx, msg.Chat.ID, "Не могу обработать изображение: vision не настроен. Пришли данные текстом.")
		return
	}
	r.intake.SubmitPhoto(ctx, msg.Chat.ID, msg.MediaGroupID, fileID, msg.Caption)
}

func (r *Router) reply(ctx context.Context, chatID int64, text string) {
	if _, err := r.sender.SendMessage(ctx, &telego.SendMessageParams{
		ChatID:    telego.ChatID{ID: chatID},
		Text:      telegramfmt.Format(text),
		ParseMode: telegramfmt.ParseMode,
	}); err != nil {
		r.log.Error(logging.Msg("router:reply", "не удалось отправить сообщение"),
			"chat_id", chatID, "error", err)
	}
}

// Command извлекает имя команды из текста: "/help@bot" → "help".
// Для не-команд возвращает пустую строку.
func Command(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	cmd := strings.TrimPrefix(fields[0], "/")
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	return strings.ToLower(cmd)
}
