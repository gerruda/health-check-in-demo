package app

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"healthbot/internal/analysis"
	"healthbot/internal/config"
	"healthbot/internal/intake"
	"healthbot/internal/nlu"
	"healthbot/internal/persona"
	"healthbot/internal/settings"
	"healthbot/internal/store"
)

// commandHandlers реализует команды /settings, /set, /status (FR-5/FR-7).
type commandHandlers struct {
	cfg      config.Config
	settings *settings.Store
	reader   *store.Reader
	log      *slog.Logger
	now      func() time.Time
}

func newCommandHandlers(cfg config.Config, logger *slog.Logger, cache *store.Cache, index store.IndexQuerier) *commandHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &commandHandlers{
		cfg:      cfg,
		settings: settings.NewStore(cfg.SettingsDir),
		reader:   store.NewReader(store.NewPaths(cfg.DataRoot), store.WithCache(cache), store.WithIndex(index), store.WithLogger(logger)),
		log:      logger,
		now:      time.Now,
	}
}

func (h *commandHandlers) personFor(chatID int64) string {
	if h.cfg.WifeChatID != 0 && chatID == h.cfg.WifeChatID {
		return settings.PersonWife
	}
	return settings.PersonAthlete
}

// Settings реализует router.SettingsHandler.
func (h *commandHandlers) Settings(chatID int64) (string, error) {
	person := h.personFor(chatID)
	current, err := h.settings.Load(person)
	if err != nil {
		return "", err
	}
	return formatSettings(person, current, h.cfg), nil
}

// Set реализует router.SettingsHandler.
func (h *commandHandlers) Set(chatID int64, key, value string) (string, error) {
	person := h.personFor(chatID)
	if _, err := h.settings.Apply(person, key, value); err != nil {
		return "", err
	}
	return fmt.Sprintf("Отметил: %s = %s. Полный список — /settings.", key, value), nil
}

// Reset реализует router.SettingsHandler.
func (h *commandHandlers) Reset(chatID int64) (string, error) {
	person := h.personFor(chatID)
	if err := h.settings.Reset(person); err != nil {
		return "", err
	}
	return "Вернул дефолты контракта — снова базовый ритм. /settings — проверить.", nil
}

// CheckinReply реализует intake.Coach: зона/HRV_7/ориентир для только что
// сохранённого чек-ина (FR-3.1). Тон — «Ритм» (FR-8).
func (h *commandHandlers) CheckinReply(chatID int64, extracted intake.Extracted) (string, error) {
	personName := h.personFor(chatID)
	person := nlu.Athlete
	if personName == settings.PersonWife {
		person = nlu.Wife
	}

	prefs, err := h.settings.Load(personName)
	if err != nil {
		return "", err
	}
	values, err := h.reader.CheckinValues(person, 14)
	if err != nil {
		return "", err
	}
	if len(values) == 0 {
		return "", fmt.Errorf("app: нет данных чек-ина для ответа")
	}

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

	target := values[len(values)-1]
	if extracted.Date != "" {
		for index := len(values) - 1; index >= 0; index-- {
			if values[index].Date.Format("2006-01-02") == extracted.Date {
				target = values[index]
				break
			}
		}
	}

	result := analysis.Evaluate(analysisInput(target), thresholds)
	trend := analysis.TrendHRV(lastHRV(values, 7))
	counts := zoneCounts(values, thresholds)

	plan := ""
	if result.Zone == analysis.ZoneGreen && extracted.Checkin != nil {
		plan = strings.TrimSpace(extracted.Checkin.PlannedTraining)
	}

	return analysis.CheckinReply(analysis.CheckinReplyInput{
		Zone:       result,
		HRV:        trend,
		ZoneCounts: counts,
		Plan:       plan,
	}), nil
}

