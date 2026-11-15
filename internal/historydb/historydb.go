// Package historydb — SQLite read-model истории (этап 2 плана масштабирования).
// Markdown остаётся каноном: индекс перестраивается через Rebuild и
// обновляется write-through после каждой записи.
package historydb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // чистый Go SQLite-драйвер

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

// DB — SQLite read-model.
type DB struct {
	db *sql.DB
}

// Open открывает/создаёт базу и применяет схему.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("historydb: создать каталог: %w", err)
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("historydb: открыть %q: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := migrate(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &DB{db: sqlDB}, nil
}

// Close закрывает базу.
func (d *DB) Close() error { return d.db.Close() }

func migrate(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS checkins (
  person TEXT NOT NULL,
  date TEXT NOT NULL,
  sleep_h REAL,
  hrv INTEGER,
  rhr INTEGER,
  mood INTEGER,
  weight_kg REAL,
  kcal INTEGER,
  protein_g INTEGER,
  fat_g INTEGER,
  carbs_g INTEGER,
  PRIMARY KEY (person, date)
);
CREATE TABLE IF NOT EXISTS trainings (
  person TEXT NOT NULL,
  date TEXT NOT NULL,
  kind TEXT NOT NULL,
  done INTEGER NOT NULL,
  duration_min INTEGER,
  rpe INTEGER,
  calories INTEGER,
  payload TEXT NOT NULL,
  PRIMARY KEY (person, date, kind)
);
CREATE INDEX IF NOT EXISTS idx_checkins_person_date ON checkins(person, date);
CREATE INDEX IF NOT EXISTS idx_trainings_person_date ON trainings(person, date);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("historydb: миграция: %w", err)
	}
	return nil
}

