// Package scheduler — проактивные напоминания бота (воскресный разбор).
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"healthbot/internal/logging"
)

// ReviewFunc собирает текст разбора для chat_id.
type ReviewFunc func(ctx context.Context, chatID int64) (string, error)

// Notifier отправляет текст в чат.
type Notifier func(ctx context.Context, chatID int64, text string) error

// ScheduleFunc возвращает персональное расписание: включён ли разбор и время.
type ScheduleFunc func(chatID int64) (enabled bool, target time.Time)

// Options настраивает планировщик.
type Options struct {
	Chats     []int64
	Target    time.Time // дефолт, если Schedule не задан
	Schedule  ScheduleFunc
	Now       func() time.Time
	Review    ReviewFunc
	Notify    Notifier
	StatePath string
	Interval  time.Duration
	Logger    *slog.Logger
}

// RunOnce отправляет разбор, если сейчас воскресенье после Target и сегодня
// ещё не отправляли. Возвращает число отправленных сообщений.
func RunOnce(ctx context.Context, options Options) int {
	now := time.Now()
	if options.Now != nil {
		now = options.Now()
	}
	if now.Weekday() != time.Sunday {
		return 0
	}
	state := loadState(options.StatePath)
	date := now.Format("2006-01-02")
	sent := 0
	for _, chatID := range options.Chats {
		target := options.Target
		enabled := true
		if options.Schedule != nil {
			enabled, target = options.Schedule(chatID)
		}
		if !enabled || now.Hour() < target.Hour() || (now.Hour() == target.Hour() && now.Minute() < target.Minute()) {
			continue
		}
		if chatID == 0 || state[chatID] == date {
			continue
		}
		text, err := options.Review(ctx, chatID)
		if err != nil {
			logger(options).Warn(logging.Msg("scheduler:RunOnce", "не удалось собрать воскресный разбор"),
				"chat_id", chatID, "error", err)
			continue
		}
		if text == "" {
			continue
		}
		if err := options.Notify(ctx, chatID, text); err != nil {
			logger(options).Error(logging.Msg("scheduler:RunOnce", "не удалось отправить воскресный разбор"),
				"chat_id", chatID, "error", err)
			continue
		}
		state[chatID] = date
		sent++
	}
	if sent > 0 {
		if err := saveState(options.StatePath, state); err != nil {
			logger(options).Warn(logging.Msg("scheduler:RunOnce", "не удалось сохранить состояние разбора"), "error", err)
		}
	}
	return sent
}

// Run запускает проверку раз в минуту до отмены ctx.
func Run(ctx context.Context, options Options) {
	interval := options.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	// проверка на старте: если бот поднялся в воскресенье после времени — отправим
	RunOnce(ctx, options)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			RunOnce(ctx, options)
		}
	}
}

func logger(options Options) *slog.Logger {
	if options.Logger != nil {
		return options.Logger
	}
	return slog.Default()
}

var stateCache = struct {
	mu   sync.Mutex
	path string
	last map[int64]string
}{}

func loadState(path string) map[int64]string {
	if path == "" {
		return make(map[int64]string)
	}
	stateCache.mu.Lock()
	defer stateCache.mu.Unlock()
	if stateCache.path == path && stateCache.last != nil {
		copied := make(map[int64]string, len(stateCache.last))
		for key, value := range stateCache.last {
			copied[key] = value
		}
		return copied
	}
	state := make(map[int64]string)
	data, err := os.ReadFile(path) // #nosec G304 -- путь задаёт приложение
	if err == nil {
		var raw map[string]string
		if json.Unmarshal(data, &raw) == nil {
			for key, value := range raw {
				var chatID int64
				if _, scanErr := fmt.Sscanf(key, "%d", &chatID); scanErr == nil {
					state[chatID] = value
				}
			}
		}
	}
	return state
}

func saveState(path string, state map[int64]string) error {
	if path == "" {
		return nil
	}
	raw := make(map[string]string, len(state))
	for key, value := range state {
		raw[fmt.Sprintf("%d", key)] = value
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil {
		return mkdirErr
	}
	tmp, err := os.CreateTemp(dir, ".sunday-review-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	stateCache.mu.Lock()
	stateCache.path = path
	stateCache.last = make(map[int64]string, len(state))
	for key, value := range state {
		stateCache.last[key] = value
	}
	stateCache.mu.Unlock()
	return nil
}
