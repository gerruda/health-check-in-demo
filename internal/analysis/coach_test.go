package analysis_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/analysis"
)

func countLines(text string) int {
	return len(strings.Split(strings.TrimSpace(text), "\n"))
}

func TestCheckinReplyGreen(t *testing.T) {
	reply := analysis.CheckinReply(analysis.CheckinReplyInput{
		Zone: analysis.Result{
			Zone:   analysis.ZoneGreen,
			Reason: "HRV 35 мс (≥ 31) и пульс 57 (≤ 60) — по плану",
		},
		HRV:        analysis.TrendHRV([]int{33, 34, 35}),
		ZoneCounts: analysis.ZoneCounts{Green: 5, Yellow: 2, Red: 0},
	})

	assert.Contains(t, reply, "🟢")
	assert.Contains(t, reply, "HRV_7")
	assert.Contains(t, reply, "по плану")
	assert.LessOrEqual(t, countLines(reply), 10)
}

func TestCheckinReplyYellowNotes(t *testing.T) {
	reply := analysis.CheckinReply(analysis.CheckinReplyInput{
		Zone: analysis.Result{
			Zone:   analysis.ZoneYellow,
			Reason: "сон 6.00 ч < 6.5 ч — недосып",
			Notes:  []string{"колени 5/10 — убрать нагрузку на колени"},
		},
		HRV:  analysis.TrendHRV([]int{31, 30, 29}),
		Plan: "одна лёгкая, без сдвоенных",
	})

	assert.Contains(t, reply, "🟡")
	assert.Contains(t, reply, "колени 5/10")
	assert.Contains(t, reply, "одна лёгкая")
	assert.LessOrEqual(t, countLines(reply), 10)
}

func TestCheckinReplyRedFlag(t *testing.T) {
	reply := analysis.CheckinReply(analysis.CheckinReplyInput{
		Zone: analysis.Result{Zone: analysis.ZoneRed, Reason: "пульс 68 > 62 — красная зона"},
		Flag: "пульс +10 к базе 2 дня подряд",
	})

	assert.Contains(t, reply, "🔴")
	assert.Contains(t, reply, "отдых")
	assert.Contains(t, strings.ToLower(reply), "сигнал")
	assert.Contains(t, reply, "пульс +10")
	assert.LessOrEqual(t, countLines(reply), 10)
}

func TestCheckinReplyNoTrend(t *testing.T) {
	reply := analysis.CheckinReply(analysis.CheckinReplyInput{
		Zone: analysis.Result{Zone: analysis.ZoneGreen, Reason: "в норме"},
	})

	assert.Contains(t, reply, "HRV_7")
	assert.Contains(t, reply, "нет данных")
}

func TestNutritionReplyInTargets(t *testing.T) {
	result := analysis.CheckNutrition(analysis.NutritionInput{
		Kcal:     intPtr(2300),
		ProteinG: intPtr(130),
		FatG:     intPtr(82),
		CarbsG:   intPtr(210),
	}, analysis.AthleteNutritionTargets())

	reply := analysis.NutritionReply(analysis.NutritionReplyInput{
		Result:   result,
		Averages: "за 7 дней: 2280 ккал",
		Alcohol:  "нет",
	})

	assert.Contains(t, reply, "2300")
	assert.Contains(t, reply, "✅")
	assert.Contains(t, reply, "2280")
	assert.Contains(t, reply, "Алкоголь")
	assert.NotContains(t, strings.ToLower(reply), "корректировка:")
	assert.LessOrEqual(t, countLines(reply), 8)
}

func TestNutritionReplyOutOfRange(t *testing.T) {
	result := analysis.CheckNutrition(analysis.NutritionInput{
		Kcal:     intPtr(2600),
		ProteinG: intPtr(100),
		FatG:     intPtr(95),
		CarbsG:   intPtr(260),
	}, analysis.AthleteNutritionTargets())

	reply := analysis.NutritionReply(analysis.NutritionReplyInput{Result: result})

	assert.Contains(t, reply, "⚠️")
	assert.Contains(t, strings.ToLower(reply), "корректировка:")
	assert.LessOrEqual(t, countLines(reply), 8)
}
