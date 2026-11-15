package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func TestCacheInvalidatesAfterWrites(t *testing.T) {
	root := t.TempDir()
	cache := store.NewCache()
	reader := store.NewReader(store.NewPaths(root), store.WithCache(cache))
	writer := store.NewWriter(store.NewPaths(root), store.WithWriterCache(cache))

	values, err := reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	assert.Empty(t, values)

	hrv := 35
	_, err = writer.WriteCheckin(nlu.Checkin{Date: "2026-11-12", Person: nlu.Athlete, HRV: &hrv})
	require.NoError(t, err)

	values, err = reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.NotNil(t, values[0].HRV)
	assert.Equal(t, 35, *values[0].HRV)

	// merge в тот же файл (новая секция) — кэш должен обновиться
	mood := 7
	_, err = writer.WriteCheckin(nlu.Checkin{Date: "2026-11-12", Person: nlu.Athlete, Mood: &mood})
	require.NoError(t, err)
	values, err = reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.NotNil(t, values[0].Mood)
	assert.Equal(t, 7, *values[0].Mood)

	// новый файл — кэш видит изменения
	hrv38 := 38
	_, err = writer.WriteCheckin(nlu.Checkin{Date: "2026-11-13", Person: nlu.Athlete, HRV: &hrv38})
	require.NoError(t, err)
	values, err = reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 2)
	require.NotNil(t, values[1].HRV)
	assert.Equal(t, 38, *values[1].HRV)
}

func TestCacheInvalidatesNutritionAndTrainings(t *testing.T) {
	root := t.TempDir()
	reader := store.NewReader(store.NewPaths(root))
	writer := store.NewWriter(store.NewPaths(root))

	nutrition, err := reader.NutritionValues(nlu.Athlete, 7)
	require.NoError(t, err)
	assert.Empty(t, nutrition)

	kcal := 2300
	_, err = writer.WriteNutrition(nlu.Nutrition{Date: "2026-11-12", Person: nlu.Athlete, Kcal: &kcal})
	require.NoError(t, err)
	nutrition, err = reader.NutritionValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, nutrition, 1)
	assert.Equal(t, 2300, *nutrition[0].Kcal)

	trainings, err := reader.TrainingFiles(nlu.Athlete)
	require.NoError(t, err)
	assert.Empty(t, trainings)
	_, err = writer.WriteTraining(nlu.Training{Date: "2026-11-12", Person: nlu.Athlete, Kind: "sila-A", Done: true})
	require.NoError(t, err)
	trainings, err = reader.TrainingFiles(nlu.Athlete)
	require.NoError(t, err)
	require.Len(t, trainings, 1)
}

func TestCacheConcurrentReadWrite(t *testing.T) {
	root := t.TempDir()
	reader := store.NewReader(store.NewPaths(root))
	writer := store.NewWriter(store.NewPaths(root))

	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := 0; index < 20; index++ {
				_, _ = reader.CheckinValues(nlu.Athlete, 7)
				_, _ = reader.TrainingFiles(nlu.Athlete)
			}
		}()
	}
	for day := 0; day < 20; day++ {
		wg.Add(1)
		go func(day int) {
			defer wg.Done()
			hrv := 30 + day
			date := time.Date(2026, 11, 1+day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
			_, _ = writer.WriteCheckin(nlu.Checkin{Date: date, Person: nlu.Athlete, HRV: &hrv})
		}(day)
	}
	wg.Wait()
}

func TestCacheDoesNotSharePeople(t *testing.T) {
	root := t.TempDir()
	reader := store.NewReader(store.NewPaths(root))
	writer := store.NewWriter(store.NewPaths(root))
	hrv := 35
	_, err := writer.WriteCheckin(nlu.Checkin{Date: "2026-11-12", Person: nlu.Athlete, HRV: &hrv})
	require.NoError(t, err)

	athlete, err := reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	wife, err := reader.CheckinValues(nlu.Wife, 7)
	require.NoError(t, err)
	assert.Len(t, athlete, 1)
	assert.Empty(t, wife)
}

func TestCacheSeesExternalFileEditWithSameDirMtime(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	hrv := 35
	_, err := writer.WriteCheckin(nlu.Checkin{Date: "2026-11-12", Person: nlu.Athlete, HRV: &hrv})
	require.NoError(t, err)

	reader := store.NewReader(store.NewPaths(root))
	values, err := reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.NotNil(t, values[0].HRV)
	assert.Equal(t, 35, *values[0].HRV)

	checkinsDir := filepath.Join(root, "данные_пользователя", "ежедневные_чекины")
	dirInfo, err := os.Stat(checkinsDir)
	require.NoError(t, err)
	path := filepath.Join(checkinsDir, "2026-11-12.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	changed := strings.Replace(string(data), "HRV: 35 мс", "HRV: 40 мс", 1)
	require.NoError(t, os.WriteFile(path, []byte(changed), 0o600)) // #nosec G703 -- файл из t.TempDir()
	// эмулируем грубое разрешение mtime: возвращаем каталогу старую метку
	require.NoError(t, os.Chtimes(checkinsDir, dirInfo.ModTime(), dirInfo.ModTime()))

	values, err = reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.NotNil(t, values[0].HRV)
	assert.Equal(t, 40, *values[0].HRV, "внешняя правка должна инвалидировать кэш")
}
