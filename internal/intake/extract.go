// Package intake принимает данные от пользователя: скриншоты (vision) и текст
// чек-ина (FR-6.8). Извлекает структурированные данные через LLM по JSON-схеме,
// нормализует значения и отдаёт сервису для ответа в чат.
package intake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"healthbot/internal/llm"
	"healthbot/internal/logging"
)

// Input — вход для извлечения: изображения (0..N) и/или текст.
type Input struct {
	Images [][]byte
	Text   string
}

// Extracted — данные, распознанные на скриншотах или в тексте.
type Extracted struct {
	Kinds     []string   `json:"kinds"`
	Date      string     `json:"date,omitempty"`
	Nutrition *Nutrition `json:"nutrition,omitempty"`
	Checkin   *Checkin   `json:"checkin,omitempty"`
	Training  *Training  `json:"training,omitempty"`
	Notes     string     `json:"notes,omitempty"`
}

// Training — тренировка со скриншота фитнес-приложения/часов (FR-2.2).
type Training struct {
	Date        string             `json:"date,omitempty"`
	Kind        string             `json:"kind,omitempty"`
	DurationMin *int               `json:"duration_min,omitempty"`
	AvgHR       *int               `json:"avg_hr,omitempty"`
	Calories    *int               `json:"calories,omitempty"`
	Exercises   []TrainingExercise `json:"exercises,omitempty"`
	Notes       string             `json:"notes,omitempty"`
}

