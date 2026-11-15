package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"healthbot/internal/nlu"
)

var (
	trainingCaloriesPattern = regexp.MustCompile(`(?m)^- Калории:\s*(\d+)\s*ккал`)
	exerciseWeightPattern   = regexp.MustCompile(`(?i)^(\d+(?:[.,]\d+)?)\s*кг$`)
	exerciseSetsRepsPattern = regexp.MustCompile(`^(\d+)×(\d+)$`)
	exerciseSetsPattern     = regexp.MustCompile(`^(\d+)\s*подх\.$`)
	exerciseRPEPattern      = regexp.MustCompile(`(?i)^RPE\s*(\d+)$`)
	painLinePattern         = regexp.MustCompile(`(?i)^([^:]+):\s*(\d+)\s*/\s*10$`)
)

// TrainingSessions возвращает последние limit тренировок с упражнениями/болью
// (для разбора FR-3.3 и прогрессии).
func (r *Reader) TrainingSessions(person nlu.Person, limit int) ([]nlu.Training, error) {
	dir, err := r.paths.DataDir(person)
	if err != nil {
		return nil, err
	}
	trainingsDir := filepath.Join(dir, trainingsDirName)
	key := string(person) + ":trainingSessions"
	stamp, canCache := dirStamp(trainingsDir)
	if canCache {
		if cached, ok := r.cache.get(key, stamp); ok {
			all := cached.([]nlu.Training)
			return limitTrainings(all, limit), nil
		}
	}

	entries, err := os.ReadDir(trainingsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: прочитать каталог %q: %w", trainingsDir, err)
	}
	type fileInfo struct {
		name string
		date time.Time
		kind string
	}
	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := trainingFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		date, parseErr := time.Parse("2006-01-02", match[1])
		if parseErr != nil {
			continue
		}
		files = append(files, fileInfo{name: entry.Name(), date: date, kind: match[2]})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].date.Equal(files[j].date) {
			return files[i].kind < files[j].kind
		}
		return files[i].date.Before(files[j].date)
	})

	trainings := make([]nlu.Training, 0, len(files))
	for _, file := range files {
		content, readErr := os.ReadFile(filepath.Join(trainingsDir, file.name))
		if readErr != nil {
			if r.log != nil {
				r.log.Warn("store: тренировка пропущена при чтении сессии", "person", person, "file", file.name, "error", readErr)
			}
			continue
		}
		trainings = append(trainings, parseTrainingSession(file.date, file.kind, person, string(content)))
	}
	if canCache {
		r.cache.set(key, stamp, trainings)
	}
	return limitTrainings(trainings, limit), nil
}

func limitTrainings(all []nlu.Training, limit int) []nlu.Training {
	if limit <= 0 || len(all) <= limit {
		result := make([]nlu.Training, len(all))
		copy(result, all)
		return result
	}
	result := make([]nlu.Training, limit)
	copy(result, all[len(all)-limit:])
	return result
}

func parseTrainingSession(date time.Time, kind string, person nlu.Person, content string) nlu.Training {
	training := nlu.Training{
		Date:   date.Format("2006-01-02"),
		Person: person,
		Kind:   kind,
		Done:   !lineTrainingSkip.MatchString(content),
	}
	if match := lineTrainingDuration.FindStringSubmatch(content); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			training.DurationMin = &value
		}
	}
	if match := lineTrainingRPE.FindStringSubmatch(content); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			training.RPESession = &value
		}
	}
	if match := trainingCaloriesPattern.FindStringSubmatch(content); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			training.Calories = &value
		}
	}

	sections := splitMarkdownSections(content)
	if section, ok := sections["Упражнения"]; ok {
		for _, line := range strings.Split(section, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- ") {
				continue
			}
			if exercise := parseTrainingExercise(strings.TrimPrefix(line, "- ")); exercise.Name != "" {
				training.Exercises = append(training.Exercises, exercise)
			}
		}
	}
	if section, ok := sections["Самочувствие"]; ok {
		for _, line := range strings.Split(section, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				training.Felt = strings.TrimSpace(strings.TrimPrefix(line, "- "))
				break
			}
		}
	}
	if section, ok := sections["Боль"]; ok {
		pains := make(map[string]int)
		for _, line := range strings.Split(section, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			if match := painLinePattern.FindStringSubmatch(line); match != nil {
				if value, err := strconv.Atoi(match[2]); err == nil {
					pains[strings.ToLower(strings.TrimSpace(match[1]))] = value
				}
			}
		}
		if len(pains) > 0 {
			training.Pain = pains
		}
	}
	if section, ok := sections["Итог"]; ok {
		if match := regexp.MustCompile(`(?m)^- Кардио:\s*(.+)$`).FindStringSubmatch(section); match != nil {
			training.Cardio = strings.TrimSpace(match[1])
		}
	}
	// файл мог быть перезаписан дневником-отчётом (FR-3.3) — дочитываем его
	parseReportTraining(content, &training)
	return training
}

func splitMarkdownSections(content string) map[string]string {
	sections := make(map[string]string)
	var title string
	var builder strings.Builder
	flush := func() {
		if title != "" {
			sections[title] = builder.String()
		}
		builder.Reset()
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			title = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	flush()
	return sections
}

func parseTrainingExercise(raw string) nlu.Exercise {
	note := ""
	if index := strings.Index(raw, " — "); index >= 0 {
		note = strings.TrimSpace(raw[index+len(" — "):])
		raw = raw[:index]
	}
	parts := strings.Split(raw, " · ")
	exercise := nlu.Exercise{Name: strings.TrimSpace(parts[0]), Note: note}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if match := exerciseWeightPattern.FindStringSubmatch(part); match != nil {
			if weight, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64); err == nil {
				exercise.WeightKG = &weight
			}
			continue
		}
		if match := exerciseSetsRepsPattern.FindStringSubmatch(part); match != nil {
			if sets, err := strconv.Atoi(match[1]); err == nil {
				exercise.Sets = &sets
			}
			if reps, err := strconv.Atoi(match[2]); err == nil {
				exercise.Reps = &reps
			}
			continue
		}
		if match := exerciseSetsPattern.FindStringSubmatch(part); match != nil {
			if sets, err := strconv.Atoi(match[1]); err == nil {
				exercise.Sets = &sets
			}
			continue
		}
		if match := exerciseRPEPattern.FindStringSubmatch(part); match != nil {
			if rpe, err := strconv.Atoi(match[1]); err == nil {
				exercise.RPE = &rpe
			}
		}
	}
	return exercise
}
