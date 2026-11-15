package app

import (
	"fmt"
	"strings"

	"healthbot/internal/analysis"
	"healthbot/internal/nlu"
	"healthbot/internal/persona"
	"healthbot/internal/settings"
)

// TrainingReply реализует intake.TrainingCoach: разбор тренировки vs предыдущей
// и следующая тренировка с числами (FR-3.3/FR-3.4).
func (h *commandHandlers) TrainingReply(chatID int64, training nlu.Training) (string, error) {
	personName := h.personFor(chatID)
	person := nlu.Athlete
	if personName == settings.PersonWife {
		person = nlu.Wife
	}
	prefs, err := h.settings.Load(personName)
	if err != nil {
		return "", err
	}
	sessions, err := h.reader.TrainingSessions(person, 20)
	if err != nil {
		return "", err
	}

	current := training
	previous, sameKind := findPreviousTraining(sessions, current)
	if len(current.Exercises) == 0 {
		for _, session := range sameKind {
			if session.Date == current.Date {
				current = session
				break
			}
		}
	}

	zone := analysis.ZoneUnknown
	values, err := h.reader.CheckinValues(person, 7)
	if err != nil {
		return "", err
	}
	if len(values) > 0 {
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
		zone = analysis.Evaluate(analysisInput(values[len(values)-1]), thresholds).Zone
	}

	rpeHistory := make([]int, 0, 4)
	for _, session := range sameKind {
		if session.RPESession != nil {
			rpeHistory = append(rpeHistory, *session.RPESession)
		}
	}
	plan := analysis.NextSession(previous, &current, zone, rpeHistory)

	lines := []string{trainingHeadline(current)}
	if previous != nil {
		currentVolume := analysis.TrainingVolume(current)
		previousVolume := analysis.TrainingVolume(*previous)
		line := fmt.Sprintf("Объём: %g (было %g", currentVolume, previousVolume)
		if previousVolume > 0 {
			delta := (currentVolume - previousVolume) / previousVolume * 100
			line += fmt.Sprintf(", %+.0f%%", delta)
		}
		line += ")."
		lines = append(lines, line)
	} else {
		lines = append(lines, "Первая такая тренировка — база для прогрессии.")
	}

	if plan.Deload {
		lines = append(lines, fmt.Sprintf("Разгрузка: −25%% объёма, вес держим. %s.", plan.Reason))
	} else {
		lines = append(lines, "Следующая:")
	}
	for _, exercise := range plan.Exercises {
		line := "• " + exercise.Name
		if exercise.WeightKG != nil {
			line += fmt.Sprintf(" · %g кг", *exercise.WeightKG)
		}
		if exercise.Sets != nil && exercise.Reps != nil {
			line += fmt.Sprintf(" · %d×%d", *exercise.Sets, *exercise.Reps)
		}
		if exercise.Change != "" {
			line += " — " + exercise.Change
		}
		lines = append(lines, line)
	}
	if plan.Deload {
		lines[len(lines)-1] += "."
	}

	if notes := painSubstitutions(current.Pain); len(notes) > 0 {
		lines = append(lines, strings.Join(notes, " "))
	}
	lines = append(lines, "", persona.TrainingAside(plan.Deload))
	return strings.Join(lines, "\n"), nil
}

func findPreviousTraining(sessions []nlu.Training, current nlu.Training) (*nlu.Training, []nlu.Training) {
	sameKind := make([]nlu.Training, 0, len(sessions))
	for _, session := range sessions {
		if session.Kind == current.Kind {
			sameKind = append(sameKind, session)
		}
	}
	if len(sameKind) == 0 {
		return nil, sameKind
	}
	currentIndex := -1
	for index := range sameKind {
		if sameKind[index].Date == current.Date {
			currentIndex = index
		}
	}
	if currentIndex > 0 {
		previous := sameKind[currentIndex-1]
		return &previous, sameKind
	}
	if currentIndex == -1 && len(sameKind) > 0 {
		previous := sameKind[len(sameKind)-1]
		if previous.Date != current.Date {
			return &previous, sameKind
		}
	}
	return nil, sameKind
}

func trainingHeadline(training nlu.Training) string {
	parts := []string{"🌿 Отметил тренировку:", trainingKindLabel(training.Kind)}
	if training.DurationMin != nil {
		parts = append(parts, fmt.Sprintf("%d мин", *training.DurationMin))
	}
	if training.RPESession != nil {
		parts = append(parts, fmt.Sprintf("RPE %d", *training.RPESession))
	}
	return strings.Join(parts, " ")
}

func painSubstitutions(pain map[string]int) []string {
	if len(pain) == 0 {
		return nil
	}
	var notes []string
	for joint, level := range pain {
		lower := strings.ToLower(joint)
		switch {
		case strings.HasPrefix(lower, "плеч"):
			notes = append(notes, fmt.Sprintf("Плечо %d/10: без классических отжиманий в отказ — жим гантелей/эспандер.", level))
		case strings.HasPrefix(lower, "колен"):
			notes = append(notes, fmt.Sprintf("Колени %d/10: без глубоких приседов/выпадов/прыжков — ягодичный мост/жим ногами.", level))
		}
	}
	return notes
}
