package intake

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"healthbot/internal/nlu"
)

var (
	mcKindPattern           = regexp.MustCompile(`(?i)\bMC\s*(\d+)\b`)
	exerciseWeightKgPattern = regexp.MustCompile(`(?i)^(.+?)\s+(\d+(?:[.,]\d+)?)\s*(?:кг|kg)\s+(\d+)\s*[xх×]\s*(\d+)(?:\s*(?:rpe|@)\s*(\d+))?$`)
	exerciseTriplePattern   = regexp.MustCompile(`(?i)^(.+?)\s+(\d+(?:[.,]\d+)?)\s*[xх×]\s*(\d+)\s*[xх×]\s*(\d+)(?:\s*(?:rpe|@)\s*(\d+))?$`)
	exerciseDoublePattern   = regexp.MustCompile(`(?i)^(.+?)\s+(\d+(?:[.,]\d+)?)\s*[xх×]\s*(\d+)(?:\s*(?:rpe|@)\s*(\d+))?$`)
	exerciseWeightPattern   = regexp.MustCompile(`(?i)^(.+?)\s+(\d+(?:[.,]\d+)?)\s*(?:кг|kg)$`)
	exerciseWeightKgReps    = regexp.MustCompile(`(?i)^(.+?)\s+(\d+(?:[.,]\d+)?)\s*(?:кг|kg)\s+(\d+)\s*(?:раз(?:а)?|повтор[а-яё]*|повт\.?)(?:\s+на каждую ногу)?$`)
	exerciseRepsPattern     = regexp.MustCompile(`(?i)^(.+?)\s+(?:по\s+)?(\d+)\s*(?:раз(?:а)?|повтор[а-яё]*|повт\.?)(?:\s+(на\s+[а-яё\s]+))?$`)
	exerciseDetectPattern   = regexp.MustCompile(`(?i)\d+(?:[.,]\d+)?\s*(?:кг|kg)?\s*[xх×]\s*\d+`)

	segmentSplitPattern   = regexp.MustCompile(`(?:\n|;|\.\s+|,\s+)`)
	connectorSplitPattern = regexp.MustCompile(`(?i)\s+(?:затем|потом|далее)\s+`)
	leadingConnector      = regexp.MustCompile(`(?i)^(?:затем|потом|далее)\s+`)
	cardioSegmentPattern  = regexp.MustCompile(`(?i)кардио|лестниц|этаж|дорожк|вело|бег`)
	cardioPrefixPattern   = regexp.MustCompile(`(?i)^кардио\s*`)
	warmupPattern         = regexp.MustCompile(`(?i)разминк`)
	globalSetsPattern     = regexp.MustCompile(`(?i)(?:все\s+упражнения\s+)?(?:по\s+)?(\d+)\s+подход[а-яё]*`)

	rpeSessionPattern = regexp.MustCompile(`(?i)\brpe\s*(?:сесси[а-яё]*|тренировки)?\s*[:=]?\s*(\d+)`)
	durationPattern   = regexp.MustCompile(`(?i)(\d+)\s*(?:мин|минут)`)
	avgHRPattern      = regexp.MustCompile(`(?i)(?:ср\.?\s*|средний\s*)пульс\s*(\d+)|пульс\s*(\d+)\s*(?:уд|bpm)`)
	painPattern       = regexp.MustCompile(`(?i)(колен[а-яё]*|плеч[а-яё]*|поясниц[а-яё]*|спин[а-яё]*|локт[а-яё]*|локот[а-яё]*|бедр[а-яё]*|голен[а-яё]*)\s*(\d+)\s*/\s*10`)
	feltPattern       = regexp.MustCompile(`(?i)(?:самочувствие|ощущения)\s*[:—-]\s*(.+)`)
	exercisePrefix    = regexp.MustCompile(`(?i)^(?:тренировка|тренинг|отчёт|отчет|сделал[а]?|провёл|провел[а]?)\b[^:]*:\s*`)
	exerciseBullet    = regexp.MustCompile(`^[-•*]\s*`)
)

