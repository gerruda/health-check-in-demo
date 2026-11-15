package intake_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
)

func startAndDrain(t *testing.T, svc *intake.Service, replies chan string, chatID int64, form string) {
	t.Helper()
	require.True(t, svc.StartForm(context.Background(), chatID, form))
	_ = waitReply(t, replies)
}

func answer(t *testing.T, svc *intake.Service, replies chan string, chatID int64, text string) {
	t.Helper()
	require.True(t, svc.HandleFormAnswer(context.Background(), chatID, text))
	_ = waitReply(t, replies)
}

func TestCheckinFormSavesAllFields(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)

	startAndDrain(t, svc, replies, 1, "checkin")
	answer(t, svc, replies, 1, "7:30")
	answer(t, svc, replies, 1, "30")
	answer(t, svc, replies, 1, "60")
	answer(t, svc, replies, 1, "6")
	answer(t, svc, replies, 1, "0")
	answer(t, svc, replies, 1, "силовая A")

	require.Len(t, saver.checkins, 1)
	checkin := saver.checkins[0]
	require.NotNil(t, checkin.SleepH)
	assert.InDelta(t, 7.5, *checkin.SleepH, 0.01)
	require.NotNil(t, checkin.HRV)
	assert.Equal(t, 30, *checkin.HRV)
	require.NotNil(t, checkin.RHR)
	assert.Equal(t, 60, *checkin.RHR)
	require.NotNil(t, checkin.Mood)
	assert.Equal(t, 6, *checkin.Mood)
	require.NotNil(t, checkin.AlcoholDoses)
	assert.InDelta(t, 0.0, *checkin.AlcoholDoses, 0.01)
	assert.Equal(t, "силовая A", checkin.PlannedTraining)
	assert.False(t, svc.HasActiveForm(1))
}

func TestFormInvalidFieldRetries(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)
	startAndDrain(t, svc, replies, 1, "checkin")

	require.True(t, svc.HandleFormAnswer(context.Background(), 1, "не число"))
	reply := waitReply(t, replies)
	assert.Contains(t, reply, "Попробуй ещё раз")
	assert.True(t, svc.HasActiveForm(1))
	assert.Empty(t, saver.checkins)
}

func TestFormCancel(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)
	startAndDrain(t, svc, replies, 1, "food")

	require.True(t, svc.CancelForm(context.Background(), 1))
	_ = waitReply(t, replies)
	assert.False(t, svc.HasActiveForm(1))
	assert.Empty(t, saver.nutrition)
}

func TestMeasureFormSaves(t *testing.T) {
	now := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)
	startAndDrain(t, svc, replies, 1, "measure")
	for _, value := range []string{"103.2", "34.1", "62.0", "15", "45.7", "112", "110 38.5 108 61 40"} {
		answer(t, svc, replies, 1, value)
	}
	require.Len(t, saver.measurements, 1)
	measurement := saver.measurements[0]
	require.NotNil(t, measurement.WeightKG)
	assert.InDelta(t, 103.2, *measurement.WeightKG, 0.01)
	require.NotNil(t, measurement.WaistCM)
	assert.InDelta(t, 112, *measurement.WaistCM, 0.01)
	assert.InDelta(t, 38.5, measurement.Tape["biceps"], 0.01)
}

func TestTrainingFormUsesReportParser(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)
	startAndDrain(t, svc, replies, 1, "training")
	for _, value := range []string{"sila-A", "присед 60x5x5, жим 40x8", "8", "45", "120"} {
		answer(t, svc, replies, 1, value)
	}
	require.Len(t, saver.trainings, 1)
	training := saver.trainings[0]
	assert.Equal(t, "sila-A", training.Kind)
	require.NotNil(t, training.RPESession)
	assert.Equal(t, 8, *training.RPESession)
	require.NotNil(t, training.DurationMin)
	assert.Equal(t, 45, *training.DurationMin)
	assert.Len(t, training.Exercises, 2)
}

func TestFormDoesNotMixWithPending(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	saver := &fakeSaver{}
	svc, replies := newSavingService(&fakeProvider{}, &fakeDownloader{}, saver, nil, now)
	svc.SubmitTraining(context.Background(), 1, "силовая A: присед 60x5x5")
	_ = waitReply(t, replies)
	startAndDrain(t, svc, replies, 1, "food")
	assert.True(t, svc.HasActiveForm(1))
	require.True(t, svc.HandleFormAnswer(context.Background(), 1, "2300"))
	_ = waitReply(t, replies)
}
