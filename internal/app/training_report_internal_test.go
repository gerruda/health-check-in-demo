package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func ptrIntTest(value int) *int { return &value }

func reportExercise(name string, sets, reps int) nlu.Exercise {
	setsValue := sets
	repsValue := reps
	return nlu.Exercise{Name: name, Sets: &setsValue, Reps: &repsValue}
}

func TestTrainingReportTemplate(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	handlers.now = func() time.Time { return time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC) }
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))

	previous := nlu.Training{
		Date: "2026-11-10", Person: nlu.Athlete, Kind: "ulichnaya", Done: true,
		Exercises: []nlu.Exercise{reportExercise("подтягивания узким хватом", 3, 3), reportExercise("отжимания на брусьях", 3, 8)},
	}
	_, err := writer.WriteTraining(previous)
	require.NoError(t, err)

	hrv, rhr := 30, 60
	sleep := 7.0
	_, err = writer.WriteCheckin(nlu.Checkin{Date: "2026-11-12", Person: nlu.Athlete, HRV: &hrv, RHR: &rhr, SleepH: &sleep})
	require.NoError(t, err)
	if _, setErr := handlers.Set(1, "hrv_base", "33"); setErr != nil {
		t.Fatal(setErr)
	}

	current := nlu.Training{
		Date: "2026-11-12", Person: nlu.Athlete, Kind: "ulichnaya", Done: true,
		Cardio: "подъем по лестнице на 32 этаж",
		Exercises: []nlu.Exercise{
			{Name: "Суставная разминка", Note: "разминка"},
			reportExercise("подтягивания узким хватом", 2, 2),
			reportExercise("отжимания на брусьях", 2, 8),
			{Name: "Удары с эспандером (джеб, кросс, хук)", Sets: ptrIntTest(2), Reps: ptrIntTest(15), Note: "на руку и на удар"},
			reportExercise("тяга эспандера к груди", 2, 15),
		},
	}
	report, err := handlers.TrainingReport(1, current)
	require.NoError(t, err)

	assert.Contains(t, report, "# Уличная тренировка: 12.11.2026")
	assert.Contains(t, report, "## Утренние показатели")
	assert.Contains(t, report, "HRV: 30 мс")
	assert.Contains(t, report, "Пульс покоя: 60 уд/мин")
	assert.Contains(t, report, "## Разминка/заминка")
	assert.Contains(t, report, "## Основные упражнения (2 подхода)")
	assert.Contains(t, report, "| Упражнение | Подходы | Повторения | RPE | Примечания |")
	assert.Contains(t, report, "Суставная разминка")
	assert.Contains(t, report, "## Кардио-дополнение")
	assert.Contains(t, report, "## Общая статистика")
	assert.Contains(t, report, "## Анализ")
	assert.Contains(t, report, "## Прогрессия (последние 6 уличных тренировок)")
	assert.Contains(t, report, "| Упражнение |")
}

func TestSplitWarmupUsesNameNotLLMNote(t *testing.T) {
	warmup, main := splitWarmup([]nlu.Exercise{
		{Name: "подтягивания узким хватом", Note: "разминка"},
		{Name: "Суставная разминка", Note: "разминка"},
	})
	require.Len(t, warmup, 1)
	assert.Contains(t, warmup[0].Name, "Суставная")
	require.Len(t, main, 1)
	assert.Contains(t, main[0].Name, "подтягивания")
}

func TestTrainingReportWithoutCheckinDoesNotInventMorningMetrics(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	handlers.now = func() time.Time { return time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC) }
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))

	// чек-ин есть только за предыдущий день, и он «плохой»
	hrv, rhr := 34, 120
	_, err := writer.WriteCheckin(nlu.Checkin{Date: "2026-11-13", Person: nlu.Athlete, HRV: &hrv, RHR: &rhr})
	require.NoError(t, err)

	current := nlu.Training{
		Date: "2026-11-14", Person: nlu.Athlete, Kind: "sila-A", Done: true,
		Exercises: []nlu.Exercise{reportExercise("присед", 5, 5)},
	}
	report, err := handlers.TrainingReport(1, current)
	require.NoError(t, err)
	assert.Contains(t, report, "нет данных чек-ина")
	assert.NotContains(t, report, "HRV: 34")
	assert.NotContains(t, report, "Пульс покоя: 120")
}

func TestTrainingReportRoundTripPreservesTraining(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	handlers.now = func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))

	duration, rpe, avgHR, calories := 45, 7, 123, 195
	current := nlu.Training{
		Date: "2026-11-14", Person: nlu.Athlete, Kind: "ulichnaya", Done: true,
		DurationMin: &duration, RPESession: &rpe, AvgHR: &avgHR, Calories: &calories,
		Cardio: "лестница 32 этаж", Felt: "тяжело", Pain: map[string]int{"плечо": 3},
		Exercises: []nlu.Exercise{reportExercise("подтягивания", 3, 3)},
	}
	report, err := handlers.TrainingReport(1, current)
	require.NoError(t, err)
	assert.Contains(t, report, "- Длительность: 45 мин")
	assert.Contains(t, report, "- Калории: 195 ккал")
	assert.Contains(t, report, "## Боль")
	assert.Contains(t, report, "## Самочувствие")

	_, err = writer.WriteTrainingReport(current, report)
	require.NoError(t, err)
	reader := store.NewReader(store.NewPaths(cfg.DataRoot))
	sessions, err := reader.TrainingSessions(nlu.Athlete, 5)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	got := sessions[0]
	require.NotNil(t, got.DurationMin)
	assert.Equal(t, 45, *got.DurationMin)
	require.NotNil(t, got.AvgHR)
	assert.Equal(t, 123, *got.AvgHR)
	require.NotNil(t, got.Calories)
	assert.Equal(t, 195, *got.Calories)
	assert.Equal(t, "лестница 32 этаж", got.Cardio)
	assert.Equal(t, "тяжело", got.Felt)
	assert.Equal(t, 3, got.Pain["плечо"])
	require.Len(t, got.Exercises, 1)
	assert.Equal(t, "подтягивания", got.Exercises[0].Name)
}
