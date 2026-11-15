package settings_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/settings"
)

func TestWeeklyReviewSettings(t *testing.T) {
	store := settings.NewStore(t.TempDir())
	loaded, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.False(t, loaded.WeeklyReviewOn(), "по умолчанию разбор выключен")

	_, err = store.Apply(settings.PersonAthlete, "weekly_review", "on")
	require.NoError(t, err)
	_, err = store.Apply(settings.PersonAthlete, "weekly_review_time", "10:30")
	require.NoError(t, err)
	loaded, err = store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.True(t, loaded.WeeklyReviewOn())
	assert.Equal(t, "10:30", loaded.WeeklyReviewTimeOrDefault("09:00"))

	_, err = store.Apply(settings.PersonAthlete, "weekly_review", "off")
	require.NoError(t, err)
	loaded, err = store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.False(t, loaded.WeeklyReviewOn())
}

func TestWeeklyReviewInvalidTime(t *testing.T) {
	store := settings.NewStore(t.TempDir())
	_, err := store.Apply(settings.PersonAthlete, "weekly_review_time", "25:00")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ЧЧ:ММ")
}

func TestRussianWeeklyReviewAliases(t *testing.T) {
	store := settings.NewStore(t.TempDir())
	_, err := store.Apply(settings.PersonAthlete, "Воскресный разбор", "выкл")
	require.NoError(t, err)
	_, err = store.Apply(settings.PersonAthlete, "Время разбора", "10:30")
	require.NoError(t, err)
	loaded, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.False(t, loaded.WeeklyReviewOn())
	assert.Equal(t, "10:30", loaded.WeeklyReviewTimeOrDefault("09:00"))
}

func TestRussianWeeklyReviewValues(t *testing.T) {
	store := settings.NewStore(t.TempDir())
	_, err := store.Apply(settings.PersonAthlete, "Воскресный разбор", "выкл")
	require.NoError(t, err)
	loaded, err := store.Load(settings.PersonAthlete)
	require.NoError(t, err)
	assert.False(t, loaded.WeeklyReviewOn())
}
