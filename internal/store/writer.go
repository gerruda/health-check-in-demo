package store

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"healthbot/internal/metrics"
	"healthbot/internal/nlu"
)

// Writer сохраняет данные в файловую БЗ (FR-2): атомарная запись tmp→rename,
// merge секций markdown без затирания старых данных (FR-2.5/2.6).
type Writer struct {
	paths Paths
	cache *Cache
	index IndexUpdater
}

// WriterOption настраивает Writer.
type WriterOption func(*Writer)

// WithWriterCache подключает общий кэш для инвалидации после записи.
func WithWriterCache(cache *Cache) WriterOption {
	return func(w *Writer) {
		w.cache = cache
	}
}

// WithIndexUpdater подключает write-through обновление read-model.
func WithIndexUpdater(index IndexUpdater) WriterOption {
	return func(w *Writer) {
		w.index = index
	}
}

// NewWriter создаёт writer для путей файловой БЗ.
func NewWriter(paths Paths, opts ...WriterOption) *Writer {
	writer := &Writer{paths: paths}
	for _, opt := range opts {
		opt(writer)
	}
	return writer
}

// WriteCheckin сохраняет/мержит чек-ин в ежедневные_чекины/YYYY-MM-DD.md.
func (w *Writer) WriteCheckin(checkin nlu.Checkin) (path string, err error) {
	defer func() { metrics.Default().ObserveSave("checkin", saveStatus(err)) }()
	date, err := parseDate(checkin.Date)
	if err != nil {
		return "", err
	}
	path, err = w.paths.CheckinFile(checkin.Person, date)
	if err != nil {
		return "", err
	}
	if err := w.writeMerged(path, renderCheckin(checkin)); err != nil {
		return "", err
	}
	w.invalidate(checkin.Person)
	if w.index != nil {
		_ = w.index.UpsertCheckin(checkin)
	}
	return path, nil
}

// WriteTraining сохраняет/мержит тренировку (FR-2.2); пропуск — тоже данные (FR-2.7).
func (w *Writer) WriteTraining(training nlu.Training) (path string, err error) {
	defer func() { metrics.Default().ObserveSave("training", saveStatus(err)) }()
	date, err := parseDate(training.Date)
	if err != nil {
		return "", err
	}
	path, err = w.paths.TrainingFile(training.Person, date, training.Kind)
	if err != nil {
		return "", err
	}
	if err := w.writeMerged(path, renderTraining(training)); err != nil {
		return "", err
	}
	w.invalidate(training.Person)
	if w.index != nil {
		_ = w.index.UpsertTraining(training)
	}
	return path, nil
}

// WriteNutrition сохраняет питание (FR-2.3) секцией в чек-ине за дату питания.
func (w *Writer) WriteNutrition(nutrition nlu.Nutrition) (path string, err error) {
	defer func() { metrics.Default().ObserveSave("nutrition", saveStatus(err)) }()
	date, err := parseDate(nutrition.Date)
	if err != nil {
		return "", err
	}
	path, err = w.paths.CheckinFile(nutrition.Person, date)
	if err != nil {
		return "", err
	}
	if err := w.writeMerged(path, renderNutrition(nutrition)); err != nil {
		return "", err
	}
	w.invalidate(nutrition.Person)
	if w.index != nil {
		_ = w.index.UpsertNutrition(nutrition)
	}
	return path, nil
}

func (w *Writer) invalidate(person nlu.Person) {
	if w.cache != nil {
		w.cache.InvalidatePerson(string(person))
	}
}

func (w *Writer) writeMerged(path string, newDocument document) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeAtomic(path, []byte(renderDocument(newDocument)))
		}
		return fmt.Errorf("store: прочитать %q: %w", path, err)
	}

	merged, changed := mergeDocuments(parseDocument(string(data)), newDocument, time.Now())
	if !changed {
		return nil
	}
	return writeAtomic(path, []byte(renderDocument(merged)))
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("store: создать каталог %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("store: создать временный файл: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: записать временный файл: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: закрыть временный файл: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("store: переименовать %q → %q: %w", tmpName, path, err)
	}
	return nil
}

// --- markdown-документ и merge ---

type document struct {
	header   string
	sections []documentSection
}

type documentSection struct {
	title string
	lines []string
}

func parseDocument(content string) document {
	doc := document{}
	current := -1

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "## "):
			doc.sections = append(doc.sections, documentSection{
				title: strings.TrimSpace(strings.TrimPrefix(line, "## ")),
			})
			current = len(doc.sections) - 1
		case strings.HasPrefix(line, "# "):
			if doc.header == "" {
				doc.header = line
			}
		case line != "" && current >= 0:
			doc.sections[current].lines = append(doc.sections[current].lines, line)
		}
	}
	return doc
}

