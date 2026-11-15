package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/config"
	"healthbot/internal/intake"
	"healthbot/internal/nlu"
	"healthbot/internal/router"
	"healthbot/internal/store"
)

var e2eNow = time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

type e2eSender struct {
	mu    sync.Mutex
	texts []string
}

func (s *e2eSender) SendMessage(_ context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, params.Text)
	return &telego.Message{}, nil
}

func (s *e2eSender) record(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, text)
}

func (s *e2eSender) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

type e2eChat struct{}

func (e2eChat) Ask(context.Context, string) (string, error) { return "Ответ LLM", nil }

type e2eAsk struct{ question string }

func (a *e2eAsk) Ask(_ context.Context, text string) (string, error) {
	a.question = text
	return "Ответ по базе знаний", nil
}

type e2eDownloader struct{}

func (e2eDownloader) Download(context.Context, string) ([]byte, error) { return nil, nil }

type e2eHarness struct {
	t       *testing.T
	cfg     config.Config
	log     *slog.Logger
	sender  *e2eSender
	writer  *store.Writer
	handler *commandHandlers
	router  *router.Router
	ask     *e2eAsk
	intake  *intake.Service
}

func newE2E(t *testing.T) *e2eHarness {
	t.Helper()
	cfg := config.Config{
		DataRoot:      t.TempDir(),
		SettingsDir:   t.TempDir(),
		AthleteChatID: 1,
		WifeChatID:    2,
		OllamaModel:   "qwen2.5:3b",
		OllamaURL:     "http://localhost:11434",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sender := &e2eSender{}
	cache := store.NewCache()
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot), store.WithWriterCache(cache))
	handlers := newCommandHandlers(cfg, logger, cache, nil)
	handlers.now = func() time.Time { return e2eNow }

	saver := &storeSaver{writer: writer, root: cfg.DataRoot}
	extractor := intake.NewExtractor(nil, nil, logger)
	svc := intake.NewService(extractor, e2eDownloader{}, func(_ context.Context, _ int64, text string) error {
		sender.record(text)
		return nil
	}, logger,
		intake.WithSaver(saver),
		intake.WithClock(func() time.Time { return e2eNow }),
		intake.WithPersonResolver(func(chatID int64) nlu.Person {
			if chatID == 2 {
				return nlu.Wife
			}
			return nlu.Athlete
		}),
		intake.WithCoach(handlers),
		intake.WithTrainingCoach(handlers),
		intake.WithTrainingReporter(handlers),
		intake.WithNutritionCoach(handlers),
	)

	ask := &e2eAsk{}
	r := buildRouter(sender, logger, cfg, e2eChat{}, ask, svc, nil, handlers)
	return &e2eHarness{t: t, cfg: cfg, log: logger, sender: sender, writer: writer, handler: handlers, router: r, ask: ask, intake: svc}
}

func (h *e2eHarness) update(chatID int64, text string) {
	h.t.Helper()
	h.router.HandleUpdate(context.Background(), telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			Chat: telego.Chat{ID: chatID},
			Text: text,
		},
	})
}

func (h *e2eHarness) seedAthlete() {
	h.t.Helper()
	hrv, rhr, mood := 32, 58, 7
	sleep, weight := 7.5, 103.2
	_, err := h.writer.WriteCheckin(nlu.Checkin{
		Date: "2026-11-11", Person: nlu.Athlete,
		HRV: &hrv, RHR: &rhr, Mood: &mood, SleepH: &sleep, WeightKG: &weight,
	})
	require.NoError(h.t, err)
	duration, rpe := 45, 7
	weightExercise := 60.0
	sets, reps := 5, 5
	_, err = h.writer.WriteTraining(nlu.Training{
		Date: "2026-11-11", Person: nlu.Athlete, Kind: "sila-A", Done: true,
		DurationMin: &duration, RPESession: &rpe,
		Exercises: []nlu.Exercise{{Name: "присед", WeightKG: &weightExercise, Sets: &sets, Reps: &reps}},
	})
	require.NoError(h.t, err)
	_, err = h.writer.WriteMeasurement(nlu.Measurement{
		Date: "2026-11-08", Person: nlu.Athlete,
		WeightKG: floatPtrE2E(103.5), WaistCM: floatPtrE2E(112), Visceral: intPtrE2E(16),
	})
	require.NoError(h.t, err)
	_, err = h.writer.WriteMeasurement(nlu.Measurement{
		Date: "2026-11-12", Person: nlu.Athlete,
		WeightKG: floatPtrE2E(102.8), WaistCM: floatPtrE2E(110.5), Visceral: intPtrE2E(15),
	})
	require.NoError(h.t, err)
}

