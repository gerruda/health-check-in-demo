package analysis_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/analysis"
)

func TestAthleteNutritionTargets(t *testing.T) {
	targets := analysis.AthleteNutritionTargets()

	assert.Equal(t, 2300, targets.Kcal)
	assert.Equal(t, 2200, targets.KcalMin)
	assert.Equal(t, 2400, targets.KcalMax)
	assert.Equal(t, 108, targets.ProteinMin)
	assert.Equal(t, 135, targets.ProteinMax)
	assert.Equal(t, 80, targets.FatMin)
	assert.Equal(t, 90, targets.FatMax)
	assert.Equal(t, 200, targets.CarbsMin)
	assert.Equal(t, 250, targets.CarbsMax)
}

func TestWifeNutritionTargets(t *testing.T) {
	targets := analysis.WifeNutritionTargets("лютеиновая")

	assert.Equal(t, 1800, targets.KcalMin)
	assert.Equal(t, 1900, targets.KcalMax)
	assert.Equal(t, 112, targets.ProteinMin)
	assert.Equal(t, 112, targets.ProteinMax)
	assert.Equal(t, 75, targets.FatMin)
	assert.Equal(t, 85, targets.FatMax)
}

func TestWifeTargetsPhase(t *testing.T) {
	tests := []struct {
		phase string
		min   int
		max   int
	}{
		{"менструация", 120, 150},
		{"фолликулярная", 150, 170},
		{"овуляция", 150, 170},
		{"лютеиновая", 130, 150},
		{"menstrual", 120, 150},
		{"follicular", 150, 170},
		{"ovulation", 150, 170},
		{"luteal", 130, 150},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			targets := analysis.WifeNutritionTargets(tt.phase)
			assert.Equal(t, tt.min, targets.CarbsMin)
			assert.Equal(t, tt.max, targets.CarbsMax)
		})
	}
}

func TestCheckNutritionInTargets(t *testing.T) {
	result := analysis.CheckNutrition(analysis.NutritionInput{
		Kcal:     intPtr(2300),
		ProteinG: intPtr(130),
		FatG:     intPtr(82),
		CarbsG:   intPtr(210),
	}, analysis.AthleteNutritionTargets())

	assert.True(t, result.KcalOK)
	assert.True(t, result.ProteinOK)
	assert.True(t, result.FatOK)
	assert.True(t, result.CarbsOK)
	assert.Contains(t, result.Summary, "2300")
	assert.Contains(t, result.Summary, "130")
	assert.Contains(t, result.Summary, "82")
	assert.Contains(t, result.Summary, "210")
	assert.Empty(t, result.Fix, "в цели — правка не нужна")
}

func TestCheckNutritionOutOfRange(t *testing.T) {
	result := analysis.CheckNutrition(analysis.NutritionInput{
		Kcal:     intPtr(2600),
		ProteinG: intPtr(100),
		FatG:     intPtr(95),
		CarbsG:   intPtr(260),
	}, analysis.AthleteNutritionTargets())

	assert.False(t, result.KcalOK)
	assert.False(t, result.ProteinOK)
	assert.False(t, result.FatOK)
	assert.False(t, result.CarbsOK)
	assert.NotEmpty(t, result.Fix, "нужна одна конкретная правка")
	assert.True(t,
		strings.Contains(strings.ToLower(result.Fix), "ккал") ||
			strings.Contains(strings.ToLower(result.Fix), "белок"),
		"правка про первый вышедший за цель макрос: %q", result.Fix)
}

func TestCheckNutritionMissingValues(t *testing.T) {
	result := analysis.CheckNutrition(analysis.NutritionInput{
		Kcal: intPtr(2300),
	}, analysis.AthleteNutritionTargets())

	assert.True(t, result.KcalOK)
	assert.False(t, result.ProteinOK, "нет данных — не «в цели»")
	assert.Contains(t, result.Summary, "нет данных")
}
