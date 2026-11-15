package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
	"healthbot/internal/tools"
)

func ptrInt(value int) *int { return &value }

func ptrFloat(value float64) *float64 { return &value }

func TestLastCheckinCompactSummary(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	_, err := writer.WriteCheckin(nlu.Checkin{
		Date: "2026-11-01", Person: nlu.Athlete,
		HRV: ptrInt(30), SleepH: ptrFloat(6.5), RHR: ptrInt(60),
	})
	require.NoError(t, err)
	_, err = writer.WriteCheckin(nlu.Checkin{
		Date: "2026-11-03", Person: nlu.Athlete,
		HRV: ptrInt(34), SleepH: ptrFloat(7.4), RHR: ptrInt(57), Mood: ptrInt(7),
	})
	require.NoError(t, err)
	_, err = writer.WriteTraining(nlu.Training{
		Date: "2026-11-02", Person: nlu.Athlete, Kind: "sila-A", Done: true,
	})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "данные_пользователя", "ежедневные_чекины"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "данные_пользователя", "ежедневные_чекины", "_шаблон.md"), []byte("# Шаблон"), 0o600))

	tool := tools.NewLastCheckin(root)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"person":"athlete"}`))
	require.NoError(t, err)

	assert.Contains(t, out, "Сводка (атлет)")
	assert.Contains(t, out, "03.11.2026")
	assert.Contains(t, out, "HRV 34 мс")
	assert.Contains(t, out, "сон 7:24")
	assert.Contains(t, out, "HRV_7")
	assert.Contains(t, out, "Последние тренировки")
	assert.Less(t, len(out), 1500, "в LLM идёт компактная сводка, не сырой файл")
	assert.NotContains(t, out, "# Чек-ин", "сырые markdown-файлы не отдаём")
}

func TestLastCheckinWife(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	_, err := writer.WriteCheckin(nlu.Checkin{
		Date: "2026-11-02", Person: nlu.Wife, SleepH: ptrFloat(7.4), HRV: ptrInt(35),
	})
	require.NoError(t, err)

	tool := tools.NewLastCheckin(root)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"person":"wife"}`))
	require.NoError(t, err)
	assert.Contains(t, out, "02.11.2026")
	assert.Contains(t, out, "Сводка (жена)")
	assert.Contains(t, out, "сон 7:24")
}

func TestLastCheckinNoData(t *testing.T) {
	tool := tools.NewLastCheckin(t.TempDir())

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"person":"athlete"}`))
	require.NoError(t, err)
	assert.Contains(t, out, "нет данных")
}

func TestLastCheckinInvalidPerson(t *testing.T) {
	tool := tools.NewLastCheckin(t.TempDir())

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"person":"dog"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "person")
}

func TestRegistryDescriptorsAndDispatch(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	_, err := writer.WriteCheckin(nlu.Checkin{Date: "2026-11-05", Person: nlu.Athlete, HRV: ptrInt(40)})
	require.NoError(t, err)

	registry := tools.NewRegistry(tools.NewLastCheckin(root))
	descriptors := registry.Descriptors()
	require.Len(t, descriptors, 1)
	assert.Equal(t, "read_last_checkin", descriptors[0].Name)
	assert.NotEmpty(t, descriptors[0].Description)
	assert.NotEmpty(t, descriptors[0].Parameters)

	out, err := registry.Execute(context.Background(), "read_last_checkin", json.RawMessage(`{"person":"athlete"}`))
	require.NoError(t, err)
	assert.Contains(t, out, "HRV 40 мс")

	_, err = registry.Execute(context.Background(), "unknown_tool", nil)
	require.Error(t, err)
}
