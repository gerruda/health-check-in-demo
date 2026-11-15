package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// visionClassifySchema — первый проход: только тип экрана.
var visionClassifySchema = json.RawMessage(`{
	"type": "object",
	"properties": {"kind": {"type": "string", "enum": ["sleep_quality", "sleep_metrics", "sleep", "workout", "nutrition", "other"]}},
	"required": ["kind"]
}`)

var sleepVisionSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"date_text": {"type": "string"},
		"sleep": {"type": "string"},
		"sleep_score": {"type": "number"},
		"hrv_ms": {"type": "number"},
		"rhr": {"type": "number"},
		"avg_hr_night": {"type": "number"},
		"spo2_avg": {"type": "number"},
		"spo2_min": {"type": "number"},
		"respiration_avg": {"type": "number"},
		"respiration_min": {"type": "number"},
		"body_battery": {"type": "number"}
	}
}`)

var sleepQualityVisionSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"date_text": {"type": "string"},
		"sleep": {"type": "string"},
		"sleep_score": {"type": "number"}
	}
}`)

var sleepMetricsVisionSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"date_text": {"type": "string"},
		"hrv_ms": {"type": "number"},
		"rhr": {"type": "number"},
		"avg_hr_night": {"type": "number"},
		"spo2_avg": {"type": "number"},
		"spo2_min": {"type": "number"},
		"respiration_avg": {"type": "number"},
		"respiration_min": {"type": "number"},
		"body_battery": {"type": "number"}
	}
}`)

var workoutVisionSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"date_text": {"type": "string"},
		"kind": {"type": "string"},
		"duration_min": {"type": "number"},
		"avg_hr": {"type": "number"},
		"calories": {"type": "number"},
		"notes": {"type": "string"},
		"exercises": {"type": "array", "items": {"type": "object", "properties": {
			"name": {"type": "string"}, "sets": {"type": "number"},
			"reps": {"type": "number"}, "weight_kg": {"type": "number"}
		}}}
	}
}`)

var nutritionVisionSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"date_text": {"type": "string"},
		"kcal": {"type": "number"},
		"protein_g": {"type": "number"},
		"fat_g": {"type": "number"},
		"carbs_g": {"type": "number"},
		"meals": {"type": "array", "items": {"type": "object", "properties": {"name": {"type": "string"}, "kcal": {"type": "number"}}}}
	}
}`)

func buildClassifyPrompt(now time.Time, caption string) string {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		caption = "нет"
	}
	return fmt.Sprintf(classifyPromptTemplate, now.Format("2006-01-02"), caption)
}

const classifyPromptTemplate = `Сегодня: %s. Подпись пользователя: %s.
Определи тип экрана смартфона:
- sleep_quality — экран с баллами/качеством/длительностью сна («Баллы», «Длительность», «Качество сна»);
- sleep_metrics — экран метрик сна без баллов/длительности («Ср. ВСР ночью», «SpO2», «дыхание», «Body Battery», «Средняя ЧСС ночью»);
- sleep — сон/готовность, если не удаётся отнести к двум выше;
- workout — тренировка (общее время, средняя ЧСС, калории, таблица упражнений);
- nutrition — дневник питания (ккал, белки/жиры/углеводы, приёмы пищи);
- other — всё остальное.
Ответ: только JSON {"kind":"sleep|workout|nutrition|other"}.`

const sleepVisionPrompt = `Экран сна/метрик часов. Заполни ТОЛЬКО явно видимые значения, иначе null.
- sleep — строка рядом с подписью «Длительность»/«Длительность сна» (например «8 ч 9 м»).
  НЕ бери среднюю ЧСС, пульс, баллы и вкладки «1 д/7 дн.» — если подписи длительности нет, sleep=null.
