package analysis

import (
	"fmt"
	"strings"
)

// NutritionTargets — цели по ккал/макросам (контракт §4).
type NutritionTargets struct {
	Kcal       int
	KcalMin    int
	KcalMax    int
	ProteinMin int
	ProteinMax int
	FatMin     int
	FatMax     int
	CarbsMin   int
	CarbsMax   int
}

// AthleteNutritionTargets — цели атлета: 2300 ккал (2200–2400),
// Б 108–135, Ж 80–90, У 200–250 (контракт §4).
func AthleteNutritionTargets() NutritionTargets {
	return NutritionTargets{
		Kcal: 2300, KcalMin: 2200, KcalMax: 2400,
		ProteinMin: 108, ProteinMax: 135,
		FatMin: 80, FatMax: 90,
		CarbsMin: 200, CarbsMax: 250,
	}
}

// WifeNutritionTargets — цели жены: 1800–1900 ккал, Б 112, Ж 75–85,
// углеводы по фазе МЦ (контракт §4).
func WifeNutritionTargets(cyclePhase string) NutritionTargets {
	targets := NutritionTargets{
		Kcal: 1850, KcalMin: 1800, KcalMax: 1900,
		ProteinMin: 112, ProteinMax: 112,
		FatMin: 75, FatMax: 85,
		CarbsMin: 130, CarbsMax: 170,
	}

	switch normalizeCyclePhase(cyclePhase) {
	case "menstrual":
		targets.CarbsMin, targets.CarbsMax = 120, 150
	case "follicular":
		targets.CarbsMin, targets.CarbsMax = 150, 170
	case "ovulation":
		targets.CarbsMin, targets.CarbsMax = 150, 170
	case "luteal":
		targets.CarbsMin, targets.CarbsMax = 130, 150
	}
	return targets
}

// NutritionInput — фактические значения за день (nil = нет данных).
type NutritionInput struct {
	Kcal     *int
	ProteinG *int
	FatG     *int
	CarbsG   *int
}

// MetricCheck — результат проверки одного макроса.
type MetricCheck struct {
	Name  string
	Value *int
	Min   int
	Max   int
	OK    bool
}

// NutritionResult — флаги по макросам, сводка и одна конкретная правка.
type NutritionResult struct {
	KcalOK    bool
	ProteinOK bool
	FatOK     bool
	CarbsOK   bool
	Checks    []MetricCheck
	Summary   string
	Fix       string
}

// CheckNutrition сравнивает факт с целями (FR-3.5) и предлагает одну правку.
func CheckNutrition(input NutritionInput, targets NutritionTargets) NutritionResult {
	result := NutritionResult{
		Checks: []MetricCheck{
			checkMetric("ккал", input.Kcal, targets.KcalMin, targets.KcalMax),
			checkMetric("белок", input.ProteinG, targets.ProteinMin, targets.ProteinMax),
			checkMetric("жиры", input.FatG, targets.FatMin, targets.FatMax),
			checkMetric("углеводы", input.CarbsG, targets.CarbsMin, targets.CarbsMax),
		},
	}
	result.KcalOK = result.Checks[0].OK
	result.ProteinOK = result.Checks[1].OK
	result.FatOK = result.Checks[2].OK
	result.CarbsOK = result.Checks[3].OK
	result.Summary = nutritionSummary(result.Checks)
	result.Fix = nutritionFix(input, targets)
	return result
}

func checkMetric(name string, value *int, minValue, maxValue int) MetricCheck {
	check := MetricCheck{Name: name, Value: value, Min: minValue, Max: maxValue}
	if value != nil && *value >= minValue && *value <= maxValue {
		check.OK = true
	}
	return check
}

func nutritionSummary(checks []MetricCheck) string {
	parts := make([]string, 0, len(checks))
	labels := []string{"ккал", "Б", "Ж", "У"}
	for index, check := range checks {
		if check.Value == nil {
			parts = append(parts, labels[index]+" нет данных")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d (%d–%d)", labels[index], *check.Value, check.Min, check.Max))
	}
	return strings.Join(parts, " · ")
}

// nutritionFix возвращает одну правку по первому вышедшему за цель макросу.
func nutritionFix(input NutritionInput, targets NutritionTargets) string {
	if input.Kcal != nil && *input.Kcal > targets.KcalMax {
		return fmt.Sprintf("корректировка: ккал — снизить на %d ккал (цель ≤ %d)", *input.Kcal-targets.KcalMax, targets.KcalMax)
	}
	if input.Kcal != nil && *input.Kcal < targets.KcalMin {
		return fmt.Sprintf("корректировка: ккал — добавить %d ккал (цель ≥ %d)", targets.KcalMin-*input.Kcal, targets.KcalMin)
	}
	if input.ProteinG != nil && *input.ProteinG < targets.ProteinMin {
		return fmt.Sprintf("корректировка: белок — добавить %d г (цель ≥ %d г)", targets.ProteinMin-*input.ProteinG, targets.ProteinMin)
	}
	if input.ProteinG != nil && *input.ProteinG > targets.ProteinMax {
		return fmt.Sprintf("корректировка: белок — снизить на %d г (цель ≤ %d г)", *input.ProteinG-targets.ProteinMax, targets.ProteinMax)
	}
	if input.FatG != nil && *input.FatG > targets.FatMax {
		return fmt.Sprintf("корректировка: жиры — снизить на %d г (цель ≤ %d г)", *input.FatG-targets.FatMax, targets.FatMax)
	}
	if input.FatG != nil && *input.FatG < targets.FatMin {
		return fmt.Sprintf("корректировка: жиры — добавить %d г (цель ≥ %d г)", targets.FatMin-*input.FatG, targets.FatMin)
	}
	if input.CarbsG != nil && *input.CarbsG > targets.CarbsMax {
		return fmt.Sprintf("корректировка: углеводы — снизить на %d г (цель ≤ %d г)", *input.CarbsG-targets.CarbsMax, targets.CarbsMax)
	}
	if input.CarbsG != nil && *input.CarbsG < targets.CarbsMin {
		return fmt.Sprintf("корректировка: углеводы — добавить %d г (цель ≥ %d г)", targets.CarbsMin-*input.CarbsG, targets.CarbsMin)
	}
	return ""
}

func normalizeCyclePhase(phase string) string {
	lower := strings.ToLower(strings.TrimSpace(phase))
	switch {
	case lower == "m", strings.Contains(lower, "менстру"), strings.Contains(lower, "menstr"):
		return "menstrual"
	case lower == "f", strings.Contains(lower, "фолликул"), strings.Contains(lower, "follic"):
		return "follicular"
	case lower == "o", strings.Contains(lower, "овул"), strings.Contains(lower, "ovul"):
		return "ovulation"
	case lower == "l", strings.Contains(lower, "лютеин"), strings.Contains(lower, "luteal"):
		return "luteal"
	default:
		return ""
	}
}
