package logging_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/logging"
)

func TestMsgFormat(t *testing.T) {
	assert.Equal(t, "[router:HandleUpdate] сообщение", logging.Msg("router:HandleUpdate", "сообщение"))
	assert.Equal(t, "[app:Run] запуск", logging.Msg("app:Run", "запуск"))
}

func TestNewWritesJSONLogFile(t *testing.T) {
	dir := t.TempDir()

	logger, closer, err := logging.New(dir)
	require.NoError(t, err)

	logger.Info("проверка", "person", "athlete")
	require.NoError(t, closer.Close())

	path := filepath.Join(dir, "healthbot.log")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"msg":"проверка"`)
	assert.Contains(t, string(data), `"person":"athlete"`)
}

func TestNewCreatesNestedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "вложенный", "logs")

	logger, closer, err := logging.New(dir)
	require.NoError(t, err)
	require.NotNil(t, logger)
	require.NoError(t, closer.Close())

	_, err = os.Stat(filepath.Join(dir, "healthbot.log"))
	require.NoError(t, err)
}

func TestNewRedactsSecrets(t *testing.T) {
	dir := t.TempDir()

	logger, closer, err := logging.New(dir, "SECRET-TOKEN")
	require.NoError(t, err)

	logger.Info(logging.Msg("app:Run", "запуск"), "token", "SECRET-TOKEN")
	logger.Error(logging.Msg("router:reply", "не удалось отправить"),
		"error", errors.New("boom SECRET-TOKEN"))
	require.NoError(t, closer.Close())

	data, err := os.ReadFile(filepath.Join(dir, "healthbot.log"))
	require.NoError(t, err)
	out := string(data)
	assert.NotContains(t, out, "SECRET-TOKEN", "секрет не должен попадать в лог")
	assert.Contains(t, out, "***", "секрет должен заменяться маской")
}

func TestNewRedactsNestedGroups(t *testing.T) {
	dir := t.TempDir()

	logger, closer, err := logging.New(dir, "SECRET-TOKEN")
	require.NoError(t, err)

	logger.Info("cfg", "config", slog.GroupValue(
		slog.String("token", "SECRET-TOKEN"),
		slog.Group("nested", slog.String("token", "SECRET-TOKEN")),
	))
	require.NoError(t, closer.Close())

	data, err := os.ReadFile(filepath.Join(dir, "healthbot.log"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "SECRET-TOKEN")
}

func TestSanitize(t *testing.T) {
	assert.Equal(t, "токен ***", logging.Sanitize("токен SECRET", "SECRET"))
	assert.Equal(t, "без секретов", logging.Sanitize("без секретов", ""))
	assert.Equal(t, "текст", logging.Sanitize("текст"))

	err := logging.SanitizeError(errors.New("bad SECRET"), "SECRET")
	require.Error(t, err)
	assert.Equal(t, "bad ***", err.Error())
	assert.NoError(t, logging.SanitizeError(nil, "SECRET"))
}
