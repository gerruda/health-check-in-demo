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

func TestReaderNutritionValues(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "ежедневные_чекины")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-11.md"), []byte(`# Чек-ин 2026-11-11

## Питание
- 2480 ккал · Б 114 · Ж 97 · У 291
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-12.md"), []byte(`# Чек-ин 2026-11-12

## Питание
- 2300 ккал · Б 130 · Ж 82 · У 210
- описание приёма
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-10.md"), []byte("# Чек-ин 2026-11-10\n"), 0o600))

	reader := store.NewReader(store.NewPaths(root))
	values, err := reader.NutritionValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, "2026-11-11", values[0].Date.Format("2006-01-02"))
	require.NotNil(t, values[0].Kcal)
	assert.Equal(t, 2480, *values[0].Kcal)
	require.NotNil(t, values[0].ProteinG)
	assert.Equal(t, 114, *values[0].ProteinG)
	require.NotNil(t, values[1].CarbsG)
	assert.Equal(t, 210, *values[1].CarbsG)
}

func TestReaderCheckinWeight(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "ежедневные_чекины")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-12.md"), []byte(`# Чек-ин 2026-11-12

## Утренние показатели
- Вес: 102.4 кг
- HRV: 35 мс (база ~33)
`), 0o600))

	reader := store.NewReader(store.NewPaths(root))
	values, err := reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.NotNil(t, values[0].WeightKG)
	assert.InDelta(t, 102.4, *values[0].WeightKG, 0.01)
}

func TestReaderTrainingFilesDetails(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "тренировки")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-11_sila-A.md"), []byte(`# Тренировка 2026-11-11 — sila-A

## Итог
- Длительность: 55 мин
- RPE сессии: 8/10
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-11-09_fitboxing.md"), []byte(`# Тренировка 2026-11-09 — fitboxing (не сделано)

## Пропуск
- не сделано
`), 0o600))

	reader := store.NewReader(store.NewPaths(root))
	trainings, err := reader.TrainingFiles(nlu.Athlete)
	require.NoError(t, err)
	require.Len(t, trainings, 2)

	first := trainings[0]
	assert.Equal(t, "2026-11-09", first.Date.Format("2006-01-02"))
	assert.False(t, first.Done)
	assert.Nil(t, first.DurationMin)

	second := trainings[1]
	assert.True(t, second.Done)
	require.NotNil(t, second.DurationMin)
	assert.Equal(t, 55, *second.DurationMin)
	require.NotNil(t, second.RPESession)
	assert.Equal(t, 8, *second.RPESession)
}