// TrainingExercise — упражнение из таблицы тренировки.
type TrainingExercise struct {
	Name     string   `json:"name"`
	WeightKG *float64 `json:"weight_kg,omitempty"`
	Sets     *int     `json:"sets,omitempty"`
	Reps     *int     `json:"reps,omitempty"`
	RPE      *int     `json:"rpe,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// Nutrition — питание за день.
type Nutrition struct {
	Kcal     *int   `json:"kcal,omitempty"`
	ProteinG *int   `json:"protein_g,omitempty"`
	FatG     *int   `json:"fat_g,omitempty"`
	CarbsG   *int   `json:"carbs_g,omitempty"`
	Meals    []Meal `json:"meals,omitempty"`
}

// Meal — приём пищи.
type Meal struct {
	Name string `json:"name"`
	Kcal *int   `json:"kcal,omitempty"`
}

// Checkin — показатели чек-ина (готовность, сон, алкоголь и т.п.).
type Checkin struct {
	SleepH           *float64 `json:"sleep_h,omitempty"`
	HRV              *int     `json:"hrv,omitempty"`
	RHR              *int     `json:"rhr,omitempty"`
	Mood             *int     `json:"mood,omitempty"`
	AlcoholDoses     *float64 `json:"alcohol_doses,omitempty"`
	AlcoholNote      string   `json:"alcohol_note,omitempty"`
	WeightKG         *float64 `json:"weight_kg,omitempty"`
	Steps            *int     `json:"steps,omitempty"`
	PlannedTraining  string   `json:"planned_training,omitempty"`
	CycleDay         *int     `json:"cycle_day,omitempty"`
	CyclePhase       string   `json:"cycle_phase,omitempty"`
	KneesPain        *int     `json:"knees_pain,omitempty"`
	Craving          *int     `json:"craving,omitempty"`
	PostprandialWalk *bool    `json:"postprandial_walk,omitempty"`
	Fatigue          *int     `json:"fatigue,omitempty"`
	TempDelta        *float64 `json:"temperature_c,omitempty"`
	LoadGoal         *int     `json:"load_goal,omitempty"`
	SleepScore       *int     `json:"sleep_score,omitempty"`
	SpO2Avg          *int     `json:"spo2_avg,omitempty"`
	SpO2Min          *int     `json:"spo2_min,omitempty"`
	RespirationAvg   *int     `json:"respiration_avg,omitempty"`
	RespirationMin   *int     `json:"respiration_min,omitempty"`
	BodyBattery      *int     `json:"body_battery,omitempty"`
	AvgHRNight       *int     `json:"avg_hr_night,omitempty"`
	Context          string   `json:"context,omitempty"`
}

// Extractor извлекает данные через LLM (текст и/или скриншоты).
type Extractor struct {
	text   llm.Provider
	vision llm.Provider
	log    *slog.Logger
	now    func() time.Time
}

// NewExtractor создаёт экстрактор. vision может быть nil (скриншоты выключены).
func NewExtractor(text, vision llm.Provider, log *slog.Logger) *Extractor {
	if log == nil {
		log = slog.Default()
	}
	return &Extractor{text: text, vision: vision, log: log, now: time.Now}
}

// HasVision сообщает, настроен ли vision-провайдер.
func (e *Extractor) HasVision() bool {
	return e.vision != nil
}

// Extract распознаёт данные из изображений (если есть) или текста.
func (e *Extractor) Extract(ctx context.Context, in Input) (Extracted, error) {
	now := e.now()

	if len(in.Images) > 0 {
		if e.vision == nil {
			return Extracted{}, errors.New("intake: vision-провайдер не настроен")
		}
		merged := Extracted{}
		var errs []error
		for index, image := range in.Images {
			part, err := e.extractImage(ctx, image, in.Text, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("изображение %d: %w", index+1, err))
				continue
			}
			merged = mergeExtracted(merged, part)
		}
		if merged.Nutrition == nil && merged.Checkin == nil && merged.Training == nil {
			return Extracted{}, fmt.Errorf("intake: не удалось распознать данные на скриншотах: %w", errors.Join(errs...))
		}
		e.log.Info(logging.Msg("intake:Extract", "данные со скриншотов распознаны"),
			"kinds", strings.Join(merged.Kinds, ","),
			"images", len(in.Images),
			"has_nutrition", merged.Nutrition != nil,
			"has_checkin", merged.Checkin != nil,
			"has_training", merged.Training != nil)
		return merged, nil
	}

	if strings.TrimSpace(in.Text) == "" {
		return Extracted{}, errors.New("intake: нечего распознавать (нет изображений и текста)")
	}

	// Уровень 1: детерминированные правила (FR-6.8.7). LLM — fallback ниже.
	if parsed, ok := parseCheckinText(in.Text, now); ok {
		e.log.Info(logging.Msg("intake:Extract", "чек-ин распознан правилами"),
			"kinds", strings.Join(parsed.Kinds, ","),
			"date", parsed.Date)
		return parsed, nil
	}

	if e.text == nil {
		return Extracted{}, errors.New("intake: LLM-провайдер не настроен")
	}

	raw, err := e.text.Extract(ctx, buildTextPrompt(now, in.Text), textSchema)
	if err != nil {
		return Extracted{}, err
	}
	extracted, err := parseExtracted(raw, now)
	if err != nil {
		return Extracted{}, err
	}
	e.log.Info(logging.Msg("intake:Extract", "данные из текста распознаны"),
		"kinds", strings.Join(extracted.Kinds, ","),
		"has_nutrition", extracted.Nutrition != nil,
		"has_checkin", extracted.Checkin != nil)
	return extracted, nil
}

// LooksLikeNutrition — эвристика для свободного текста с 4 числами питания.
func LooksLikeNutrition(text string) bool {
	lower := strings.ToLower(text)
	hasMarker := false
	for _, marker := range []string{"ккал", "калори", "белк", "протеин", "жир", "углевод", "бжу", "питани"} {
		if strings.Contains(lower, marker) {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return false
	}
	digits := 0
	for _, r := range lower {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= 2
}

// LooksLikeCheckin — эвристика маршрутизации текста (FR-6.8.8): есть маркер
// данных и хотя бы одно число. Вопросы без чисел уходят в LLM-чат.
func LooksLikeCheckin(text string) bool {
	lower := strings.ToLower(text)
	hasMarker := false
	for _, marker := range checkinMarkers {
		if strings.Contains(lower, marker) {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return false
	}
	for _, r := range lower {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

var checkinMarkers = []string{
	"сон", "hrv", "пульс", "настроени", "алкогол", "вес", "шаги",
	"мц", "колени", "голод", "чек-ин", "чек ин", "усталост",
	"температур", "давлени", "самочувств", "стресс",
}

const textPromptTemplate = `Ты извлекаешь данные чек-ина из текста для health-проекта.
Сегодня: %s.
Извлеки, если есть: сон (ЧЧ:ММ), HRV (мс), пульс покоя, настроение (1-10),
алкоголь (дозы и что/когда), вес (кг), шаги, усталость, температуру (отклонение),
план тренировки, контекст; для жены — день МЦ и фазу, боль в коленях (0-10),
голод (1-10), постпрандиальную ходьбу (да/нет).
date_text — дата КАК В ТЕКСТЕ (например «вчера»); если даты нет — пусто. НЕ вычисляй дату.
Не выдумывай значения: чего нет — не заполняй. Верни один JSON без повторений.
Текст пользователя:
%s
`

var textSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"date_text": {"type": "string", "description": "дата как в тексте, дословно"},
		"checkin": {
			"type": "object",
			"properties": {
				"sleep": {"type": "string"},
				"hrv_ms": {"type": "number"},
				"rhr": {"type": "number"},
				"mood": {"type": "number"},
				"alcohol_doses": {"type": "number"},
				"alcohol_note": {"type": "string"},
				"weight_kg": {"type": "number"},
				"steps": {"type": "number"},
				"planned_training": {"type": "string"},
				"cycle_day": {"type": "number"},
				"cycle_phase": {"type": "string"},
				"knees_pain": {"type": "number"},
				"craving": {"type": "number"},
				"postprandial_walk": {"type": "boolean"},
				"fatigue": {"type": "number"},
				"temperature_c": {"type": "number"},
				"load_goal": {"type": "number"},
				"context": {"type": "string"}
			}
		},
		"notes": {"type": "string"}
	}
}`)

