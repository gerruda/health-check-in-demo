package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"healthbot/internal/nlu"
)

var (
	checkinFilePattern   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)
	trainingFilePattern  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})_(.+)\.md$`)
	lineSleepPattern     = regexp.MustCompile(`(?m)^- Сон:\s*(\d{1,2}):(\d{2})`)
	lineHRVPattern       = regexp.MustCompile(`(?m)^- HRV:\s*(\d+)\s*мс`)
	lineRHRPattern       = regexp.MustCompile(`(?m)^- Пульс покоя:\s*(\d+)`)
	lineMoodPattern      = regexp.MustCompile(`(?m)^- Настроение:\s*(\d+)/10`)
	lineWeightPattern    = regexp.MustCompile(`(?m)^- Вес:\s*(\d+(?:[.,]\d+)?)\s*кг`)
	nutritionLinePattern = regexp.MustCompile(`(?m)^- (\d+)\s*ккал(?:\s*·\s*Б\s*(\d+))?(?:\s*·\s*Ж\s*(\d+))?(?:\s*·\s*У\s*(\d+))?`)
	lineTrainingDuration = regexp.MustCompile(`(?m)^- Длительность:\s*(\d+)\s*мин`)
	lineTrainingRPE      = regexp.MustCompile(`(?m)^- RPE сессии:\s*(\d+)/10`)
	lineTrainingSkip     = regexp.MustCompile(`(?m)(?:\(не сделано\)|^- не сделано$)`)
)

// DailyValues — показатели одного чек-ина, нужные для зон/HRV_7.
type DailyValues struct {
	Date     time.Time
	SleepH   *float64
	HRV      *int
	RHR      *int
	Mood     *int
	WeightKG *float64
}

// TrainingFile — найденный файл тренировки с итогами.
type TrainingFile struct {
	Date        time.Time
	Kind        string
	Done        bool
	DurationMin *int
	RPESession  *int
}

// DailyNutrition — питание из секции «Питание» чек-ина за дату.
type DailyNutrition struct {
	Date     time.Time
	Kcal     *int
	ProteinG *int
	FatG     *int
	CarbsG   *int
}

// Reader читает историю из файловой БЗ (FR-2): даты чек-инов, последняя дата,
// содержимое файла. Парсинг полей для трендов — на M2.
type Reader struct {
	paths Paths
	cache *Cache
	index IndexQuerier
	log   *slog.Logger
}

// ReaderOption настраивает Reader.
type ReaderOption func(*Reader)

// WithCache подключает общий in-memory кэш (например, с Writer).
func WithCache(cache *Cache) ReaderOption {
	return func(r *Reader) {
		r.cache = cache
	}
}

// WithLogger подключает логгер для пропущенных/битых файлов.
func WithLogger(logger *slog.Logger) ReaderOption {
	return func(r *Reader) {
		r.log = logger
	}
}

// WithIndex подключает SQLite read-model; при ошибке — fallback на Markdown.
func WithIndex(index IndexQuerier) ReaderOption {
	return func(r *Reader) {
		r.index = index
	}
}

// WithoutCache отключает кэш (бенчмарки/диагностика).
func WithoutCache() ReaderOption {
	return func(r *Reader) {
		r.cache = nil
	}
}

// NewReader создаёт reader для путей файловой БЗ. По умолчанию — свой кэш.
func NewReader(paths Paths, opts ...ReaderOption) *Reader {
	reader := &Reader{paths: paths, cache: NewCache()}
	for _, opt := range opts {
		opt(reader)
	}
	return reader
}

// CheckinDates возвращает даты чек-инов (по возрастанию), игнорируя шаблоны.
func (r *Reader) CheckinDates(person nlu.Person) ([]time.Time, error) {
	dir, err := r.paths.DataDir(person)
	if err != nil {
		return nil, err
	}
	return readCheckinDates(filepath.Join(dir, checkinsDirName))
}

func readCheckinDates(checkinsDir string) ([]time.Time, error) {
	entries, err := os.ReadDir(checkinsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: прочитать каталог %q: %w", checkinsDir, err)
	}

	dates := make([]time.Time, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := checkinFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		date, err := time.Parse("2006-01-02", match[1])
		if err != nil {
			continue
		}
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	return dates, nil
}

// LatestCheckinDate возвращает последнюю дату чек-ина (ok=false, если файлов нет).
func (r *Reader) LatestCheckinDate(person nlu.Person) (time.Time, bool, error) {
	dates, err := r.CheckinDates(person)
	if err != nil {
		return time.Time{}, false, err
	}
	if len(dates) == 0 {
		return time.Time{}, false, nil
	}
	return dates[len(dates)-1], true, nil
}

// checkinSnapshot — разобранная история чек-инов (метрики+питание) за один
// проход по файлам; кэшируется по отпечатку каталога.
type checkinSnapshot struct {
	Values    []DailyValues
	Nutrition []DailyNutrition
}

// CheckinValues возвращает последние limit чек-инов с разобранными показателями
// (сон/HRV/пульс/настроение) — для зон и HRV_7.
func (r *Reader) CheckinValues(person nlu.Person, limit int) ([]DailyValues, error) {
	if r.index != nil {
		if values, err := r.index.CheckinValues(person, limit); err == nil {
			return values, nil
		}
	}
	snapshot, err := r.loadCheckinSnapshot(person)
	if err != nil {
		return nil, err
	}
	values := snapshot.Values
	if limit > 0 && len(values) > limit {
		values = values[len(values)-limit:]
	}
	// копия среза: наружу не отдаём внутренний кэш
	result := make([]DailyValues, len(values))
	copy(result, values)
	return result, nil
}

// NutritionValues возвращает питание из последних limit чек-инов (дни с данными).
func (r *Reader) NutritionValues(person nlu.Person, limit int) ([]DailyNutrition, error) {
	if r.index != nil {
		if values, err := r.index.NutritionValues(person, limit); err == nil {
			return values, nil
		}
	}
	snapshot, err := r.loadCheckinSnapshot(person)
	if err != nil {
		return nil, err
	}
	values := snapshot.Nutrition
	if limit > 0 && len(values) > limit {
		values = values[len(values)-limit:]
	}
	result := make([]DailyNutrition, len(values))
	copy(result, values)
	return result, nil
}

func (r *Reader) loadCheckinSnapshot(person nlu.Person) (checkinSnapshot, error) {
	dir, err := r.paths.DataDir(person)
	if err != nil {
		return checkinSnapshot{}, err
	}
	checkinsDir := filepath.Join(dir, checkinsDirName)
	key := string(person) + ":checkins"
	stamp, canCache := dirStamp(checkinsDir)
	if canCache {
		if cached, ok := r.cache.get(key, stamp); ok {
			return cached.(checkinSnapshot), nil
		}
	}

	dates, err := readCheckinDates(checkinsDir)
	if err != nil {
		return checkinSnapshot{}, err
	}
	snapshot := checkinSnapshot{
		Values:    make([]DailyValues, 0, len(dates)),
		Nutrition: make([]DailyNutrition, 0, len(dates)),
	}
	for _, date := range dates {
		content, readErr := r.ReadCheckin(person, date)
		if readErr != nil {
			if r.log != nil {
				r.log.Warn("store: чек-ин пропущен при чтении истории", "person", person, "date", date.Format("2006-01-02"), "error", readErr)
			}
			continue
		}
		snapshot.Values = append(snapshot.Values, parseDailyValues(date, content))
		if entry, ok := parseNutrition(date, content); ok {
			snapshot.Nutrition = append(snapshot.Nutrition, entry)
		}
	}
	if canCache {
		r.cache.set(key, stamp, snapshot)
	}
	return snapshot, nil
}

func parseNutrition(date time.Time, content string) (DailyNutrition, bool) {
	matches := nutritionLinePattern.FindStringSubmatch(content)
	if matches == nil {
		return DailyNutrition{}, false
	}
	entry := DailyNutrition{Date: date}
	entry.Kcal = parseOptionalInt(matches[1])
	if len(matches) > 2 {
		entry.ProteinG = parseOptionalInt(matches[2])
	}
	if len(matches) > 3 {
		entry.FatG = parseOptionalInt(matches[3])
	}
	if len(matches) > 4 {
		entry.CarbsG = parseOptionalInt(matches[4])
	}
	return entry, true
}

// TrainingFiles возвращает файлы тренировок (дата, тип) по возрастанию даты.
func (r *Reader) TrainingFiles(person nlu.Person) ([]TrainingFile, error) {
	dir, err := r.paths.DataDir(person)
	if err != nil {
		return nil, err
	}
	trainingsDir := filepath.Join(dir, trainingsDirName)
	key := string(person) + ":trainings"
	stamp, canCache := dirStamp(trainingsDir)
	if canCache {
		if cached, ok := r.cache.get(key, stamp); ok {
			trainings := cached.([]TrainingFile)
			result := make([]TrainingFile, len(trainings))
			copy(result, trainings)
			return result, nil
		}
	}

	entries, err := os.ReadDir(trainingsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: прочитать каталог %q: %w", trainingsDir, err)
	}

	trainings := make([]TrainingFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := trainingFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		date, parseErr := time.Parse("2006-01-02", match[1])
		if parseErr != nil {
			continue
		}
		training := TrainingFile{Date: date, Kind: match[2], Done: true}
		content, readErr := os.ReadFile(filepath.Join(trainingsDir, entry.Name()))
		if readErr != nil {
			if r.log != nil {
				r.log.Warn("store: тренировка пропущена при чтении", "person", person, "file", entry.Name(), "error", readErr)
			}
		}
		if readErr == nil {
			text := string(content)
			if lineTrainingSkip.MatchString(text) {
				training.Done = false
			}
			if value := matchFirstInt(lineTrainingDuration, text); value != nil {
				training.DurationMin = value
			}
			if value := matchFirstInt(lineTrainingRPE, text); value != nil {
				training.RPESession = value
			}
		}
		trainings = append(trainings, training)
	}
	sort.Slice(trainings, func(i, j int) bool {
		if trainings[i].Date.Equal(trainings[j].Date) {
			return trainings[i].Kind < trainings[j].Kind
		}
		return trainings[i].Date.Before(trainings[j].Date)
	})
	if canCache {
		r.cache.set(key, stamp, trainings)
	}
	return trainings, nil
}

func parseDailyValues(date time.Time, content string) DailyValues {
	values := DailyValues{Date: date}
	if match := lineSleepPattern.FindStringSubmatch(content); match != nil {
		var hours, minutes int
		if _, err := fmt.Sscanf(match[1], "%d", &hours); err == nil {
			if _, err := fmt.Sscanf(match[2], "%d", &minutes); err == nil {
				sleep := float64(hours) + float64(minutes)/60
				values.SleepH = &sleep
			}
		}
	}
	if match := lineHRVPattern.FindStringSubmatch(content); match != nil {
		var hrv int
		if _, err := fmt.Sscanf(match[1], "%d", &hrv); err == nil {
			values.HRV = &hrv
		}
	}
	if match := lineRHRPattern.FindStringSubmatch(content); match != nil {
		var rhr int
		if _, err := fmt.Sscanf(match[1], "%d", &rhr); err == nil {
			values.RHR = &rhr
		}
	}
	if match := lineMoodPattern.FindStringSubmatch(content); match != nil {
		var mood int
		if _, err := fmt.Sscanf(match[1], "%d", &mood); err == nil {
			values.Mood = &mood
		}
	}
	if match := lineWeightPattern.FindStringSubmatch(content); match != nil {
		if weight, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64); err == nil {
			values.WeightKG = &weight
		}
	}
	return values
}

// ReadCheckin читает чек-ин за дату.
func (r *Reader) ReadCheckin(person nlu.Person, date time.Time) (string, error) {
	path, err := r.paths.CheckinFile(person, date)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("store: прочитать чек-ин %q: %w", path, err)
	}
	return string(data), nil
}

func parseOptionalInt(raw string) *int {
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &value
}

func matchFirstInt(pattern *regexp.Regexp, text string) *int {
	match := pattern.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	return parseOptionalInt(match[1])
}
