package nlu_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
)

func TestPersonValid(t *testing.T) {
	assert.True(t, nlu.Athlete.Valid())
	assert.True(t, nlu.Wife.Valid())
	assert.False(t, nlu.Person("dog").Valid())
	assert.False(t, nlu.Person("").Valid())
}

func TestCheckinJSONRoundtrip(t *testing.T) {
	sleep := 7.55
	hrv := 38
	checkin := nlu.Checkin{
		Date:   "2026-11-11",
		Person: nlu.Athlete,
		SleepH: &sleep,
		HRV:    &hrv,
	}

	data, err := json.Marshal(checkin)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"sleep_h":7.55`)
	assert.Contains(t, string(data), `"person":"athlete"`)
	assert.Contains(t, string(data), `"hrv":38`)

	var decoded nlu.Checkin
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, checkin.Date, decoded.Date)
	assert.Equal(t, checkin.Person, decoded.Person)
	require.NotNil(t, decoded.SleepH)
	assert.InDelta(t, 7.55, *decoded.SleepH, 0.001)
	require.NotNil(t, decoded.HRV)
	assert.Equal(t, 38, *decoded.HRV)
}