var trainingTypeMarkers = []struct {
	marker string
	kind   string
}{
	{"фитбокс", "fitboxing"},
	{"бокс", "fitboxing"},
	{"силовая b", "sila-B"},
	{"силовая-б", "sila-B"},
	{"sila-b", "sila-B"},
	{"силовая a", "sila-A"},
	{"силовая-а", "sila-A"},
	{"sila-a", "sila-A"},
	{"силов", "sila-A"},
	{"уличн", "ulichnaya"},
	{"street", "ulichnaya"},
	{"восстанов", "vosstanovlenie"},
	{"кардио", "kardio"},
	{"прогул", "progulka"},
}

// LooksLikeTraining — эвристика маршрутизации (FR-6.8.8): похоже на отчёт о
// тренировке, а не на чек-ин/вопрос. Требует упражнения (число×число) или
// явный отчёт («сделал», «пропуск», «RPE», «подход»).
func LooksLikeTraining(text string) bool {
	if exerciseDetectPattern.MatchString(text) {
		return true
	}
	lower := strings.ToLower(text)
	hasMarker := false
	for _, marker := range []string{"тренировк", "тренинг", "фитбокс", "бокс", "кардио", "прогулк", "силов", "подход", "rpe", "пропуск", "не сделал", "восстанов"} {
		if strings.Contains(lower, marker) {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return false
	}
	return strings.Contains(lower, "сделал") || strings.Contains(lower, "пропуск") ||
		strings.Contains(lower, "не сделал") || strings.Contains(lower, "rpe") ||
		strings.Contains(lower, "подход")
}

// ParseTraining разбирает текстовый отчёт о тренировке (FR-4/FR-6.8).
// Возвращает false, если в тексте нет пригодных данных.
func ParseTraining(text string, person nlu.Person, now time.Time) (nlu.Training, bool) {
	lower := strings.ToLower(text)
	date := parseDateText(text, now)
	if date == "" {
		date = now.Format("2006-01-02")
	}
	done := !strings.Contains(lower, "не сделал") && !strings.Contains(lower, "пропуск") &&
		!strings.Contains(lower, "не тренировался") && !strings.Contains(lower, "не смог")
	training := nlu.Training{
		Person: person,
		Date:   date,
		Kind:   normalizeTrainingKind(person, detectTrainingKind(lower)),
		Done:   done,
	}

	if match := durationPattern.FindStringSubmatch(text); match != nil {
		if value, err := strconv.Atoi(match[1]); err == nil {
			training.DurationMin = &value
		}
	}
	if match := avgHRPattern.FindStringSubmatch(text); match != nil {
		raw := match[1]
		if raw == "" {
			raw = match[2]
		}
		if value, err := strconv.Atoi(raw); err == nil {
			training.AvgHR = &value
		}
	}
	if matches := rpeSessionPattern.FindAllStringSubmatch(text, -1); len(matches) > 0 {
		if value, err := strconv.Atoi(matches[len(matches)-1][1]); err == nil {
			training.RPESession = &value
		}
	}
	if match := feltPattern.FindStringSubmatch(text); match != nil {
		training.Felt = strings.TrimSpace(match[1])
	}

	var cardioParts []string
	for _, segment := range splitTrainingSegments(text) {
		for _, part := range connectorSplitPattern.Split(segment, -1) {
			part = leadingConnector.ReplaceAllString(strings.TrimSpace(part), "")
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if cardioSegmentPattern.MatchString(strings.ToLower(part)) {
				cardioParts = append(cardioParts, cleanCardio(part))
				continue
			}
			exercise, ok := parseExercise(part)
			if ok {
				training.Exercises = append(training.Exercises, exercise)
			}
		}
	}

	if match := globalSetsPattern.FindStringSubmatch(text); match != nil {
		if defaultSets, err := strconv.Atoi(match[1]); err == nil {
			for index := range training.Exercises {
				if training.Exercises[index].Sets == nil && training.Exercises[index].Reps != nil {
					sets := defaultSets
					training.Exercises[index].Sets = &sets
				}
			}
		}
	}

	pains := make(map[string]int)
	for _, match := range painPattern.FindAllStringSubmatch(text, -1) {
		if value, err := strconv.Atoi(match[2]); err == nil {
			pains[normalizePainKey(match[1])] = value
		}
	}
	if len(pains) > 0 {
		training.Pain = pains
	}

	if strings.Contains(strings.ToLower(text), "джеб") || strings.Contains(strings.ToLower(text), "кросс") || strings.Contains(strings.ToLower(text), "хук") {
		for index := range training.Exercises {
			if strings.Contains(strings.ToLower(training.Exercises[index].Name), "удар") {
				training.Exercises[index].Name += " (джеб, кросс, хук)"
				break
			}
		}
	}

	if len(cardioParts) > 0 {
		training.Cardio = strings.Join(cardioParts, "; ")
	} else if isCardioKind(training.Kind) {
		training.Cardio = strings.TrimSpace(text)
	}

	ok := len(training.Exercises) > 0 || !training.Done ||
		training.DurationMin != nil || training.AvgHR != nil
	return training, ok
}

func splitTrainingSegments(text string) []string {
	return segmentSplitPattern.Split(text, -1)
}

func cleanCardio(raw string) string {
	text := leadingConnector.ReplaceAllString(strings.TrimSpace(raw), "")
	text = cardioPrefixPattern.ReplaceAllString(text, "")
	return strings.TrimSpace(strings.Trim(text, " ."))
}

func parseExercise(raw string) (nlu.Exercise, bool) {
	segment := exerciseBullet.ReplaceAllString(strings.TrimSpace(raw), "")
	segment = exercisePrefix.ReplaceAllString(segment, "")
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return nlu.Exercise{}, false
	}
	if match := exerciseWeightKgReps.FindStringSubmatch(segment); match != nil {
		weight := parseFloatValue(match[2])
		reps := parseIntValue(match[3])
		exercise := nlu.Exercise{Name: cleanExerciseName(match[1]), WeightKG: &weight, Reps: &reps}
		applyPerLegNote(&exercise, segment)
		return exercise, true
	}
	if match := exerciseRepsPattern.FindStringSubmatch(segment); match != nil {
		reps := parseIntValue(match[2])
		exercise := nlu.Exercise{Name: cleanExerciseName(match[1]), Reps: &reps}
		note := strings.TrimSpace(match[3])
		if note != "" {
			exercise.Note = note
		} else {
			applyPerLegNote(&exercise, segment)
		}
		return exercise, true
	}
	if match := exerciseWeightKgPattern.FindStringSubmatch(segment); match != nil {
		weight := parseFloatValue(match[2])
		sets := parseIntValue(match[3])
		reps := parseIntValue(match[4])
		exercise := nlu.Exercise{Name: cleanExerciseName(match[1]), WeightKG: &weight, Sets: &sets, Reps: &reps}
		if match[5] != "" {
			rpe := parseIntValue(match[5])
			exercise.RPE = &rpe
		}
		return exercise, true
	}
	if match := exerciseTriplePattern.FindStringSubmatch(segment); match != nil {
		weight := parseFloatValue(match[2])
		reps := parseIntValue(match[3])
		sets := parseIntValue(match[4])
		exercise := nlu.Exercise{Name: cleanExerciseName(match[1]), WeightKG: &weight, Sets: &sets, Reps: &reps}
		if match[5] != "" {
			rpe := parseIntValue(match[5])
			exercise.RPE = &rpe
		}
		return exercise, true
	}
	if match := exerciseDoublePattern.FindStringSubmatch(segment); match != nil {
		first := parseFloatValue(match[2])
		second := parseIntValue(match[3])
		exercise := nlu.Exercise{Name: cleanExerciseName(match[1])}
		if first >= 20 {
			exercise.WeightKG = &first
			exercise.Reps = &second
		} else {
			sets := int(first)
			exercise.Sets = &sets
			exercise.Reps = &second
		}
		if match[4] != "" {
			rpe := parseIntValue(match[4])
			exercise.RPE = &rpe
		}
		return exercise, true
	}
	if match := exerciseWeightPattern.FindStringSubmatch(segment); match != nil {
		weight := parseFloatValue(match[2])
		return nlu.Exercise{Name: cleanExerciseName(match[1]), WeightKG: &weight}, true
	}
	if warmupPattern.MatchString(segment) {
		name := cleanExerciseName(segment)
		if name != "" {
			return nlu.Exercise{Name: name, Note: "разминка"}, true
		}
	}
	return nlu.Exercise{}, false
}

