package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func TestTrainingSessionsParsesFile(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	weight := 14.0
	sets, reps, rpe := 4, 10, 5
	duration, calories := 55, 320
	_, err := writer.WriteTraining(nlu.Training{
		Date: "2026-11-11", Person: nlu.Athlete, Kind: "sila-A", Done: true,
		DurationMin: &duration, Calories: &calories, RPESession: &rpe,
		Exercises: []nlu.Exercise{{Name: "жим гантелей", WeightKG: &weight, Sets: &sets, Reps: &reps, RPE: &rpe}},
		Felt:      "хорошо",
		Pain:      map[string]int{"плечо": 2},
	})
	require.NoError(t, err)

	reader := store.NewReader(store.NewPaths(root))
	sessions, err := reader.TrainingSessions(nlu.Athlete, 5)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	session := sessions[0]
	assert.Equal(t, "2026-11-11", session.Date)
	assert.Equal(t, "sila-A", session.Kind)
	assert.True(t, session.Done)
	require.NotNil(t, session.DurationMin)
	assert.Equal(t, 55, *session.DurationMin)
	require.NotNil(t, session.Calories)
	assert.Equal(t, 320, *session.Calories)
	assert.Equal(t, "хорошо", session.Felt)
	assert.Equal(t, 2, session.Pain["плечо"])
	require.Len(t, session.Exercises, 1)
	exercise := session.Exercises[0]
	assert.Equal(t, "жим гантелей", exercise.Name)
	require.NotNil(t, exercise.WeightKG)
	assert.InDelta(t, 14, *exercise.WeightKG, 0.01)
	require.NotNil(t, exercise.Sets)
	assert.Equal(t, 4, *exercise.Sets)
	require.NotNil(t, exercise.Reps)
	assert.Equal(t, 10, *exercise.Reps)
}

func TestTrainingSessionsLimitAndSkip(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	for _, date := range []string{"2026-11-01", "2026-11-05", "2026-11-10"} {
		_, err := writer.WriteTraining(nlu.Training{Date: date, Person: nlu.Athlete, Kind: "sila-A", Done: true})
		require.NoError(t, err)
	}
	_, err := writer.WriteTraining(nlu.Training{
		Date: "2026-11-09", Person: nlu.Athlete, Kind: "fitboxing", Done: false, Felt: "устал",
	})
	require.NoError(t, err)

	reader := store.NewReader(store.NewPaths(root))
	sessions, err := reader.TrainingSessions(nlu.Athlete, 2)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	assert.Equal(t, "2026-11-09", sessions[0].Date)
	assert.False(t, sessions[0].Done)
	assert.Equal(t, "2026-11-10", sessions[1].Date)
}

var _ = time.Now