func buildTextPrompt(now time.Time, text string) string {
	return fmt.Sprintf(textPromptTemplate, now.Format("2006-01-02"), strings.TrimSpace(text))
}

// --- разбор и нормализация ---

type flexInt int

func (v *flexInt) UnmarshalJSON(data []byte) error {
	parsed, err := parseFlexNumber(data)
	if err != nil {
		return err
	}
	*v = flexInt(math.Round(parsed))
	return nil
}

type flexFloat float64

func (v *flexFloat) UnmarshalJSON(data []byte) error {
	parsed, err := parseFlexNumber(data)
	if err != nil {
		return err
	}
	*v = flexFloat(parsed)
	return nil
}

func parseFlexNumber(data []byte) (float64, error) {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return 0, nil
	}
	text = strings.Trim(text, `"`)
	text = strings.ReplaceAll(text, ",", ".")
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("intake: число %q: %w", text, err)
	}
	return value, nil
}

type wireExtracted struct {
	Kind      string         `json:"kind"`
	Kinds     []string       `json:"kinds"`
	Date      string         `json:"date"`
	DateText  string         `json:"date_text"`
	Nutrition *wireNutrition `json:"nutrition"`
	Checkin   *wireCheckin   `json:"checkin"`
	Training  *wireTraining  `json:"training"`
	Notes     string         `json:"notes"`
}

type wireNutrition struct {
	Kcal     *flexInt   `json:"kcal"`
	ProteinG *flexInt   `json:"protein_g"`
	FatG     *flexInt   `json:"fat_g"`
	CarbsG   *flexInt   `json:"carbs_g"`
	Meals    []wireMeal `json:"meals"`
}

type wireMeal struct {
	Name string   `json:"name"`
	Kcal *flexInt `json:"kcal"`
}

type wireTraining struct {
	Kind        string   `json:"kind"`
	DateText    string   `json:"date_text"`
	DurationMin *flexInt `json:"duration_min"`
	AvgHR       *flexInt `json:"avg_hr"`
	Calories    *flexInt `json:"calories"`
	Exercises   []struct {
		Name     string     `json:"name"`
		WeightKG *flexFloat `json:"weight_kg"`
		Sets     *flexInt   `json:"sets"`
		Reps     *flexInt   `json:"reps"`
		RPE      *flexInt   `json:"rpe"`
		Note     string     `json:"note"`
	} `json:"exercises"`
	Notes string `json:"notes"`
}

type wireCheckin struct {
	Sleep            json.RawMessage `json:"sleep"`
	HRV              *flexInt        `json:"hrv_ms"`
	RHR              *flexInt        `json:"rhr"`
	Mood             *flexInt        `json:"mood"`
	AlcoholDoses     *flexFloat      `json:"alcohol_doses"`
	AlcoholNote      string          `json:"alcohol_note"`
	WeightKG         *flexFloat      `json:"weight_kg"`
	Steps            *flexInt        `json:"steps"`
	PlannedTraining  string          `json:"planned_training"`
	CycleDay         *flexInt        `json:"cycle_day"`
	CyclePhase       string          `json:"cycle_phase"`
	KneesPain        *flexInt        `json:"knees_pain"`
	Craving          *flexInt        `json:"craving"`
	PostprandialWalk *bool           `json:"postprandial_walk"`
	Fatigue          *flexInt        `json:"fatigue"`
	TemperatureC     *flexFloat      `json:"temperature_c"`
	LoadGoal         *flexInt        `json:"load_goal"`
	SleepScore       *flexInt        `json:"sleep_score"`
	SpO2Avg          *flexInt        `json:"spo2_avg"`
	SpO2Min          *flexInt        `json:"spo2_min"`
	RespirationAvg   *flexInt        `json:"respiration_avg"`
	RespirationMin   *flexInt        `json:"respiration_min"`
	BodyBattery      *flexInt        `json:"body_battery"`
	AvgHRNight       *flexInt        `json:"avg_hr_night"`
	Context          string          `json:"context"`
}

