package store_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func writeCheckinFile(t *testing.T, dir, date, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, date+".md"), []byte(content), 0o600))
}

func TestReaderDates(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "ежедневные_чекины")
	writeCheckinFile(t, dir, "2026-11-03", "# Чек-ин 2026-11-03")
	writeCheckinFile(t, dir, "2026-11-01", "# Чек-ин 2026-11-01")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "_шаблон.md"), []byte("# Шаблон"), 0o600))

	reader := store.NewReader(store.NewPaths(root))
	dates, err := reader.CheckinDates(nlu.Athlete)
	require.NoError(t, err)
	require.Len(t, dates, 2)
	assert.Equal(t, "2026-11-01", dates[0].Format("2006-01-02"))
	assert.Equal(t, "2026-11-03", dates[1].Format("2006-01-02"))

	latest, ok, err := reader.LatestCheckinDate(nlu.Athlete)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "2026-11-03", latest.Format("2006-01-02"))
}

func TestReaderEmpty(t *testing.T) {
	reader := store.NewReader(store.NewPaths(t.TempDir()))

	dates, err := reader.CheckinDates(nlu.Wife)
	require.NoError(t, err)
	assert.Empty(t, dates)

	_, ok, err := reader.LatestCheckinDate(nlu.Wife)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestReaderReadCheckin(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "ежедневные_чекины")
	writeCheckinFile(t, dir, "2026-11-03", "# Чек-ин 2026-11-03\n- HRV: 34 мс")

	reader := store.NewReader(store.NewPaths(root))
	date := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)

	content, err := reader.ReadCheckin(nlu.Athlete, date)
	require.NoError(t, err)
	assert.Contains(t, content, "HRV: 34")

	_, err = reader.ReadCheckin(nlu.Athlete, time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC))
	require.Error(t, err)
}

func TestReaderLogsSkippedFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "данные_пользователя", "ежедневные_чекины")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "2026-11-12.md")))

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	reader := store.NewReader(store.NewPaths(root), store.WithLogger(logger))
	_, err := reader.CheckinValues(nlu.Athlete, 7)
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "чек-ин пропущен")
	assert.Contains(t, logs.String(), "2026-11-12")
}
