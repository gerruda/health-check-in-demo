package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"healthbot/internal/llm"
	"healthbot/internal/logging"
	"healthbot/internal/nlu"
)

// trainingSchema — структура отчёта под шаблон дневника (FR-2.2/FR-3.3).
var trainingSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "kind": {"type": "string", "description": "Тип тренировки (fitboxing|sila-A|sila-B|ulichnaya|vosstanovlenie|silovaya|kardio|progulka|MC<N>_...)"},
    "date_text": {"type": "string", "description": "Дата дословно: вчера/сегодня/11 ноября/2026-11-11"},
    "duration_min": {"type": "integer", "description": "Длительность, мин"},
    "avg_hr": {"type": "integer", "description": "Средний пульс"},
    "rpe_session": {"type": "integer", "description": "RPE сессии 1-10"},
    "exercises": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "weight_kg": {"type": "number"},
          "sets": {"type": "integer"},
          "reps": {"type": "integer"},
          "rpe": {"type": "integer"},
          "note": {"type": "string"}
        },
        "required": ["name"]
      }
    },
    "cardio": {"type": "string", "description": "Кардио отдельно от упражнений (лестница, бег, дорожка)"},
    "felt": {"type": "string", "description": "Самочувствие/причина пропуска"},
    "pain": {"type": "object", "additionalProperties": {"type": "integer"}, "description": "Боль: сустав → 0-10"},
    "done": {"type": "boolean"},
    "needs_more": {"type": "string", "description": "Если данных мало — один короткий вопрос пользователю"}
  },
  "required": ["done", "needs_more", "exercises"]
}`)

type trainingWire struct {
	Kind        string `json:"kind"`
	DateText    string `json:"date_text"`
	DurationMin *int   `json:"duration_min"`
	AvgHR       *int   `json:"avg_hr"`
	RPESession  *int   `json:"rpe_session"`
	Exercises   []struct {
		Name     string   `json:"name"`
		WeightKG *float64 `json:"weight_kg"`
		Sets     *int     `json:"sets"`
		Reps     *int     `json:"reps"`
		RPE      *int     `json:"rpe"`
		Note     string   `json:"note"`
	} `json:"exercises"`
	Cardio    string         `json:"cardio"`
	Felt      string         `json:"felt"`
	Pain      map[string]int `json:"pain"`
	Done      *bool          `json:"done"`
	NeedsMore string         `json:"needs_more"`
}

// TrainingExtractor извлекает отчёт о тренировке активной LLM в структуру
// шаблона (FR-2.2): универсально к формулировкам, с валидацией и вопросом.
type TrainingExtractor struct {
	provider llm.Provider
	log      *slog.Logger
	now      func() time.Time
}

// NewTrainingExtractor создаёт экстрактор тренировок.
func NewTrainingExtractor(provider llm.Provider, log *slog.Logger, now func() time.Time) *TrainingExtractor {
	if log == nil {
		log = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	return &TrainingExtractor{provider: provider, log: log, now: now}
}

// HasTrainingData сообщает, есть ли в отчёте достаточно данных для файла.
func HasTrainingData(training nlu.Training) bool {
	return !training.Done || len(training.Exercises) > 0 ||
		strings.TrimSpace(training.Cardio) != "" ||
		training.DurationMin != nil || training.AvgHR != nil
}

// Extract разбирает текст тренировки в структуру шаблона.
func (e *TrainingExtractor) Extract(ctx context.Context, text string, person nlu.Person) (nlu.Training, string, error) {
	raw, err := e.provider.Extract(ctx, buildTrainingPrompt(e.now(), person, text), trainingSchema)
	if err != nil {
		return nlu.Training{}, "", fmt.Errorf("training: LLM: %w", err)
	}

	var wire trainingWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nlu.Training{}, "", fmt.Errorf("training: разобрать JSON: %w", err)
	}

	training := nlu.Training{
		Person:      person,
		Kind:        normalizeTrainingKind(person, canonicalTrainingKind(wire.Kind)),
		DurationMin: wire.DurationMin,
		AvgHR:       wire.AvgHR,
		RPESession:  wire.RPESession,
		Cardio:      strings.TrimSpace(wire.Cardio),
		Felt:        strings.TrimSpace(wire.Felt),
		Pain:        normalizePain(wire.Pain),
	}
	training.Done = wire.Done == nil || *wire.Done

	date := parseDateText(wire.DateText, e.now())
	if date == "" {
		date = e.now().Format("2006-01-02")
	}
	training.Date = date

	for _, item := range wire.Exercises {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		exercise := nlu.Exercise{
			Name:     name,
			WeightKG: sanitizeWeight(item.WeightKG),
			Sets:     sanitizeRPE(item.Sets),
			Reps:     sanitizeRPE(item.Reps),
			RPE:      sanitizeRPE(item.RPE),
			Note:     strings.TrimSpace(item.Note),
		}
		training.Exercises = append(training.Exercises, exercise)
	}
	if training.RPESession != nil && (*training.RPESession < 1 || *training.RPESession > 10) {
		training.RPESession = nil
	}
	if training.AvgHR != nil && (*training.AvgHR < 20 || *training.AvgHR > 250) {
		training.AvgHR = nil
	}
	if training.DurationMin != nil && *training.DurationMin <= 0 {
		training.DurationMin = nil
	}

	question := strings.TrimSpace(wire.NeedsMore)
	if question == "" && !HasTrainingData(training) {
		question = "Уточни, пожалуйста: упражнения, подходы/повторы и RPE."
	}

	e.log.Info(logging.Msg("intake:trainingLLM", "отчёт о тренировке разобран моделью"),
		"kind", training.Kind, "exercises", len(training.Exercises),
		"has_cardio", training.Cardio != "", "needs_more", question != "")
	return training, question, nil
}

// canonicalTrainingKind приводит ответ модели (рус/eng) к коду типа файла.
func canonicalTrainingKind(kind string) string {
	raw := strings.TrimSpace(kind)
	lower := strings.ToLower(raw)
	switch lower {
	case "фитбоксинг", "фитбокс", "бокс", "boxing":
		return "fitboxing"
	case "силовая a", "силовая-а", "sila-a", "sila a":
		return "sila-A"
	case "силовая b", "силовая-б", "sila-b", "sila b":
		return "sila-B"
	case "уличная", "уличн", "street", "ulichnaya":
		return "ulichnaya"
	case "восстановление", "восстановительная", "vosstanovlenie":
		return "vosstanovlenie"
	case "силовая", "silovaya":
		return "silovaya"
	case "кардио", "kardio":
		return "kardio"
	case "прогулка", "прогул", "progulka":
		return "progulka"
	}
	if match := mcKindPattern.FindStringSubmatch(lower); match != nil {
		base := "silovaya"
		switch {
		case strings.Contains(lower, "кардио"), strings.Contains(lower, "kardio"):
			base = "kardio"
		case strings.Contains(lower, "прогул"), strings.Contains(lower, "progulka"):
			base = "progulka"
		}
		return "MC" + match[1] + "_" + base
	}
	return raw
}

// mergeTraining берёт правила как базу (структура/дата/тип надёжнее),
// дополняет упражнениями и кардио модели; пропуски заполняются взаимно.
func mergeRuleAndLLMTraining(primary, fallback nlu.Training, sourceText string, trustRules bool) nlu.Training {
	if strings.TrimSpace(primary.Kind) == "" {
		primary.Kind = fallback.Kind
	}
	// Если правила уверенно разобрали текст, отсутствие RPE/длительности/пульса
	// означает «не указано» — не даём LLM выдумать числа (иначе бот не уточнит).
	if !trustRules {
		if primary.DurationMin == nil {
			primary.DurationMin = fallback.DurationMin
		}
		if primary.AvgHR == nil {
			primary.AvgHR = fallback.AvgHR
		}
		if primary.RPESession == nil {
			primary.RPESession = fallback.RPESession
		}
		if strings.TrimSpace(primary.Felt) == "" {
			primary.Felt = fallback.Felt
		}
		if len(primary.Pain) == 0 && len(fallback.Pain) > 0 {
			primary.Pain = fallback.Pain
		}
	}
	if strings.TrimSpace(fallback.Cardio) != "" && strings.TrimSpace(primary.Cardio) == "" {
		primary.Cardio = fallback.Cardio
	}
	if primary.Done && !fallback.Done && len(primary.Exercises) == 0 {
		primary.Done = false
	}

	if len(primary.Exercises) == 0 {
		primary.Exercises = fallback.Exercises
		return primary
	}
	for index := range primary.Exercises {
		fillExercise(&primary.Exercises[index], fallback.Exercises, !trustRules)
	}
	if trustRules {
		// правила разобрали текст — состав упражнений берём из них, LLM только
		// дополняет недостающие подходы/повторы/вес у совпавших упражнений
		return primary
	}
	// добавляем упражнения модели, которых не хватает правилам, но только если
	// название действительно встречается в исходном тексте (анти-галлюцинации)
	for _, candidate := range fallback.Exercises {
		found := false
		for _, existing := range primary.Exercises {
			if exerciseNamesMatch(existing.Name, candidate.Name) {
				found = true
				break
			}
		}
		if !found && exerciseGrounded(candidate.Name, sourceText) {
			primary.Exercises = append(primary.Exercises, candidate)
		}
	}
	return primary
}

func exerciseGrounded(name, sourceText string) bool {
	if strings.TrimSpace(sourceText) == "" {
		return false
	}
	lower := strings.ToLower(sourceText)
	words := strings.Fields(strings.ToLower(name))
	significant := 0
	for _, word := range words {
		word = strings.Trim(word, ".,;:!?()")
		if len([]rune(word)) < 4 {
			continue
		}
		significant++
		if !strings.Contains(lower, word) {
			return false
		}
	}
	return significant > 0
}

func fillExercise(exercise *nlu.Exercise, candidates []nlu.Exercise, includeNote bool) {
	for _, candidate := range candidates {
		if !exerciseNamesMatch(exercise.Name, candidate.Name) {
			continue
		}
		if exercise.WeightKG == nil {
			exercise.WeightKG = candidate.WeightKG
		}
		if exercise.Sets == nil {
			exercise.Sets = candidate.Sets
		}
		if exercise.Reps == nil {
			exercise.Reps = candidate.Reps
		}
		if exercise.RPE == nil {
			exercise.RPE = candidate.RPE
		}
		if includeNote && exercise.Note == "" {
			exercise.Note = candidate.Note
		}
		return
	}
}

func exerciseNamesMatch(left, right string) bool {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	if left == "" || right == "" {
		return false
	}
	return strings.Contains(left, right) || strings.Contains(right, left)
}

func buildTrainingPrompt(now time.Time, person nlu.Person, text string) string {
	kinds := "fitboxing, sila-A, sila-B, ulichnaya, vosstanovlenie"
	if person == nlu.Wife {
		kinds = "silovaya, kardio, progulka, MC<N>_silovaya|MC<N>_kardio|MC<N>_progulka"
	}
	return fmt.Sprintf(`Ты разбираешь отчёт о тренировке в структуру для шаблона дневника.
