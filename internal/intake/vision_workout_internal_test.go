package intake

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeaningfulExerciseNameDropsTableHeaders(t *testing.T) {
	for _, name := range []string{"Подход 1", "Упражнение 2", "Сет 3", "", "—"} {
		assert.False(t, meaningfulExerciseName(name), name)
	}
	assert.True(t, meaningfulExerciseName("присед"))
	assert.True(t, meaningfulExerciseName("жим гантелей"))
}

func TestNormalizeTrainingDropsGenericExercises(t *testing.T) {
	var wire wireTraining
	require.NoError(t, json.Unmarshal([]byte(`{
		"duration_min":45,"avg_hr":123,"calories":195,
		"exercises":[
			{"name":"Подход 1","sets":3,"reps":10},
			{"name":"присед","sets":5,"reps":5}
		]
	}`), &wire))
	training := normalizeTraining(&wire)
	require.Len(t, training.Exercises, 1)
	assert.Equal(t, "присед", training.Exercises[0].Name)
}
