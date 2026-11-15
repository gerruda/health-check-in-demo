package app

import (
	"fmt"
	"strings"

	"healthbot/internal/nlu"
	"healthbot/internal/persona"
	"healthbot/internal/settings"
	"healthbot/internal/store"
)

type nutritionTargets struct {
	kcal       int
	proteinMin int
	proteinMax int
	fatMax     int
	carbsMin   int
	carbsMax   int
}

func defaultNutritionTargets(person nlu.Person) nutritionTargets {
	if person == nlu.Wife {
		return nutritionTargets{kcal: 1850, proteinMin: 112, proteinMax: 112, fatMax: 85, carbsMin: 130, carbsMax: 170}
	}
	return nutritionTargets{kcal: 2300, proteinMin: 108, proteinMax: 135, fatMax: 90, carbsMin: 200, carbsMax: 250}
}

func nutritionTargetsFor(person nlu.Person, prefs settings.Settings) nutritionTargets {
	targets := defaultNutritionTargets(person)
	if prefs.Kcal != nil {
		targets.kcal = *prefs.Kcal
	}
	if prefs.ProteinMin != nil {
		targets.proteinMin = *prefs.ProteinMin
	}
	if prefs.ProteinMax != nil {
		targets.proteinMax = *prefs.ProteinMax
	}
	if prefs.FatMax != nil {
		targets.fatMax = *prefs.FatMax
	}
	if prefs.CarbsMin != nil {
		targets.carbsMin = *prefs.CarbsMin
	}
	if prefs.CarbsMax != nil {
		targets.carbsMax = *prefs.CarbsMax
	}
	return targets
}

// NutritionReply реализует intake.NutritionCoach (FR-3.5): текущие цифры vs цели,
// 7-дневное среднее и одна правка. Без морализаторства (FR-8).
func (h *commandHandlers) NutritionReply(chatID int64, nutrition nlu.Nutrition) (string, error) {
	personName := h.personFor(chatID)
	person := nlu.Athlete
	if personName == settings.PersonWife {
		person = nlu.Wife
	}
	prefs, err := h.settings.Load(personName)
	if err != nil {
		return "", err
	}
	targets := nutritionTargetsFor(person, prefs)

	current := []string{}
	if nutrition.Kcal != nil {
		current = append(current, fmt.Sprintf("%d/%d ккал", *nutrition.Kcal, targets.kcal))
	}
	if nutrition.ProteinG != nil {
		current = append(current, fmt.Sprintf("Б %d/%s", *nutrition.ProteinG, proteinGoal(targets)))
	}
	if nutrition.FatG != nil {
		current = append(current, fmt.Sprintf("Ж %d/≤%d", *nutrition.FatG, targets.fatMax))
	}
	if nutrition.CarbsG != nil {
		current = append(current, fmt.Sprintf("У %d/%s", *nutrition.CarbsG, carbsGoal(targets)))
	}
	lines := []string{"🌿 Питание: " + strings.Join(current, " · ")}

	values, err := h.reader.NutritionValues(person, 7)
	if err != nil {
		return "", err
	}
	if len(values) > 1 {
		parts := make([]string, 0, 2)
		if kcal := averageNutrition(values, func(value store.DailyNutrition) *int { return value.Kcal }); kcal != nil {
			parts = append(parts, fmt.Sprintf("%d ккал", *kcal))
		}
		if protein := averageNutrition(values, func(value store.DailyNutrition) *int { return value.ProteinG }); protein != nil {
			parts = append(parts, fmt.Sprintf("Б %d", *protein))
		}
		if len(parts) > 0 {
			lines = append(lines, fmt.Sprintf("7 дней: %s.", strings.Join(parts, " · ")))
		}
	}

	adjustment := nutritionAdjustment(nutrition, targets)
	lines = append(lines, "Правка: "+adjustment)
	lines = append(lines, "", persona.NutritionAside(adjustment))
	return strings.Join(lines, "\n"), nil
}

func proteinGoal(targets nutritionTargets) string {
	if targets.proteinMin == targets.proteinMax {
		return fmt.Sprintf("%d", targets.proteinMin)
	}
	return fmt.Sprintf("%d–%d", targets.proteinMin, targets.proteinMax)
}

func carbsGoal(targets nutritionTargets) string {
	if targets.carbsMin == targets.carbsMax {
		return fmt.Sprintf("%d", targets.carbsMin)
	}
	return fmt.Sprintf("%d–%d", targets.carbsMin, targets.carbsMax)
}

func nutritionAdjustment(nutrition nlu.Nutrition, targets nutritionTargets) string {
	switch {
	case nutrition.ProteinG != nil && *nutrition.ProteinG < targets.proteinMin:
		return "белка ниже цели — добавь 20–30 г (творог/рыба/курица), калории под это оставь в рамках."
	case nutrition.Kcal != nil && *nutrition.Kcal > targets.kcal+200:
		return "ккал заметно выше цели — завтра без перебора, белок держим приоритетом."
	case nutrition.FatG != nil && *nutrition.FatG > targets.fatMax:
		return "жиры выше лимита — чуть меньше масла/орехов, остальное по плану."
	case nutrition.Kcal != nil && *nutrition.Kcal < targets.kcal-400:
		return "ккал сильно ниже цели — добавь порцию белка и углеводов, чтобы не терять мышцы."
	default:
		return "в цели — держим ритм и белок; без давления."
	}
}
