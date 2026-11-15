package app

import (
	"context"
	"fmt"
	"strings"

	"healthbot/internal/analysis"
	"healthbot/internal/nlu"
	"healthbot/internal/persona"
	"healthbot/internal/settings"
	"healthbot/internal/store"
)

// Review реализует воскресный разбор (FR-3.7): анализ недели, замеры и
// стратегия на следующую. Тот же текст отправляет автопланировщик.
func (h *commandHandlers) Review(ctx context.Context, chatID int64) (string, error) {
	personName := h.personFor(chatID)
	person := nlu.Athlete
	if personName == settings.PersonWife {
		person = nlu.Wife
	}
	week, err := h.Week(ctx, chatID)
	if err != nil {
		return "", err
	}
	measurements, err := h.reader.Measurements(person, 5)
	if err != nil {
		return "", err
	}
	values, err := h.reader.CheckinValues(person, 14)
	if err != nil {
		return "", err
	}
	trainings, err := h.reader.TrainingFiles(person)
	if err != nil {
		return "", err
	}
	nutrition, err := h.reader.NutritionValues(person, 7)
	if err != nil {
		return "", err
	}

	lines := []string{"🌿 Воскресный разбор", "", "Анализ недели:", week, ""}
	lines = append(lines, measurementAnalysis(measurements)...)
	lines = append(lines, "Стратегия на следующую неделю:")
	lines = append(lines, strategyLines(h, person, personName, values, nutrition, trainings, measurements)...)
	lines = append(lines, "", persona.ReviewAside())
	return strings.Join(lines, "\n"), nil
}

func measurementAnalysis(measurements []nlu.Measurement) []string {
	if len(measurements) == 0 {
		return []string{"Замеры: пока нет. Начни с /measure — вес, талия, состав тела."}
	}
	latest := measurements[len(measurements)-1]
	parts := make([]string, 0, 4)
	if latest.WeightKG != nil {
		parts = append(parts, fmt.Sprintf("вес %.1f кг", *latest.WeightKG))
	}
	if latest.WaistCM != nil {
		parts = append(parts, fmt.Sprintf("талия %.1f см", *latest.WaistCM))
	}
	if latest.Visceral != nil {
		parts = append(parts, fmt.Sprintf("висцеральный %d", *latest.Visceral))
	}
	lines := []string{}
	if len(parts) > 0 {
		lines = append(lines, "Сейчас: "+strings.Join(parts, ", ")+".")
	}
	if len(measurements) >= 2 {
		previous := measurements[len(measurements)-2]
		deltas := make([]string, 0, 3)
		if latest.WeightKG != nil && previous.WeightKG != nil {
			deltas = append(deltas, fmt.Sprintf("вес %+.1f кг", *latest.WeightKG-*previous.WeightKG))
		}
		if latest.WaistCM != nil && previous.WaistCM != nil {
			deltas = append(deltas, fmt.Sprintf("талия %+.1f см", *latest.WaistCM-*previous.WaistCM))
		}
		if latest.Visceral != nil && previous.Visceral != nil {
			deltas = append(deltas, fmt.Sprintf("висцеральный %+d", *latest.Visceral-*previous.Visceral))
		}
		if len(deltas) > 0 {
			lines = append(lines, "Δ к прошлому замеру: "+strings.Join(deltas, ", ")+".")
		}
	}
	return lines
}