func applyPerLegNote(exercise *nlu.Exercise, segment string) {
	if strings.Contains(strings.ToLower(segment), "на каждую ногу") {
		exercise.Note = "на каждую ногу"
	}
}

func cleanExerciseName(raw string) string {
	name := strings.TrimSpace(raw)
	if index := strings.LastIndex(name, ":"); index >= 0 {
		name = name[index+1:]
	}
	name = strings.Trim(name, " -–—:.")
	name = regexp.MustCompile(`(?i)^упражнени[а-яё]*\s+`).ReplaceAllString(name, "")
	name = strings.Join(strings.Fields(name), " ")
	return strings.TrimSpace(name)
}

func detectTrainingKind(lower string) string {
	if match := mcKindPattern.FindStringSubmatch(lower); match != nil {
		base := "silovaya"
		switch {
		case strings.Contains(lower, "кардио"):
			base = "kardio"
		case strings.Contains(lower, "прогул"):
			base = "progulka"
		}
		return "MC" + match[1] + "_" + base
	}
	for _, item := range trainingTypeMarkers {
		if strings.Contains(lower, item.marker) {
			return item.kind
		}
	}
	return ""
}

func normalizeTrainingKind(person nlu.Person, kind string) string {
	if strings.HasPrefix(kind, "MC") {
		if person == nlu.Wife {
			return kind
		}
		return "sila-A"
	}
	if person == nlu.Wife {
		switch kind {
		case "":
			return "silovaya"
		case "silovaya", "kardio", "progulka":
			return kind
		case "ulichnaya", "vosstanovlenie", "sila-A", "sila-B", "fitboxing":
			return "silovaya"
		default:
			return "silovaya"
		}
	}
	switch kind {
	case "":
		return "sila-A"
	case "silovaya", "progulka":
		return "sila-A"
	case "kardio":
		return "ulichnaya"
	case "MC":
		return "sila-A"
	default:
		return kind
	}
}

