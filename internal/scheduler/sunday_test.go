package scheduler_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/scheduler"
)

func TestRunOnceOnlySundayAfterTarget(t *testing.T) {
	saturday := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC) // суббота
	sundayEarly := time.Date(2026, 11, 15, 8, 59, 0, 0, time.UTC)
	sunday := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	var calls atomic.Int32

	options := scheduler.Options{
		Chats:  []int64{1, 2},
		Target: time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC),
		Review: func(context.Context, int64) (string, error) {
			calls.Add(1)
			return "разбор", nil
		},
		Notify:    func(context.Context, int64, string) error { return nil },
		StatePath: filepath.Join(t.TempDir(), "state.json"),
	}
	for _, now := range []time.Time{saturday, sundayEarly} {
		options.Now = func() time.Time { return now }
		assert.Zero(t, scheduler.RunOnce(context.Background(), options))
	}
	options.Now = func() time.Time { return sunday }
	assert.Equal(t, 2, scheduler.RunOnce(context.Background(), options))
	assert.Equal(t, int32(2), calls.Load())
	assert.Zero(t, scheduler.RunOnce(context.Background(), options), "повтор в тот же день не отправляется")
}

func TestRunOncePersistsState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "sunday.json")
	sunday := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	options := scheduler.Options{
		Chats:     []int64{7},
		Target:    time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC),
		Now:       func() time.Time { return sunday },
		Review:    func(context.Context, int64) (string, error) { return "разбор", nil },
		Notify:    func(context.Context, int64, string) error { return nil },
		StatePath: statePath,
	}
	require.Equal(t, 1, scheduler.RunOnce(context.Background(), options))
	// второй планировщик (как после рестарта) читает состояние с диска
	second := options
	assert.Zero(t, scheduler.RunOnce(context.Background(), second))
}

func TestRunOnceUsesPerChatSchedule(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	var sent []int64
	options := scheduler.Options{
		Chats: []int64{1, 2},
		Schedule: func(chatID int64) (bool, time.Time) {
			switch chatID {
			case 1:
				return true, time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
			default:
				return false, time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
			}
		},
		Now: func() time.Time { return now },
		Review: func(context.Context, int64) (string, error) {
			return "разбор", nil
		},
		Notify: func(_ context.Context, chatID int64, _ string) error {
			sent = append(sent, chatID)
			return nil
		},
		StatePath: filepath.Join(t.TempDir(), "state.json"),
	}
	require.Equal(t, 1, scheduler.RunOnce(context.Background(), options))
	assert.Equal(t, []int64{1}, sent)
}
