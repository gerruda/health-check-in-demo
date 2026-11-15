package intake_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
)

func TestVisionTwoPhaseSleepQuality(t *testing.T) {
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"sleep_quality"}`),
		json.RawMessage(`{"date_text":"Сегодня","sleep":"8 ч 9 м","sleep_score":60}`),
	}}
	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.NoError(t, err)
	require.NotNil(t, ex.Checkin)
	require.NotNil(t, ex.Checkin.SleepH)
	assert.InDelta(t, 8.15, *ex.Checkin.SleepH, 0.01)
	require.NotNil(t, ex.Checkin.SleepScore)
	assert.Equal(t, 60, *ex.Checkin.SleepScore)
	assert.Nil(t, ex.Checkin.HRV, "на экране качества нет HRV")
	assert.Equal(t, 2, provider.visionCalls, "классификация + извлечение")
}

func TestVisionTwoPhaseSleepMetrics(t *testing.T) {
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"sleep_metrics"}`),
		json.RawMessage(`{"date_text":"Сегодня","hrv_ms":34,"rhr":65,"avg_hr_night":70,"spo2_avg":96,"spo2_min":86,"respiration_avg":19,"respiration_min":10,"body_battery":46}`),
	}}
	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.NoError(t, err)
	require.NotNil(t, ex.Checkin)
	require.NotNil(t, ex.Checkin.HRV)
	assert.Equal(t, 34, *ex.Checkin.HRV)
	require.NotNil(t, ex.Checkin.RHR)
	assert.Equal(t, 65, *ex.Checkin.RHR)
	assert.Nil(t, ex.Checkin.SleepH, "метрики — не длительность сна")
	assert.Contains(t, ex.Checkin.Context, "SpO2 96%")
}

func TestVisionWorkoutOCR(t *testing.T) {
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"workout"}`),
		json.RawMessage(`{"text":"Общее время 31:15\nСредняя частота пульса 92 уд/м\nВсего калорий 107\nПодход / Имя / Время / Повтор. / Вес\n1 / Неизвестно / 31:15 / 4 / -\n11 нояб. @ 12:55"}`),
	}}
	ex, err := newExtractor(provider).Extract(context.Background(), intake.Input{Images: [][]byte{[]byte("img")}})
	require.NoError(t, err)
	require.NotNil(t, ex.Training)
	assert.Equal(t, "2026-11-11", ex.Training.Date)
	require.NotNil(t, ex.Training.DurationMin)
	assert.Equal(t, 31, *ex.Training.DurationMin)
	require.NotNil(t, ex.Training.AvgHR)
	assert.Equal(t, 92, *ex.Training.AvgHR)
	require.NotNil(t, ex.Training.Calories)
	assert.Equal(t, 107, *ex.Training.Calories)
	require.Len(t, ex.Training.Exercises, 1)
	assert.Equal(t, "Неизвестно", ex.Training.Exercises[0].Name)
	require.NotNil(t, ex.Training.Exercises[0].Reps)
	assert.Equal(t, 4, *ex.Training.Exercises[0].Reps)
}

func TestServiceSavesVisionAlbum(t *testing.T) {
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.Local)
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"sleep_quality"}`),
		json.RawMessage(`{"date_text":"Сегодня","sleep":"8 ч 9 м","sleep_score":60}`),
		json.RawMessage(`{"kind":"workout"}`),
		json.RawMessage(`{"text":"Общее время 31:15\nСредняя частота пульса 92 уд/м\nВсего калорий 107\nПодход / Имя / Время / Повтор. / Вес\n1 / Неизвестно / 31:15 / 4 / -\n11 нояб. @ 12:55"}`),
	}}
	saver := &fakeSaver{}
	downloader := &fakeDownloader{files: map[string][]byte{"a": []byte("a"), "b": []byte("b")}}
	svc, replies := newSavingService(provider, downloader, saver, nil, now)

	svc.SubmitPhoto(context.Background(), 1, "group", "a", "")
	svc.SubmitPhoto(context.Background(), 1, "group", "b", "")

	reply := waitReply(t, replies)
	require.Len(t, saver.checkins, 1)
	require.Len(t, saver.trainings, 1)
	assert.Equal(t, "2026-11-12", saver.checkins[0].Date)
	assert.Equal(t, "2026-11-11", saver.trainings[0].Date)
	require.NotNil(t, saver.trainings[0].Calories)
	assert.Equal(t, 107, *saver.trainings[0].Calories)
	assert.Contains(t, reply, "сон 08:09")
	assert.Contains(t, reply, "тренировка")
	assert.Contains(t, reply, "Сохранено:")
	assert.Contains(t, reply, "31 мин")
}

func TestDuplicateScreenshotSkipped(t *testing.T) {
	now := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"workout"}`),
		json.RawMessage(`{"text":"13 нояб. 2026\nОбщее время 45 мин\nСредний пульс 123\nВсего калорий 195"}`),
	}}
	download := &fakeDownloader{files: map[string][]byte{"photo": []byte("same-image")}}
	svc, replies := newSavingService(provider, download, &fakeSaver{}, nil, now)

	svc.SubmitPhoto(context.Background(), 1, "", "photo", "")
	first := waitReply(t, replies)
	require.Contains(t, first, "Сохранено:")

	callsBefore := provider.visionCalls
	svc.SubmitPhoto(context.Background(), 1, "", "photo", "")
	second := waitReply(t, replies)
	assert.Contains(t, second, "уже обрабатывал")
	assert.Equal(t, callsBefore, provider.visionCalls, "дубль не должен доходить до vision")
}

func TestVisionTrainingWithoutDateAsksDate(t *testing.T) {
	now := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"workout"}`),
		json.RawMessage(`{"text":"Общее время 45 мин\nСредний пульс 123\nВсего калорий 195"}`),
	}}
	saver := &fakeSaver{}
	download := &fakeDownloader{files: map[string][]byte{"photo": []byte("no-date-image")}}
	svc, replies := newSavingService(provider, download, saver, nil, now)

	svc.SubmitPhoto(context.Background(), 1, "", "photo", "")
	reply := waitReply(t, replies)
	assert.Contains(t, reply, "не увидел дату")
	assert.Empty(t, saver.trainings, "без даты тренировка не сохраняется")

	require.True(t, svc.TryAnswerPending(context.Background(), 1, "13.11"))
	update := waitReply(t, replies)
	assert.Contains(t, update, "Сохранено:")
	require.Len(t, saver.trainings, 1)
	assert.Equal(t, "2026-11-13", saver.trainings[0].Date)
}