func parseExtracted(raw json.RawMessage, now time.Time) (Extracted, error) {
	var wire wireExtracted
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Extracted{}, fmt.Errorf("intake: разобрать JSON модели: %w", err)
	}

	extracted := Extracted{Notes: strings.TrimSpace(wire.Notes)}
	kind := strings.ToLower(strings.TrimSpace(wire.Kind))
	if kind != "" && kind != "other" {
		extracted.Kinds = appendUnique(extracted.Kinds, kind)
	}
	if len(wire.Kinds) > 0 {
		extracted.Kinds = appendUnique(extracted.Kinds, wire.Kinds...)
	}

	if wire.Nutrition != nil {
		nutrition, err := normalizeNutrition(wire.Nutrition)
		if err != nil {
			return Extracted{}, err
		}
		extracted.Nutrition = nutrition
		extracted.Kinds = appendUnique(extracted.Kinds, "nutrition")
	}
	if wire.Checkin != nil {
		checkin, err := normalizeCheckin(wire.Checkin)
		if err != nil {
			return Extracted{}, err
		}
		if checkinHasData(checkin) {
			extracted.Checkin = checkin
			extracted.Kinds = appendUnique(extracted.Kinds, "checkin")
		}
	}
	if wire.Training != nil {
		training := normalizeTraining(wire.Training)
		if trainingHasData(training) {
			extracted.Training = training
			extracted.Kinds = appendUnique(extracted.Kinds, "training")
		}
	}

	date := strings.TrimSpace(wire.Date)
	if date == "" {
		date = parseDateText(wire.DateText, now)
	}
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return Extracted{}, fmt.Errorf("intake: дата %q не в формате YYYY-MM-DD", date)
		}
	}
	extracted.Date = date
	if extracted.Training != nil && extracted.Training.Date == "" {
		extracted.Training.Date = date
	}

	if extracted.Nutrition == nil && extracted.Checkin == nil && extracted.Training == nil {
		return Extracted{}, errors.New("intake: не распознаны данные питания, чек-ина или тренировки")
	}
	return extracted, nil
}

func mergeExtracted(dst, src Extracted) Extracted {
	dst.Kinds = appendUnique(dst.Kinds, src.Kinds...)
	if dst.Date == "" {
		dst.Date = src.Date
	} else if src.Nutrition != nil && src.Date != "" {
		// Дата дневника питания важнее (обычно «вчера»), чем дата экрана готовности.
		dst.Date = src.Date
	}
	dst.Nutrition = mergeNutrition(dst.Nutrition, src.Nutrition)
	dst.Checkin = mergeCheckin(dst.Checkin, src.Checkin)
	dst.Training = mergeTraining(dst.Training, src.Training)
	if src.Notes != "" {
		if dst.Notes != "" {
			dst.Notes += " | "
		}
		dst.Notes += src.Notes
	}
	return dst
}

func appendUnique(values []string, extra ...string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range extra {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func mergeNutrition(dst, src *Nutrition) *Nutrition {
	if src == nil {
		return dst
	}
	if dst == nil {
		return src
	}
	if dst.Kcal == nil {
		dst.Kcal = src.Kcal
	}
	if dst.ProteinG == nil {
		dst.ProteinG = src.ProteinG
	}
	if dst.FatG == nil {
		dst.FatG = src.FatG
	}
	if dst.CarbsG == nil {
		dst.CarbsG = src.CarbsG
	}
	dst.Meals = append(dst.Meals, src.Meals...)
	return dst
}

func mergeCheckin(dst, src *Checkin) *Checkin {
	if src == nil {
		return dst
	}
	if dst == nil {
		return src
	}
	if dst.SleepH == nil {
		dst.SleepH = src.SleepH
	}
	if dst.HRV == nil {
		dst.HRV = src.HRV
	}
	if dst.RHR == nil {
		dst.RHR = src.RHR
	}
	if dst.Mood == nil {
		dst.Mood = src.Mood
	}
	if dst.AlcoholDoses == nil {
		dst.AlcoholDoses = src.AlcoholDoses
	}
	if dst.AlcoholNote == "" {
		dst.AlcoholNote = src.AlcoholNote
	}
	if dst.WeightKG == nil {
		dst.WeightKG = src.WeightKG
	}
	if dst.Steps == nil {
		dst.Steps = src.Steps
	}
	if dst.PlannedTraining == "" {
		dst.PlannedTraining = src.PlannedTraining
	}
	if dst.CycleDay == nil {
		dst.CycleDay = src.CycleDay
	}
	if dst.CyclePhase == "" {
		dst.CyclePhase = src.CyclePhase
	}
	if dst.KneesPain == nil {
		dst.KneesPain = src.KneesPain
	}
	if dst.Craving == nil {
		dst.Craving = src.Craving
	}
	if dst.PostprandialWalk == nil {
		dst.PostprandialWalk = src.PostprandialWalk
	}
	if dst.Fatigue == nil {
		dst.Fatigue = src.Fatigue
	}
	if dst.TempDelta == nil {
		dst.TempDelta = src.TempDelta
	}
	if dst.LoadGoal == nil {
		dst.LoadGoal = src.LoadGoal
	}
	if dst.Context == "" {
		dst.Context = src.Context
	}
	return dst
}

// parseDateText переводит дословную дату со скриншота/из текста в YYYY-MM-DD.
// Модель даты не считает — это делает код детерминированно.
func parseDateText(text string, today time.Time) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}
	if parsed, err := time.Parse("2006-01-02", strings.TrimSpace(text)); err == nil {
		return parsed.Format("2006-01-02")
	}
	if strings.Contains(lower, "позавчера") {
		return today.AddDate(0, 0, -2).Format("2006-01-02")
	}
	if strings.Contains(lower, "вчера") {
		return today.AddDate(0, 0, -1).Format("2006-01-02")
	}
	if strings.Contains(lower, "сегодня") {
		return today.Format("2006-01-02")
	}
	for _, match := range dayMonthPattern.FindAllStringSubmatch(lower, -1) {
		day, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		month, ok := monthNames[match[2]]
		if !ok || day < 1 || day > 31 {
			continue
		}
		candidate := time.Date(today.Year(), month, day, 0, 0, 0, 0, today.Location())
		if candidate.After(today.AddDate(0, 0, 1)) {
			candidate = candidate.AddDate(-1, 0, 0)
		}
		return candidate.Format("2006-01-02")
	}
	return ""
}