- sleep_score — число ТОЛЬКО рядом с подписью «Баллы»/«Качество» (например 60 из «60/100»). Если подписи «Баллы» нет — null (не бери «50 возбужденные состояния»).
- hrv_ms — «Ср. ВСР ночью»/ВСР/HRV («34 мс»).
- rhr — «Частота пульса в состоянии покоя»/«Пульс покоя» («65 уд/м»).
- avg_hr_night — «Средняя ЧСС ночью».
- spo2_avg, spo2_min — SpO2, ТОЛЬКО если на экране есть подпись SpO2/кислород.
- respiration_avg, respiration_min — частота дыхания, только если есть подпись.
- body_battery — изменение Body Battery/энергии, только если есть подпись; диапазон -100..100.
- date_text — дата на экране дословно («Сегодня», «Вчера»), не вычисляй.
Не выдумывай числа. Верни JSON без повторов.`

const sleepQualityVisionPrompt = `Экран качества/длительности сна. Заполни ТОЛЬКО:
- sleep — строка рядом с подписью «Длительность»/«Длительность сна» (например «8 ч 9 м»); НЕ пульс, НЕ баллы;
- sleep_score — число рядом с подписью «Баллы»/«Качество» (60 из «60/100»);
- date_text — дата экрана дословно.
Не заполняй HRV, пульс, SpO2, дыхание, Body Battery — их на этом экране нет.
Если подписи нет — null. Верни JSON без повторов.`

const sleepMetricsVisionPrompt = `Экран метрик сна/готовности. Заполни ТОЛЬКО явно видимые значения:
- hrv_ms — «Ср. ВСР ночью»/ВСР/HRV (34 мс);
- rhr — «Частота пульса в состоянии покоя»/«Пульс покоя» (65);
- avg_hr_night — «Средняя ЧСС ночью» (70);
- spo2_avg/spo2_min — SpO2 (96/86);
- respiration_avg/respiration_min — частота дыхания (19/10);
- body_battery — изменение Body Battery (+46).
НЕ бери «50 возбужденные состояния» как баллы; НЕ заполняй sleep/score — их здесь нет.
date_text — дата экрана. Если подписи нет — null. JSON без повторов.`

const workoutVisionPrompt = `Экран тренировки. Заполни:
- duration_min — «Общее время» (31:15 → 31; 45 мин → 45);
- avg_hr — «Средняя частота пульса» / «Средний пульс»;
- calories — «Всего калорий»;
- kind — тип: «Силовые тренажеры»/«Сил. трен.» → silovaya; кардио/бег → kardio; прогулка → progulka;
- exercises — строки таблицы «Подход/Имя/Время/Повтор./Вес»: name, sets (Подход), reps (Повтор.), weight_kg (Вес; «--» → null);
- notes — оценка/тип из заголовка;
- date_text — дата из самого верха экрана («13 нояб. 2026», «11 нояб. @ 12:55»), не сегодняшняя и не вычисляй.
Не бери «31:15» как сон. Не выдумывай вес. Верни JSON без повторов.`

const nutritionVisionPrompt = `Дневник питания. Извлеки: date_text (дата экрана дословно), kcal, protein_g, fat_g, carbs_g, meals[{name,kcal}]. Если поля нет — не заполняй нулём. Верни JSON без повторов.`

// extractImage — двухфазный разбор: классификация типа экрана, затем
// специализированный промпт (сон/тренировка/питание). Старый комбинированный
// ответ модели поддерживается как fallback.
func (e *Extractor) extractImage(ctx context.Context, image []byte, caption string, now time.Time) (Extracted, error) {
	raw, err := e.vision.ExtractVision(ctx, buildClassifyPrompt(now, caption), visionClassifySchema, [][]byte{image})
	if err != nil {
		return Extracted{}, err
	}

	// обратная совместимость: некоторые провайдеры/тесты возвращают сразу данные
	if part, parseErr := parseExtracted(raw, now); parseErr == nil && extractedHasData(part) {
		return part, nil
	}

	kind := parseVisionKind(raw)
	switch kind {
	case "sleep_quality":
		raw, err := e.vision.ExtractVision(ctx, sleepQualityVisionPrompt, sleepQualityVisionSchema, [][]byte{image})
		if err != nil {
			return Extracted{}, err
		}
		return parseSleepQualityVision(raw, now)
	case "sleep_metrics":
		raw, err := e.vision.ExtractVision(ctx, sleepMetricsVisionPrompt, sleepMetricsVisionSchema, [][]byte{image})
		if err != nil {
			return Extracted{}, err
		}
		return parseSleepMetricsVision(raw, now)
	case "sleep":
		if part, err := e.extractSleepQuality(ctx, image, now); err == nil {
			return part, nil
		}
		if part, err := e.extractSleepMetrics(ctx, image, now); err == nil {
			return part, nil
		}
		raw, err := e.vision.ExtractVision(ctx, sleepVisionPrompt, sleepVisionSchema, [][]byte{image})
		if err != nil {
			return Extracted{}, err
		}
		return parseSleepVision(raw, now)
	case "workout":
		if part, err := e.extractWorkoutOCR(ctx, image, now); err == nil {
			return part, nil
		}
		raw, err := e.vision.ExtractVision(ctx, workoutVisionPrompt, workoutVisionSchema, [][]byte{image})
		if err != nil {
			return Extracted{}, err
		}
		return parseWorkoutVision(raw, now)
	case "nutrition":
		raw, err := e.vision.ExtractVision(ctx, nutritionVisionPrompt, nutritionVisionSchema, [][]byte{image})
		if err != nil {
			return Extracted{}, err
		}
		return parseNutritionVision(raw, now)
	default:
		return Extracted{}, fmt.Errorf("intake: на изображении не найдены данные (тип %q)", kind)
	}
}

func (e *Extractor) extractSleepQuality(ctx context.Context, image []byte, now time.Time) (Extracted, error) {
	raw, err := e.vision.ExtractVision(ctx, sleepQualityVisionPrompt, sleepQualityVisionSchema, [][]byte{image})
	if err != nil {
		return Extracted{}, err
	}
	return parseSleepQualityVision(raw, now)
}

func (e *Extractor) extractSleepMetrics(ctx context.Context, image []byte, now time.Time) (Extracted, error) {
	raw, err := e.vision.ExtractVision(ctx, sleepMetricsVisionPrompt, sleepMetricsVisionSchema, [][]byte{image})
	if err != nil {
		return Extracted{}, err
	}
	return parseSleepMetricsVision(raw, now)
}

func extractedHasData(part Extracted) bool {
	return part.Nutrition != nil || part.Checkin != nil || part.Training != nil
}

func parseVisionKind(raw json.RawMessage) string {
	var wire struct {
		Kind  string   `json:"kind"`
		Kinds []string `json:"kinds"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ""
	}
	kind := strings.ToLower(strings.TrimSpace(wire.Kind))
	if kind == "" && len(wire.Kinds) > 0 {
		kind = strings.ToLower(strings.TrimSpace(wire.Kinds[0]))
	}
	switch kind {
	case "checkin":
		return "sleep"
	case "training", "workout":
		return "workout"
	case "nutrition":
		return "nutrition"
	default:
		return kind
	}
}

