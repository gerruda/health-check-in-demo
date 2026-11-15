package settings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/settings"
)

func newStore(t *testing.T) (*settings.Store, string) {
	t.Helper()
	dir := t.TempDir()
	return settings.NewStore(dir), dir
}

func TestLoadMissing(t *testing.T) {
	store, _ := newStore(t)

	loaded, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.Nil(t, loaded.Kcal)
	assert.Empty(t, loaded.History)
}

func TestApplyKcalAndHistory(t *testing.T) {
	store, dir := newStore(t)

	updated, err := store.Apply(settings.PersonAthlete, "kcal", "2400")
	require.NoError(t, err)
	require.NotNil(t, updated.Kcal)
	assert.Equal(t, 2400, *updated.Kcal)

	// файл создан и читается
	path := filepath.Join(dir, "athlete.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "kcal: 2400")
	assert.Contains(t, string(data), "history:")

	reloaded, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	require.NotNil(t, reloaded.Kcal)
	assert.Equal(t, 2400, *reloaded.Kcal)
	require.Len(t, reloaded.History, 1)
	assert.Equal(t, "kcal", reloaded.History[0].Key)
	assert.Equal(t, "2400", reloaded.History[0].Value)
	assert.Equal(t, time.Now().Format("2006-01-02"), reloaded.History[0].Date)
}

func TestInvalidKcal(t *testing.T) {
	tests := []string{"900", "5000", "abc"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			store, dir := newStore(t)

			_, err := store.Apply(settings.PersonAthlete, "kcal", value)
			require.Error(t, err)

			_, statErr := os.Stat(filepath.Join(dir, "athlete.yaml"))
			assert.True(t, os.IsNotExist(statErr), "файл не должен меняться")
		})
	}
}

func TestInvalidHRVBase(t *testing.T) {
	for _, value := range []string{"10", "14", "151"} {
		store, _ := newStore(t)
		_, err := store.Apply(settings.PersonAthlete, "hrv_base", value)
		require.Error(t, err, "значение %s должно быть отклонено", value)
	}

	store, _ := newStore(t)
	updated, err := store.Apply(settings.PersonAthlete, "hrv_base", "15")
	require.NoError(t, err)
	require.NotNil(t, updated.HRVBaseManual)
	assert.Equal(t, 15, *updated.HRVBaseManual)

	updated, err = store.Apply(settings.PersonAthlete, "hrv_base", "150")
	require.NoError(t, err)
	require.NotNil(t, updated.HRVBaseManual)
	assert.Equal(t, 150, *updated.HRVBaseManual)
}

func TestHRVBaseAuto(t *testing.T) {
	store, _ := newStore(t)

	updated, err := store.Apply(settings.PersonAthlete, "hrv_base", "40")
	require.NoError(t, err)
	require.NotNil(t, updated.HRVBaseManual)

	updated, err = store.Apply(settings.PersonAthlete, "hrv_base", "auto")
	require.NoError(t, err)
	assert.Nil(t, updated.HRVBaseManual, "auto возвращает расчёт от 7-дневного среднего")

	assert.Len(t, updated.History, 2)
}

func TestApplyAllKeys(t *testing.T) {
	tests := []struct {
		key   string
		value string
		check func(t *testing.T, loaded settings.Settings)
	}{
		{"kcal", "2400", func(t *testing.T, s settings.Settings) { require.NotNil(t, s.Kcal); assert.Equal(t, 2400, *s.Kcal) }},
		{"protein_min", "110", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.ProteinMin)
			assert.Equal(t, 110, *s.ProteinMin)
		}},
		{"protein_max", "140", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.ProteinMax)
			assert.Equal(t, 140, *s.ProteinMax)
		}},
		{"fat_max", "90", func(t *testing.T, s settings.Settings) { require.NotNil(t, s.FatMax); assert.Equal(t, 90, *s.FatMax) }},
		{"carbs_min", "150", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.CarbsMin)
			assert.Equal(t, 150, *s.CarbsMin)
		}},
		{"carbs_max", "300", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.CarbsMax)
			assert.Equal(t, 300, *s.CarbsMax)
		}},
		{"weight_goal", "95.5", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.WeightGoalKG)
			assert.InDelta(t, 95.5, *s.WeightGoalKG, 0.001)
		}},
		{"hrv_base", "33", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.HRVBaseManual)
			assert.Equal(t, 33, *s.HRVBaseManual)
		}},
		{"rhr_green_max", "60", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.RHRGreenMax)
			assert.Equal(t, 60, *s.RHRGreenMax)
		}},
		{"rhr_red_min", "63", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.RHRRedMin)
			assert.Equal(t, 63, *s.RHRRedMin)
		}},
		{"hrv_green_margin", "3", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.HRVGreenMargin)
			assert.Equal(t, 3, *s.HRVGreenMargin)
		}},
		{"hrv_red_margin", "6", func(t *testing.T, s settings.Settings) {
			require.NotNil(t, s.HRVRedMargin)
			assert.Equal(t, 6, *s.HRVRedMargin)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			store, _ := newStore(t)
			_, err := store.Apply(settings.PersonAthlete, tt.key, tt.value)
			require.NoError(t, err)

			loaded, err := store.Load(settings.PersonAthlete)
			require.NoError(t, err)
			tt.check(t, loaded)
		})
	}
}

