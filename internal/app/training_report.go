package app

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"healthbot/internal/analysis"
	"healthbot/internal/nlu"
	"healthbot/internal/settings"
	"healthbot/internal/store"
)

// TrainingReport реализует intake.TrainingReporter: собирает markdown-отчёт по
// шаблону дневника (утро, разминка, таблица, кардио, статистика, анализ,
// прогрессия за 6 тренировок).
func (h *commandHandlers) TrainingReport(chatID int64, training nlu.Training) (string, error) {
	personName := h.personFor(chatID)
	person := nlu.Athlete
	if personName == settings.PersonWife {
		person = nlu.Wife
	}
	prefs, err := h.settings.Load(personName)
	if err != nil {
		return "", err
	}
	sessions, err := h.reader.TrainingSessions(person, 50)
	if err != nil {
		return "", err
	}
	values, err := h.reader.CheckinValues(person, 14)
	if err != nil {
		return "", err
	}

	sameKind := sessionsOfKind(sessions, training.Kind)
	previous := previousSession(sameKind, training.Date)
	checkin := checkinForDate(values, training.Date)
	thresholds := h.thresholdsFor(person, personName, prefs, values)

	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s тренировка: %s\n\n", trainingKindTitle(training.Kind), formatDateRU(parseDateOrNow(training.Date, h.now())))

	builder.WriteString("## Утренние показатели\n")
	builder.WriteString(morningMetrics(checkin, thresholds))
	builder.WriteString("\n")

	warmup, main := splitWarmup(training.Exercises)
	builder.WriteString("## Разминка/заминка\n")
	if len(warmup) == 0 {
		builder.WriteString("- не указана\n")
	} else {
		for _, exercise := range warmup {
			fmt.Fprintf(&builder, "- %s\n", exercise.Name)
		}
	}
	builder.WriteString("\n")

	setsHeader := ""
	if commonSets := commonExerciseSets(main); commonSets > 0 {
		setsHeader = fmt.Sprintf(" (%d подхода)", commonSets)
	}
	fmt.Fprintf(&builder, "## Основные упражнения%s\n", setsHeader)
	builder.WriteString("| Упражнение | Подходы | Повторения | RPE | Примечания |\n")
	builder.WriteString("|------------|---------|------------|-----|------------|\n")
	for _, exercise := range main {
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %s |\n",
			tableCell(exercise.Name),
			optionalInt(exercise.Sets),
			repsCell(exercise),
			rpeCell(exercise, training.RPESession),
			tableCell(exercise.Note))
	}
	builder.WriteString("\n")

	if training.Cardio != "" {
		builder.WriteString("## Кардио-дополнение\n- " + training.Cardio + "\n\n")
	}

	builder.WriteString("## Общая статистика\n")
	if training.DurationMin != nil {
		fmt.Fprintf(&builder, "- Длительность: %d мин\n", *training.DurationMin)
	}
	if training.Calories != nil {
		fmt.Fprintf(&builder, "- Калории: %d ккал\n", *training.Calories)
	}
	if training.AvgHR != nil {
		fmt.Fprintf(&builder, "- Средний пульс: %d\n", *training.AvgHR)
	} else {
		builder.WriteString("- Средний пульс: не указан\n")
	}
	if training.RPESession != nil {
		fmt.Fprintf(&builder, "- RPE: %d\n", *training.RPESession)
	} else {
		builder.WriteString("- RPE: не указан\n")
	}
	fmt.Fprintf(&builder, "- Общий объём: %s\n\n", volumeLine(main))

	if training.Felt != "" {
		builder.WriteString("## Самочувствие\n- " + training.Felt + "\n\n")
	}
	if len(training.Pain) > 0 {
		builder.WriteString("## Боль\n")
		keys := make([]string, 0, len(training.Pain))
		for key := range training.Pain {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&builder, "- %s: %d/10\n", key, training.Pain[key])
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Анализ\n")
	for _, line := range analysisLines(training, previous, checkin, thresholds, main) {
		builder.WriteString("- " + line + "\n")
	}
	builder.WriteString("\n")

	builder.WriteString("## Прогрессия (последние 6 " + trainingKindGenitivePlural(training.Kind) + " тренировок)\n")
	builder.WriteString(progressionTable(sameKind, training))
	return builder.String(), nil
}