// Week реализует router.WeekHandler: сводка последних 7 дней.
func (h *commandHandlers) Week(_ context.Context, chatID int64) (string, error) {
	personName := h.personFor(chatID)
	person := nlu.Athlete
	if personName == settings.PersonWife {
		person = nlu.Wife
	}

	prefs, err := h.settings.Load(personName)
	if err != nil {
		return "", err
	}
	values, err := h.reader.CheckinValues(person, 21)
	if err != nil {
		return "", err
	}
	nutrition, err := h.reader.NutritionValues(person, 21)
	if err != nil {
		return "", err
	}
	trainings, err := h.reader.TrainingFiles(person)
	if err != nil {
		return "", err
	}

	start, end := weekWindow(h.now())
	weekValues := filterByWindow(values, start, end)
	weekNutrition := filterNutritionByWindow(nutrition, start, end)
	weekTrainings := filterTrainingsByWindow(trainings, start, end)

	lines := []string{fmt.Sprintf("🌿 Неделя %s: %s–%s",
		personGenitive(personName), formatDateRU(start), formatDateRU(end))}
	if len(weekValues) == 0 && len(weekNutrition) == 0 && len(weekTrainings) == 0 {
		lines = append(lines, "", "За последние 7 дней нет данных. Пришли чек-ин или отчёт о тренировке.")
		lines = append(lines, "", persona.WeekAside(0, 0, 0, 0, 0))
		return strings.Join(lines, "\n"), nil
	}

	green, yellow, red := 0, 0, 0
	if len(weekValues) > 0 {
		base := defaultHRVBase(person)
		if prefs.HRVBaseManual != nil {
			base = *prefs.HRVBaseManual
		} else if average := averageHRV(weekValues, 7); average > 0 {
			base = average
		}
		thresholds := analysis.AthleteThresholds(base)
		if person == nlu.Wife {
			thresholds = analysis.WifeThresholds(base)
		}
		applyThresholdOverrides(&thresholds, prefs)

		trend := analysis.TrendHRV(lastHRV(weekValues, 7))
		if trend.Count > 0 {
			counts := zoneCounts(weekValues, thresholds)
			green, yellow, red = counts.Green, counts.Yellow, counts.Red
			lines = append(lines, fmt.Sprintf("HRV: %.1f мс в среднем · %s %s · 🟢%d / 🟡%d / 🔴%d",
				trend.Average, trend.Direction.Arrow(), trend.Direction.Name(), counts.Green, counts.Yellow, counts.Red))
		}
		if sleepLine := weekSleepLine(weekValues, thresholds.SleepYellowMax); sleepLine != "" {
			lines = append(lines, sleepLine)
		}
		if rhrLine := weekRHRLine(weekValues); rhrLine != "" {
			lines = append(lines, rhrLine)
		}
	}

	lines = append(lines, "")
	lines = append(lines, weekNutritionLine(weekNutrition))
	lines = append(lines, "Цели: "+goalsLine(personName, prefs))
	lines = append(lines, "")
	lines = append(lines, weekTrainingLine(weekTrainings))
	if weightLine := weekWeightLine(weekValues); weightLine != "" {
		lines = append(lines, weightLine)
	}

	if len(weekValues) > 0 {
		latest := weekValues[len(weekValues)-1]
		base := defaultHRVBase(person)
		if prefs.HRVBaseManual != nil {
			base = *prefs.HRVBaseManual
		} else if average := averageHRV(weekValues, 7); average > 0 {
			base = average
		}
		thresholds := analysis.AthleteThresholds(base)
		if person == nlu.Wife {
			thresholds = analysis.WifeThresholds(base)
		}
		applyThresholdOverrides(&thresholds, prefs)
		result := analysis.Evaluate(analysisInput(latest), thresholds)
		lines = append(lines, "Ориентир: "+analysis.PlanForZone(result.Zone))
	}
	lines = append(lines, "", persona.WeekAside(green, yellow, red, len(weekTrainings), len(weekValues)))
	return strings.Join(lines, "\n"), nil
}

