package intake

import (
	"fmt"
	"strings"
)

// normalizeTraining приводит тренировку со скриншота к безопасной структуре:
// мусорные/вне диапазонов поля отбрасываются, а не ломают весь разбор.
func normalizeTraining(wire *wireTraining) *Training {
	training := &Training{
		Kind:        strings.TrimSpace(wire.Kind),
		DurationMin: intPtr(wire.DurationMin),
		AvgHR:       intPtr(wire.AvgHR),
		Calories:    intPtr(wire.Calories),
		Notes:       strings.TrimSpace(wire.Notes),
	}
	if training.DurationMin != nil && (*training.DurationMin <= 0 || *training.DurationMin > 600) {
		training.DurationMin = nil
	}
	if training.AvgHR != nil && (*training.AvgHR < 30 || *training.AvgHR > 220) {
		training.AvgHR = nil
	}
	if training.Calories != nil && (*training.Calories < 0 || *training.Calories > 5000) {
		training.Calories = nil
	}
	for _, item := range wire.Exercises {
		name := strings.TrimSpace(item.Name)
		if !meaningfulExerciseName(name) {
			continue
		}
		exercise := TrainingExercise{
			Name:     name,
			WeightKG: floatPtr(item.WeightKG),
			Sets:     intPtr(item.Sets),
			Reps:     intPtr(item.Reps),
			RPE:      intPtr(item.RPE),
			Note:     strings.TrimSpace(item.Note),
		}
		if exercise.WeightKG != nil && (*exercise.WeightKG <= 0 || *exercise.WeightKG > 500) {
			exercise.WeightKG = nil
		}
		if exercise.Sets != nil && (*exercise.Sets <= 0 || *exercise.Sets > 100) {
			exercise.Sets = nil
		}
		if exercise.Reps != nil && (*exercise.Reps <= 0 || *exercise.Reps > 1000) {
			exercise.Reps = nil
		}
		if exercise.RPE != nil && (*exercise.RPE < 1 || *exercise.RPE > 10) {
			exercise.RPE = nil
		}
		training.Exercises = append(training.Exercises, exercise)
	}
	return training
}

// meaningfulExerciseName отбрасывает строки-заголовки таблицы, которые модель
// иногда принимает за упражнения («Подход 1», «Упражнение 2» и т.п.).
func meaningfulExerciseName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" || lower == "-" || lower == "—" {
		return false
	}
	for _, prefix := range []string{"подход", "упражнение", "упражнения", "сет", "повтор"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	letters := 0
	for _, r := range lower {
		if r >= 'a' && r <= 'z' || r >= 'а' && r <= 'я' || r == 'ё' {
			letters++
		}
	}
	return letters > 0
}

func mergeTraining(dst, src *Training) *Training {
	if src == nil {
		return dst
	}
	if dst == nil {
		return src
	}
	if dst.Date == "" {
		dst.Date = src.Date
	}
	if dst.Kind == "" {
		dst.Kind = src.Kind
	}
	if dst.DurationMin == nil {
		dst.DurationMin = src.DurationMin
	}
	if dst.AvgHR == nil {
		dst.AvgHR = src.AvgHR
	}
	if dst.Calories == nil {
		dst.Calories = src.Calories
	}
	if dst.Notes == "" {
		dst.Notes = src.Notes
	}
	for _, candidate := range src.Exercises {
		found := false
		for index := range dst.Exercises {
			if exerciseVisionNamesMatch(dst.Exercises[index].Name, candidate.Name) {
				if dst.Exercises[index].Sets == nil {
					dst.Exercises[index].Sets = candidate.Sets
				}
				if dst.Exercises[index].Reps == nil {
					dst.Exercises[index].Reps = candidate.Reps
				}
				if dst.Exercises[index].WeightKG == nil {
					dst.Exercises[index].WeightKG = candidate.WeightKG
				}
				found = true
				break
			}
		}
		if !found {
			dst.Exercises = append(dst.Exercises, candidate)
		}
	}
	return dst
}

func exerciseVisionNamesMatch(left, right string) bool {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	return left != "" && right != "" && (strings.Contains(left, right) || strings.Contains(right, left))
}

// formatTraining — строка для ответа в чат.
func formatTraining(training *Training) string {
	if training == nil {
		return ""
	}
	parts := []string{"тренировка"}
	if kind := strings.TrimSpace(training.Kind); kind != "" {
		parts = append(parts, kind)
	}
	if training.DurationMin != nil {
		parts = append(parts, fmt.Sprintf("%d мин", *training.DurationMin))
	}
	if training.AvgHR != nil {
		parts = append(parts, fmt.Sprintf("ср. пульс %d", *training.AvgHR))
	}
	if training.Calories != nil {
		parts = append(parts, fmt.Sprintf("%d ккал", *training.Calories))
	}
	if len(training.Exercises) > 0 {
		parts = append(parts, fmt.Sprintf("упражнений %d", len(training.Exercises)))
	}
	return strings.Join(parts, " · ")
}