// Rebuild полностью перестраивает индекс из Markdown-канона.
func (d *DB) Rebuild(ctx context.Context, dataRoot string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("historydb: начать ребилд: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM checkins`); err != nil {
		return fmt.Errorf("historydb: очистить checkins: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM trainings`); err != nil {
		return fmt.Errorf("historydb: очистить trainings: %w", err)
	}

	reader := store.NewReader(store.NewPaths(dataRoot), store.WithoutCache())
	for _, person := range []nlu.Person{nlu.Athlete, nlu.Wife} {
		values, valuesErr := reader.CheckinValues(person, 0)
		if valuesErr != nil {
			return fmt.Errorf("historydb: чек-ины %s: %w", person, valuesErr)
		}
		nutrition, nutritionErr := reader.NutritionValues(person, 0)
		if nutritionErr != nil {
			return fmt.Errorf("historydb: питание %s: %w", person, nutritionErr)
		}
		nutritionByDate := make(map[string]store.DailyNutrition, len(nutrition))
		for _, item := range nutrition {
			nutritionByDate[item.Date.Format("2006-01-02")] = item
		}
		for _, value := range values {
			date := value.Date.Format("2006-01-02")
			entry := nutritionByDate[date]
			if err := upsertCheckinTx(ctx, tx, person, date, value, entry); err != nil {
				return err
			}
			delete(nutritionByDate, date)
		}
		for date, entry := range nutritionByDate {
			if err := upsertCheckinTx(ctx, tx, person, date, store.DailyValues{}, entry); err != nil {
				return err
			}
		}

		trainings, trainingsErr := reader.TrainingSessions(person, 0)
		if trainingsErr != nil {
			return fmt.Errorf("historydb: тренировки %s: %w", person, trainingsErr)
		}
		for _, training := range trainings {
			if err := upsertTrainingTx(ctx, tx, training); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("historydb: зафиксировать ребилд: %w", err)
	}
	return nil
}

// UpsertCheckin обновляет чек-ин после записи Markdown.
func (d *DB) UpsertCheckin(checkin nlu.Checkin) error {
	return upsertCheckin(d.db, checkin)
}

// UpsertNutrition обновляет питание после записи Markdown.
func (d *DB) UpsertNutrition(nutrition nlu.Nutrition) error {
	return upsertNutrition(d.db, nutrition)
}

// UpsertTraining обновляет тренировку после записи Markdown.
func (d *DB) UpsertTraining(training nlu.Training) error {
	return upsertTraining(d.db, training)
}

func upsertCheckinTx(ctx context.Context, tx *sql.Tx, person nlu.Person, date string, values store.DailyValues, nutrition store.DailyNutrition) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO checkins(person,date,sleep_h,hrv,rhr,mood,weight_kg,kcal,protein_g,fat_g,carbs_g)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(person,date) DO UPDATE SET
 sleep_h=COALESCE(excluded.sleep_h,checkins.sleep_h),
 hrv=COALESCE(excluded.hrv,checkins.hrv),
 rhr=COALESCE(excluded.rhr,checkins.rhr),
 mood=COALESCE(excluded.mood,checkins.mood),
 weight_kg=COALESCE(excluded.weight_kg,checkins.weight_kg),
 kcal=COALESCE(excluded.kcal,checkins.kcal),
 protein_g=COALESCE(excluded.protein_g,checkins.protein_g),
 fat_g=COALESCE(excluded.fat_g,checkins.fat_g),
 carbs_g=COALESCE(excluded.carbs_g,checkins.carbs_g)`,
		string(person), date,
		nullFloat(values.SleepH), nullInt(values.HRV), nullInt(values.RHR), nullInt(values.Mood), nullFloat(values.WeightKG),
		nullInt(nutrition.Kcal), nullInt(nutrition.ProteinG), nullInt(nutrition.FatG), nullInt(nutrition.CarbsG))
	if err != nil {
		return fmt.Errorf("historydb: upsert чек-ина %s: %w", date, err)
	}
	return nil
}

func upsertCheckin(db *sql.DB, checkin nlu.Checkin) error {
	entry := store.DailyValues{SleepH: checkin.SleepH, HRV: checkin.HRV, RHR: checkin.RHR, Mood: checkin.Mood, WeightKG: checkin.WeightKG}
	_, err := db.Exec(`INSERT INTO checkins(person,date,sleep_h,hrv,rhr,mood,weight_kg)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(person,date) DO UPDATE SET
 sleep_h=COALESCE(excluded.sleep_h,checkins.sleep_h),
 hrv=COALESCE(excluded.hrv,checkins.hrv),
 rhr=COALESCE(excluded.rhr,checkins.rhr),
 mood=COALESCE(excluded.mood,checkins.mood),
 weight_kg=COALESCE(excluded.weight_kg,checkins.weight_kg)`,
		string(checkin.Person), checkin.Date, nullFloat(entry.SleepH), nullInt(entry.HRV), nullInt(entry.RHR), nullInt(entry.Mood), nullFloat(entry.WeightKG))
	if err != nil {
		return fmt.Errorf("historydb: upsert чек-ина: %w", err)
	}
	return nil
}

func upsertNutrition(db *sql.DB, nutrition nlu.Nutrition) error {
	_, err := db.Exec(`INSERT INTO checkins(person,date,kcal,protein_g,fat_g,carbs_g)
VALUES(?,?,?,?,?,?)
ON CONFLICT(person,date) DO UPDATE SET
 kcal=COALESCE(excluded.kcal,checkins.kcal),
 protein_g=COALESCE(excluded.protein_g,checkins.protein_g),
 fat_g=COALESCE(excluded.fat_g,checkins.fat_g),
 carbs_g=COALESCE(excluded.carbs_g,checkins.carbs_g)`,
		string(nutrition.Person), nutrition.Date, nullInt(nutrition.Kcal), nullInt(nutrition.ProteinG), nullInt(nutrition.FatG), nullInt(nutrition.CarbsG))
	if err != nil {
		return fmt.Errorf("historydb: upsert питания: %w", err)
	}
	return nil
}

func upsertTrainingTx(ctx context.Context, tx *sql.Tx, training nlu.Training) error {
	payload, err := json.Marshal(training)
	if err != nil {
		return fmt.Errorf("historydb: сериализовать тренировку: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO trainings(person,date,kind,done,duration_min,rpe,calories,payload)
VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(person,date,kind) DO UPDATE SET
 done=excluded.done, duration_min=excluded.duration_min, rpe=excluded.rpe,
 calories=excluded.calories, payload=excluded.payload`,
		string(training.Person), training.Date, training.Kind, boolInt(training.Done),
		nullInt(training.DurationMin), nullInt(training.RPESession), nullInt(training.Calories), string(payload))
	if err != nil {
		return fmt.Errorf("historydb: upsert тренировки: %w", err)
	}
	return nil
}

func upsertTraining(db *sql.DB, training nlu.Training) error {
	// частичная запись (например, только RPE) не должна терять упражнения из индекса
	if existing, err := loadTrainingPayload(db, training.Person, training.Date, training.Kind); err == nil {
		training = mergeTrainingForIndex(training, existing)
	}
	payload, err := json.Marshal(training)
	if err != nil {
		return fmt.Errorf("historydb: сериализовать тренировку: %w", err)
	}
	_, err = db.Exec(`INSERT INTO trainings(person,date,kind,done,duration_min,rpe,calories,payload)
VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(person,date,kind) DO UPDATE SET
 done=excluded.done, duration_min=excluded.duration_min, rpe=excluded.rpe,
 calories=excluded.calories, payload=excluded.payload`,
		string(training.Person), training.Date, training.Kind, boolInt(training.Done),
		nullInt(training.DurationMin), nullInt(training.RPESession), nullInt(training.Calories), string(payload))
	if err != nil {
		return fmt.Errorf("historydb: upsert тренировки: %w", err)
	}
	return nil
}

