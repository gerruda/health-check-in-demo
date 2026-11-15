package analysis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/analysis"
	"healthbot/internal/nlu"
)

func trainingWith(exercise string, weight float64, sets, reps, rpe int, kind string) nlu.Training {
	exerciseName := exercise
	exerciseWeight := weight
	exerciseSets := sets
	exerciseReps := reps
	exerciseRPE := rpe
	sessionRPE := rpe
	return nlu.Training{
		Kind:       kind,
		RPESession: &sessionRPE,
		Exercises: []nlu.Exercise{{
			Name: exerciseName, WeightKG: &exerciseWeight,
			Sets: &exerciseSets, Reps: &exerciseReps, RPE: &exerciseRPE,
		}},
	}
}

func TestNextSessionU8DumbbellRPE5(t *testing.T) {
	previous := trainingWith("жим гантелей", 14, 4, 10, 5, "sila-A")
	current := trainingWith("жим гантелей", 14, 4, 10, 5, "sila-A")

	plan := analysis.NextSession(&previous, &current, analysis.ZoneGreen, []int{5, 5})
	assert.False(t, plan.Deload)
	require.Len(t, plan.Exercises, 1)
	require.NotNil(t, plan.Exercises[0].WeightKG)
	assert.InDelta(t, 16, *plan.Exercises[0].WeightKG, 0.01, "U8: +2 кг на гантели")
	assert.Contains(t, plan.Exercises[0].Change, "+2")
}

func TestNextSessionU9Deload(t *testing.T) {
	previous := trainingWith("присед", 60, 5, 5, 7, "sila-A")
	current := trainingWith("присед", 60, 5, 5, 9, "sila-A")

	plan := analysis.NextSession(&previous, &current, analysis.ZoneYellow, []int{7, 8, 9})
	assert.True(t, plan.Deload)
	assert.Equal(t, -25, plan.VolumeDeltaPercent)
	require.Len(t, plan.Exercises, 1)
	require.NotNil(t, plan.Exercises[0].WeightKG)
	assert.InDelta(t, 60, *plan.Exercises[0].WeightKG, 0.01, "вес держим")
	require.NotNil(t, plan.Exercises[0].Sets)
	assert.Less(t, *plan.Exercises[0].Sets, 5, "объём снижаем")
}

func TestNextSessionRedZoneDeload(t *testing.T) {
	current := trainingWith("присед", 60, 5, 5, 8, "sila-A")
	plan := analysis.NextSession(nil, &current, analysis.ZoneRed, nil)
	assert.True(t, plan.Deload)
}

func TestNextSessionBarbellRPE5(t *testing.T) {
	previous := trainingWith("присед", 60, 5, 5, 5, "sila-A")
	current := trainingWith("присед", 60, 5, 5, 5, "sila-A")
	plan := analysis.NextSession(&previous, &current, analysis.ZoneGreen, []int{5, 5})
	require.Len(t, plan.Exercises, 1)
	require.NotNil(t, plan.Exercises[0].WeightKG)
	assert.InDelta(t, 62.5, *plan.Exercises[0].WeightKG, 0.01, "U8/FR-3.4: +2.5 кг штанга")
}

func TestNextSessionBodyweight(t *testing.T) {
	previous := trainingWith("подтягивания", 0, 3, 3, 6, "ulichnaya")
	previous.Exercises[0].WeightKG = nil
	current := trainingWith("подтягивания", 0, 3, 3, 6, "ulichnaya")
	current.Exercises[0].WeightKG = nil
	plan := analysis.NextSession(&previous, &current, analysis.ZoneGreen, nil)
	require.Len(t, plan.Exercises, 1)
	assert.Nil(t, plan.Exercises[0].WeightKG)
	require.NotNil(t, plan.Exercises[0].Reps)
	assert.Equal(t, 4, *plan.Exercises[0].Reps, "без веса добавляем повтор")
}

func TestTrainingVolume(t *testing.T) {
	training := trainingWith("жим", 14, 4, 10, 7, "sila-A")
	assert.InDelta(t, 560, analysis.TrainingVolume(training), 0.01)
}