type sleepVisionWire struct {
	DateText       string   `json:"date_text"`
	Sleep          string   `json:"sleep"`
	SleepScore     *flexInt `json:"sleep_score"`
	HRV            *flexInt `json:"hrv_ms"`
	RHR            *flexInt `json:"rhr"`
	AvgHRNight     *flexInt `json:"avg_hr_night"`
	SpO2Avg        *flexInt `json:"spo2_avg"`
	SpO2Min        *flexInt `json:"spo2_min"`
	RespirationAvg *flexInt `json:"respiration_avg"`
	RespirationMin *flexInt `json:"respiration_min"`
	BodyBattery    *flexInt `json:"body_battery"`
}

func parseSleepVision(raw json.RawMessage, now time.Time) (Extracted, error) {
	var wire sleepVisionWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Extracted{}, fmt.Errorf("intake: разобрать сон: %w", err)
	}
	sleepRaw, _ := json.Marshal(strings.TrimSpace(wire.Sleep))
	sleepHours, _ := parseSleep(sleepRaw)
	checkin := &Checkin{
		SleepH:         sleepHours,
		HRV:            positiveInt(wire.HRV),
		RHR:            positiveInt(wire.RHR),
		AvgHRNight:     positiveInt(wire.AvgHRNight),
		SpO2Avg:        positiveInt(wire.SpO2Avg),
		SpO2Min:        positiveInt(wire.SpO2Min),
		RespirationAvg: positiveInt(wire.RespirationAvg),
		RespirationMin: positiveInt(wire.RespirationMin),
		BodyBattery:    intPtr(wire.BodyBattery),
		SleepScore:     positiveInt(wire.SleepScore),
	}
	sanitizeCheckin(checkin)
	appendCheckinExtras(checkin)
	if !checkinHasData(checkin) {
		return Extracted{}, fmt.Errorf("intake: на экране сна не нашлось метрик")
	}
	date := parseDateText(wire.DateText, now)
	return Extracted{Date: date, Checkin: checkin, Kinds: []string{"checkin"}}, nil
}