func loadTrainingPayload(db *sql.DB, person nlu.Person, date, kind string) (nlu.Training, error) {
	var payload string
	err := db.QueryRow(`SELECT payload FROM trainings WHERE person=? AND date=? AND kind=?`,
		string(person), date, kind).Scan(&payload)
	if err != nil {
		return nlu.Training{}, err
	}
	var training nlu.Training
	if err := json.Unmarshal([]byte(payload), &training); err != nil {
		return nlu.Training{}, err
	}
	return training, nil
}

func mergeTrainingForIndex(current, previous nlu.Training) nlu.Training {
	if current.DurationMin == nil {
		current.DurationMin = previous.DurationMin
	}
	if current.AvgHR == nil {
		current.AvgHR = previous.AvgHR
	}
	if current.RPESession == nil {
		current.RPESession = previous.RPESession
	}
	if current.Calories == nil {
		current.Calories = previous.Calories
	}
	if current.Cardio == "" {
		current.Cardio = previous.Cardio
	}
	if current.Felt == "" {
		current.Felt = previous.Felt
	}
	if len(current.Pain) == 0 {
		current.Pain = previous.Pain
	}
	if !current.Done && previous.Done {
		current.Done = previous.Done
	}
	for _, candidate := range previous.Exercises {
		found := false
		for _, existing := range current.Exercises {
			if strings.EqualFold(existing.Name, candidate.Name) {
				found = true
				break
			}
		}
		if !found {
			current.Exercises = append(current.Exercises, candidate)
		}
	}
	return current
}

// CheckinValues реализует store.IndexQuerier.
func (d *DB) CheckinValues(person nlu.Person, limit int) ([]store.DailyValues, error) {
	query := `SELECT date,sleep_h,hrv,rhr,mood,weight_kg FROM checkins WHERE person=? ORDER BY date DESC`
	args := []any{string(person)}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("historydb: запрос чек-инов: %w", err)
	}
	defer func() { _ = rows.Close() }()

	values := make([]store.DailyValues, 0, limit)
	for rows.Next() {
		var date string
		var sleep, weight sql.NullFloat64
		var hrv, rhr, mood sql.NullInt64
		if err := rows.Scan(&date, &sleep, &hrv, &rhr, &mood, &weight); err != nil {
			return nil, fmt.Errorf("historydb: скан чек-ина: %w", err)
		}
		parsed, err := time.Parse("2006-01-02", date)
		if err != nil {
			continue
		}
		values = append(values, store.DailyValues{
			Date: parsed, SleepH: floatPtr(sleep), HRV: intPtr(hrv), RHR: intPtr(rhr), Mood: intPtr(mood), WeightKG: floatPtr(weight),
		})
	}
	reverseValues(values)
	return values, rows.Err()
}

// NutritionValues реализует store.IndexQuerier.
func (d *DB) NutritionValues(person nlu.Person, limit int) ([]store.DailyNutrition, error) {
	query := `SELECT date,kcal,protein_g,fat_g,carbs_g FROM checkins WHERE person=?
AND (kcal IS NOT NULL OR protein_g IS NOT NULL OR fat_g IS NOT NULL OR carbs_g IS NOT NULL)
ORDER BY date DESC`
	args := []any{string(person)}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("historydb: запрос питания: %w", err)
	}
	defer func() { _ = rows.Close() }()

	values := make([]store.DailyNutrition, 0, limit)
	for rows.Next() {
		var date string
		var kcal, protein, fat, carbs sql.NullInt64
		if err := rows.Scan(&date, &kcal, &protein, &fat, &carbs); err != nil {
			return nil, fmt.Errorf("historydb: скан питания: %w", err)
		}
		parsed, err := time.Parse("2006-01-02", date)
		if err != nil {
			continue
		}
		values = append(values, store.DailyNutrition{Date: parsed, Kcal: intPtr(kcal), ProteinG: intPtr(protein), FatG: intPtr(fat), CarbsG: intPtr(carbs)})
	}
	reverseNutrition(values)
	return values, rows.Err()
}

// TrainingSessions реализует store.IndexQuerier.
func (d *DB) TrainingSessions(person nlu.Person, limit int) ([]nlu.Training, error) {
	query := `SELECT payload FROM trainings WHERE person=? ORDER BY date DESC`
	args := []any{string(person)}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("historydb: запрос тренировок: %w", err)
	}
	defer func() { _ = rows.Close() }()

	values := make([]nlu.Training, 0, limit)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("historydb: скан тренировки: %w", err)
		}
		var training nlu.Training
		if err := json.Unmarshal([]byte(payload), &training); err != nil {
			continue
		}
		values = append(values, training)
	}
	reverseTrainings(values)
	return values, rows.Err()
}

func nullInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	parsed := value.Float64
	return &parsed
}

func intPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	parsed := int(value.Int64)
	return &parsed
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func reverseValues(values []store.DailyValues) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseNutrition(values []store.DailyNutrition) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseTrainings(values []nlu.Training) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
