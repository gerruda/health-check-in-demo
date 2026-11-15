package intake_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
	"healthbot/internal/nlu"
)

var combinedMessage = `запиши сегодняшний чекин: чсс в покое 60, вариабильность чсс 30, длительность сна 7часов. утром была уличная тренировка, упражнения подтягивания 2 раза, отжимания на брусьях 8 повторений, удары с эспандером по 15 раз на руку и на удар. удары джеб, кросс, хук. тяга эспандера к груди 15 раз. все упражнения 2 подхода. затем подъем по лестнице на 32 этаж`

func TestParseCombinedUserMessage(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	checkin, ok := intake.ParseCheckinForTest(combinedMessage, now)
	require.True(t, ok)
	require.NotNil(t, checkin.Checkin)
	require.NotNil(t, checkin.Checkin.HRV)
	assert.Equal(t, 30, *checkin.Checkin.HRV)
	require.NotNil(t, checkin.Checkin.RHR)
	assert.Equal(t, 60, *checkin.Checkin.RHR)
	require.NotNil(t, checkin.Checkin.SleepH)
	assert.InDelta(t, 7.0, *checkin.Checkin.SleepH, 0.01)

	training, ok := intake.ParseTraining(combinedMessage, nlu.Athlete, now)
	require.True(t, ok)
	assert.Equal(t, "ulichnaya", training.Kind)
	require.Len(t, training.Exercises, 4)
	assert.Contains(t, training.Exercises[0].Name, "подтягивания")
	require.NotNil(t, training.Exercises[0].Reps)
	assert.Equal(t, 2, *training.Exercises[0].Reps)
	assert.Contains(t, training.Exercises[2].Name, "удар")
	assert.Contains(t, training.Exercises[2].Name, "джеб")
	require.NotNil(t, training.Exercises[2].Reps)
	assert.Equal(t, 15, *training.Exercises[2].Reps)
	assert.Contains(t, training.Cardio, "лестниц")
}
