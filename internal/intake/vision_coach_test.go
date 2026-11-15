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

func workoutVisionProvider() *fakeProvider {
	return &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"workout"}`),
		json.RawMessage(`{"text":"Общее время 31:15\nСредняя частота пульса 92 уд/м\nВсего калорий 107\nПодход / Имя / Время / Повтор. / Вес\n1 / Жим / 31:15 / 4 / 14\n11 нояб. @ 12:55"}`),
	}}
}

func TestServiceVisionTrainingCoachReply(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := workoutVisionProvider()
	saver := &fakeSaver{}
	download := &fakeDownloader{files: map[string][]byte{"w": []byte("img")}}
	coach := &fakeTrainingCoach{reply: "🌿 Следующая: жим 16 кг · 4×10"}
	svc, replies := newSavingService(provider, download, saver, nil, now,
		intake.WithTrainingCoach(coach))

	svc.SubmitPhoto(context.Background(), 1, "", "w", "")

	reply := waitReply(t, replies)
	require.Equal(t, 1, coach.calls)
	assert.Contains(t, reply, "Следующая")
	assert.Contains(t, reply, "Сохранено:")
	assert.NotContains(t, reply, "Отметил данные")
}

func TestServiceVisionAlbumCheckinAndTrainingCoaches(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	provider := &fakeProvider{visionResponses: []json.RawMessage{
		json.RawMessage(`{"kind":"sleep_quality"}`),
		json.RawMessage(`{"date_text":"Сегодня","sleep":"8 ч 9 м","sleep_score":60}`),
		json.RawMessage(`{"kind":"workout"}`),
		json.RawMessage(`{"text":"Общее время 31:15\nСредняя частота пульса 92 уд/м\nВсего калорий 107\nПодход / Имя / Время / Повтор. / Вес\n1 / Жим / 31:15 / 4 / 14\n11 нояб. @ 12:55"}`),
	}}
	saver := &fakeSaver{}
	download := &fakeDownloader{files: map[string][]byte{"s": []byte("a"), "w": []byte("b")}}
	checkinCoach := &fakeCoach{reply: "🌿 Доброе утро.\n🟢 Зона: зелёная"}
	trainingCoach := &fakeTrainingCoach{reply: "🌿 Следующая: жим 16 кг · 4×10"}
	svc, replies := newSavingService(provider, download, saver, nil, now,
		intake.WithCoach(checkinCoach), intake.WithTrainingCoach(trainingCoach))

	svc.SubmitPhoto(context.Background(), 1, "g", "s", "")
	svc.SubmitPhoto(context.Background(), 1, "g", "w", "")

	reply := waitReply(t, replies)
	assert.Contains(t, reply, "Доброе утро")
	assert.Contains(t, reply, "Следующая")
	require.Len(t, saver.checkins, 1)
	require.Len(t, saver.trainings, 1)
}
