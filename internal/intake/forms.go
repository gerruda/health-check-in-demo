package intake

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"healthbot/internal/logging"
	"healthbot/internal/nlu"
)

const formTimeout = 15 * time.Minute

// MeasurementSaver сохраняет недельный замер (реализация — app поверх store).
type MeasurementSaver interface {
	SaveMeasurement(measurement nlu.Measurement) (string, error)
}

type formField struct {
	key    string
	prompt string
	parse  func(string) (string, error)
}

type formDefinition struct {
	name   string
	intro  string
	fields []formField
}

type formState struct {
	name    string
	step    int
	values  map[string]string
	expires time.Time
}

func (s *Service) formDefinitions() map[string]formDefinition {
	return map[string]formDefinition{
		"checkin": {
			name:  "checkin",
			intro: "🌿 Чек-ин по шагам. /cancel — выйти.",
			fields: []formField{
				{key: "sleep", prompt: "Сон в часах (например 7:30 или 7.5):", parse: parseSleepField},
				{key: "hrv", prompt: "HRV, мс:", parse: parseIntField(10, 150)},
				{key: "rhr", prompt: "Пульс покоя, уд/мин:", parse: parseIntField(30, 120)},
				{key: "mood", prompt: "Настроение 1–10:", parse: parseIntField(1, 10)},
				{key: "alcohol", prompt: "Алкоголь вчера, доз (0 — нет):", parse: parseFloatField(0, 20)},
				{key: "plan", prompt: "Плановая тренировка (или «нет»):", parse: parseTextField},
			},
		},
		"training": {
			name:  "training",
			intro: "🌿 Тренировка по шагам. /cancel — выйти.",
			fields: []formField{
				{key: "kind", prompt: "Тип: sila-A, sila-B, fitboxing, ulichnaya, vosstanovlenie (жена: silovaya, kardio, progulka):", parse: parseTextField},
				{key: "exercises", prompt: "Упражнения как в отчёте: «присед 60x5x5, жим 40x8»:", parse: parseTextField},
				{key: "rpe", prompt: "RPE сессии 1–10:", parse: parseIntField(1, 10)},
				{key: "duration", prompt: "Длительность, мин:", parse: parseIntField(1, 600)},
				{key: "avg_hr", prompt: "Средний пульс:", parse: parseIntField(30, 220)},
			},
		},
		"food": {
			name:  "food",
			intro: "🌿 Питание за день. /cancel — выйти.",
			fields: []formField{
				{key: "kcal", prompt: "Ккал:", parse: parseIntField(0, 10000)},
				{key: "protein", prompt: "Белок, г:", parse: parseIntField(0, 1000)},
				{key: "fat", prompt: "Жиры, г:", parse: parseIntField(0, 1000)},
				{key: "carbs", prompt: "Углеводы, г:", parse: parseIntField(0, 1000)},
			},
		},
		"measure": {
			name:  "measure",
			intro: "🌿 Недельный замер по шагам. /cancel — выйти.",
			fields: []formField{
				{key: "weight", prompt: "Вес, кг:", parse: parseFloatField(20, 300)},
				{key: "fat", prompt: "Жир, %:", parse: parseFloatField(0, 70)},
				{key: "muscle", prompt: "Мышцы, %:", parse: parseFloatField(0, 80)},
				{key: "visceral", prompt: "Висцеральный индекс:", parse: parseIntField(0, 40)},
				{key: "water", prompt: "Вода, %:", parse: parseFloatField(0, 80)},
				{key: "waist", prompt: "Талия, см:", parse: parseFloatField(30, 250)},
				{key: "tape", prompt: "Лента: грудь бицепс ягодицы бедро икра через пробел (или «-»):", parse: parseTapeField},
			},
		},
	}
}