// Status реализует router.StatusHandler: HRV_7, зона, цели, тренировка, LLM.
func (h *commandHandlers) Status(_ context.Context, chatID int64) (string, error) {
	personName := h.personFor(chatID)
	person := nlu.Athlete
	if personName == settings.PersonWife {
		person = nlu.Wife
	}

	prefs, err := h.settings.Load(personName)
	if err != nil {
		return "", err
	}
	values, err := h.reader.CheckinValues(person, 14)
	if err != nil {
		return "", err
	}

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

	lines := []string{"Сводка: " + personLabel(personName)}
	zoneLabel := "не определена"

	trend := analysis.TrendHRV(lastHRV(values, 7))
	if trend.Count == 0 {
		lines = append(lines, "HRV_7: нет данных")
	} else {
		counts := zoneCounts(values, thresholds)
		line := fmt.Sprintf("HRV_7: %.1f мс · %s %s", trend.Average, trend.Direction.Arrow(), trend.Direction.Name())
		if total := counts.Green + counts.Yellow + counts.Red; total > 0 {
			line += fmt.Sprintf(" · 🟢%d / 🟡%d / 🔴%d", counts.Green, counts.Yellow, counts.Red)
		}
		lines = append(lines, line)
	}

	if len(values) > 0 {
		latest := values[len(values)-1]
		result := analysis.Evaluate(analysisInput(latest), thresholds)
		zoneLabel = result.Zone.Name()
		lines = append(lines, fmt.Sprintf("Зона (%s): %s %s — %s",
			formatDateRU(latest.Date), result.Zone.Emoji(), result.Zone.Name(), result.Reason))
		lines = append(lines, "Ориентир: "+analysis.PlanForZone(result.Zone))
	} else {
		lines = append(lines, "Зона: ⚪ не определена (нет чек-инов)")
	}

	lines = append(lines, "Цели: "+goalsLine(personName, prefs))

	trainings, err := h.reader.TrainingFiles(person)
	if err != nil {
		return "", err
	}
	if len(trainings) > 0 {
		last := trainings[len(trainings)-1]
		lines = append(lines, "Последняя тренировка: "+formatDateRU(last.Date)+" — "+last.Kind)
	} else {
		lines = append(lines, "Последняя тренировка: нет данных")
	}
	lines = append(lines, "", persona.StatusAside(zoneLabel))
	return strings.Join(lines, "\n"), nil
}

func formatSettings(person string, prefs settings.Settings, cfg config.Config) string {
	lines := []string{"Настройки: " + personLabel(person), ""}
	lines = append(lines, "Цели (kcal · protein_min · protein_max · fat_max · carbs_min · carbs_max):")
	lines = append(lines, "  "+goalsValues(person, prefs))

	base := "auto (7-дневное среднее)"
	if prefs.HRVBaseManual != nil {
		base = fmt.Sprintf("%d* мс", *prefs.HRVBaseManual)
	}
	greenMargin := prefs.GreenMargin()
	redMargin := prefs.RedMargin()
	greenMarginText := fmt.Sprintf("%d", greenMargin)
	redMarginText := fmt.Sprintf("%d", redMargin)
	if prefs.HRVGreenMargin != nil {
		greenMarginText += "*"
	}
	if prefs.HRVRedMargin != nil {
		redMarginText += "*"
	}

	greenRHR := defaultRHRGreen(person)
	redRHR := defaultRHRRed(person)
	greenRHRText := fmt.Sprintf("%d", greenRHR)
	redRHRText := fmt.Sprintf("%d", redRHR)
	if prefs.RHRGreenMax != nil {
		greenRHRText = fmt.Sprintf("%d*", *prefs.RHRGreenMax)
	}
	if prefs.RHRRedMin != nil {
		redRHRText = fmt.Sprintf("%d*", *prefs.RHRRedMin)
	}
	lines = append(lines, "", "Зоны (hrv_base · hrv_green_margin · hrv_red_margin · rhr_green_max · rhr_red_min):")
	lines = append(lines, fmt.Sprintf("  база HRV %s · допуски %s/%s мс · пульс %s/%s",
		base, greenMarginText, redMarginText, greenRHRText, redRHRText))

	if prefs.WeightGoalKG != nil {
		lines = append(lines, "", fmt.Sprintf("Цель веса (weight_goal): %.1f* кг", *prefs.WeightGoalKG))
	}

	weekly := "выкл"
	if prefs.WeeklyReviewOn() {
		weekly = "вкл"
	}
	defaultReviewTime := cfg.SundayReviewTime
	if defaultReviewTime == "" {
		defaultReviewTime = "09:00"
	}
	lines = append(lines, "", "Воскресный разбор (weekly_review · weekly_review_time):")
	lines = append(lines, fmt.Sprintf("  %s · %s", weekly, prefs.WeeklyReviewTimeOrDefault(defaultReviewTime)))

	lines = append(lines, "")
	llmLine := "LLM: " + cfg.LLMMode + "/" + llmModelName(cfg)
	if cfg.VisionEnabled() {
		llmLine += " · vision " + cfg.VisionMode() + "/" + cfg.VisionModel()
	}
	lines = append(lines, llmLine)

	lines = append(lines, "")
	if len(prefs.History) > 0 {
		start := len(prefs.History) - 3
		if start < 0 {
			start = 0
		}
		history := make([]string, 0, len(prefs.History)-start)
		for _, change := range prefs.History[start:] {
			history = append(history, fmt.Sprintf("%s: %s = %s", change.Date, change.Key, change.Value))
		}
		lines = append(lines, "История: "+strings.Join(history, "; "))
		lines = append(lines, "")
	}
	lines = append(lines, "Изменить: /set <ключ> <значение>, например /set weekly_review off.")
	return strings.Join(lines, "\n")
}

