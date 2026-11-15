// Package store отвечает за файловую базу проекта (FR-2): пути/имена файлов,
// атомарную запись с merge секций и чтение истории. Пакет не знает про Telegram.
package store

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"healthbot/internal/nlu"
)

const (
	athleteDataDir = "данные_пользователя"
	wifeDataDir    = "данные_жены"

	checkinsDirName     = "ежедневные_чекины"
	trainingsDirName    = "тренировки"
	measurementsDirName = "замеры"
)

var (
	athleteTrainingKinds = map[string]bool{
		"fitboxing": true, "sila-A": true, "sila-B": true,
		"ulichnaya": true, "vosstanovlenie": true,
	}
	wifeTrainingKinds = map[string]bool{
		"silovaya": true, "kardio": true, "progulka": true,
	}
	wifeCycleKindPattern = regexp.MustCompile(`^MC\d+_[A-Za-zА-Яа-яЁё0-9_-]+$`)
)

// Paths строит пути файловой БЗ для заданного корня DATA_ROOT.
type Paths struct {
	root string
}

// NewPaths создаёт правила путей для корня данных проекта.
func NewPaths(root string) Paths {
	return Paths{root: root}
}

// Root возвращает корень данных.
func (p Paths) Root() string {
	return p.root
}

// DataDir возвращает корневой каталог данных человека.
func (p Paths) DataDir(person nlu.Person) (string, error) {
	if strings.TrimSpace(p.root) == "" {
		return "", fmt.Errorf("store: пуст DATA_ROOT")
	}
	switch person {
	case nlu.Athlete:
		return filepath.Join(p.root, athleteDataDir), nil
	case nlu.Wife:
		return filepath.Join(p.root, wifeDataDir), nil
	default:
		return "", fmt.Errorf("store: неизвестный person %q", person)
	}
}

// CheckinFile возвращает путь чек-ина за дату: ежедневные_чекины/YYYY-MM-DD.md.
func (p Paths) CheckinFile(person nlu.Person, date time.Time) (string, error) {
	dir, err := p.DataDir(person)
	if err != nil {
		return "", err
	}
	dateText, err := formatDate(date)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, checkinsDirName, dateText+".md"), nil
}

// TrainingFile возвращает путь тренировки: тренировки/YYYY-MM-DD_<тип>.md.
func (p Paths) TrainingFile(person nlu.Person, date time.Time, kind string) (string, error) {
	dir, err := p.DataDir(person)
	if err != nil {
		return "", err
	}
	if err = validateTrainingKind(person, kind); err != nil {
		return "", err
	}
	dateText, err := formatDate(date)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, trainingsDirName, dateText+"_"+kind+".md"), nil
}

// MeasurementFile возвращает путь замера: замеры/YYYY-MM-DD.md.
func (p Paths) MeasurementFile(person nlu.Person, date time.Time) (string, error) {
	dir, err := p.DataDir(person)
	if err != nil {
		return "", err
	}
	dateText, err := formatDate(date)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, measurementsDirName, dateText+".md"), nil
}

func formatDate(date time.Time) (string, error) {
	if date.IsZero() {
		return "", fmt.Errorf("store: не задана дата")
	}
	return date.Format("2006-01-02"), nil
}

func validateTrainingKind(person nlu.Person, kind string) error {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return fmt.Errorf("store: не задан тип тренировки")
	}
	if strings.ContainsAny(kind, `/\`) || strings.Contains(kind, "..") || strings.Contains(kind, " ") {
		return fmt.Errorf("store: недопустимый тип тренировки %q", kind)
	}

	switch person {
	case nlu.Athlete:
		if !athleteTrainingKinds[kind] {
			return fmt.Errorf("store: неизвестный тип тренировки атлета %q", kind)
		}
	case nlu.Wife:
		if !wifeTrainingKinds[kind] && !wifeCycleKindPattern.MatchString(kind) {
			return fmt.Errorf("store: неизвестный тип тренировки жены %q", kind)
		}
	default:
		return fmt.Errorf("store: неизвестный person %q", person)
	}
	return nil
}