func strategyLines(h *commandHandlers, person nlu.Person, personName string, values []store.DailyValues, nutrition []store.DailyNutrition, trainings []store.TrainingFile, measurements []nlu.Measurement) []string {
	lines := make([]string, 0, 3)
	weekStart, _ := weekWindow(h.now())
	weekValues := filterByWindow(values, weekStart, weekStart.AddDate(0, 0, 6))
	weekTrainings := filterTrainingsByWindow(trainings, weekStart, weekStart.AddDate(0, 0, 6))
	weekNutrition := filterNutritionByWindow(nutrition, weekStart, weekStart.AddDate(0, 0, 6))

	prefs, _ := h.settings.Load(personName)
	thresholds := reviewThresholds(h, person, personName, prefs, values)
	counts := zoneCounts(weekValues, thresholds)
	sleepAvg := averageSleep(weekValues)
	totalZones := counts.Green + counts.Yellow + counts.Red
	recovery := (totalZones > 0 && counts.Green < counts.Yellow+counts.Red) ||
		(sleepAvg > 0 && sleepAvg < 7) || len(weekTrainings) >= 5

	if recovery {
		lines = append(lines, "• Неделя восстановления: 2 силовые, без сдвоенных, RPE 6–7; сон 7.5–8 ч.")
	} else {
		lines = append(lines, "• Неделя нагрузки: 3 силовые + 1–2 кардио; прогрессия только при RPE ≤7.")
	}

	targets := reviewNutritionTargets(person, prefs)
	proteinAvg := averageNutrition(weekNutrition, func(value store.DailyNutrition) *int { return value.ProteinG })
	if proteinAvg != nil && *proteinAvg < targets.proteinMin {
		lines = append(lines, fmt.Sprintf("• Белок: в среднем %d г, цель %d–%d — добавь порцию белка в завтрак/ужин.", *proteinAvg, targets.proteinMin, targets.proteinMax))
	} else {
		lines = append(lines, "• Питание: держим белок и дефицит 300–500 ккал без ускорения.")
	}

	if len(measurements) < 2 {
		lines = append(lines, "• Замеры: повтори в воскресенье утром в тех же условиях, чтобы увидеть Δ.")
	} else {
		latest := measurements[len(measurements)-1]
		previous := measurements[len(measurements)-2]
		weightDelta, waistDelta := measurementDeltas(latest, previous)
		if (weightDelta != nil && *weightDelta >= 0) || (waistDelta != nil && *waistDelta >= 0) {
			lines = append(lines, "• Замеры: талия/вес без движения — проверь дефицит, шаги и сон; не ускоряй темп.")
		} else {
			lines = append(lines, "• Замеры: темп 0.5–1% веса в неделю — не ускоряй; талия/висцеральный приоритет.")
		}
	}
	return lines
}

func reviewThresholds(h *commandHandlers, person nlu.Person, personName string, prefs settings.Settings, values []store.DailyValues) analysis.Thresholds {
	base := defaultHRVBase(person)
	if prefs.HRVBaseManual != nil {
		base = *prefs.HRVBaseManual
	} else if average := averageHRV(values, 7); average > 0 {
		base = average
	}
	thresholds := analysis.AthleteThresholds(base)
	if person == nlu.Wife {
		thresholds = analysis.WifeThresholds(base)
	}
	applyThresholdOverrides(&thresholds, prefs)
	return thresholds
}

func averageSleep(values []store.DailyValues) float64 {
	total, count := 0.0, 0
	for _, value := range values {
		if value.SleepH != nil {
			total += *value.SleepH
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

type reviewNutritionGoals struct {
	proteinMin, proteinMax int
}

func reviewNutritionTargets(person nlu.Person, prefs settings.Settings) reviewNutritionGoals {
	goals := reviewNutritionGoals{proteinMin: 108, proteinMax: 135}
	if person == nlu.Wife {
		goals = reviewNutritionGoals{proteinMin: 112, proteinMax: 112}
	}
	if prefs.ProteinMin != nil {
		goals.proteinMin = *prefs.ProteinMin
	}
	if prefs.ProteinMax != nil {
		goals.proteinMax = *prefs.ProteinMax
	}
	return goals
}

func measurementDeltas(latest, previous nlu.Measurement) (*float64, *float64) {
	var weight, waist *float64
	if latest.WeightKG != nil && previous.WeightKG != nil {
		value := *latest.WeightKG - *previous.WeightKG
		weight = &value
	}
	if latest.WaistCM != nil && previous.WaistCM != nil {
		value := *latest.WaistCM - *previous.WaistCM
		waist = &value
	}
	return weight, waist
}