func isCardioKind(kind string) bool {
	return kind == "kardio" || kind == "progulka" || kind == "ulichnaya" || kind == "vosstanovlenie" ||
		strings.HasPrefix(kind, "MC") && strings.HasSuffix(kind, "kardio")
}

func normalizePainKey(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "колен"):
		return "колено"
	case strings.HasPrefix(lower, "плеч"):
		return "плечо"
	case strings.HasPrefix(lower, "поясниц"), strings.HasPrefix(lower, "спин"):
		return "спина"
	case strings.HasPrefix(lower, "локт"), strings.HasPrefix(lower, "локот"):
		return "локоть"
	case strings.HasPrefix(lower, "бедр"):
		return "бедро"
	case strings.HasPrefix(lower, "голен"):
		return "голень"
	default:
		return lower
	}
}

func parseFloatValue(raw string) float64 {
	value, _ := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
	return value
}

func parseIntValue(raw string) int {
	value, _ := strconv.Atoi(raw)
	return value
}

// FormatTrainingReply — подтверждение тренировки в голосе «Ритма» (FR-8).
func FormatTrainingReply(training nlu.Training, savedPath string) string {
	kind := trainingKindName(training.Kind)
	if !training.Done {
		reply := "🌿 Отметил пропуск: " + kind + "."
		if training.Felt != "" {
			reply += " Причина: " + training.Felt + "."
		}
		if savedPath != "" {
			reply += "\nСохранено: " + savedPath
		}
		return reply
	}

	reply := fmt.Sprintf("🌿 Отметил тренировку: %s", kind)
	if len(training.Exercises) > 0 {
		limit := 3
		if len(training.Exercises) < limit {
			limit = len(training.Exercises)
		}
		names := make([]string, 0, limit)
		for index, exercise := range training.Exercises {
			if index >= 3 {
				break
			}
			names = append(names, exercise.Name)
		}
		reply += fmt.Sprintf(", упражнений %d (%s)", len(training.Exercises), strings.Join(names, ", "))
	}
	reply += "."
	if savedPath != "" {
		reply += "\nСохранено: " + savedPath
	}
	return reply
}

func trainingKindName(kind string) string {
	switch kind {
	case "fitboxing":
		return "фитбоксинг"
	case "sila-A":
		return "силовая A"
	case "sila-B":
		return "силовая B"
	case "ulichnaya":
		return "уличная"
	case "vosstanovlenie":
		return "восстановление"
	case "silovaya":
		return "силовая"
	case "kardio":
		return "кардио"
	case "progulka":
		return "прогулка"
	default:
		return kind
	}
}
