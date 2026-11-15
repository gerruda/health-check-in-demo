package analysis

import (
	"fmt"
	"math"
	"strings"

	"healthbot/internal/nlu"
)

// NextExercise — рекомендация по упражнению на следующую тренировку.
type NextExercise struct {
	Name     string
	WeightKG *float64
	Sets     *int
	Reps     *int
	Change   string
}

// NextSessionPlan — план следующей тренировки по текущей и предыдущей.
type NextSessionPlan struct {
	Deload             bool
	VolumeDeltaPercent int
	Exercises          []NextExercise
	Reason             string
}

// TrainingVolume — тоннаж сессии: вес×подходы×повторы (без веса — подходы×повторы).
func TrainingVolume(training nlu.Training) float64 {
	volume := 0.0
	for _, exercise := range training.Exercises {
		sets, reps := 1, 1
		if exercise.Sets != nil {
			sets = *exercise.Sets
		}
		if exercise.Reps != nil {
			reps = *exercise.Reps
		}
		weight := 0.0
		if exercise.WeightKG != nil {
			weight = *exercise.WeightKG
		}
		if weight > 0 {
			volume += weight * float64(sets) * float64(reps)
		} else {
			volume += float64(sets * reps)
		}
	}
	return math.Round(volume*10) / 10
}

// NextSession считает следующую тренировку (FR-3.4, U8–U9):
// RPE ≤5 — прибавка веса (штанга +2.5, гантели +2), иначе закрепляем;
// рост RPE 2 сессии + жёлтая/красная зона — deload −25% объёма, вес держим.
func NextSession(previous, current *nlu.Training, zone Zone, rpeHistory []int) NextSessionPlan {
	plan := NextSessionPlan{}
	if current == nil {
		return plan
	}

	rising := rpeRising(rpeHistory)
	deload := zone == ZoneRed || (zone == ZoneYellow && rising)
	var currentRPE *int
	if current.RPESession != nil {
		currentRPE = current.RPESession
	}
	if currentRPE != nil && *currentRPE >= 9 && (zone == ZoneYellow || zone == ZoneRed) {
		deload = true
	}

	if deload {
		plan.Deload = true
		plan.VolumeDeltaPercent = -25
		plan.Reason = "зона восстановления и/или RPE растёт — снижаем объём, вес держим"
	} else {
		plan.Reason = "RPE позволяет — точечно добавляем нагрузку"
	}

	previousByName := make(map[string]nlu.Exercise, len(exerciseSlice(previous)))
	for _, exercise := range exerciseSlice(previous) {
		previousByName[normalizeExerciseName(exercise.Name)] = exercise
	}

	for _, exercise := range current.Exercises {
		next := NextExercise{Name: exercise.Name, WeightKG: exercise.WeightKG, Sets: exercise.Sets, Reps: exercise.Reps}
		if deload {
			if exercise.Sets != nil && *exercise.Sets > 1 {
				sets := maxInt(1, *exercise.Sets-1)
				next.Sets = &sets
			}
			next.Change = fmt.Sprintf("deload −25%% объёма, вес %.1f кг держим", valueOrZero(exercise.WeightKG))
			plan.Exercises = append(plan.Exercises, next)
			continue
		}

		rpe := exerciseRPE(exercise, current)
		hasWeight := exercise.WeightKG != nil && *exercise.WeightKG > 0
		if hasWeight && rpe != nil && *rpe <= 5 {
			step := 2.5
			if isDumbbell(exercise.Name) {
				step = 2
			}
			weight := *exercise.WeightKG + step
			next.WeightKG = &weight
			next.Change = fmt.Sprintf("+%g кг", step)
		} else if !hasWeight && rpe != nil && *rpe <= 7 && exercise.Reps != nil {
			reps := *exercise.Reps + 1
			next.Reps = &reps
			next.Change = "+1 повтор"
		} else {
			next.Change = "закрепляем технику"
		}
		if previousExercise, ok := previousByName[normalizeExerciseName(exercise.Name)]; ok && previousExercise.WeightKG != nil && exercise.WeightKG != nil {
			if *exercise.WeightKG > *previousExercise.WeightKG {
				next.Change = "вес уже вырос — закрепляем"
			}
		}
		plan.Exercises = append(plan.Exercises, next)
	}
	return plan
}

func exerciseSlice(training *nlu.Training) []nlu.Exercise {
	if training == nil {
		return nil
	}
	return training.Exercises
}

func exerciseRPE(exercise nlu.Exercise, training *nlu.Training) *int {
	if exercise.RPE != nil {
		return exercise.RPE
	}
	return training.RPESession
}

func rpeRising(history []int) bool {
	if len(history) < 2 {
		return false
	}
	last := history[len(history)-1]
	prev := history[len(history)-2]
	return last > prev && last >= 8
}

func isDumbbell(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "гантел") || strings.Contains(lower, "dumbbell")
}

func normalizeExerciseName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
