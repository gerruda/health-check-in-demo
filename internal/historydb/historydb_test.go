package historydb_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/historydb"
	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func openTestDB(t *testing.T) *historydb.DB {
	t.Helper()
	db, err := historydb.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRebuildFromMarkdown(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	hrv, rhr, mood := 30, 60, 5
	sleep := 7.0
	weight := 102.4
	_, err := writer.WriteCheckin(nlu.Checkin{
		Date: "2026-11-11", Person: nlu.Athlete,
		HRV: &hrv, RHR: &rhr, Mood: &mood, SleepH: &sleep, WeightKG: &weight,
	})
	require.NoError(t, err)
	kcal, protein, fat, carbs := 2300, 130, 82, 210
	_, err = writer.WriteNutrition(nlu.Nutrition{
		Date: "2026-11-11", Person: nlu.Athlete,
		Kcal: &kcal, ProteinG: &protein, FatG: &fat, CarbsG: &carbs,
	})
	require.NoError(t, err)
	duration, rpe := 45, 8
	exerciseWeight := 14.0
	sets, reps := 4, 10
	_, err = writer.WriteTraining(nlu.Training{
		Date: "2026-11-11", Person: nlu.Athlete, Kind: "sila-A", Done: true,
		DurationMin: &duration, RPESession: &rpe,
		Exercises: []nlu.Exercise{{Name: "жим гантелей", WeightKG: &exerciseWeight, Sets: &sets, Reps: &reps}},
	})
	require.NoError(t, err)

	db := openTestDB(t)
	require.NoError(t, db.Rebuild(context.Background(), root))

	values, err := db.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.NotNil(t, values[0].HRV)
	assert.Equal(t, 30, *values[0].HRV)
	require.NotNil(t, values[0].SleepH)
	assert.InDelta(t, 7.0, *values[0].SleepH, 0.01)
	require.NotNil(t, values[0].WeightKG)
	assert.InDelta(t, 102.4, *values[0].WeightKG, 0.01)

	nutrition, err := db.NutritionValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, nutrition, 1)
	require.NotNil(t, nutrition[0].Kcal)
	assert.Equal(t, 2300, *nutrition[0].Kcal)

	trainings, err := db.TrainingSessions(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, trainings, 1)
	assert.Equal(t, "sila-A", trainings[0].Kind)
	require.NotNil(t, trainings[0].RPESession)
	assert.Equal(t, 8, *trainings[0].RPESession)
	assert.Len(t, trainings[0].Exercises, 1)

	wife, err := db.CheckinValues(nlu.Wife, 7)
	require.NoError(t, err)
	assert.Empty(t, wife, "профили не смешиваются")
}

func TestUpsertWriteThrough(t *testing.T) {
	root := t.TempDir()
	db := openTestDB(t)
	writer := store.NewWriter(store.NewPaths(root), store.WithIndexUpdater(db))

	hrv := 35
	_, err := writer.WriteCheckin(nlu.Checkin{Date: "2026-11-12", Person: nlu.Athlete, HRV: &hrv})
	require.NoError(t, err)
	values, err := db.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 1)
	assert.Equal(t, 35, *values[0].HRV)

	kcal := 2400
	_, err = writer.WriteNutrition(nlu.Nutrition{Date: "2026-11-12", Person: nlu.Athlete, Kcal: &kcal})
	require.NoError(t, err)
	nutrition, err := db.NutritionValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, nutrition, 1)
	assert.Equal(t, 2400, *nutrition[0].Kcal)

	_, err = writer.WriteTraining(nlu.Training{Date: "2026-11-12", Person: nlu.Athlete, Kind: "ulichnaya", Done: true})
	require.NoError(t, err)
	trainings, err := db.TrainingSessions(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, trainings, 1)
	assert.Equal(t, "ulichnaya", trainings[0].Kind)
}

func TestLimitAndOrder(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	for _, date := range []string{"2026-11-01", "2026-11-02", "2026-11-03"} {
		hrv := 30
		_, err := writer.WriteCheckin(nlu.Checkin{Date: date, Person: nlu.Athlete, HRV: &hrv})
		require.NoError(t, err)
	}
	db := openTestDB(t)
	require.NoError(t, db.Rebuild(context.Background(), root))
	values, err := db.CheckinValues(nlu.Athlete, 2)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, "2026-11-02", values[0].Date.Format("2006-01-02"))
	assert.Equal(t, "2026-11-03", values[1].Date.Format("2006-01-02"))
}

func TestUpsertTrainingPreservesExercises(t *testing.T) {
	root := t.TempDir()
	db := openTestDB(t)
	writer := store.NewWriter(store.NewPaths(root), store.WithIndexUpdater(db))

	weight := 14.0
	sets, reps := 4, 10
	_, err := writer.WriteTraining(nlu.Training{
		Date: "2026-11-12", Person: nlu.Athlete, Kind: "sila-A", Done: true,
		Exercises: []nlu.Exercise{{Name: "жим гантелей", WeightKG: &weight, Sets: &sets, Reps: &reps}},
	})
	require.NoError(t, err)
	duration := 45
	_, err = writer.WriteTraining(nlu.Training{
		Date: "2026-11-12", Person: nlu.Athlete, Kind: "sila-A", Done: true, DurationMin: &duration,
	})
	require.NoError(t, err)

	trainings, err := db.TrainingSessions(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, trainings, 1)
	require.NotNil(t, trainings[0].DurationMin)
	assert.Equal(t, 45, *trainings[0].DurationMin)
	assert.Len(t, trainings[0].Exercises, 1, "упражнения не затёрты частичной записью")
}
