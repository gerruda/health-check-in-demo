// Package tools содержит инструменты, доступные LLM (FR-6.7.2).
// Инструменты работают с файловой базой проекта и не зависят от Telegram.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"healthbot/internal/analysis"
	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

const (
	// NameReadLastCheckin — имя инструмента чтения истории.
	NameReadLastCheckin = "read_last_checkin"

	// defaultMaxChars — страховка по размеру контекста (LLM получает сводку).
	defaultMaxChars = 1500
)

// Tool — инструмент, доступный LLM.
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// LastCheckin отдаёт LLM компактную сводку: последний чек-ин, HRV_7/зоны и
// последние тренировки. Сырые markdown-файлы и вся история в промпт не идут
// (контекст/токены — FR-6.7).
type LastCheckin struct {
	reader   *store.Reader
	maxChars int
	root     string
}

// LastCheckinOption настраивает инструмент.
type LastCheckinOption func(*LastCheckin)

// WithCache подключает общий кэш истории (с Writer/Reader).
func WithCache(cache *store.Cache) LastCheckinOption {
	return func(t *LastCheckin) {
		t.reader = store.NewReader(store.NewPaths(t.root), store.WithCache(cache))
	}
}

// NewLastCheckin создаёт инструмент для корня данных проекта (DATA_ROOT).
func NewLastCheckin(dataRoot string, opts ...LastCheckinOption) *LastCheckin {
	tool := &LastCheckin{
		reader:   store.NewReader(store.NewPaths(dataRoot)),
		maxChars: defaultMaxChars,
	}
	tool.root = dataRoot
	for _, opt := range opts {
		opt(tool)
	}
	return tool
}

// Name реализует Tool.
func (t *LastCheckin) Name() string { return NameReadLastCheckin }

// Description реализует Tool.
func (t *LastCheckin) Description() string {
	return "Читает компактную сводку истории (последний чек-ин, HRV_7, зоны, последние тренировки). Сырые файлы не возвращает."
}

// Schema реализует Tool.
func (t *LastCheckin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"person": {
				"type": "string",
				"enum": ["athlete", "wife"],
				"description": "Чей профиль: athlete — атлет, wife — жена"
			}
		},
		"required": ["person"]
	}`)
}

// Execute реализует Tool.
func (t *LastCheckin) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var params struct {
		Person string `json:"person"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("read_last_checkin: разобрать аргументы: %w", err)
		}
	}

	person, label, ok := parsePerson(params.Person)
	if !ok {
		return "", fmt.Errorf("read_last_checkin: неизвестный person %q (ожидается athlete или wife)", params.Person)
	}

	values, err := t.reader.CheckinValues(person, 7)
	if err != nil {
		return "", fmt.Errorf("read_last_checkin: %w", err)
	}
	trainings, err := t.reader.TrainingFiles(person)
	if err != nil {
		return "", fmt.Errorf("read_last_checkin: %w", err)
	}

	if len(values) == 0 && len(trainings) == 0 {
		return "нет данных: чек-инов и тренировок не найдено", nil
	}

	lines := []string{"Сводка (" + label + "):"}
	thresholds := analysis.AthleteThresholds(33)
	if person == nlu.Wife {
		thresholds = analysis.WifeThresholds(34)
	}

	if len(values) > 0 {
		latest := values[len(values)-1]
		parts := make([]string, 0, 6)
		if latest.SleepH != nil {
			parts = append(parts, "сон "+formatSleep(*latest.SleepH))
		}
		if latest.HRV != nil {
			parts = append(parts, fmt.Sprintf("HRV %d мс", *latest.HRV))
		}
		if latest.RHR != nil {
			parts = append(parts, fmt.Sprintf("пульс %d", *latest.RHR))
		}
		if latest.Mood != nil {
			parts = append(parts, fmt.Sprintf("настроение %d/10", *latest.Mood))
		}
		if latest.WeightKG != nil {
			parts = append(parts, fmt.Sprintf("вес %.1f кг", *latest.WeightKG))
		}
		line := "Последний чек-ин " + latest.Date.Format("02.01.2006")
		if len(parts) > 0 {
			line += ": " + strings.Join(parts, ", ")
		}
		lines = append(lines, line+".")

		hrvValues := lastHRV(values)
		if len(hrvValues) > 0 {
			trend := analysis.TrendHRV(hrvValues)
			counts := zoneCounts(values, thresholds)
			line := fmt.Sprintf("HRV_7: %.1f мс · %s %s · 🟢%d / 🟡%d / 🔴%d",
				trend.Average, trend.Direction.Arrow(), trend.Direction.Name(),
				counts.Green, counts.Yellow, counts.Red)
			lines = append(lines, line+".")
		}
	}

	if len(trainings) > 0 {
		start := len(trainings) - 3
		if start < 0 {
			start = 0
		}
		parts := make([]string, 0, 3)
		for _, training := range trainings[start:] {
			state := ""
			if !training.Done {
				state = " (пропуск)"
			}
			parts = append(parts, training.Date.Format("02.01.2006")+" "+training.Kind+state)
		}
		lines = append(lines, "Последние тренировки: "+strings.Join(parts, "; ")+".")
	}

	out := strings.Join(lines, "\n")
	if len(out) > t.maxChars {
		out = out[:t.maxChars]
	}
	return out, nil
}

func parsePerson(raw string) (nlu.Person, string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "athlete", "атлет":
		return nlu.Athlete, "атлет", true
	case "wife", "жена":
		return nlu.Wife, "жена", true
	default:
		return "", "", false
	}
}

func formatSleep(hours float64) string {
	minutes := int(hours*60 + 0.5)
	return fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
}

func lastHRV(values []store.DailyValues) []int {
	collected := make([]int, 0, len(values))
	for _, value := range values {
		if value.HRV != nil {
			collected = append(collected, *value.HRV)
		}
	}
	return collected
}

func zoneCounts(values []store.DailyValues, thresholds analysis.Thresholds) analysis.ZoneCounts {
	counts := analysis.ZoneCounts{}
	for _, value := range values {
		result := analysis.Evaluate(analysis.Input{
			HRV: value.HRV, RHR: value.RHR, SleepH: value.SleepH, Mood: value.Mood,
		}, thresholds)
		switch result.Zone {
		case analysis.ZoneGreen:
			counts.Green++
		case analysis.ZoneYellow:
			counts.Yellow++
		case analysis.ZoneRed:
			counts.Red++
		}
	}
	return counts
}