Сегодня: %s. Пользователь: %s. Допустимые типы: %s.

Заполни поля JSON:
- kind — тип из списка; если неясно — пустая строка;
- date_text — дата дословно из текста (вчера/сегодня/11 ноября/2026-11-11); не вычисляй сам;
- duration_min, avg_hr, rpe_session — если указаны;
- exercises — упражнения: name обязательно; weight_kg, sets, reps, rpe, note — если есть.
  «Все упражнения по 3 подхода» → sets=3 ВСЕМ упражнениям; не задавай вопрос
  про подходы, если это указано в тексте.
  «N раз» / «N повторений» → reps=N. «На каждую ногу» → note.
  Разминка → упражнение с note «разминка».
- cardio — отдельно: лестница/этаж/бег/велосипед/дорожка, НЕ упражнение;
- felt — самочувствие; pain — объект «сустав: 0-10»;
- done=false для «не сделал/пропуск»;
- needs_more — если данных мало, ОДИН короткий уточняющий вопрос (например про подходы/повторы), иначе пустая строка.

Не выдумывай веса/повторы, ставь только то, что есть в тексте. Названия
упражнений бери дословно. note «на каждую ногу» — только тому упражнению,
к которому это сказано. Если в тексте есть только часть данных — заполни что есть, остальное оставь пустым/null, а вопрос задай в needs_more.

