// Package settings хранит переопределения целей и зон (FR-7): бот-owned YAML
// поверх дефолтов контракта, валидация диапазонов, история изменений.
// Пакет не знает про Telegram и конфиг бота — путь к каталогу задаётся снаружи.
package settings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Люди проекта.
const (
	PersonAthlete = "athlete"
	PersonWife    = "wife"
)

// Дефолтные допуски зон (FR-7.2).
const (
	DefaultHRVGreenMargin = 2
	DefaultHRVRedMargin   = 5
)

// Change — запись истории изменения настройки (FR-7.3).
type Change struct {
	Date  string `yaml:"date"`
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// Settings — переопределения пользователя поверх дефолтов контракта.
// nil = дефолт из CLAUDE.md/CLAUDE-zheny.md/контракта (FR-7.3).
type Settings struct {
	Kcal           *int     `yaml:"kcal,omitempty"`
	ProteinMin     *int     `yaml:"protein_min,omitempty"`
	ProteinMax     *int     `yaml:"protein_max,omitempty"`
	FatMax         *int     `yaml:"fat_max,omitempty"`
	CarbsMin       *int     `yaml:"carbs_min,omitempty"`
	CarbsMax       *int     `yaml:"carbs_max,omitempty"`
	WeightGoalKG   *float64 `yaml:"weight_goal_kg,omitempty"`
	HRVBaseManual  *int     `yaml:"hrv_base_manual,omitempty"`
	RHRGreenMax    *int     `yaml:"rhr_green_max,omitempty"`
	RHRRedMin      *int     `yaml:"rhr_red_min,omitempty"`
	HRVGreenMargin *int     `yaml:"hrv_green_margin,omitempty"`
	HRVRedMargin   *int     `yaml:"hrv_red_margin,omitempty"`
	// Еженедельный разбор: nil = выключен; время — ЧЧ:ММ.
	WeeklyReviewEnabled *bool    `yaml:"weekly_review,omitempty"`
	WeeklyReviewTime    string   `yaml:"weekly_review_time,omitempty"`
	History             []Change `yaml:"history"`
}

// GreenMargin возвращает допуск зелёной зоны с учётом дефолта.
func (s Settings) GreenMargin() int {
	if s.HRVGreenMargin != nil {
		return *s.HRVGreenMargin
	}
	return DefaultHRVGreenMargin
}

// RedMargin возвращает допуск красной зоны с учётом дефолта.
func (s Settings) RedMargin() int {
	if s.HRVRedMargin != nil {
		return *s.HRVRedMargin
	}
	return DefaultHRVRedMargin
}

// WeeklyReviewOn — включён ли еженедельный разбор (по умолчанию выключен).
func (s Settings) WeeklyReviewOn() bool {
	return s.WeeklyReviewEnabled != nil && *s.WeeklyReviewEnabled
}

// WeeklyReviewTimeOrDefault — время разбора или дефолт.
func (s Settings) WeeklyReviewTimeOrDefault(fallback string) string {
	if strings.TrimSpace(s.WeeklyReviewTime) != "" {
		return s.WeeklyReviewTime
	}
	return fallback
}

// Keys возвращает список ключей для справки/сообщений об ошибках (FR-7.2).
func Keys() []string {
	return []string{
		"kcal", "protein_min", "protein_max", "fat_max",
		"carbs_min", "carbs_max", "weight_goal", "hrv_base",
		"rhr_green_max", "rhr_red_min", "hrv_green_margin", "hrv_red_margin",
		"weekly_review", "weekly_review_time",
	}
}

// Store — хранилище настроек в каталоге (обычно «настройки»).
type Store struct {
	dir string
}

// NewStore создаёт хранилище настроек для каталога dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Load читает настройки человека. Отсутствие файла — пустые переопределения.
func (s *Store) Load(person string) (Settings, error) {
	path, err := s.Path(person)
	if err != nil {
		return Settings{}, err
	}
	data, err := readFile(path)
	if err != nil {
		return Settings{}, err
	}
	if data == nil {
		return Settings{}, nil
	}

	var loaded Settings
	if err := unmarshalYAML(data, &loaded); err != nil {
		return Settings{}, fmt.Errorf("settings: разобрать %q: %w", path, err)
	}
	return loaded, nil
}

// Apply проверяет и сохраняет одно переопределение (FR-7.2/7.4), дописывая
// историю. При любой ошибке файл не меняется.
func (s *Store) Apply(person, key, value string) (Settings, error) {
	current, err := s.Load(person)
	if err != nil {
		return Settings{}, err
	}

	if err = applyKey(&current, key, value); err != nil {
		return Settings{}, err
	}
	if err = validate(current); err != nil {
		return Settings{}, err
	}

	current.History = append(current.History, Change{
		Date:  time.Now().Format("2006-01-02"),
		Key:   key,
		Value: value,
	})

	path, err := s.Path(person)
	if err != nil {
		return Settings{}, err
	}
	if err := saveSettings(path, current); err != nil {
		return Settings{}, err
	}
	return current, nil
}

// Reset удаляет все переопределения профиля (FR-7.5).
func (s *Store) Reset(person string) error {
	path, err := s.Path(person)
	if err != nil {
		return err
	}
	if err := removeFile(path); err != nil {
		return fmt.Errorf("settings: сбросить %q: %w", path, err)
	}
	return nil
}

func applyKey(settings *Settings, key, value string) error {
	key = normalizeKeyAlias(key)
	switch key {
	case "kcal":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.Kcal = &parsed
	case "protein_min":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.ProteinMin = &parsed
	case "protein_max":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.ProteinMax = &parsed
	case "fat_max":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.FatMax = &parsed
	case "carbs_min":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.CarbsMin = &parsed
	case "carbs_max":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.CarbsMax = &parsed
	case "weight_goal":
		parsed, err := parseFloat(value, key)
		if err != nil {
			return err
		}
		settings.WeightGoalKG = &parsed
	case "hrv_base":
		if strings.EqualFold(strings.TrimSpace(value), "auto") {
			settings.HRVBaseManual = nil
			return nil
		}
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.HRVBaseManual = &parsed
	case "rhr_green_max":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.RHRGreenMax = &parsed
	case "rhr_red_min":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.RHRRedMin = &parsed
	case "hrv_green_margin":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.HRVGreenMargin = &parsed
	case "hrv_red_margin":
		parsed, err := parseInt(value, key)
		if err != nil {
			return err
		}
		settings.HRVRedMargin = &parsed
	case "weekly_review":
		parsed, err := parseBool(value, key)
		if err != nil {
			return err
		}
		settings.WeeklyReviewEnabled = &parsed
	case "weekly_review_time":
		parsed := strings.TrimSpace(value)
		if _, err := time.Parse("15:04", parsed); err != nil {
			return fmt.Errorf("settings: weekly_review_time: ожидается ЧЧ:ММ, получено %q", value)
		}
		settings.WeeklyReviewTime = parsed
	default:
		return fmt.Errorf("settings: неизвестный ключ %q (доступно: %s)", key, strings.Join(Keys(), ", "))
	}
	return nil
}

func normalizeKeyAlias(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "воскресный разбор", "воскресный", "разбор":
		return "weekly_review"
	case "время разбора", "время воскресного разбора":
		return "weekly_review_time"
	case "калории", "ккал":
		return "kcal"
	default:
		return strings.ToLower(strings.TrimSpace(key))
	}
}