// StartForm запускает форму и отправляет первый вопрос.
func (s *Service) StartForm(ctx context.Context, chatID int64, name string) bool {
	definition, ok := s.formDefinitions()[name]
	if !ok {
		return false
	}
	s.formMu.Lock()
	s.forms[chatID] = &formState{name: name, values: make(map[string]string), expires: s.now().Add(formTimeout)}
	s.formMu.Unlock()
	s.clearPending(chatID)
	s.sendFormMessage(ctx, chatID, definition)
	return true
}

// HandleFormAnswer принимает ответ на текущий вопрос формы. true — сообщение
// поглощено формой (даже если ответ невалиден и шаг повторён).
func (s *Service) HandleFormAnswer(ctx context.Context, chatID int64, text string) bool {
	s.formMu.Lock()
	state, ok := s.forms[chatID]
	if !ok {
		s.formMu.Unlock()
		return false
	}
	if s.now().After(state.expires) {
		delete(s.forms, chatID)
		s.formMu.Unlock()
		return false
	}
	definition := s.formDefinitions()[state.name]
	if state.step >= len(definition.fields) {
		delete(s.forms, chatID)
		s.formMu.Unlock()
		return false
	}
	field := definition.fields[state.step]
	value, err := field.parse(text)
	if err != nil {
		s.formMu.Unlock()
		s.sendText(ctx, chatID, "🌿 "+err.Error()+". Попробуй ещё раз или /cancel.")
		return true
	}
	state.values[field.key] = value
	state.step++
	state.expires = s.now().Add(formTimeout)
	if state.step >= len(definition.fields) {
		values := state.values
		delete(s.forms, chatID)
		s.formMu.Unlock()
		s.applyForm(ctx, chatID, state.name, values)
		return true
	}
	next := definition.fields[state.step]
	s.formMu.Unlock()
	s.sendText(ctx, chatID, next.prompt)
	return true
}

// HasActiveForm сообщает, идёт ли форма.
func (s *Service) HasActiveForm(chatID int64) bool {
	s.formMu.Lock()
	defer s.formMu.Unlock()
	state, ok := s.forms[chatID]
	if !ok {
		return false
	}
	if s.now().After(state.expires) {
		delete(s.forms, chatID)
		return false
	}
	return true
}

// CancelForm отменяет форму/уточнение.
func (s *Service) CancelForm(ctx context.Context, chatID int64) bool {
	s.formMu.Lock()
	_, hasForm := s.forms[chatID]
	delete(s.forms, chatID)
	s.formMu.Unlock()
	s.clearPending(chatID)
	if hasForm {
		s.sendText(ctx, chatID, "🌿 Ок, форму закрыл. Можно писать свободным текстом.")
	}
	return hasForm
}

func (s *Service) sendFormMessage(ctx context.Context, chatID int64, definition formDefinition) {
	lines := []string{definition.intro}
	if len(definition.fields) > 0 {
		lines = append(lines, "", definition.fields[0].prompt)
	}
	s.sendText(ctx, chatID, strings.Join(lines, "\n"))
}

func (s *Service) applyForm(ctx context.Context, chatID int64, name string, values map[string]string) {
	if s.saver == nil {
		s.sendText(ctx, chatID, "🌿 Сохранение не настроено.")
		return
	}
	switch name {
	case "checkin":
		s.applyCheckinForm(ctx, chatID, values)
	case "training":
		s.applyTrainingForm(ctx, chatID, values)
	case "food":
		s.applyFoodForm(ctx, chatID, values)
	case "measure":
		s.applyMeasureForm(ctx, chatID, values)
	}
}