var (
	sleepHourMinutePattern = regexp.MustCompile(`(?i)(\d{1,2})\s*(?:ч|h|час[а-яё]*|hours?)\s*(\d{1,2})?\s*(?:м|min[а-яё]*|minutes?|m)?`)
	sleepMinutesPattern    = regexp.MustCompile(`(?i)(\d{1,3})\s*(?:мин[а-яё]*|minutes?|m)\b`)
	dayMonthPattern        = regexp.MustCompile(`(\d{1,2})\s+([а-яё]+)`)
	monthNames             = map[string]time.Month{
		"январь": time.January, "января": time.January, "янв": time.January,
		"февраль": time.February, "февраля": time.February, "фев": time.February,
		"март": time.March, "марта": time.March, "мар": time.March,
		"апрель": time.April, "апреля": time.April, "апр": time.April,
		"май": time.May, "мая": time.May,
		"июнь": time.June, "июня": time.June, "июн": time.June,
		"июль": time.July, "июля": time.July, "июл": time.July,
		"август": time.August, "августа": time.August, "авг": time.August,
		"сентябрь": time.September, "сентября": time.September, "сент": time.September, "сен": time.September,
		"октябрь": time.October, "октября": time.October, "окт": time.October,
		"ноябрь": time.November, "ноября": time.November, "нояб": time.November,
		"декабрь": time.December, "декабря": time.December, "дек": time.December,
	}
)

func normalizeNutrition(wire *wireNutrition) (*Nutrition, error) {
	nutrition := &Nutrition{
		Kcal:     intPtr(wire.Kcal),
		ProteinG: intPtr(wire.ProteinG),
		FatG:     intPtr(wire.FatG),
		CarbsG:   intPtr(wire.CarbsG),
	}
	for _, meal := range wire.Meals {
		nutrition.Meals = append(nutrition.Meals, Meal{
			Name: strings.TrimSpace(meal.Name),
			Kcal: intPtr(meal.Kcal),
		})
	}

	if err := checkIntRange("ккал", nutrition.Kcal, 0, 6000); err != nil {
		return nil, err
	}
	for name, value := range map[string]*int{
		"белок": nutrition.ProteinG, "жиры": nutrition.FatG, "углеводы": nutrition.CarbsG,
	} {
		if err := checkIntRange(name, value, 0, 500); err != nil {
			return nil, err
		}
	}
	for _, meal := range nutrition.Meals {
		if err := checkIntRange("ккал приёма пищи", meal.Kcal, 0, 3000); err != nil {
			return nil, err
		}
	}
	return nutrition, nil
}

