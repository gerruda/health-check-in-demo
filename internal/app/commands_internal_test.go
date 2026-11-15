package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/config"
	"healthbot/internal/intake"
	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestHandlers(t *testing.T) (*commandHandlers, config.Config) {
	t.Helper()
	cfg := config.Config{
		DataRoot:       t.TempDir(),
		SettingsDir:    t.TempDir(),
		WifeChatID:     2,
		LLMMode:        config.ModeOllama,
		OllamaModel:    "qwen2.5:3b",
		LLMVisionMode:  config.ModeOllama,
		LLMVisionModel: "qwen2.5vl:3b",
	}
	handlers := newCommandHandlers(cfg, discardLogger(), nil, nil)
	return handlers, cfg
}

func TestSettingsDefaultsAndOverrides(t *testing.T) {
	handlers, cfg := newTestHandlers(t)

	reply, err := handlers.Settings(1)
	require.NoError(t, err)
	assert.Contains(t, reply, "2300", "дефолт ккал атлета из контракта")
	assert.Contains(t, reply, "HRV", "база HRV")
	assert.Contains(t, reply, "auto", "база HRV по умолчанию auto")
	assert.Contains(t, reply, cfg.OllamaModel, "активный LLM")

	_, err = handlers.Set(1, "kcal", "2400")
	require.NoError(t, err)

	reply, err = handlers.Settings(1)
	require.NoError(t, err)
	assert.Contains(t, reply, "2400*", "переопределение помечается *")
}

func TestSetInvalidValue(t *testing.T) {
	handlers, _ := newTestHandlers(t)

	_, err := handlers.Set(1, "kcal", "900")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "диапазон")
}

func TestSetReset(t *testing.T) {
	handlers, _ := newTestHandlers(t)

	_, err := handlers.Set(1, "hrv_base", "34")
	require.NoError(t, err)

	resetReply, err := handlers.Reset(1)
	require.NoError(t, err)
	assert.Contains(t, resetReply, "Вернул")

	reply, err := handlers.Settings(1)
	require.NoError(t, err)
	assert.NotContains(t, reply, "34*")
	assert.Contains(t, reply, "auto")
}

func TestSettingsSeparatePeople(t *testing.T) {
	handlers, _ := newTestHandlers(t)

	_, err := handlers.Set(2, "kcal", "1850")
	require.NoError(t, err)

	athlete, err := handlers.Settings(1)
	require.NoError(t, err)
	assert.Contains(t, athlete, "2300")
	assert.NotContains(t, athlete, "1850*")

	wife, err := handlers.Settings(2)
	require.NoError(t, err)
	assert.Contains(t, wife, "1850*")
}

func TestCheckinReplyCoach(t *testing.T) {
	handlers, cfg := newTestHandlers(t)

	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))
	sleep := 7.5
	hrv := 35
	rhr := 57
	mood := 7
	_, err := writer.WriteCheckin(nlu.Checkin{
		Date: "2026-11-12", Person: nlu.Athlete,
		SleepH: &sleep, HRV: &hrv, RHR: &rhr, Mood: &mood,
	})
	require.NoError(t, err)
	_, err = handlers.Set(1, "hrv_base", "33")
	require.NoError(t, err)

	reply, err := handlers.CheckinReply(1, intake.Extracted{
		Date: "2026-11-12",
		Checkin: &intake.Checkin{
			HRV: &hrv, RHR: &rhr, SleepH: &sleep, Mood: &mood,
			PlannedTraining: "силовая A",
		},
	})
	require.NoError(t, err)
	assert.Contains(t, reply, "🌿")
	assert.Contains(t, reply, "HRV_7")
	assert.Contains(t, reply, "🟢")
	assert.Contains(t, reply, "Ориентир")
	assert.Contains(t, reply, "силовая A", "план пользователя попадает в ориентир")
}

func TestStatusWithCheckins(t *testing.T) {
	handlers, cfg := newTestHandlers(t)

	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))
	sleep := 7.5
	hrv := 38
	rhr := 58
	mood := 7
	_, err := writer.WriteCheckin(nlu.Checkin{
		Date:   "2026-11-12",
		Person: nlu.Athlete,
		SleepH: &sleep,
		HRV:    &hrv,
		RHR:    &rhr,
		Mood:   &mood,
	})
	require.NoError(t, err)
	_, err = handlers.Set(1, "hrv_base", "33")
	require.NoError(t, err)

	reply, err := handlers.Status(context.Background(), 1)
	require.NoError(t, err)
	assert.Contains(t, reply, "HRV_7")
	assert.Contains(t, reply, "38", "HRV из файла")
	assert.Contains(t, reply, "🟢", "зона зелёная при HRV 38 и пульсе 58")
	assert.Contains(t, reply, "Ориентир", "коуч-ориентир в статусе")
	assert.Contains(t, reply, "2300", "цели")
	assert.NotContains(t, reply, "LLM", "модели показываются только в /settings")
	assert.NotContains(t, reply, "Vision", "модели показываются только в /settings")
	assert.Contains(t, reply, "Ритм ровный", "живая фраза к зелёной зоне")
}

func TestStatusNoCheckins(t *testing.T) {
	handlers, _ := newTestHandlers(t)

	reply, err := handlers.Status(context.Background(), 1)
	require.NoError(t, err)
	assert.Contains(t, reply, "нет данных")
	assert.Contains(t, reply, "мало данных", "живая фраза при отсутствии чек-инов")
}

func TestStatusLastTraining(t *testing.T) {
	handlers, cfg := newTestHandlers(t)

	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))
	_, err := writer.WriteTraining(nlu.Training{
		Date:   "2026-11-11",
		Person: nlu.Athlete,
		Kind:   "sila-A",
		Done:   true,
	})
	require.NoError(t, err)

	reply, err := handlers.Status(context.Background(), 1)
	require.NoError(t, err)
	assert.Contains(t, reply, "sila-A")
	assert.Contains(t, reply, "11.11.2026")
}

func TestSettingsShowWeeklyReview(t *testing.T) {
	handlers, _ := newTestHandlers(t)
	reply, err := handlers.Settings(1)
	require.NoError(t, err)
	assert.Contains(t, reply, "Цели (kcal")
	assert.Contains(t, reply, "hrv_base")
	assert.Contains(t, reply, "Воскресный разбор (weekly_review · weekly_review_time):")
	assert.Contains(t, reply, "  выкл · 09:00")

	_, err = handlers.Set(1, "weekly_review", "on")
	require.NoError(t, err)
	_, err = handlers.Set(1, "weekly_review_time", "10:30")
	require.NoError(t, err)
	reply, err = handlers.Settings(1)
	require.NoError(t, err)
	assert.Contains(t, reply, "  вкл · 10:30")
	assert.Contains(t, reply, "Цели (kcal")
}
