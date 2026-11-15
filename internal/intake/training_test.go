package intake_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/intake"
	"healthbot/internal/nlu"
)

func TestLooksLikeTraining(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"присед 60x5x5, жим 40x8, RPE 7", true},
		{"сделал силовую: присед 60x5x5", true},
		{"не сделал тренировку, устал", true},
		{"тренировка: жим гантелей 14 кг 4×10", true},
		{"сон 7:33, HRV 38, пульс 58", false},
		{"тренировка силовая", false},
		{"как дела?", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			assert.Equal(t, tt.want, intake.LooksLikeTraining(tt.text))
		})
	}
}

func TestParseTrainingStrength(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	text := "тренировка: силовая A, присед 60x5x5, жим гантелей 14 кг 4×10 RPE 7, RPE 8, 55 мин"

	training, ok := intake.ParseTraining(text, nlu.Athlete, now)
	require.True(t, ok)
	assert.Equal(t, "sila-A", training.Kind)
	assert.Equal(t, "2026-11-12", training.Date)
	require.Len(t, training.Exercises, 2)

	squat := training.Exercises[0]
	assert.Contains(t, squat.Name, "присед")
	require.NotNil(t, squat.WeightKG)
	assert.InDelta(t, 60, *squat.WeightKG, 0.01)
	require.NotNil(t, squat.Reps)
	assert.Equal(t, 5, *squat.Reps)
	require.NotNil(t, squat.Sets)
	assert.Equal(t, 5, *squat.Sets)

	press := training.Exercises[1]
	assert.Contains(t, press.Name, "жим")
	require.NotNil(t, press.WeightKG)
	assert.InDelta(t, 14, *press.WeightKG, 0.01)
	require.NotNil(t, press.Sets)
	assert.Equal(t, 4, *press.Sets)
	require.NotNil(t, press.Reps)
	assert.Equal(t, 10, *press.Reps)
	require.NotNil(t, press.RPE)
	assert.Equal(t, 7, *press.RPE)

	require.NotNil(t, training.DurationMin)
	assert.Equal(t, 55, *training.DurationMin)
	require.NotNil(t, training.RPESession)
	assert.Equal(t, 8, *training.RPESession)
	assert.True(t, training.Done)
}

func TestParseTrainingWifeCycle(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	training, ok := intake.ParseTraining("MC2 силовая: присед 40x3x10, самочувствие: тяжело", nlu.Wife, now)
	require.True(t, ok)
	assert.Equal(t, "MC2_silovaya", training.Kind)
	assert.Equal(t, nlu.Wife, training.Person)
	assert.Contains(t, training.Felt, "тяжело")
	require.Len(t, training.Exercises, 1)
}

func TestParseTrainingSkip(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	training, ok := intake.ParseTraining("не сделал тренировку, колено 5/10", nlu.Athlete, now)
	require.True(t, ok)
	assert.False(t, training.Done)
	require.NotNil(t, training.Pain)
	assert.Equal(t, 5, training.Pain["колено"])
}

func TestParseTrainingNaturalText(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	text := `Вчера была тренировка. Уличная. Суставная разминка затем подтягивания узким хватом 3 раза, отжимания на брусьях 10 раз, зашагивания на платформу с эспандером 15 раз на каждую ногу. Все упражнения по 3 подхода. RPE 8. Затем кардио подъем по лестнице на 32 этаж.`

	training, ok := intake.ParseTraining(text, nlu.Athlete, now)
	require.True(t, ok)
	assert.Equal(t, "ulichnaya", training.Kind)
	assert.Equal(t, "2026-11-11", training.Date, "«вчера» → предыдущий день")
	assert.True(t, training.Done)
	require.Len(t, training.Exercises, 4, "разминка + три упражнения")

	assert.Equal(t, "Суставная разминка", training.Exercises[0].Name)
	assert.Nil(t, training.Exercises[0].Reps)

	pullups := training.Exercises[1]
	assert.Equal(t, "подтягивания узким хватом", pullups.Name)
	require.NotNil(t, pullups.Reps)
	assert.Equal(t, 3, *pullups.Reps)
	require.NotNil(t, pullups.Sets)
	assert.Equal(t, 3, *pullups.Sets)

	dips := training.Exercises[2]
	assert.Equal(t, "отжимания на брусьях", dips.Name)
	require.NotNil(t, dips.Reps)
	assert.Equal(t, 10, *dips.Reps)

	steps := training.Exercises[3]
	assert.Contains(t, steps.Name, "зашагивания")
	require.NotNil(t, steps.Reps)
	assert.Equal(t, 15, *steps.Reps)
	assert.Contains(t, steps.Note, "на каждую ногу")

	require.NotNil(t, training.RPESession)
	assert.Equal(t, 8, *training.RPESession)
	assert.Contains(t, training.Cardio, "лестниц")
	assert.Contains(t, training.Cardio, "32")
}

func TestParseTrainingCardio(t *testing.T) {
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	training, ok := intake.ParseTraining("кардио 30 мин, средний пульс 130", nlu.Wife, now)
	require.True(t, ok)
	assert.Equal(t, "kardio", training.Kind)
	require.NotNil(t, training.DurationMin)
	assert.Equal(t, 30, *training.DurationMin)
	require.NotNil(t, training.AvgHR)
	assert.Equal(t, 130, *training.AvgHR)
	assert.NotEmpty(t, training.Cardio)
}