func (h *commandHandlers) thresholdsFor(person nlu.Person, personName string, prefs settings.Settings, values []store.DailyValues) analysis.Thresholds {
	base := defaultHRVBase(person)
	if prefs.HRVBaseManual != nil {
		base = *prefs.HRVBaseManual
	} else if average := averageHRV(values, 7); average > 0 {
		base = average
	}
	thresholds := analysis.AthleteThresholds(base)
	if person == nlu.Wife {
		thresholds = analysis.WifeThresholds(base)
	}
	applyThresholdOverrides(&thresholds, prefs)
	return thresholds
}

func trainingKindGenitivePlural(kind string) string {
	switch kind {
	case "ulichnaya":
		return "уличных"
	case "sila-A", "sila-B", "silovaya":
		return "силовых"
	case "fitboxing":
		return "фитбоксинга"
	case "kardio":
		return "кардио"
	case "progulka":
		return "прогулок"
	default:
		return "таких"
	}
}

func trainingKindTitle(kind string) string {
	label := trainingKindLabel(kind)
	if label == "" {
		return "Тренировка"
	}
	runes := []rune(label)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

func sessionsOfKind(sessions []nlu.Training, kind string) []nlu.Training {
	result := make([]nlu.Training, 0, len(sessions))
	for _, session := range sessions {
		if session.Kind == kind && session.Done {
			result = append(result, session)
		}
	}
	return result
}

func previousSession(sessions []nlu.Training, date string) *nlu.Training {
	var previous *nlu.Training
	for index := range sessions {
		if sessions[index].Date >= date {
			continue
		}
		previous = &sessions[index]
	}
	return previous
}

func checkinForDate(values []store.DailyValues, date string) *store.DailyValues {
	for index := len(values) - 1; index >= 0; index-- {
		if values[index].Date.Format("2006-01-02") == date {
			return &values[index]
		}
	}
	return nil
}

func morningMetrics(checkin *store.DailyValues, thresholds analysis.Thresholds) string {
	if checkin == nil {
		return "- нет данных чек-ина\n"
	}
	lines := make([]string, 0, 3)
	if checkin.HRV != nil {
		result := analysis.Evaluate(analysis.Input{HRV: checkin.HRV, RHR: checkin.RHR, SleepH: checkin.SleepH, Mood: checkin.Mood}, thresholds)
		lines = append(lines, formatMorningHRV(*checkin.HRV, result.Zone))
	} else {
		lines = append(lines, "- HRV: нет данных")
	}
	if checkin.RHR != nil {
		lines = append(lines, fmt.Sprintf("- Пульс покоя: %d уд/мин (%s)", *checkin.RHR, rhrState(*checkin.RHR, thresholds)))
	} else {
		lines = append(lines, "- Пульс покоя: нет данных")
	}
	if checkin.SleepH != nil {
		lines = append(lines, fmt.Sprintf("- Сон: %d ч %02d мин (%s)", int(*checkin.SleepH), int(math.Round((*checkin.SleepH-math.Floor(*checkin.SleepH))*60)), sleepState(*checkin.SleepH, thresholds)))
	} else {
		lines = append(lines, "- Сон: нет данных")
	}
	return strings.Join(lines, "\n") + "\n"
}

func formatMorningHRV(hrv int, zone analysis.Zone) string {
	return fmt.Sprintf("- HRV: %d мс (%s зона)", hrv, strings.ToLower(zone.Name()))
}

func rhrState(value int, thresholds analysis.Thresholds) string {
	switch {
	case value > thresholds.RHRRedMin:
		return "выше"
	case value >= thresholds.RHRGreenMax:
		return "граница"
	default:
		return "норма"
	}
}

func sleepState(hours float64, thresholds analysis.Thresholds) string {
	if hours < thresholds.SleepYellowMax {
		return "недосып"
	}
	return "норма"
}

func splitWarmup(exercises []nlu.Exercise) ([]nlu.Exercise, []nlu.Exercise) {
	warmup := make([]nlu.Exercise, 0, 2)
	main := make([]nlu.Exercise, 0, len(exercises))
	for _, exercise := range exercises {
		lowerName := strings.ToLower(exercise.Name)
		if strings.Contains(lowerName, "размин") || strings.Contains(lowerName, "замин") {
			warmup = append(warmup, exercise)
			continue
		}
		main = append(main, exercise)
	}
	return warmup, main
}

func commonExerciseSets(exercises []nlu.Exercise) int {
	counts := map[int]int{}
	for _, exercise := range exercises {
		if exercise.Sets != nil {
			counts[*exercise.Sets]++
		}
	}
	best, bestCount := 0, 0
	for sets, count := range counts {
		if count > bestCount {
			best, bestCount = sets, count
		}
	}
	return best
}

func rpeCell(exercise nlu.Exercise, sessionRPE *int) string {
	if exercise.RPE != nil {
		return fmt.Sprintf("%d", *exercise.RPE)
	}
	if sessionRPE != nil {
		return fmt.Sprintf("%d", *sessionRPE)
	}
	return "—"
}

func optionalInt(value *int) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%d", *value)
}

