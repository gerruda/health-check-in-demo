package store

import "healthbot/internal/nlu"

// IndexQuerier — read-model истории (SQLite). Реализация — internal/historydb.
// Reader использует его, если подключён; при ошибке падает обратно на Markdown.
type IndexQuerier interface {
	CheckinValues(person nlu.Person, limit int) ([]DailyValues, error)
	NutritionValues(person nlu.Person, limit int) ([]DailyNutrition, error)
	TrainingSessions(person nlu.Person, limit int) ([]nlu.Training, error)
}

// Index — полный read-model (чтение + write-through).
type Index interface {
	IndexQuerier
	IndexUpdater
}

// IndexUpdater — write-through обновление read-model после записи Markdown.
type IndexUpdater interface {
	UpsertCheckin(checkin nlu.Checkin) error
	UpsertNutrition(nutrition nlu.Nutrition) error
	UpsertTraining(training nlu.Training) error
}