type sleepQualityVisionWire struct {
	DateText   string   `json:"date_text"`
	Sleep      string   `json:"sleep"`
	SleepScore *flexInt `json:"sleep_score"`
}

func parseSleepQualityVision(raw json.RawMessage, now time.Time) (Extracted, error) {
	var wire sleepQualityVisionWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Extracted{}, fmt.Errorf("intake: разобрать качество сна: %w", err)
	}
	sleepRaw, _ := json.Marshal(strings.TrimSpace(wire.Sleep))
	sleepHours, _ := parseSleep(sleepRaw)
	checkin := &Checkin{SleepH: sleepHours, SleepScore: positiveInt(wire.SleepScore)}
	sanitizeCheckin(checkin)
	if !checkinHasData(checkin) {
		return Extracted{}, fmt.Errorf("intake: на экране качества сна не нашлось данных")
	}
	date := parseDateText(wire.DateText, now)
	return Extracted{Date: date, Checkin: checkin, Kinds: []string{"checkin"}}, nil
}

type sleepMetricsVisionWire struct {
	DateText       string   `json:"date_text"`
	HRV            *flexInt `json:"hrv_ms"`
	RHR            *flexInt `json:"rhr"`
	AvgHRNight     *flexInt `json:"avg_hr_night"`
	SpO2Avg        *flexInt `json:"spo2_avg"`
	SpO2Min        *flexInt `json:"spo2_min"`
	RespirationAvg *flexInt `json:"respiration_avg"`
	RespirationMin *flexInt `json:"respiration_min"`
	BodyBattery    *flexInt `json:"body_battery"`
}

func parseSleepMetricsVision(raw json.RawMessage, now time.Time) (Extracted, error) {
	var wire sleepMetricsVisionWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Extracted{}, fmt.Errorf("intake: разобрать метрики сна: %w", err)
	}
	checkin := &Checkin{
		HRV:            positiveInt(wire.HRV),
		RHR:            positiveInt(wire.RHR),
		AvgHRNight:     positiveInt(wire.AvgHRNight),
		SpO2Avg:        positiveInt(wire.SpO2Avg),
		SpO2Min:        positiveInt(wire.SpO2Min),
		RespirationAvg: positiveInt(wire.RespirationAvg),
		RespirationMin: positiveInt(wire.RespirationMin),
		BodyBattery:    intPtr(wire.BodyBattery),
	}
	sanitizeCheckin(checkin)
	appendCheckinExtras(checkin)
	if !checkinHasData(checkin) {
		return Extracted{}, fmt.Errorf("intake: на экране метрик сна не нашлось данных")
	}
	date := parseDateText(wire.DateText, now)
	return Extracted{Date: date, Checkin: checkin, Kinds: []string{"checkin"}}, nil
}