func floatPtrE2E(value float64) *float64 { return &value }
func intPtrE2E(value int) *int           { return &value }

func (h *e2eHarness) waitContains(want string) string {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, text := range h.sender.sent() {
			if strings.Contains(text, want) {
				return text
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("не дождались ответа с %q; отправлено: %v", want, h.sender.sent())
	return ""
}

func (h *e2eHarness) last() string {
	h.t.Helper()
	sent := h.sender.sent()
	require.NotEmpty(h.t, sent)
	return sent[len(sent)-1]
}

func (h *e2eHarness) fileExists(pattern string) string {
	h.t.Helper()
	matches, err := filepath.Glob(filepath.Join(h.cfg.DataRoot, pattern))
	require.NoError(h.t, err)
	require.NotEmpty(h.t, matches, "файл %s не создан", pattern)
	return matches[0]
}

func TestE2ECommandSmoke(t *testing.T) {
	h := newE2E(t)
	h.seedAthlete()

	cases := []struct{ command, want string }{
		{"/help", "Команды"},
		{"/start", "Ритм"},
		{"/help@health_check_in_bot", "Команды"},
		{"/settings", "Настройки"},
		{"/status", "HRV"},
		{"/week", "Неделя"},
		{"/review", "Стратегия на следующую неделю"},
	}
	for _, testCase := range cases {
		t.Run(testCase.command, func(t *testing.T) {
			h.update(1, testCase.command)
			assert.Contains(t, h.last(), testCase.want)
		})
	}

	h.update(1, "/ask что говорит база про сон?")
	assert.Contains(t, h.last(), "Ответ по базе знаний")
	assert.Equal(t, "что говорит база про сон?", h.ask.question)

	h.update(1, "/unknown")
	assert.Contains(t, h.last(), "Пока не умею")
}

func TestE2ESetVariants(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/settings")
	assert.Contains(t, h.last(), "Воскресный разбор (weekly_review · weekly_review_time):")
	assert.Contains(t, h.last(), "выкл · 09:00")

	h.update(1, "/set kcal 2400")
	assert.Contains(t, h.last(), "Отметил")
	h.update(1, "/settings")
	assert.Contains(t, h.last(), "2400*")

	h.update(1, "/set weekly_review on")
	assert.Contains(t, h.last(), "Отметил")
	h.update(1, "/set weekly_review_time 10:30")
	assert.Contains(t, h.last(), "Отметил")
	h.update(1, "/settings")
	assert.Contains(t, h.last(), "вкл · 10:30")

	// русский составной алиас /set
	h.update(1, "/set Воскресный разбор выкл")
	assert.Contains(t, h.last(), "Отметил")
	h.update(1, "/settings")
	assert.Contains(t, h.last(), "выкл · 10:30")

	h.update(1, "/set kcal 900")
	assert.Contains(t, h.last(), "диапазон")
	h.update(1, "/set")
	assert.Contains(t, h.last(), "Формат")
	h.update(1, "/set reset")
	assert.Contains(t, h.last(), "Вернул")
}

func TestE2ECheckinForm(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/checkin")
	assert.Contains(t, h.last(), "Чек-ин по шагам")
	for _, answer := range []string{"7:30", "30", "60", "6", "0", "силовая A"} {
		h.update(1, answer)
	}
	h.waitContains("Чек-ин")
	path := h.fileExists("данные_пользователя/ежедневные_чекины/2026-11-12.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Сон: 7:30")
	assert.Contains(t, content, "HRV: 30")
	assert.Contains(t, content, "силовая A")
}

func TestE2EFoodAndMeasureForms(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/food")
	assert.Contains(t, h.last(), "Питание за день")
	for _, answer := range []string{"2300", "130", "82", "210"} {
		h.update(1, answer)
	}
	h.waitContains("Питание")
	food := h.fileExists("данные_пользователя/ежедневные_чекины/2026-11-12.md")
	data, err := os.ReadFile(food)
	require.NoError(t, err)
	assert.Contains(t, string(data), "2300")

	h.update(1, "/measure")
	assert.Contains(t, h.last(), "Недельный замер")
	for _, answer := range []string{"102.8", "34.1", "62.0", "15", "46.2", "110.5", "110 38.5 108 61 40"} {
		h.update(1, answer)
	}
	h.waitContains("Замер записан")
	measurement := h.fileExists("данные_пользователя/замеры/2026-11-12.md")
	data, err = os.ReadFile(measurement)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Вес: 102.8")
	assert.Contains(t, string(data), "Бицепс: 38.5")
}

func TestE2ETrainingFormAndCancel(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/training")
	assert.Contains(t, h.last(), "Тренировка по шагам")
	for _, answer := range []string{"sila-A", "присед 60x5x5", "8", "45", "120"} {
		h.update(1, answer)
	}
	h.waitContains("Сохранено")
	training := h.fileExists("данные_пользователя/тренировки/2026-11-12_sila-A.md")
	data, err := os.ReadFile(training)
	require.NoError(t, err)
	assert.Contains(t, string(data), "присед")
	assert.Contains(t, string(data), "RPE: 8")

	h.update(1, "/checkin")
	assert.Contains(t, h.last(), "Чек-ин по шагам")
	h.update(1, "/cancel")
	h.waitContains("Ок, форму закрыл")
}

func TestE2ETrainingTextCommand(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/training силовая A: присед 60x5x5, RPE 7, 45 минут, средний пульс 120")
	h.waitContains("Сохранено")
	training := h.fileExists("данные_пользователя/тренировки/2026-11-12_sila-A.md")
	data, err := os.ReadFile(training)
	require.NoError(t, err)
	assert.Contains(t, string(data), "присед")
}

func TestE2ECancelWithoutForm(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/cancel")
	assert.Contains(t, h.last(), "Нет активной формы")
}

func TestE2EWifeCommands(t *testing.T) {
	h := newE2E(t)
	// чат жены: свои данные и свои настройки
	hrv := 34
	_, err := h.writer.WriteCheckin(nlu.Checkin{Date: "2026-11-11", Person: nlu.Wife, HRV: &hrv})
	require.NoError(t, err)
	h.update(2, "/status")
	assert.Contains(t, h.last(), "HRV")
	h.update(2, "/settings")
	assert.Contains(t, h.last(), "жен")
	h.update(1, "/settings")
	assert.Contains(t, h.last(), "атлет")
}

func TestE2EInvalidFormRetries(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/checkin")
	h.update(1, "не число")
	assert.Contains(t, h.last(), "Попробуй ещё раз")
	for _, answer := range []string{"7:30", "30", "60", "6", "0", "силовая A"} {
		h.update(1, answer)
	}
	h.waitContains("Чек-ин")
	h.fileExists("данные_пользователя/ежедневные_чекины/2026-11-12.md")
}

func TestE2EAskVariantsAndChat(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/ask@health_check_in_bot почему плато?")
	assert.Contains(t, h.last(), "Ответ по базе знаний")
	assert.Equal(t, "почему плато?", h.ask.question)

	h.update(1, "/ask")
	assert.Contains(t, h.last(), "Напиши вопрос")

	h.update(1, "привет, как дела?")
	assert.Contains(t, h.last(), "Ответ LLM")
}

func TestE2EInvalidWeeklySettings(t *testing.T) {
	h := newE2E(t)
	h.update(1, "/set weekly_review_time 25:00")
	assert.Contains(t, h.last(), "ЧЧ:ММ")
	h.update(1, "/set weekly_review maybe")
	assert.Contains(t, h.last(), "on/off")
}
