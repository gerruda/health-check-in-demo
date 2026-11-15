package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func TestWeekSummary(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	now := time.Date(2026, 11, 12, 8, 0, 0, 0, time.Local)
	handlers.now = func() time.Time { return now }

	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))

	writeCheckin := func(date string, weight, hrv, rhr, sleep float64, mood int) {
		weightValue := weight
		hrvValue := int(hrv)
		rhrValue := int(rhr)
		moodValue := mood
		_, err := writer.WriteCheckin(nlu.Checkin{
			Date: date, Person: nlu.Athlete,
			WeightKG: &weightValue, HRV: &hrvValue, RHR: &rhrValue, SleepH: &sleep, Mood: &moodValue,
		})
		require.NoError(t, err)
	}
	writeCheckin("2026-11-06", 103.0, 30, 60, 6.5, 5)
	writeCheckin("2026-11-12", 102.4, 35, 57, 7.5, 7)
	writeCheckin("2026-10-01", 105.0, 25, 70, 5.0, 3) // вне недели

	kcal, protein, fat, carbs := 2300, 130, 82, 210
	_, err := writer.WriteNutrition(nlu.Nutrition{
		Date: "2026-11-12", Person: nlu.Athlete,
		Kcal: &kcal, ProteinG: &protein, FatG: &fat, CarbsG: &carbs,
	})
	require.NoError(t, err)

	duration := 55
	rpe := 8
	_, err = writer.WriteTraining(nlu.Training{
		Date: "2026-11-11", Person: nlu.Athlete, Kind: "sila-A", Done: true,
		DurationMin: &duration, RPESession: &rpe,
	})
	require.NoError(t, err)
	_, err = writer.WriteTraining(nlu.Training{
		Date: "2026-11-09", Person: nlu.Athlete, Kind: "fitboxing", Done: false,
		Felt: "устал",
	})
	require.NoError(t, err)

	if _, setErr := handlers.Set(1, "hrv_base", "33"); setErr != nil {
		t.Fatal(setErr)
	}

	reply, err := handlers.Week(context.Background(), 1)
	require.NoError(t, err)
	assert.Contains(t, reply, "Неделя")
	assert.Contains(t, reply, "06.11.2026–12.11.2026")
	assert.Contains(t, reply, "HRV")
	assert.Contains(t, reply, "🟢")
	assert.Contains(t, reply, "2300")
	assert.Contains(t, reply, "Тренировки: 2")
	assert.Contains(t, reply, "55 мин")
	assert.Contains(t, reply, "102.4")
	assert.Contains(t, reply, "Ориентир")
	assert.NotContains(t, reply, "105.0", "старые данные вне недели не попадают")
	assert.Contains(t, reply, "следующий шаг", "живая фраза к неделе")
}

func TestWeekSummaryNoData(t *testing.T) {
	handlers, _ := newTestHandlers(t)
	handlers.now = func() time.Time { return time.Date(2026, 11, 12, 8, 0, 0, 0, time.Local) }

	reply, err := handlers.Week(context.Background(), 1)
	require.NoError(t, err)
	assert.Contains(t, reply, "нет данных")
	assert.Contains(t, reply, "пустая", "живая фраза к пустой неделе")
}