func parseInt(value, key string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("settings: %s: ожидается целое число, получено %q", key, value)
	}
	return parsed, nil
}

func parseBool(value, key string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "да", "вкл":
		return true, nil
	case "off", "false", "0", "нет", "выкл":
		return false, nil
	default:
		return false, fmt.Errorf("settings: %s: ожидается on/off, получено %q", key, value)
	}
}

func parseFloat(value, key string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(value), ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("settings: %s: ожидается число, получено %q", key, value)
	}
	return parsed, nil
}

func validate(settings Settings) error {
	checks := []error{
		checkInt("kcal", settings.Kcal, 1200, 4000),
		checkInt("protein_min", settings.ProteinMin, 40, 300),
		checkInt("protein_max", settings.ProteinMax, 40, 300),
		checkInt("fat_max", settings.FatMax, 30, 200),
		checkInt("carbs_min", settings.CarbsMin, 0, 600),
		checkInt("carbs_max", settings.CarbsMax, 0, 600),
		checkFloat("weight_goal", settings.WeightGoalKG, 40, 200),
		checkInt("hrv_base", settings.HRVBaseManual, 15, 150),
		checkInt("rhr_green_max", settings.RHRGreenMax, 35, 80),
		checkInt("rhr_red_min", settings.RHRRedMin, 50, 120),
		checkInt("hrv_green_margin", settings.HRVGreenMargin, 0, 10),
		checkInt("hrv_red_margin", settings.HRVRedMargin, 1, 20),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}

	if settings.ProteinMin != nil && settings.ProteinMax != nil && *settings.ProteinMin > *settings.ProteinMax {
		return fmt.Errorf("settings: protein_min=%d больше protein_max=%d", *settings.ProteinMin, *settings.ProteinMax)
	}
	if settings.CarbsMin != nil && settings.CarbsMax != nil && *settings.CarbsMin > *settings.CarbsMax {
		return fmt.Errorf("settings: carbs_min=%d больше carbs_max=%d", *settings.CarbsMin, *settings.CarbsMax)
	}
	if settings.RHRGreenMax != nil && settings.RHRRedMin != nil && *settings.RHRGreenMax >= *settings.RHRRedMin {
		return fmt.Errorf("settings: rhr_green_max=%d должен быть меньше rhr_red_min=%d", *settings.RHRGreenMax, *settings.RHRRedMin)
	}
	if settings.HRVGreenMargin != nil && settings.HRVRedMargin != nil && *settings.HRVGreenMargin >= *settings.HRVRedMargin {
		return fmt.Errorf("settings: hrv_green_margin=%d должен быть меньше hrv_red_margin=%d", *settings.HRVGreenMargin, *settings.HRVRedMargin)
	}
	if settings.WeeklyReviewTime != "" {
		if _, err := time.Parse("15:04", settings.WeeklyReviewTime); err != nil {
			return fmt.Errorf("settings: weekly_review_time ожидает ЧЧ:ММ")
		}
	}
	return nil
}

func checkInt(name string, value *int, minValue, maxValue int) error {
	if value == nil {
		return nil
	}
	if *value < minValue || *value > maxValue {
		return fmt.Errorf("settings: %s=%d вне диапазона %d..%d", name, *value, minValue, maxValue)
	}
	return nil
}

func checkFloat(name string, value *float64, minValue, maxValue float64) error {
	if value == nil {
		return nil
	}
	if *value < minValue || *value > maxValue {
		return fmt.Errorf("settings: %s=%.1f вне диапазона %.1f..%.1f", name, *value, minValue, maxValue)
	}
	return nil
}
