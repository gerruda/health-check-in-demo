package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func newWriter(t *testing.T) (*store.Writer, string) {
	t.Helper()
	root := t.TempDir()
	return store.NewWriter(store.NewPaths(root)), root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestWriteCheckinCreatesFile(t *testing.T) {
	writer, root := newWriter(t)

	checkin := nlu.Checkin{
		Date:   "2026-11-11",
		Person: nlu.Athlete,
		SleepH: floatPtr(7.55),
		HRV:    intPtr(38),
		RHR:    intPtr(58),
		Mood:   intPtr(7),
	}

	path, err := writer.WriteCheckin(checkin)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "данные_пользователя", "ежедневные_чекины", "2026-11-11.md"), path)

	content := readFile(t, path)
	assert.Contains(t, content, "# Чек-ин 2026-11-11")
	assert.Contains(t, content, "- Сон: 7:33")
	assert.Contains(t, content, "- HRV: 38 мс")
	assert.Contains(t, content, "- Пульс покоя: 58 уд/мин")
	assert.Contains(t, content, "- Настроение: 7/10")
}

func TestWriteWifeCheckin(t *testing.T) {
	writer, _ := newWriter(t)

	checkin := nlu.Checkin{
		Date:      "2026-11-11",
		Person:    nlu.Wife,
		SleepH:    floatPtr(7.4),
		HRV:       intPtr(36),
		CycleDay:  intPtr(4),
		KneesPain: intPtr(2),
	}

	path, err := writer.WriteCheckin(checkin)
	require.NoError(t, err)
	assert.Contains(t, path, "данные_жены")

	content := readFile(t, path)
	assert.Contains(t, content, "День МЦ: 4")
	assert.Contains(t, content, "Колени: 2/10")
}

func TestWriteCheckinMerge(t *testing.T) {
	writer, _ := newWriter(t)

	base := nlu.Checkin{
		Date:   "2026-11-11",
		Person: nlu.Athlete,
		SleepH: floatPtr(7.55),
		HRV:    intPtr(38),
		Mood:   intPtr(7),
	}
	path, err := writer.WriteCheckin(base)
	require.NoError(t, err)

	// I2: правка настроения дописывается, старое сохраняется
	updated := base
	updated.Mood = intPtr(8)
	updated.AlcoholDoses = floatPtr(1)
	_, err = writer.WriteCheckin(updated)
	require.NoError(t, err)

	content := readFile(t, path)
	assert.Contains(t, content, "- Настроение: 7/10", "старое значение сохраняется")
	assert.Contains(t, content, "- Настроение: 8/10", "новое значение дописывается")
	assert.Equal(t, 1, strings.Count(content, "- Настроение: 7/10"), "без дубликатов")
	assert.Equal(t, 1, strings.Count(content, "- Сон: 7:33"), "неизменённые строки не дублируются")
}

func TestWriteCheckinIdempotent(t *testing.T) {
	writer, _ := newWriter(t)

	checkin := nlu.Checkin{
		Date:   "2026-11-11",
		Person: nlu.Athlete,
		SleepH: floatPtr(7.55),
		HRV:    intPtr(38),
	}

	path, err := writer.WriteCheckin(checkin)
	require.NoError(t, err)
	first := readFile(t, path)

	_, err = writer.WriteCheckin(checkin)
	require.NoError(t, err)
	second := readFile(t, path)

	assert.Equal(t, first, second, "идентичная запись не должна менять файл")
}

func TestWriteTraining(t *testing.T) {
	writer, root := newWriter(t)

	training := nlu.Training{
		Date:        "2026-11-11",
		Person:      nlu.Athlete,
		Kind:        "sila-A",
		DurationMin: intPtr(30),
		AvgHR:       intPtr(124),
		RPESession:  intPtr(7),
		Done:        true,
		Felt:        "норм",
		Cardio:      "лестница 16 эт. × 2",
		Exercises: []nlu.Exercise{
			{Name: "Жим гантелей", WeightKG: floatPtr(14), Sets: intPtr(4), Reps: intPtr(10), RPE: intPtr(7), Note: "ок"},
		},
		Pain: map[string]int{"плечо": 2},
	}

	path, err := writer.WriteTraining(training)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "данные_пользователя", "тренировки", "2026-11-11_sila-A.md"), path)

	content := readFile(t, path)
	assert.Contains(t, content, "# Тренировка 2026-11-11 — sila-A")
	assert.Contains(t, content, "Жим гантелей")
	assert.Contains(t, content, "14 кг")
	assert.Contains(t, content, "4×10")
	assert.Contains(t, content, "RPE 7")
	assert.Contains(t, content, "Плечо: 2/10")
	assert.Contains(t, content, "норм")
}

func TestWriteTrainingSkipped(t *testing.T) {
	writer, _ := newWriter(t)

	training := nlu.Training{
		Date:   "2026-11-11",
		Person: nlu.Athlete,
		Kind:   "fitboxing",
		Done:   false,
		Felt:   "устал, решил пропустить",
	}

	path, err := writer.WriteTraining(training)
	require.NoError(t, err)
	content := readFile(t, path)
	assert.Contains(t, content, "не сделано")
	assert.Contains(t, content, "устал")
}

func TestWriteTrainingInvalidKind(t *testing.T) {
	writer, root := newWriter(t)

	training := nlu.Training{Date: "2026-11-11", Person: nlu.Athlete, Kind: "silovaya"}

	_, err := writer.WriteTraining(training)
	require.Error(t, err)

	// файл не создан
	files, readErr := os.ReadDir(filepath.Join(root, "данные_пользователя", "тренировки"))
	if readErr == nil {
		assert.Empty(t, files, "при неверном типе файл не создаётся")
	} else {
		assert.True(t, os.IsNotExist(readErr))
	}
}

func TestWriteNutrition(t *testing.T) {
	writer, root := newWriter(t)

	nutrition := nlu.Nutrition{
		Date:     "2026-11-10",
		Person:   nlu.Athlete,
		Kcal:     intPtr(2480),
		ProteinG: intPtr(114),
		FatG:     intPtr(97),
		CarbsG:   intPtr(291),
	}

	path, err := writer.WriteNutrition(nutrition)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "данные_пользователя", "ежедневные_чекины", "2026-11-10.md"), path)

	content := readFile(t, path)
	assert.Contains(t, content, "## Питание")
	assert.Contains(t, content, "2480 ккал")
	assert.Contains(t, content, "Б 114")
	assert.Contains(t, content, "Ж 97")
	assert.Contains(t, content, "У 291")
}

func TestNoTempFiles(t *testing.T) {
	writer, root := newWriter(t)

	_, err := writer.WriteCheckin(nlu.Checkin{Date: "2026-11-11", Person: nlu.Athlete, HRV: intPtr(38)})
	require.NoError(t, err)

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