func renderDocument(doc document) string {
	var builder strings.Builder
	builder.WriteString(doc.header)
	builder.WriteString("\n")
	for _, section := range doc.sections {
		builder.WriteString("\n## ")
		builder.WriteString(section.title)
		builder.WriteString("\n")
		for _, line := range section.lines {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func mergeDocuments(oldDocument, newDocument document, now time.Time) (document, bool) {
	merged := document{header: oldDocument.header}
	for _, section := range oldDocument.sections {
		merged.sections = append(merged.sections, documentSection{
			title: section.title,
			lines: append([]string(nil), section.lines...),
		})
	}

	changed := false
	if merged.header == "" && newDocument.header != "" {
		merged.header = newDocument.header
		changed = true
	}

	for _, newSection := range newDocument.sections {
		index := -1
		for i := range merged.sections {
			if merged.sections[i].title == newSection.title {
				index = i
				break
			}
		}
		if index == -1 {
			merged.sections = append(merged.sections, documentSection{
				title: newSection.title,
				lines: append([]string(nil), newSection.lines...),
			})
			changed = true
			continue
		}

		existing := make(map[string]struct{}, len(merged.sections[index].lines))
		for _, line := range merged.sections[index].lines {
			existing[line] = struct{}{}
		}

		added := false
		for _, line := range newSection.lines {
			if _, ok := existing[line]; ok {
				continue
			}
			merged.sections[index].lines = append(merged.sections[index].lines, line)
			existing[line] = struct{}{}
			added = true
		}
		if added {
			merged.sections[index].lines = append(merged.sections[index].lines,
				"<!-- обновлено: "+now.Format("2006-01-02 15:04")+" -->")
			changed = true
		}
	}
	return merged, changed
}

// --- рендер чек-ина/тренировки/питания ---

func renderCheckin(checkin nlu.Checkin) document {
	header := "# Чек-ин " + checkin.Date
	if date, err := parseDate(checkin.Date); err == nil {
		header += " (" + weekdayRU[date.Weekday()] + ")"
	}

	metrics := documentSection{title: "Утренние показатели"}
	if checkin.Person == nlu.Athlete {
		metrics.title = "Утренние показатели (Amazfit Balance 2)"
	}

	if checkin.SleepH != nil {
		metrics.lines = append(metrics.lines, "- Сон: "+formatSleep(*checkin.SleepH))
	}
	if checkin.HRV != nil {
		base := 33
		if checkin.Person == nlu.Wife {
			base = 34
		}
		metrics.lines = append(metrics.lines, fmt.Sprintf("- HRV: %d мс (база ~%d)", *checkin.HRV, base))
	}
	if checkin.RHR != nil {
		metrics.lines = append(metrics.lines, fmt.Sprintf("- Пульс покоя: %d уд/мин", *checkin.RHR))
	}
	if checkin.Mood != nil {
		metrics.lines = append(metrics.lines, fmt.Sprintf("- Настроение: %d/10", *checkin.Mood))
	}
	if checkin.AlcoholDoses != nil {
		note := ""
		if checkin.AlcoholNote != "" {
			note = " — " + checkin.AlcoholNote
		}
		if *checkin.AlcoholDoses == 0 {
			metrics.lines = append(metrics.lines, "- Алкоголь вчера: нет"+note)
		} else {
			metrics.lines = append(metrics.lines, fmt.Sprintf("- Алкоголь вчера: %.1f доз%s", *checkin.AlcoholDoses, note))
		}
	}
	if checkin.Person == nlu.Wife {
		if checkin.CycleDay != nil {
			phase := ""
			if checkin.CyclePhase != "" {
				phase = " (" + checkin.CyclePhase + ")"
			}
			metrics.lines = append(metrics.lines, fmt.Sprintf("- День МЦ: %d%s", *checkin.CycleDay, phase))
		}
		if checkin.KneesPain != nil {
			metrics.lines = append(metrics.lines, fmt.Sprintf("- Колени: %d/10", *checkin.KneesPain))
		}
		if checkin.Craving != nil {
			metrics.lines = append(metrics.lines, fmt.Sprintf("- Голод/тяга: %d/10", *checkin.Craving))
		}
		if checkin.PostprandialWalk != nil {
			value := "нет"
			if *checkin.PostprandialWalk {
				value = "да"
			}
			metrics.lines = append(metrics.lines, "- Постпрандиальная ходьба: "+value)
		}
	}
	if checkin.WeightKG != nil {
		metrics.lines = append(metrics.lines, fmt.Sprintf("- Вес: %.1f кг", *checkin.WeightKG))
	}
	if checkin.Steps != nil {
		metrics.lines = append(metrics.lines, fmt.Sprintf("- Шаги: %d", *checkin.Steps))
	}
	if checkin.PlannedTraining != "" {
		metrics.lines = append(metrics.lines, "- Плановая тренировка: "+checkin.PlannedTraining)
	}

	doc := document{header: header, sections: []documentSection{metrics}}
	doc.sections = append(doc.sections,
		documentSection{title: "Зона готовности", lines: []string{"- не рассчитана (M2)"}},
		documentSection{title: "Тренд HRV (7 дней)", lines: []string{"- нет данных (M2)"}},
	)
	if checkin.Context != "" {
		doc.sections = append(doc.sections, documentSection{title: "Контекст", lines: []string{"- " + checkin.Context}})
	}
	return doc
}

func renderTraining(training nlu.Training) document {
	if !training.Done {
		doc := document{header: "# Тренировка " + training.Date + " — " + training.Kind + " (не сделано)"}
		lines := []string{"- не сделано"}
		if training.Felt != "" {
			lines = append(lines, "- причина: "+training.Felt)
		}
		doc.sections = append(doc.sections, documentSection{title: "Пропуск", lines: lines})
		return doc
	}

	doc := document{header: "# Тренировка " + training.Date + " — " + training.Kind}

	var summary []string
	if training.DurationMin != nil {
		summary = append(summary, fmt.Sprintf("- Длительность: %d мин", *training.DurationMin))
	}
	if training.AvgHR != nil {
		summary = append(summary, fmt.Sprintf("- Средний пульс: %d", *training.AvgHR))
	}
	if training.Calories != nil {
		summary = append(summary, fmt.Sprintf("- Калории: %d ккал", *training.Calories))
	}
	if training.RPESession != nil {
		summary = append(summary, fmt.Sprintf("- RPE сессии: %d/10", *training.RPESession))
	}
	if training.Cardio != "" {
		summary = append(summary, "- Кардио: "+training.Cardio)
	}
	if len(summary) > 0 {
		doc.sections = append(doc.sections, documentSection{title: "Итог", lines: summary})
	}

	if len(training.Exercises) > 0 {
		lines := make([]string, 0, len(training.Exercises))
		for _, exercise := range training.Exercises {
			lines = append(lines, formatExercise(exercise))
		}
		doc.sections = append(doc.sections, documentSection{title: "Упражнения", lines: lines})
	}
	if training.Felt != "" {
		doc.sections = append(doc.sections, documentSection{title: "Самочувствие", lines: []string{"- " + training.Felt}})
	}
	if len(training.Pain) > 0 {
		keys := make([]string, 0, len(training.Pain))
		for key, level := range training.Pain {
			if level <= 0 {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return doc
		}
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("- %s: %d/10", capitalize(key), training.Pain[key]))
		}
		doc.sections = append(doc.sections, documentSection{title: "Боль", lines: lines})
	}
	return doc
}

func renderNutrition(nutrition nlu.Nutrition) document {
	doc := document{header: "# Чек-ин " + nutrition.Date}
	var parts []string
	if nutrition.Kcal != nil {
		parts = append(parts, fmt.Sprintf("%d ккал", *nutrition.Kcal))
	}
	if nutrition.ProteinG != nil {
		parts = append(parts, fmt.Sprintf("Б %d", *nutrition.ProteinG))
	}
	if nutrition.FatG != nil {
		parts = append(parts, fmt.Sprintf("Ж %d", *nutrition.FatG))
	}
	if nutrition.CarbsG != nil {
		parts = append(parts, fmt.Sprintf("У %d", *nutrition.CarbsG))
	}

	var lines []string
	if len(parts) > 0 {
		lines = append(lines, "- "+strings.Join(parts, " · "))
	}
	if nutrition.Description != "" {
		lines = append(lines, "- "+nutrition.Description)
	}
	doc.sections = append(doc.sections, documentSection{title: "Питание", lines: lines})
	return doc
}

func formatExercise(exercise nlu.Exercise) string {
	parts := []string{exercise.Name}
	if exercise.WeightKG != nil {
		parts = append(parts, fmt.Sprintf("%g кг", *exercise.WeightKG))
	}
	if exercise.Sets != nil && exercise.Reps != nil {
		parts = append(parts, fmt.Sprintf("%d×%d", *exercise.Sets, *exercise.Reps))
	} else if exercise.Sets != nil {
		parts = append(parts, fmt.Sprintf("%d подх.", *exercise.Sets))
	}
	if exercise.RPE != nil {
		parts = append(parts, fmt.Sprintf("RPE %d", *exercise.RPE))
	}
	line := "- " + strings.Join(parts, " · ")
	if exercise.Note != "" {
		line += " — " + exercise.Note
	}
	return line
}

func formatSleep(hours float64) string {
	totalMinutes := int(math.Round(hours * 60))
	return fmt.Sprintf("%d:%02d", totalMinutes/60, totalMinutes%60)
}

func capitalize(text string) string {
	if text == "" {
		return text
	}
	runes := []rune(text)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func parseDate(text string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(text))
	if err != nil {
		return time.Time{}, fmt.Errorf("store: дата %q: ожидается YYYY-MM-DD", text)
	}
	return date, nil
}

var weekdayRU = map[time.Weekday]string{
	time.Monday:    "понедельник",
	time.Tuesday:   "вторник",
	time.Wednesday: "среда",
	time.Thursday:  "четверг",
	time.Friday:    "пятница",
	time.Saturday:  "суббота",
	time.Sunday:    "воскресенье",
}

func saveStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}
