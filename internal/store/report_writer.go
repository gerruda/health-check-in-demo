package store

import (
	"strings"

	"healthbot/internal/metrics"
	"healthbot/internal/nlu"
)

// WriteTrainingReport записывает готовый markdown-отчёт тренировки (FR-3.3):
// полная замена файла, атомарно tmp→rename.
func (w *Writer) WriteTrainingReport(training nlu.Training, markdown string) (path string, err error) {
	defer func() { metrics.Default().ObserveSave("training_report", saveStatus(err)) }()
	date, err := parseDate(training.Date)
	if err != nil {
		return "", err
	}
	path, err = w.paths.TrainingFile(training.Person, date, training.Kind)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(path, []byte(strings.TrimSpace(markdown)+"\n")); err != nil {
		return "", err
	}
	w.invalidate(training.Person)
	if w.index != nil {
		_ = w.index.UpsertTraining(training)
	}
	return path, nil
}
