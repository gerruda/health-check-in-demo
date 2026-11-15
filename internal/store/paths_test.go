package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func testDate() time.Time {
	return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
}

func TestCheckinFile(t *testing.T) {
	paths := store.NewPaths("/data")

	file, err := paths.CheckinFile(nlu.Athlete, testDate())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/data", "данные_пользователя", "ежедневные_чекины", "2026-11-11.md"), file)

	wifeFile, err := paths.CheckinFile(nlu.Wife, testDate())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/data", "данные_жены", "ежедневные_чекины", "2026-11-11.md"), wifeFile)
}

func TestTrainingFile(t *testing.T) {
	paths := store.NewPaths("/data")

	// U10 из ТЗ
	file, err := paths.TrainingFile(nlu.Athlete, testDate(), "sila-A")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/data", "данные_пользователя", "тренировки", "2026-11-11_sila-A.md"), file)

	wifeFile, err := paths.TrainingFile(nlu.Wife, testDate(), "silovaya")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/data", "данные_жены", "тренировки", "2026-11-11_silovaya.md"), wifeFile)

	cycleFile, err := paths.TrainingFile(nlu.Wife, testDate(), "MC1_силовая")
	require.NoError(t, err)
	assert.Contains(t, cycleFile, "2026-11-11_MC1_силовая.md")
}

func TestMeasurementFile(t *testing.T) {
	paths := store.NewPaths("/data")

	file, err := paths.MeasurementFile(nlu.Athlete, testDate())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/data", "данные_пользователя", "замеры", "2026-11-11.md"), file)
}

func TestPathsInvalid(t *testing.T) {
	paths := store.NewPaths("/data")

	_, err := paths.CheckinFile(nlu.Person("dog"), testDate())
	require.Error(t, err)

	// тип тренировки жены у атлета
	_, err = paths.TrainingFile(nlu.Athlete, testDate(), "silovaya")
	require.Error(t, err)

	// path traversal
	_, err = paths.TrainingFile(nlu.Athlete, testDate(), "../sila-A")
	require.Error(t, err)

	// неизвестный тип жены
	_, err = paths.TrainingFile(nlu.Wife, testDate(), "boxing")
	require.Error(t, err)

	// пустой тип
	_, err = paths.TrainingFile(nlu.Athlete, testDate(), "")
	require.Error(t, err)

	// нулевая дата
	_, err = paths.CheckinFile(nlu.Athlete, time.Time{})
	require.Error(t, err)
}

func TestPathsEmptyRoot(t *testing.T) {
	paths := store.NewPaths("")

	_, err := paths.CheckinFile(nlu.Athlete, testDate())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATA_ROOT")
}