func goalsValues(person string, prefs settings.Settings) string {
	kcal := "2300"
	protein := "108–135"
	fat := "80–90"
	carbs := "200–250"
	if person == settings.PersonWife {
		kcal = "1800–1900"
		protein = "112"
		fat = "75–85"
		carbs = "130–170"
	}
	if prefs.Kcal != nil {
		kcal = fmt.Sprintf("%d*", *prefs.Kcal)
	}
	if prefs.ProteinMin != nil || prefs.ProteinMax != nil {
		protein = macroOverride(prefs.ProteinMin, prefs.ProteinMax, 108, 135)
	}
	if prefs.FatMax != nil {
		fat = fmt.Sprintf("≤%d*", *prefs.FatMax)
	}
	if prefs.CarbsMin != nil || prefs.CarbsMax != nil {
		carbs = macroOverride(prefs.CarbsMin, prefs.CarbsMax, 200, 250)
	}
	return fmt.Sprintf("ккал %s · Б %s · Ж %s · У %s", kcal, protein, fat, carbs)
}

func goalsLine(person string, prefs settings.Settings) string {
	return goalsValues(person, prefs)
}

func macroOverride(minValue, maxValue *int, defaultMin, defaultMax int) string {
	minimum, maximum := defaultMin, defaultMax
	if minValue != nil {
		minimum = *minValue
	}
	if maxValue != nil {
		maximum = *maxValue
	}
	if minimum == maximum {
		return fmt.Sprintf("%d*", minimum)
	}
	return fmt.Sprintf("%d–%d*", minimum, maximum)
}

func defaultHRVBase(person nlu.Person) int {
	if person == nlu.Wife {
		return 34
	}
	return 33
}

func defaultRHRGreen(person string) int {
	if person == settings.PersonWife {
		return 62
	}
	return 60
}

func defaultRHRRed(person string) int {
	if person == settings.PersonWife {
		return 65
	}
	return 62
}

func applyThresholdOverrides(thresholds *analysis.Thresholds, prefs settings.Settings) {
	if prefs.RHRGreenMax != nil {
		thresholds.RHRGreenMax = *prefs.RHRGreenMax
	}
	if prefs.RHRRedMin != nil {
		thresholds.RHRRedMin = *prefs.RHRRedMin
	}
	if prefs.HRVGreenMargin != nil {
		thresholds.GreenMargin = *prefs.HRVGreenMargin
	}
	if prefs.HRVRedMargin != nil {
		thresholds.RedMargin = *prefs.HRVRedMargin
	}
}

func averageHRV(values []store.DailyValues, limit int) int {
	collected := lastHRV(values, limit)
	if len(collected) == 0 {
		return 0
	}
	sum := 0
	for _, value := range collected {
		sum += value
	}
	return int(math.Round(float64(sum) / float64(len(collected))))
}

func lastHRV(values []store.DailyValues, limit int) []int {
	collected := make([]int, 0, limit)
	for _, value := range values {
		if value.HRV != nil {
			collected = append(collected, *value.HRV)
		}
	}
	if limit > 0 && len(collected) > limit {
		collected = collected[len(collected)-limit:]
	}
	return collected
}

func zoneCounts(values []store.DailyValues, thresholds analysis.Thresholds) analysis.ZoneCounts {
	counts := analysis.ZoneCounts{}
	for _, value := range values {
		result := analysis.Evaluate(analysisInput(value), thresholds)
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

func analysisInput(values store.DailyValues) analysis.Input {
	return analysis.Input{
		HRV:    values.HRV,
		RHR:    values.RHR,
		SleepH: values.SleepH,
		Mood:   values.Mood,
	}
}

func personGenitive(person string) string {
	if person == settings.PersonWife {
		return "жены"
	}
	return "атлета"
}

func personLabel(person string) string {
	if person == settings.PersonWife {
		return "жена"
	}
	return "атлет"
}

func formatDateRU(date time.Time) string {
	return date.Format("02.01.2006")
}
