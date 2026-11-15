package app

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"healthbot/internal/store"
)

func weekWindow(now time.Time) (time.Time, time.Time) {
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return end.AddDate(0, 0, -6), end
}

func withinWindow(date, start, end time.Time) bool {
	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	return !day.Before(start) && !day.After(end)
}

func filterByWindow(values []store.DailyValues, start, end time.Time) []store.DailyValues {
	filtered := make([]store.DailyValues, 0, len(values))
	for _, value := range values {
		if withinWindow(value.Date, start, end) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func filterNutritionByWindow(values []store.DailyNutrition, start, end time.Time) []store.DailyNutrition {
	filtered := make([]store.DailyNutrition, 0, len(values))
	for _, value := range values {
		if withinWindow(value.Date, start, end) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func filterTrainingsByWindow(values []store.TrainingFile, start, end time.Time) []store.TrainingFile {
	filtered := make([]store.TrainingFile, 0, len(values))
	for _, value := range values {
		if withinWindow(value.Date, start, end) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func weekSleepLine(values []store.DailyValues, yellowMax float64) string {
	var total float64
	count := 0
	shortNights := 0
	for _, value := range values {
		if value.SleepH == nil {
			continue
		}
		total += *value.SleepH
		count++
		if *value.SleepH < yellowMax {
			shortNights++
		}
	}
	if count == 0 {
		return ""
	}
	line := "Сон: " + formatSleepHours(total/float64(count)) + " в среднем"
	if shortNights > 0 {
		line += fmt.Sprintf(" · %d ноч%s < %s", shortNights, pluralNights(shortNights), formatSleepHours(yellowMax))
	}
	return line
}

func weekRHRLine(values []store.DailyValues) string {
	total, count, maximum := 0, 0, 0
	for _, value := range values {
		if value.RHR == nil {
			continue
		}
		total += *value.RHR
		count++
		if *value.RHR > maximum {
			maximum = *value.RHR
		}
	}
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("Пульс покоя: %d в среднем (макс %d)", int(math.Round(float64(total)/float64(count))), maximum)
}

func weekNutritionLine(values []store.DailyNutrition) string {
	if len(values) == 0 {
		return "Питание: нет данных"
	}
	kcal := averageNutrition(values, func(value store.DailyNutrition) *int { return value.Kcal })
	protein := averageNutrition(values, func(value store.DailyNutrition) *int { return value.ProteinG })
	fat := averageNutrition(values, func(value store.DailyNutrition) *int { return value.FatG })
	carbs := averageNutrition(values, func(value store.DailyNutrition) *int { return value.CarbsG })

	parts := make([]string, 0, 4)
	if kcal != nil {
		parts = append(parts, fmt.Sprintf("%d ккал", *kcal))
	}
	if protein != nil {
		parts = append(parts, fmt.Sprintf("Б %d", *protein))
	}
	if fat != nil {
		parts = append(parts, fmt.Sprintf("Ж %d", *fat))
	}
	if carbs != nil {
		parts = append(parts, fmt.Sprintf("У %d", *carbs))
	}
	if len(parts) == 0 {
		return "Питание: нет данных"
	}
	return fmt.Sprintf("Питание (%d дн.): %s", len(values), strings.Join(parts, " · "))
}

func averageNutrition(values []store.DailyNutrition, get func(store.DailyNutrition) *int) *int {
	total, count := 0, 0
	for _, value := range values {
		if item := get(value); item != nil {
			total += *item
			count++
		}
	}
	if count == 0 {
		return nil
	}
	average := int(math.Round(float64(total) / float64(count)))
	return &average
}

func weekTrainingLine(values []store.TrainingFile) string {
	if len(values) == 0 {
		return "Тренировки: нет данных"
	}
	done, skipped, duration := 0, 0, 0
	kinds := make(map[string]int)
	for _, value := range values {
		if !value.Done {
			skipped++
			continue
		}
		done++
		if value.DurationMin != nil {
			duration += *value.DurationMin
		}
		kinds[trainingKindLabel(value.Kind)]++
	}

	line := fmt.Sprintf("Тренировки: %d", len(values))
	if skipped > 0 {
		line += fmt.Sprintf(" (выполнено %d, пропуск %d)", done, skipped)
	}
	if duration > 0 {
		line += " · " + formatMinutes(duration)
	}
	if len(kinds) > 0 {
		keys := make([]string, 0, len(kinds))
		for kind := range kinds {
			keys = append(keys, kind)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, kind := range keys {
			parts = append(parts, fmt.Sprintf("%s %d", kind, kinds[kind]))
		}
		line += " · " + strings.Join(parts, ", ")
	}
	return line
}

func weekWeightLine(values []store.DailyValues) string {
	var first, latest *float64
	for index := range values {
		weight := values[index].WeightKG
		if weight == nil {
			continue
		}
		if first == nil {
			first = weight
		}
		latest = weight
	}
	if latest == nil {
		return ""
	}
	line := fmt.Sprintf("Вес: %.1f кг", *latest)
	if first != nil && math.Abs(*latest-*first) >= 0.05 {
		delta := *latest - *first
		arrow := "↑"
		if delta < 0 {
			arrow = "↓"
			delta = -delta
		}
		line += fmt.Sprintf(" (%s %.1f кг за неделю)", arrow, delta)
	}
	return line
}

func formatSleepHours(hours float64) string {
	totalMinutes := int(math.Round(hours * 60))
	return fmt.Sprintf("%d:%02d", totalMinutes/60, totalMinutes%60)
}

func formatMinutes(minutes int) string {
	if minutes >= 60 {
		return fmt.Sprintf("%d ч %02d мин", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%d мин", minutes)
}

func pluralNights(count int) string {
	if count%10 == 1 && count%100 != 11 {
		return "ь"
	}
	return "и"
}

func trainingKindLabel(kind string) string {
	switch kind {
	case "fitboxing":
		return "фитбоксинг"
	case "sila-A":
		return "силовая A"
	case "sila-B":
		return "силовая B"
	case "ulichnaya":
		return "уличная"
	case "vosstanovlenie":
		return "восстановление"
	case "silovaya":
		return "силовая"
	case "kardio":
		return "кардио"
	case "progulka":
		return "прогулка"
	default:
		return kind
	}
}
