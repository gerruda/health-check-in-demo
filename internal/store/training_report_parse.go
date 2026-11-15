package store

import (
	"regexp"
	"strconv"
	"strings"

	"healthbot/internal/nlu"
)

var reportTableCellInt = regexp.MustCompile(`^\s*(\d+)`)

// parseReportTraining дополняет тренировку данными из человекочитаемого
// дневника-отчёта, если generic-секции (## Упражнения/## Итог) уже не пишутся.
func parseReportTraining(content string, training *nlu.Training) {
	sections := splitMarkdownSections(content)
	warmup := make([]nlu.Exercise, 0, 2)
	if body, ok := sectionByPrefix(sections, "Разминка/заминка"); ok {
		warmup = append(warmup, parseReportWarmup(body)...)
	}
	if body, ok := sectionByPrefix(sections, "Основные упражнения"); ok && len(training.Exercises) == 0 {
		training.Exercises = append(training.Exercises, parseReportExercises(body)...)
	}
	if len(warmup) > 0 {
		training.Exercises = append(warmup, training.Exercises...)
	}
	if body, ok := sectionByPrefix(sections, "Кардио-дополнение"); ok && training.Cardio == "" {
		training.Cardio = firstSectionLine(body)
	}
	if body, ok := sectionByPrefix(sections, "Общая статистика"); ok {
		parseReportStats(body, training)
	}
}

func sectionByPrefix(sections map[string]string, prefix string) (string, bool) {
	lowerPrefix := strings.ToLower(prefix)
	for title, body := range sections {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), lowerPrefix) {
			return body, true
		}
	}
	return "", false
}

func parseReportExercises(body string) []nlu.Exercise {
	exercises := make([]nlu.Exercise, 0, 8)
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitReportTableRow(line)
		if len(cells) < 2 {
			continue
		}
		name := strings.TrimSpace(cells[0])
		if name == "" || name == "—" || strings.EqualFold(name, "Упражнение") {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(cells[1]), "---") {
			continue
		}
		exercise := nlu.Exercise{Name: name}
		exercise.Sets = reportCellInt(cells[1])
		exercise.Reps = reportCellInt(cells[2])
		exercise.RPE = reportCellInt(cells[3])
		if len(cells) > 4 {
			exercise.Note = strings.TrimSpace(cells[4])
		}
		if exercise.Note == "—" {
			exercise.Note = ""
		}
		exercises = append(exercises, exercise)
	}
	return exercises
}

func splitReportTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func reportCellInt(cell string) *int {
	match := reportTableCellInt.FindStringSubmatch(cell)
	if match == nil {
		return nil
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}
	return &value
}

func parseReportWarmup(body string) []nlu.Exercise {
	exercises := make([]nlu.Exercise, 0, 2)
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if name == "" || strings.EqualFold(name, "не указана") || name == "—" {
			continue
		}
		exercises = append(exercises, nlu.Exercise{Name: name, Note: "разминка"})
	}
	return exercises
}

func firstSectionLine(body string) string {
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "- ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "- "))
		}
	}
	return ""
}

func parseReportStats(body string, training *nlu.Training) {
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rawLine), "- "))
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "длительность:"):
			training.DurationMin = firstReportInt(strings.TrimPrefix(line, "Длительность:"))
		case strings.HasPrefix(lower, "средний пульс:"):
			training.AvgHR = firstReportInt(strings.TrimPrefix(line, "Средний пульс:"))
		case strings.HasPrefix(lower, "rpe:"):
			training.RPESession = firstReportInt(strings.TrimPrefix(line, "RPE:"))
		case strings.HasPrefix(lower, "калории:"):
			training.Calories = firstReportInt(strings.TrimPrefix(line, "Калории:"))
		}
	}
}

func firstReportInt(text string) *int {
	match := reportTableCellInt.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}
	return &value
}
