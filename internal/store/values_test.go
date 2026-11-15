package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

const checkinTemplate = `# Чек-ин 2026-11-12 (четверг)

## Утренние показатели (Amazfit Balance 2)
- Сон: 7:33
- HRV: 38 мс (база ~33)
- Пульс покоя: 58 уд/мин
- Настроение: 7/10
`

func TestReaderCheckinValues(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "ежедневные_чекины")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-12.md"), []byte(checkinTemplate), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-11.md"), []byte(`# Чек-ин 2026-11-11

## Утренние показатели
- Сон: 6:55
- HRV: 28 мс (база ~33)
- Пульс покоя: 62 уд/мин
- Настроение: 5/10
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "_шаблон.md"), []byte("# Шаблон"), 0o600))

	reader := store.NewReader(store.NewPaths(root))
	values, err := reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, "2026-11-11", values[0].Date.Format("2006-01-02"))
	assert.Equal(t, "2026-11-12", values[1].Date.Format("2006-01-02"))

	latest := values[1]
	require.NotNil(t, latest.HRV)
	assert.Equal(t, 38, *latest.HRV)
	require.NotNil(t, latest.RHR)
	assert.Equal(t, 58, *latest.RHR)
	require.NotNil(t, latest.Mood)
	assert.Equal(t, 7, *latest.Mood)
	require.NotNil(t, latest.SleepH)
	assert.InDelta(t, 7.55, *latest.SleepH, 0.01)
}

func TestReaderCheckinValuesLimit(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_жены", "ежедневные_чекины")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	for _, date := range []string{"2026-11-01", "2026-11-02", "2026-11-03"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, date+".md"), []byte("- HRV: 30 мс"), 0o600))
	}

	reader := store.NewReader(store.NewPaths(root))
	values, err := reader.CheckinValues(nlu.Wife, 2)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, "2026-11-02", values[0].Date.Format("2006-01-02"))
	assert.Equal(t, "2026-11-03", values[1].Date.Format("2006-01-02"))
}

func TestReaderCheckinValuesEmpty(t *testing.T) {
	reader := store.NewReader(store.NewPaths(t.TempDir()))
	values, err := reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	assert.Empty(t, values)
}

func TestReaderTrainingFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "тренировки")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-10_sila-A.md"), []byte("# Тренировка"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-12_fitboxing.md"), []byte("# Тренировка"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("не тренировка"), 0o600))

	reader := store.NewReader(store.NewPaths(root))
	trainings, err := reader.TrainingFiles(nlu.Athlete)
	require.NoError(t, err)
	require.Len(t, trainings, 2)
	assert.Equal(t, "2026-11-10", trainings[0].Date.Format("2006-01-02"))
	assert.Equal(t, "sila-A", trainings[0].Kind)
	assert.Equal(t, "2026-11-12", trainings[1].Date.Format("2006-01-02"))
	assert.Equal(t, "fitboxing", trainings[1].Kind)
}