func normalizeCheckin(wire *wireCheckin) (*Checkin, error) {
	sleepHours, err := parseSleep(wire.Sleep)
	if err != nil {
		return nil, err
	}

	checkin := &Checkin{
		SleepH:           sleepHours,
		HRV:              intPtr(wire.HRV),
		RHR:              intPtr(wire.RHR),
		Mood:             intPtr(wire.Mood),
		AlcoholDoses:     floatPtr(wire.AlcoholDoses),
		AlcoholNote:      strings.TrimSpace(wire.AlcoholNote),
		WeightKG:         floatPtr(wire.WeightKG),
		Steps:            intPtr(wire.Steps),
		PlannedTraining:  strings.TrimSpace(wire.PlannedTraining),
		CycleDay:         intPtr(wire.CycleDay),
		CyclePhase:       strings.TrimSpace(wire.CyclePhase),
		KneesPain:        intPtr(wire.KneesPain),
		Craving:          intPtr(wire.Craving),
		PostprandialWalk: wire.PostprandialWalk,
		Fatigue:          intPtr(wire.Fatigue),
		TempDelta:        floatPtr(wire.TemperatureC),
		LoadGoal:         intPtr(wire.LoadGoal),
		SleepScore:       intPtr(wire.SleepScore),
		SpO2Avg:          intPtr(wire.SpO2Avg),
		SpO2Min:          intPtr(wire.SpO2Min),
		RespirationAvg:   intPtr(wire.RespirationAvg),
		RespirationMin:   intPtr(wire.RespirationMin),
		BodyBattery:      intPtr(wire.BodyBattery),
		AvgHRNight:       intPtr(wire.AvgHRNight),
		Context:          strings.TrimSpace(wire.Context),
	}

	sanitizeCheckin(checkin)
	appendCheckinExtras(checkin)
	return checkin, nil
}

func parseSleep(raw json.RawMessage) (*float64, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return nil, nil
	}
	if strings.HasPrefix(text, `"`) {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, nil
		}
		text = strings.TrimSpace(value)
	}
	if text == "" {
		return nil, nil
	}
	text = strings.ToLower(strings.ReplaceAll(text, ",", "."))

	// 8:09
	if parts := strings.SplitN(text, ":", 2); len(parts) == 2 {
		hours, errH := strconv.Atoi(strings.TrimSpace(parts[0]))
		minutes, errM := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errH == nil && errM == nil && hours >= 0 && hours <= 16 && minutes >= 0 && minutes <= 59 {
			value := float64(hours) + float64(minutes)/60
			return &value, nil
		}
	}
	// 8 ч 9 м / 8h 9m / 8 hours 9 min
	if match := sleepHourMinutePattern.FindStringSubmatch(text); match != nil {
		hours, errH := strconv.Atoi(match[1])
		minutes := 0
		if match[2] != "" {
			minutes, _ = strconv.Atoi(match[2])
		}
		if errH == nil && hours >= 0 && hours <= 16 && minutes >= 0 && minutes <= 59 {
			value := float64(hours) + float64(minutes)/60
			return &value, nil
		}
	}
	// 489 мин
	if match := sleepMinutesPattern.FindStringSubmatch(text); match != nil {
		minutes, err := strconv.Atoi(match[1])
		if err == nil && minutes > 0 && minutes <= 16*60 {
			value := float64(minutes) / 60
			return &value, nil
		}
	}
	// 8.15 — часы с долями
	if value, err := strconv.ParseFloat(text, 64); err == nil && value >= 0 && value <= 16 {
		return &value, nil
	}
	// не длительность — не ломаем весь чек-ин
	return nil, nil
}

func sanitizeCheckin(checkin *Checkin) {
	if checkin.SleepH != nil && (*checkin.SleepH < 0 || *checkin.SleepH > 16) {
		checkin.SleepH = nil
	}
	if checkin.HRV != nil && (*checkin.HRV < 10 || *checkin.HRV > 150) {
		checkin.HRV = nil
	}
	if checkin.RHR != nil && (*checkin.RHR < 30 || *checkin.RHR > 120) {
		checkin.RHR = nil
	}
	if checkin.Mood != nil && (*checkin.Mood < 1 || *checkin.Mood > 10) {
		checkin.Mood = nil
	}
	if checkin.AlcoholDoses != nil && (*checkin.AlcoholDoses < 0 || *checkin.AlcoholDoses > 20) {
		checkin.AlcoholDoses = nil
	}
	if checkin.WeightKG != nil && (*checkin.WeightKG < 30 || *checkin.WeightKG > 250) {
		checkin.WeightKG = nil
	}
	if checkin.Steps != nil && (*checkin.Steps < 0 || *checkin.Steps > 200000) {
		checkin.Steps = nil
	}
	if checkin.CycleDay != nil && (*checkin.CycleDay < 0 || *checkin.CycleDay > 60) {
		checkin.CycleDay = nil
	}
	if checkin.KneesPain != nil && (*checkin.KneesPain < 0 || *checkin.KneesPain > 10) {
		checkin.KneesPain = nil
	}
	if checkin.Craving != nil && (*checkin.Craving < 0 || *checkin.Craving > 10) {
		checkin.Craving = nil
	}
	if checkin.Fatigue != nil && (*checkin.Fatigue < 0 || *checkin.Fatigue > 100) {
		checkin.Fatigue = nil
	}
	if checkin.TempDelta != nil && (*checkin.TempDelta < -5 || *checkin.TempDelta > 5) {
		checkin.TempDelta = nil
	}
	if checkin.LoadGoal != nil && (*checkin.LoadGoal < 0 || *checkin.LoadGoal > 100) {
		checkin.LoadGoal = nil
	}
	if checkin.SleepScore != nil && (*checkin.SleepScore < 1 || *checkin.SleepScore > 100) {
		checkin.SleepScore = nil
	}
	if checkin.SpO2Avg != nil && (*checkin.SpO2Avg < 70 || *checkin.SpO2Avg > 100) {
		checkin.SpO2Avg = nil
	}
	if checkin.SpO2Min != nil && (*checkin.SpO2Min < 50 || *checkin.SpO2Min > 100) {
		checkin.SpO2Min = nil
	}
	if checkin.RespirationAvg != nil && (*checkin.RespirationAvg < 4 || *checkin.RespirationAvg > 60) {
		checkin.RespirationAvg = nil
	}
	if checkin.RespirationMin != nil && (*checkin.RespirationMin < 2 || *checkin.RespirationMin > 60) {
		checkin.RespirationMin = nil
	}
	if checkin.BodyBattery != nil && (*checkin.BodyBattery < -100 || *checkin.BodyBattery > 100) {
		checkin.BodyBattery = nil
	}
	if checkin.AvgHRNight != nil && (*checkin.AvgHRNight < 30 || *checkin.AvgHRNight > 220) {
		checkin.AvgHRNight = nil
	}
}

