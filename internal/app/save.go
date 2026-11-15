package app

import (
	"path/filepath"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

// storeSaver — адаптер store.Writer к intake.Saver. Для ответа в чат отдаёт
// путь относительно DATA_ROOT, чтобы не показывать абсолютный путь.
type storeSaver struct {
	writer *store.Writer
	root   string
}

// SaveCheckin реализует intake.Saver.
func (s *storeSaver) SaveCheckin(checkin nlu.Checkin) (string, error) {
	path, err := s.writer.WriteCheckin(checkin)
	if err != nil {
		return "", err
	}
	return s.displayPath(path), nil
}

// SaveTraining реализует intake.Saver.
func (s *storeSaver) SaveTraining(training nlu.Training) (string, error) {
	path, err := s.writer.WriteTraining(training)
	if err != nil {
		return "", err
	}
	return s.displayPath(path), nil
}

// SaveTrainingReport реализует intake.ReportSaver.
func (s *storeSaver) SaveTrainingReport(training nlu.Training, markdown string) (string, error) {
	path, err := s.writer.WriteTrainingReport(training, markdown)
	if err != nil {
		return "", err
	}
	return s.displayPath(path), nil
}

// SaveMeasurement реализует intake.MeasurementSaver.
func (s *storeSaver) SaveMeasurement(measurement nlu.Measurement) (string, error) {
	path, err := s.writer.WriteMeasurement(measurement)
	if err != nil {
		return "", err
	}
	return s.displayPath(path), nil
}

// SaveNutrition реализует intake.Saver.
func (s *storeSaver) SaveNutrition(nutrition nlu.Nutrition) (string, error) {
	path, err := s.writer.WriteNutrition(nutrition)
	if err != nil {
		return "", err
	}
	return s.displayPath(path), nil
}

func (s *storeSaver) displayPath(path string) string {
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