func (s *Service) applyCheckinForm(ctx context.Context, chatID int64, values map[string]string) {
	person := s.personFor(chatID)
	date := s.now()
	checkin := nlu.Checkin{
		Date: date.Format("2006-01-02"), Person: person,
		SleepH: optionalFloat(values["sleep"]), HRV: intPtrValue(values["hrv"]),
		RHR: intPtrValue(values["rhr"]), Mood: intPtrValue(values["mood"]),
		AlcoholDoses:    optionalFloat(values["alcohol"]),
		PlannedTraining: strings.TrimSpace(values["plan"]),
	}
	path, err := s.saver.SaveCheckin(checkin)
	if err != nil {
		s.sendText(ctx, chatID, "🌿 Не удалось сохранить чек-ин: "+err.Error())
		return
	}
	reply := "🌿 Чек-ин записан."
	if s.coach != nil {
		extracted := Extracted{Kinds: []string{"checkin"}, Date: checkin.Date, Checkin: &Checkin{
			SleepH: checkin.SleepH, HRV: checkin.HRV, RHR: checkin.RHR, Mood: checkin.Mood,
			AlcoholDoses: checkin.AlcoholDoses, PlannedTraining: checkin.PlannedTraining,
		}}
		if advice, coachErr := s.coach.CheckinReply(chatID, extracted); coachErr == nil && strings.TrimSpace(advice) != "" {
			reply = strings.TrimSpace(advice)
		} else if coachErr != nil {
			s.log.Warn(logging.Msg("intake:applyCheckinForm", "коуч не собрал ответ"), "error", coachErr)
		}
	}
	reply += "\nСохранено: " + path
	s.sendText(ctx, chatID, reply)
}

func (s *Service) applyTrainingForm(ctx context.Context, chatID int64, values map[string]string) {
	person := s.personFor(chatID)
	date := s.now()
	parsed, ok := ParseTraining(values["exercises"], person, date)
	training := parsed
	if !ok {
		training = nlu.Training{Date: date.Format("2006-01-02"), Person: person, Done: true}
	}
	training.Date = date.Format("2006-01-02")
	training.Person = person
	training.Done = true
	if kind := strings.TrimSpace(values["kind"]); kind != "" {
		training.Kind = kind
	}
	if training.Kind == "" {
		training.Kind = "sila-A"
	}
	training.RPESession = intPtrValue(values["rpe"])
	training.DurationMin = intPtrValue(values["duration"])
	training.AvgHR = intPtrValue(values["avg_hr"])

	path, saveErr := s.persistTraining(ctx, chatID, training)
	if saveErr != nil {
		s.sendText(ctx, chatID, "🌿 Не удалось сохранить тренировку: "+saveErr.Error())
		return
	}
	reply := FormatTrainingReply(training, "")
	if s.trainingCoach != nil {
		if advice, coachErr := s.trainingCoach.TrainingReply(chatID, training); coachErr == nil && strings.TrimSpace(advice) != "" {
			reply = strings.TrimSpace(advice)
		}
	}
	reply += "\nСохранено: " + path
	s.sendText(ctx, chatID, reply)
}

func (s *Service) applyFoodForm(ctx context.Context, chatID int64, values map[string]string) {
	person := s.personFor(chatID)
	nutrition := nlu.Nutrition{
		Date: s.now().Format("2006-01-02"), Person: person,
		Kcal: intPtrValue(values["kcal"]), ProteinG: intPtrValue(values["protein"]),
		FatG: intPtrValue(values["fat"]), CarbsG: intPtrValue(values["carbs"]),
	}
	path, err := s.saver.SaveNutrition(nutrition)
	if err != nil {
		s.sendText(ctx, chatID, "🌿 Не удалось сохранить питание: "+err.Error())
		return
	}
	reply := fmt.Sprintf("🌿 Питание записано: %d ккал, Б %d / Ж %d / У %d.",
		*nutrition.Kcal, *nutrition.ProteinG, *nutrition.FatG, *nutrition.CarbsG)
	if s.nutritionCoach != nil {
		if advice, coachErr := s.nutritionCoach.NutritionReply(chatID, nutrition); coachErr == nil && strings.TrimSpace(advice) != "" {
			reply = strings.TrimSpace(advice)
		} else if coachErr != nil {
			s.log.Warn(logging.Msg("intake:applyFoodForm", "не удалось собрать разбор питания"), "error", coachErr)
		}
	}
	reply += "\nСохранено: " + path
	s.sendText(ctx, chatID, reply)
}