func appendCheckinExtras(checkin *Checkin) {
	parts := make([]string, 0, 4)
	if checkin.SleepScore != nil {
		parts = append(parts, fmt.Sprintf("качество сна %d/100", *checkin.SleepScore))
	}
	if checkin.SpO2Avg != nil {
		parts = append(parts, fmt.Sprintf("SpO2 %d%%", *checkin.SpO2Avg))
	}
	if checkin.SpO2Min != nil {
		parts = append(parts, fmt.Sprintf("мин SpO2 %d%%", *checkin.SpO2Min))
	}
	if checkin.RespirationAvg != nil {
		parts = append(parts, fmt.Sprintf("дыхание %d/мин", *checkin.RespirationAvg))
	}
	if checkin.BodyBattery != nil {
		parts = append(parts, fmt.Sprintf("Body Battery %+d", *checkin.BodyBattery))
	}
	if checkin.AvgHRNight != nil {
		parts = append(parts, fmt.Sprintf("ср. ЧСС ночью %d", *checkin.AvgHRNight))
	}
	if len(parts) == 0 {
		return
	}
	extra := strings.Join(parts, ", ")
	if checkin.Context == "" {
		checkin.Context = extra
	} else {
		checkin.Context += "; " + extra
	}
}

func checkinHasData(checkin *Checkin) bool {
	return checkin.SleepH != nil || checkin.HRV != nil || checkin.RHR != nil ||
		checkin.Mood != nil || checkin.AlcoholDoses != nil || checkin.WeightKG != nil ||
		checkin.Steps != nil || checkin.PlannedTraining != "" || checkin.CycleDay != nil ||
		checkin.CyclePhase != "" || checkin.KneesPain != nil || checkin.Craving != nil ||
		checkin.PostprandialWalk != nil || checkin.Fatigue != nil || checkin.TempDelta != nil ||
		checkin.LoadGoal != nil || checkin.SleepScore != nil || checkin.SpO2Avg != nil ||
		checkin.RespirationAvg != nil || checkin.BodyBattery != nil ||
		checkin.AvgHRNight != nil || checkin.Context != ""
}

func trainingHasData(training *Training) bool {
	return training.Kind != "" || training.DurationMin != nil || training.AvgHR != nil ||
		training.Calories != nil || len(training.Exercises) > 0 || training.Notes != ""
}