func TestUnknownKey(t *testing.T) {
	store, _ := newStore(t)

	_, err := store.Apply(settings.PersonAthlete, "bogus", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestInvalidPerson(t *testing.T) {
	store, _ := newStore(t)

	_, err := store.Load("dog")
	require.Error(t, err)
	_, err = store.Apply("dog", "kcal", "2300")
	require.Error(t, err)
	require.Error(t, store.Reset("dog"))
}

func TestCrossFieldValidation(t *testing.T) {
	store, _ := newStore(t)

	_, err := store.Apply(settings.PersonAthlete, "protein_min", "150")
	require.NoError(t, err)

	_, err = store.Apply(settings.PersonAthlete, "protein_max", "100")
	require.Error(t, err, "protein_min > protein_max недопустимо")

	loaded, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	require.NotNil(t, loaded.ProteinMin)
	assert.Equal(t, 150, *loaded.ProteinMin)
	assert.Nil(t, loaded.ProteinMax, "невалидное значение не сохраняется")
}

func TestRHRCrossValidation(t *testing.T) {
	store, _ := newStore(t)

	_, err := store.Apply(settings.PersonAthlete, "rhr_green_max", "65")
	require.NoError(t, err)

	_, err = store.Apply(settings.PersonAthlete, "rhr_red_min", "60")
	require.Error(t, err, "rhr_green_max должен быть меньше rhr_red_min")
}

func TestInvalidRanges(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"kcal", "1199"},
		{"kcal", "4001"},
		{"protein_min", "30"},
		{"fat_max", "10"},
		{"carbs_max", "700"},
		{"weight_goal", "20"},
		{"weight_goal", "210"},
		{"hrv_base", "14"},
		{"rhr_green_max", "20"},
		{"rhr_red_min", "130"},
		{"hrv_green_margin", "-1"},
		{"hrv_red_margin", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			store, _ := newStore(t)
			_, err := store.Apply(settings.PersonAthlete, tt.key, tt.value)
			require.Error(t, err)
		})
	}
}

func TestReset(t *testing.T) {
	store, dir := newStore(t)

	_, err := store.Apply(settings.PersonAthlete, "kcal", "2400")
	require.NoError(t, err)

	require.NoError(t, store.Reset(settings.PersonAthlete))

	_, statErr := os.Stat(filepath.Join(dir, "athlete.yaml"))
	assert.True(t, os.IsNotExist(statErr))

	loaded, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.Nil(t, loaded.Kcal)

	require.NoError(t, store.Reset(settings.PersonAthlete), "повторный reset без ошибки")
}

func TestNoTempFiles(t *testing.T) {
	store, dir := newStore(t)

	_, err := store.Apply(settings.PersonAthlete, "kcal", "2400")
	require.NoError(t, err)

	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("остался временный файл: %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestDefaultMargins(t *testing.T) {
	var empty settings.Settings
	assert.Equal(t, 2, empty.GreenMargin())
	assert.Equal(t, 5, empty.RedMargin())

	green := 3
	red := 7
	custom := settings.Settings{HRVGreenMargin: &green, HRVRedMargin: &red}
	assert.Equal(t, 3, custom.GreenMargin())
	assert.Equal(t, 7, custom.RedMargin())
}

func TestWifeSeparateFile(t *testing.T) {
	store, dir := newStore(t)

	_, err := store.Apply(settings.PersonWife, "kcal", "1850")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "wife.yaml"))
	require.NoError(t, err)

	athlete, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.Nil(t, athlete.Kcal, "настройки атлета и жены не смешиваются")
}