Обязательно перечисли ВСЕ упражнения, которые есть в тексте, даже без веса и подходов.
Пример структуры ответа:
{"kind":"ulichnaya","date_text":"вчера","duration_min":null,"avg_hr":null,"rpe_session":8,"exercises":[{"name":"подтягивания узким хватом","sets":3,"reps":3},{"name":"отжимания на брусьях","sets":3,"reps":10}],"cardio":"подъем по лестнице на 32 этаж","felt":"","pain":{},"done":true,"needs_more":""}

Текст пользователя:
%s`, now.Format("02.01.2006"), personLabelRU(person), kinds, text)
}

func personLabelRU(person nlu.Person) string {
	if person == nlu.Wife {
		return "жена"
	}
	return "атлет"
}

func sanitizeWeight(weight *float64) *float64 {
	if weight == nil || *weight <= 0 || *weight > 500 {
		return nil
	}
	return weight
}

func sanitizeRPE(value *int) *int {
	if value == nil || *value <= 0 || *value > 100 {
		return nil
	}
	return value
}

func normalizePain(pain map[string]int) map[string]int {
	if len(pain) == 0 {
		return nil
	}
	normalized := make(map[string]int, len(pain))
	for key, value := range pain {
		if value <= 0 || value > 10 {
			continue
		}
		normalized[normalizePainKey(key)] = value
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