// FormatReply собирает краткий ответ в чат (FR-6.8.5). Если данные уже
// сохранены, передаются пути — вместо «Сохранение — на M1» пишем «Сохранено».
func FormatReply(extracted Extracted, savedPaths ...string) string {
	var builder strings.Builder
	builder.WriteString("Отметил данные")
	if extracted.Date != "" {
		builder.WriteString(" за " + formatDateHuman(extracted.Date))
	}
	builder.WriteString(":\n")

	if extracted.Checkin != nil {
		if line := formatCheckin(extracted.Checkin); line != "" {
			builder.WriteString("• " + line + "\n")
		}
	}
	if extracted.Nutrition != nil {
		if line := formatNutrition(extracted.Nutrition); line != "" {
			builder.WriteString("• " + line + "\n")
		}
	}
	if extracted.Training != nil {
		if line := formatTraining(extracted.Training); line != "" {
			builder.WriteString("• " + line + "\n")
		}
	}
	if extracted.Nutrition != nil && len(extracted.Nutrition.Meals) > 0 {
		parts := make([]string, 0, len(extracted.Nutrition.Meals))
		for _, meal := range extracted.Nutrition.Meals {
			item := meal.Name
			if meal.Kcal != nil {
				item += fmt.Sprintf(" %d", *meal.Kcal)
			}
			parts = append(parts, strings.TrimSpace(item))
		}
		builder.WriteString("  Приёмы: " + strings.Join(parts, ", ") + "\n")
	}

	if len(savedPaths) > 0 {
		builder.WriteString("\nСохранено: " + strings.Join(savedPaths, "; "))
	} else {
		builder.WriteString("\nВсё верно? Сохранение — в файл.")
	}
	return strings.TrimSpace(builder.String())
}

func formatCheckin(checkin *Checkin) string {
	parts := make([]string, 0, 8)
	if checkin.SleepH != nil {
		parts = append(parts, "сон "+formatSleep(*checkin.SleepH))
	}
	if checkin.HRV != nil {
		parts = append(parts, fmt.Sprintf("HRV %d мс", *checkin.HRV))
	}
	if checkin.RHR != nil {
		parts = append(parts, fmt.Sprintf("пульс %d", *checkin.RHR))
	}
	if checkin.Mood != nil {
		parts = append(parts, fmt.Sprintf("настроение %d/10", *checkin.Mood))
	}
	if checkin.AlcoholDoses != nil {
		parts = append(parts, fmt.Sprintf("алкоголь %.1f дозы", *checkin.AlcoholDoses))
	}
	if checkin.WeightKG != nil {
		parts = append(parts, fmt.Sprintf("вес %.1f кг", *checkin.WeightKG))
	}
	if checkin.Steps != nil {
		parts = append(parts, fmt.Sprintf("шаги %d", *checkin.Steps))
	}
	if checkin.SleepScore != nil {
		parts = append(parts, fmt.Sprintf("качество сна %d/100", *checkin.SleepScore))
	}
	if checkin.Fatigue != nil {
		parts = append(parts, fmt.Sprintf("усталость %d", *checkin.Fatigue))
	}
	if checkin.TempDelta != nil {
		parts = append(parts, fmt.Sprintf("t %+.1f °C", *checkin.TempDelta))
	}
	if checkin.LoadGoal != nil {
		parts = append(parts, fmt.Sprintf("цель нагрузки %d", *checkin.LoadGoal))
	}
	if checkin.CycleDay != nil {
		parts = append(parts, fmt.Sprintf("день МЦ %d", *checkin.CycleDay))
	}
	if checkin.KneesPain != nil {
		parts = append(parts, fmt.Sprintf("колени %d/10", *checkin.KneesPain))
	}
	return strings.Join(parts, ", ")
}

func formatNutrition(nutrition *Nutrition) string {
	parts := make([]string, 0, 4)
	if nutrition.Kcal != nil {
		parts = append(parts, fmt.Sprintf("%d ккал", *nutrition.Kcal))
	}
	if nutrition.ProteinG != nil {
		parts = append(parts, fmt.Sprintf("Б %d", *nutrition.ProteinG))
	}
	if nutrition.FatG != nil {
		parts = append(parts, fmt.Sprintf("Ж %d", *nutrition.FatG))
	}
	if nutrition.CarbsG != nil {
		parts = append(parts, fmt.Sprintf("У %d", *nutrition.CarbsG))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Питание: " + strings.Join(parts, " · ")
}

// formatDateHuman переводит дату файлов (YYYY-MM-DD) в формат региона ДД.ММ.ГГГГ.
func formatDateHuman(date string) string {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return date
	}
	return parsed.Format("02.01.2006")
}

func formatSleep(hours float64) string {
	totalMinutes := int(math.Round(hours * 60))
	hh := totalMinutes / 60
	mm := totalMinutes % 60
	return fmt.Sprintf("%02d:%02d", hh, mm)
}

func intPtr(value *flexInt) *int {
	if value == nil {
		return nil
	}
	parsed := int(*value)
	return &parsed
}

func floatPtr(value *flexFloat) *float64 {
	if value == nil {
		return nil
	}
	parsed := float64(*value)
	return &parsed
}

func checkIntRange(name string, value *int, minValue, maxValue int) error {
	if value == nil {
		return nil
	}
	if *value < minValue || *value > maxValue {
		return fmt.Errorf("intake: %s=%d вне диапазона %d..%d", name, *value, minValue, maxValue)
	}
	return nil
}
