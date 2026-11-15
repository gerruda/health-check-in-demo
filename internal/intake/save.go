package intake

import (
	"fmt"
	"time"

	"healthbot/internal/nlu"
)

// ReportSaver умеет сохранять готовый markdown-отчёт тренировки.
type ReportSaver interface {
	SaveTrainingReport(training nlu.Training, markdown string) (string, error)
}

// Saver сохраняет данные в файловую БЗ (реализация — app поверх store.Writer).
type Saver interface {
	SaveCheckin(checkin nlu.Checkin) (string, error)
	SaveNutrition(nutrition nlu.Nutrition) (string, error)
	SaveTraining(training nlu.Training) (string, error)
}

// NutritionCoach собирает разбор питания: цели, 7-дневное среднее, одна правка.
type NutritionCoach interface {
	NutritionReply(chatID int64, nutrition nlu.Nutrition) (string, error)
}

// WithNutritionCoach подключает разбор питания после сохранения.
func WithNutritionCoach(coach NutritionCoach) Option {
	return func(s *Service) {
		s.nutritionCoach = coach
	}
}

// Coach собирает ответ на сохранённый чек-ин: зона, HRV_7, ориентир (FR-3.1).
type Coach interface {
	CheckinReply(chatID int64, extracted Extracted) (string, error)
}

// WithCoach подключает коуч-ответ после сохранения чек-ина.
func WithCoach(coach Coach) Option {
	return func(s *Service) {
		s.coach = coach
	}
}

// WithSaver подключает сохранение после успешного распознавания (FR-2).
func WithSaver(saver Saver) Option {
	return func(s *Service) {
		s.saver = saver
	}
}

// WithPersonResolver определяет человека по chat_id (атлет/жена).
func WithPersonResolver(resolver func(chatID int64) nlu.Person) Option {
	return func(s *Service) {
		s.personResolver = resolver
	}
}

// WithClock подменяет текущее время (дата по умолчанию и тесты).
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// save сохраняет распознанные данные (критичные поля) и возвращает пути для чата.
func (s *Service) save(extracted Extracted, chatID int64) (paths []string, errs []error) {
	if s.saver == nil {
		return nil, nil
	}

	person := s.personFor(chatID)
	date := s.now()
	if extracted.Date != "" {
		if parsed, err := time.Parse("2006-01-02", extracted.Date); err == nil {
			date = parsed
		}
	}

	if extracted.Checkin != nil {
		checkin := toCheckin(*extracted.Checkin, person, date)
		path, err := s.saver.SaveCheckin(checkin)
		if err != nil {
			errs = append(errs, fmt.Errorf("чек-ин: %w", err))
		} else {
			paths = append(paths, path)
		}
	}

	if extracted.Nutrition != nil {
		nutrition := toNutrition(*extracted.Nutrition, person, date)
		path, err := s.saver.SaveNutrition(nutrition)
		if err != nil {
			errs = append(errs, fmt.Errorf("питание: %w", err))
		} else {
			paths = append(paths, path)
		}
	}

	if extracted.Training != nil {
		trainingDate := date
		if extracted.Training.Date != "" {
			if parsed, parseErr := time.Parse("2006-01-02", extracted.Training.Date); parseErr == nil {
				trainingDate = parsed
			}
		}
		training := toVisionTraining(*extracted.Training, person, trainingDate)
		path, err := s.saver.SaveTraining(training)
		if err != nil {
			errs = append(errs, fmt.Errorf("тренировка: %w", err))
		} else {
			paths = append(paths, path)
		}
	}

	return paths, errs
}

func (s *Service) personFor(chatID int64) nlu.Person {
	if s.personResolver != nil {
		if person := s.personResolver(chatID); person.Valid() {
			return person
		}
	}
	return nlu.Athlete
}

// toCheckin маппит критичные поля; усталость/температура/цель нагрузки не
// входят в nlu.Checkin и остаются только в анализе/ответе.
func toCheckin(source Checkin, person nlu.Person, date time.Time) nlu.Checkin {
	return nlu.Checkin{
		Date:             date.Format("2006-01-02"),
		Person:           person,
		SleepH:           source.SleepH,
		HRV:              source.HRV,
		RHR:              source.RHR,
		Mood:             source.Mood,
		AlcoholDoses:     source.AlcoholDoses,
		AlcoholNote:      source.AlcoholNote,
		WeightKG:         source.WeightKG,
		Steps:            source.Steps,
		PlannedTraining:  source.PlannedTraining,
		CycleDay:         source.CycleDay,
		CyclePhase:       source.CyclePhase,
		KneesPain:        source.KneesPain,
		Craving:          source.Craving,
		PostprandialWalk: source.PostprandialWalk,
		Context:          source.Context,
	}
}

func toVisionTraining(source Training, person nlu.Person, date time.Time) nlu.Training {
	training := nlu.Training{
		Date:        date.Format("2006-01-02"),
		Person:      person,
		Kind:        normalizeTrainingKind(person, source.Kind),
		DurationMin: source.DurationMin,
		AvgHR:       source.AvgHR,
		Calories:    source.Calories,
		Felt:        source.Notes,
		Done:        true,
	}
	for _, item := range source.Exercises {
		training.Exercises = append(training.Exercises, nlu.Exercise{
			Name: item.Name, WeightKG: item.WeightKG, Sets: item.Sets, Reps: item.Reps, RPE: item.RPE, Note: item.Note,
		})
	}
	return training
}

func toNutrition(source Nutrition, person nlu.Person, date time.Time) nlu.Nutrition {
	return nlu.Nutrition{
		Date:     date.Format("2006-01-02"),
		Person:   person,
		Kcal:     source.Kcal,
		ProteinG: source.ProteinG,
		FatG:     source.FatG,
		CarbsG:   source.CarbsG,
	}
}
