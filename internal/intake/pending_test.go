package intake_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceAsksAndAppliesMissingTrainingFields(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)

	text := `запиши сегодняшний чекин: чсс в покое 60, вариабильность чсс 30, длительность сна 7часов. утром была уличная тренировка, подтягивания 2 раза, отжимания на брусьях 8 повторений, все упражнения 2 подхода, затем подъем по лестнице на 32 этаж`
	svc.SubmitTraining(context.Background(), 1, text)

	reply := waitReply(t, replies)
	assert.Contains(t, reply, "уточни")
	assert.Contains(t, reply, "RPE")
	require.Len(t, saver.trainings, 1)
	require.Len(t, saver.checkins, 1, "чек-ин из комбинированного сообщения сохранён")

	handled := svc.TryAnswerPending(context.Background(), 1, "RPE 7, 45 минут, средний пульс 120")
	require.True(t, handled)
	updated := waitReply(t, replies)
	assert.Contains(t, updated, "Обновил")
	require.Len(t, saver.trainings, 2)
	last := saver.trainings[len(saver.trainings)-1]
	require.NotNil(t, last.RPESession)
	assert.Equal(t, 7, *last.RPESession)
	require.NotNil(t, last.DurationMin)
	assert.Equal(t, 45, *last.DurationMin)
	require.NotNil(t, last.AvgHR)
	assert.Equal(t, 120, *last.AvgHR)

	assert.False(t, svc.TryAnswerPending(context.Background(), 1, "RPE 8"), "pending очищен после раунда")
}

func TestPendingCyrillicRPEAndCardio(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)
	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5, все упражнения 3 подхода")
	_ = waitReply(t, replies)

	handled := svc.TryAnswerPending(context.Background(), 1, "рпе 8 , время тренировки 40 минут. средний пульс 135. кардио 10 минут средний пульс 140")
	require.True(t, handled)
	_ = waitReply(t, replies)
	last := saver.trainings[len(saver.trainings)-1]
	require.NotNil(t, last.RPESession)
	assert.Equal(t, 8, *last.RPESession)
	require.NotNil(t, last.DurationMin)
	assert.Equal(t, 40, *last.DurationMin)
	require.NotNil(t, last.AvgHR)
	assert.Equal(t, 135, *last.AvgHR)
	assert.Contains(t, last.Cardio, "10 мин")
	assert.Contains(t, last.Cardio, "140")
}