func (s *Service) applyMeasureForm(ctx context.Context, chatID int64, values map[string]string) {
	saver, ok := s.saver.(MeasurementSaver)
	if !ok {
		s.sendText(ctx, chatID, "🌿 Сохранение замеров не настроено.")
		return
	}
	tape := map[string]float64{}
	if values["tape"] != "" {
		parts := strings.Fields(values["tape"])
		keys := []string{"chest", "biceps", "hips", "thigh", "calf"}
		if len(parts) != len(keys) {
			s.sendText(ctx, chatID, "🌿 Лента: нужно 5 чисел (грудь бицепс ягодицы бедро икра) или «-».")
			return
		}
		for index, part := range parts {
			value, err := strconv.ParseFloat(strings.ReplaceAll(part, ",", "."), 64)
			if err != nil {
				s.sendText(ctx, chatID, "🌿 Не понял объём «"+part+"». Начни замер заново: /measure.")
				return
			}
			tape[keys[index]] = value
		}
	}
	measurement := nlu.Measurement{
		Date: s.now().Format("2006-01-02"), Person: s.personFor(chatID),
		WeightKG: optionalFloat(values["weight"]), FatPct: optionalFloat(values["fat"]),
		MusclePct: optionalFloat(values["muscle"]), Visceral: intPtrValue(values["visceral"]),
		WaterPct: optionalFloat(values["water"]), WaistCM: optionalFloat(values["waist"]),
		Tape: tape,
	}
	path, err := saver.SaveMeasurement(measurement)
	if err != nil {
		s.sendText(ctx, chatID, "🌿 Не удалось сохранить замер: "+err.Error())
		return
	}
	s.sendText(ctx, chatID, "🌿 Замер записан.\nСохранено: "+path)
}

func parseTextField(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return "", fmt.Errorf("нужно значение")
	}
	return value, nil
}

func parseIntField(minValue, maxValue int) func(string) (string, error) {
	return func(value string) (string, error) {
		value = strings.TrimSpace(strings.TrimSuffix(value, "уд/мин"))
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < minValue || parsed > maxValue {
			return "", fmt.Errorf("нужно целое число %d–%d", minValue, maxValue)
		}
		return strconv.Itoa(parsed), nil
	}
}

func parseFloatField(minValue, maxValue float64) func(string) (string, error) {
	return func(value string) (string, error) {
		raw := strings.TrimSpace(value)
		raw = strings.TrimSuffix(raw, "кг")
		raw = strings.TrimSuffix(raw, "см")
		raw = strings.TrimSuffix(raw, "%")
		parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(raw), ",", "."), 64)
		if err != nil || parsed < minValue || parsed > maxValue {
			return "", fmt.Errorf("нужно число %.0f–%.0f", minValue, maxValue)
		}
		return strconv.FormatFloat(parsed, 'f', 2, 64), nil
	}
}

func parseSleepField(value string) (string, error) {
	raw := strings.ToLower(strings.TrimSpace(value))
	raw = strings.ReplaceAll(raw, "ч", ":")
	raw = strings.ReplaceAll(raw, " ", "")
	raw = strings.TrimSuffix(raw, "мин")
	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 2)
		hours, errH := strconv.Atoi(parts[0])
		minutes := 0
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			minutes, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(parts[1]), "м"))
		}
		if errH != nil || hours < 0 || hours > 16 || minutes < 0 || minutes > 59 {
			return "", fmt.Errorf("сон в формате 7:30 или 7.5")
		}
		return strconv.FormatFloat(float64(hours)+float64(minutes)/60, 'f', 2, 64), nil
	}
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
	if err != nil || parsed < 0 || parsed > 16 {
		return "", fmt.Errorf("сон в формате 7:30 или 7.5")
	}
	return strconv.FormatFloat(parsed, 'f', 2, 64), nil
}

func parseTapeField(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" || strings.EqualFold(value, "нет") {
		return "", nil
	}
	return value, nil
}

func optionalFloat(value string) *float64 {
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(value), ",", "."), 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func intPtrValue(value string) *int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	return &parsed
}
