package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var workoutOCRSchema = json.RawMessage(`{
	"type": "object",
	"properties": {"text": {"type": "string", "description": "весь видимый текст экрана построчно"}},
	"required": ["text"]
}`)

const workoutOCRPrompt = `Извлеки ВЕСЬ видимый текст с экрана тренировки дословно, построчно,
без пересказа, без повторов и без времени из статус-бара. Сохрани строки:
«Общее время 31:15», «Средняя частота пульса 92 уд/м», «Всего калорий 107»,
таблицу «Подход / Имя / Время / Повтор. / Вес» и значения.
Обязательно включи строку с датой тренировки из самого верха экрана
(например «13 нояб. 2026» или «11 нояб. @ 12:55»), НЕ сегодняшнюю дату.
Верни JSON {"text":"..."}.`

var (
	ocrDurationPattern    = regexp.MustCompile(`(?i)(?:Общее время|Длительность)\s+(?:(\d{1,2}):)?(\d{1,2}):(\d{2})`)
	ocrDurationMinPattern = regexp.MustCompile(`(?i)(?:Общее время|Длительность)\s+(\d+)\s*мин`)
	ocrAvgHRPattern       = regexp.MustCompile(`(?i)(?:Средняя частота пульса|Средний пульс)\s+(\d+)`)
	ocrCaloriesPattern    = regexp.MustCompile(`(?i)(?:Всего калорий|Калории)\s+(\d+)`)
)

// extractWorkoutOCR читает экран тренировки как текст и разбирает кодом —
// это устойчивее к «фантазиям» vision-модели на числах.
func (e *Extractor) extractWorkoutOCR(ctx context.Context, image []byte, now time.Time) (Extracted, error) {
	raw, err := e.vision.ExtractVision(ctx, workoutOCRPrompt, workoutOCRSchema, [][]byte{image})
	if err != nil {
		return Extracted{}, err
	}
	var wire struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Extracted{}, fmt.Errorf("intake: OCR тренировки: %w", err)
	}
	training := parseWorkoutOCR(wire.Text, now)
	if !trainingHasData(training) {
		return Extracted{}, fmt.Errorf("intake: OCR тренировки не дал данных")
	}
	training.Date = parseDateText(wire.Text, now)
	return Extracted{Training: training, Kinds: []string{"training"}}, nil
}

func parseWorkoutOCR(text string, now time.Time) *Training {
	normalized := strings.ReplaceAll(text, " / ", "\n")
	normalized = strings.ReplaceAll(normalized, " | ", "\n")
	normalized = strings.ReplaceAll(normalized, "|", "\n")
	lines := make([]string, 0, 60)
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	joined := strings.Join(lines, "\n")

	training := &Training{Kind: detectWorkoutKind(joined)}
	if match := ocrDurationPattern.FindStringSubmatch(joined); match != nil {
		minutes := 0
		if match[1] != "" {
			hours, _ := strconv.Atoi(match[1])
			minutePart, _ := strconv.Atoi(match[2])
			minutes = hours*60 + minutePart
		} else {
			minutes, _ = strconv.Atoi(match[2])
		}
		if minutes > 0 && minutes <= 600 {
			training.DurationMin = &minutes
		}
	} else if match := ocrDurationMinPattern.FindStringSubmatch(joined); match != nil {
		if minutes, err := strconv.Atoi(match[1]); err == nil && minutes > 0 {
			training.DurationMin = &minutes
		}
	}
	if match := ocrAvgHRPattern.FindStringSubmatch(joined); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			training.AvgHR = &value
		}
	}
	if match := ocrCaloriesPattern.FindStringSubmatch(joined); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			training.Calories = &value
		}
	}
	training.Exercises = parseExerciseTable(lines)
	return training
}

func detectWorkoutKind(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "силов"):
		return "silovaya"
	case strings.Contains(lower, "кардио"), strings.Contains(lower, "бег"), strings.Contains(lower, "велосипед"):
		return "kardio"
	case strings.Contains(lower, "прогул"):
		return "progulka"
	default:
		return ""
	}
}

// parseExerciseTable разбирает строки таблицы: Подход, Имя, Время, Повтор., Вес.
func parseExerciseTable(lines []string) []TrainingExercise {
	start := -1
	for index, line := range lines {
		if strings.EqualFold(line, "Подход") {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return nil
	}

	var exercises []TrainingExercise
	for index := start; index < len(lines); index++ {
		line := lines[index]
		if strings.HasPrefix(strings.ToLower(line), "добавить упражнение") {
			break
		}
		set, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		if index+3 >= len(lines) {
			break
		}
		name := strings.TrimSpace(lines[index+1])
		// lines[index+2] — время, lines[index+3] — повторы, lines[index+4] — вес
		reps, repsErr := strconv.Atoi(strings.TrimSpace(lines[index+3]))
		if !meaningfulExerciseName(name) || repsErr != nil {
			continue
		}
		exercise := TrainingExercise{Name: name, Reps: &reps}
		sets := set
		exercise.Sets = &sets
		if index+4 < len(lines) {
			weightText := strings.TrimSpace(lines[index+4])
			if weight, wErr := strconv.ParseFloat(strings.ReplaceAll(weightText, ",", "."), 64); wErr == nil && weight > 0 {
				exercise.WeightKG = &weight
			}
		}
		exercises = append(exercises, exercise)
		index += 4
	}
	return exercises
}
