package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func intPtrNutrition(value int) *int { return &value }

func TestNutritionReplyGoalsAndWeeklyAverage(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))
	for _, date := range []string{"2026-11-06", "2026-11-07", "2026-11-08"} {
		kcal, protein, fat, carbs := 2200, 120, 80, 210
		_, err := writer.WriteNutrition(nlu.Nutrition{
			Date: date, Person: nlu.Athlete,
			Kcal: &kcal, ProteinG: &protein, FatG: &fat, CarbsG: &carbs,
		})
		require.NoError(t, err)
	}

	reply, err := handlers.NutritionReply(1, nlu.Nutrition{
		Date: "2026-11-08", Person: nlu.Athlete,
		Kcal: intPtrNutrition(2450), ProteinG: intPtrNutrition(95), FatG: intPtrNutrition(75), CarbsG: intPtrNutrition(230),
	})
	require.NoError(t, err)
	assert.Contains(t, reply, "2450/2300 ккал")
	assert.Contains(t, reply, "Б 95/108–135")
	assert.Contains(t, reply, "7 дней:")
	assert.Contains(t, reply, "белка ниже цели")
	assert.Contains(t, reply, "вернёмся в ритм", "живая фраза к правке")
}

func TestNutritionReplyWifeTargets(t *testing.T) {
	handlers, _ := newTestHandlers(t)
	reply, err := handlers.NutritionReply(2, nlu.Nutrition{
		Date: "2026-11-08", Person: nlu.Wife,
		Kcal: intPtrNutrition(1850), ProteinG: intPtrNutrition(112), FatG: intPtrNutrition(80), CarbsG: intPtrNutrition(150),
	})
	require.NoError(t, err)
	assert.Contains(t, reply, "1850/1850 ккал")
	assert.Contains(t, reply, "Б 112/112")
	assert.Contains(t, reply, "в цели")
}
