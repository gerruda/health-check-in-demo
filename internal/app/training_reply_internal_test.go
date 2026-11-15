package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func trainingSession(date string, weight float64, rpe int) nlu.Training {
	exerciseWeight := weight
	sets, reps := 4, 10
	exerciseRPE := rpe
	return nlu.Training{
		Date: date, Person: nlu.Athlete, Kind: "sila-A", Done: true, RPESession: &exerciseRPE,
		Exercises: []nlu.Exercise{{
			Name: "жим гантелей", WeightKG: &exerciseWeight, Sets: &sets, Reps: &reps, RPE: &exerciseRPE,
		}},
	}
}

func TestTrainingReplyProgress(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))
	_, writeErr := writer.WriteTraining(trainingSession("2026-11-10", 14, 5))
	require.NoError(t, writeErr)
	current := trainingSession("2026-11-11", 14, 5)
	_, writeErr = writer.WriteTraining(current)
	require.NoError(t, writeErr)
	hrv, rhr, mood := 35, 57, 7
	sleep := 7.5
	_, writeErr = writer.WriteCheckin(nlu.Checkin{Date: "2026-11-11", Person: nlu.Athlete, HRV: &hrv, RHR: &rhr, SleepH: &sleep, Mood: &mood})
	require.NoError(t, writeErr)
	if _, setErr := handlers.Set(1, "hrv_base", "33"); setErr != nil {
		t.Fatal(setErr)
	}

	reply, replyErr := handlers.TrainingReply(1, current)
	require.NoError(t, replyErr)
	assert.Contains(t, reply, "жим гантелей")
	assert.Contains(t, reply, "16")
	assert.Contains(t, reply, "Следующая")
}

func TestTrainingReplyDeload(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))
	_, writeErr := writer.WriteTraining(trainingSession("2026-11-10", 14, 7))
	require.NoError(t, writeErr)
	current := trainingSession("2026-11-11", 14, 9)
	_, writeErr = writer.WriteTraining(current)
	require.NoError(t, writeErr)
	hrv, rhr, mood := 30, 60, 5
	sleep := 6.0
	_, writeErr = writer.WriteCheckin(nlu.Checkin{Date: "2026-11-11", Person: nlu.Athlete, HRV: &hrv, RHR: &rhr, SleepH: &sleep, Mood: &mood})
	require.NoError(t, writeErr)
	if _, setErr := handlers.Set(1, "hrv_base", "33"); setErr != nil {
		t.Fatal(setErr)
	}

	reply, replyErr := handlers.TrainingReply(1, current)
	require.NoError(t, replyErr)
	assert.Contains(t, reply, "Разгрузка")
	assert.Contains(t, reply, "−25")
}