func repsCell(exercise nlu.Exercise) string {
	if exercise.Reps == nil {
		return "—"
	}
	note := strings.ToLower(exercise.Note)
	if strings.Contains(note, "на руку") || strings.Contains(note, "удар") || strings.Contains(strings.ToLower(exercise.Name), "удар") {
		return fmt.Sprintf("%d/удар/руку", *exercise.Reps)
	}
	return fmt.Sprintf("%d", *exercise.Reps)
}

func tableCell(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "|", "/")
	if text == "" {
		return "—"
	}
	return text
}

func volumeLine(exercises []nlu.Exercise) string {
	totalSets := 0
	for _, exercise := range exercises {
		if exercise.Sets != nil {
			totalSets += *exercise.Sets
		}
	}
	if totalSets == 0 {
		return fmt.Sprintf("%d упражнений, подходы не указаны", len(exercises))
	}
	if common := commonExerciseSets(exercises); common > 0 && common*len(exercises) == totalSets {
		return fmt.Sprintf("%d упражнения × %d подхода = %d подходов", len(exercises), common, totalSets)
	}
	return fmt.Sprintf("%d упражнений, %d подходов", len(exercises), totalSets)
}

func analysisLines(training nlu.Training, previous *nlu.Training, checkin *store.DailyValues, thresholds analysis.Thresholds, main []nlu.Exercise) []string {
	lines := make([]string, 0, 4)
	if previous != nil {
		currentVolume := analysis.TrainingVolume(training)
		previousVolume := analysis.TrainingVolume(*previous)
		if previousVolume > 0 {
			delta := (currentVolume - previousVolume) / previousVolume * 100
			lines = append(lines, fmt.Sprintf("Объём: %g против %g (%+.0f%%) — сравниваем с %s.", currentVolume, previousVolume, delta, formatDateRU(parseDateOrNow(previous.Date, timeNow()))))
		}
		currentSets := commonExerciseSets(main)
		previousSets := commonExerciseSets(previous.Exercises)
		if currentSets > 0 && previousSets > 0 && currentSets != previousSets {
			lines = append(lines, fmt.Sprintf("%d подхода вместо %d — изменение объёма при RPE %s.", currentSets, previousSets, optionalInt(training.RPESession)))
		}
		for _, exercise := range main {
			if !containsExercise(previous.Exercises, exercise.Name) {
				lines = append(lines, fmt.Sprintf("%s — новое упражнение для этой тренировки.", exercise.Name))
			}
		}
	}
	if checkin != nil && checkin.HRV != nil {
		result := analysis.Evaluate(analysis.Input{HRV: checkin.HRV, RHR: checkin.RHR, SleepH: checkin.SleepH, Mood: checkin.Mood}, thresholds)
		lines = append(lines, fmt.Sprintf("HRV %d — %s зона; RPE %s, нагрузку держим по состоянию.", *checkin.HRV, strings.ToLower(result.Zone.Name()), optionalInt(training.RPESession)))
	}
	for _, exercise := range main {
		lower := strings.ToLower(exercise.Name)
		if strings.Contains(lower, "удар") && exercise.Reps != nil && exercise.Sets != nil {
			techniques := 0
			for _, name := range []string{"джеб", "кросс", "хук"} {
				if strings.Contains(lower, name) || strings.Contains(strings.ToLower(exercise.Note), name) {
					techniques++
				}
			}
			if techniques == 0 {
				techniques = 3
			}
			total := techniques * *exercise.Reps * *exercise.Sets
			lines = append(lines, fmt.Sprintf("%s: %d техники × %d × %d = %d ударов на руку.", tableCell(exercise.Name), techniques, *exercise.Reps, *exercise.Sets, total))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "Первая тренировка такого типа — база для прогрессии.")
	}
	return lines
}