type workoutVisionWire struct {
	DateText    string   `json:"date_text"`
	Kind        string   `json:"kind"`
	DurationMin *flexInt `json:"duration_min"`
	AvgHR       *flexInt `json:"avg_hr"`
	Calories    *flexInt `json:"calories"`
	Notes       string   `json:"notes"`
	Exercises   []struct {
		Name     string     `json:"name"`
		Sets     *flexInt   `json:"sets"`
		Reps     *flexInt   `json:"reps"`
		WeightKG *flexFloat `json:"weight_kg"`
	} `json:"exercises"`
}

func parseWorkoutVision(raw json.RawMessage, now time.Time) (Extracted, error) {
	var wire workoutVisionWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Extracted{}, fmt.Errorf("intake: разобрать тренировку: %w", err)
	}
	training := &Training{
		Kind:        strings.TrimSpace(wire.Kind),
		DurationMin: positiveInt(wire.DurationMin),
		AvgHR:       positiveInt(wire.AvgHR),
		Calories:    positiveInt(wire.Calories),
		Notes:       strings.TrimSpace(wire.Notes),
	}
	for _, item := range wire.Exercises {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		exercise := TrainingExercise{
			Name:     name,
			Sets:     positiveInt(item.Sets),
			Reps:     positiveInt(item.Reps),
			WeightKG: floatPtr(item.WeightKG),
		}
		if exercise.WeightKG != nil && *exercise.WeightKG <= 0 {
			exercise.WeightKG = nil
		}
		training.Exercises = append(training.Exercises, exercise)
	}
	if !trainingHasData(training) {
		return Extracted{}, fmt.Errorf("intake: на экране тренировки не нашлось данных")
	}
	training.Date = parseDateText(wire.DateText, now)
	return Extracted{Training: training, Kinds: []string{"training"}}, nil
}

type nutritionVisionWire struct {
	DateText string   `json:"date_text"`
	Kcal     *flexInt `json:"kcal"`
	Protein  *flexInt `json:"protein_g"`
	Fat      *flexInt `json:"fat_g"`
	Carbs    *flexInt `json:"carbs_g"`
	Meals    []struct {
		Name string   `json:"name"`
		Kcal *flexInt `json:"kcal"`
	} `json:"meals"`
}

func parseNutritionVision(raw json.RawMessage, now time.Time) (Extracted, error) {
	var wire nutritionVisionWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Extracted{}, fmt.Errorf("intake: разобрать питание: %w", err)
	}
	nutrition := &Nutrition{
		Kcal:     positiveInt(wire.Kcal),
		ProteinG: positiveInt(wire.Protein),
		FatG:     positiveInt(wire.Fat),
		CarbsG:   positiveInt(wire.Carbs),
	}
	for _, meal := range wire.Meals {
		name := strings.TrimSpace(meal.Name)
		if name == "" {
			continue
		}
		nutrition.Meals = append(nutrition.Meals, Meal{Name: name, Kcal: positiveInt(meal.Kcal)})
	}
	if nutrition.Kcal == nil && nutrition.ProteinG == nil && nutrition.FatG == nil && nutrition.CarbsG == nil {
		return Extracted{}, fmt.Errorf("intake: на экране питания не нашлось данных")
	}
	date := parseDateText(wire.DateText, now)
	return Extracted{Date: date, Nutrition: nutrition, Kinds: []string{"nutrition"}}, nil
}

func positiveInt(value *flexInt) *int {
	if value == nil {
		return nil
	}
	parsed := int(*value)
	if parsed <= 0 {
		return nil
	}
	return &parsed
}