func containsExercise(exercises []nlu.Exercise, name string) bool {
	needle := strings.ToLower(strings.TrimSpace(name))
	for _, exercise := range exercises {
		hay := strings.ToLower(exercise.Name)
		if hay == needle || strings.Contains(hay, needle) || strings.Contains(needle, hay) {
			return true
		}
	}
	return false
}

func progressionTable(sessions []nlu.Training, current nlu.Training) string {
	if len(sessions) == 0 {
		return "- нет истории\n"
	}
	start := len(sessions) - 6
	if start < 0 {
		start = 0
	}
	window := sessions[start:]
	if len(window) == 0 || window[len(window)-1].Date != current.Date {
		// текущая ещё не в истории — добавляем
		window = append(window, current)
		if len(window) > 6 {
			window = window[len(window)-6:]
		}
	}

	names := make([]string, 0, 8)
	for _, session := range window {
		for _, exercise := range session.Exercises {
			if !containsExerciseName(names, exercise.Name) {
				names = append(names, exercise.Name)
			}
		}
	}

	var builder strings.Builder
	builder.WriteString("| Упражнение |")
	for _, session := range window {
		if date, err := time.Parse("2006-01-02", session.Date); err == nil {
			builder.WriteString(" " + date.Format("02.01") + " |")
		}
	}
	builder.WriteString("\n|------------|")
	for range window {
		builder.WriteString("-------|")
	}
	builder.WriteString("\n")
	for _, name := range names {
		builder.WriteString("| " + tableCell(shortExerciseName(name)) + " |")
		for _, session := range window {
			builder.WriteString(" " + progressionCell(session, name) + " |")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func progressionCell(session nlu.Training, name string) string {
	for _, exercise := range session.Exercises {
		hay := strings.ToLower(exercise.Name)
		needle := strings.ToLower(name)
		if hay == needle || strings.Contains(hay, needle) || strings.Contains(needle, hay) {
			return fmt.Sprintf("%s×%s", optionalInt(exercise.Reps), optionalInt(exercise.Sets))
		}
	}
	return "—"
}

func containsExerciseName(names []string, name string) bool {
	for _, existing := range names {
		if strings.EqualFold(existing, name) {
			return true
		}
	}
	return false
}

func shortExerciseName(name string) string {
	name = strings.TrimSpace(name)
	if index := strings.Index(name, " ("); index > 0 {
		name = name[:index]
	}
	runes := []rune(name)
	if len(runes) > 0 {
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	}
	return string(runes)
}

func parseDateOrNow(date string, now time.Time) time.Time {
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		return parsed
	}
	return now
}

func timeNow() time.Time { return time.Now() }
