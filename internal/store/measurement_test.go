package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func floatPtrStore(value float64) *float64 { return &value }
func intPtrStore(value int) *int           { return &value }

func TestMeasurementWriteRead(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	measurement := nlu.Measurement{
		Date: "2026-11-08", Person: nlu.Athlete,
		WeightKG: floatPtrStore(103.2), FatPct: floatPtrStore(34.1), MusclePct: floatPtrStore(62.0),
		Visceral: intPtrStore(15), WaterPct: floatPtrStore(46.2), WaistCM: floatPtrStore(111.0),
		Tape:     map[string]float64{"chest": 110, "biceps": 38.5, "hips": 108, "thigh": 61, "calf": 40},
		Flagship: floatPtrStore(7.4),
	}
	path, err := writer.WriteMeasurement(measurement)
	require.NoError(t, err)
	assert.Contains(t, path, "2026-11-08.md")

	reader := store.NewReader(store.NewPaths(root))
	values, err := reader.Measurements(nlu.Athlete, 5)
	require.NoError(t, err)
	require.Len(t, values, 1)
	got := values[0]
	require.NotNil(t, got.WeightKG)
	assert.InDelta(t, 103.2, *got.WeightKG, 0.01)
	require.NotNil(t, got.WaistCM)
	assert.InDelta(t, 111.0, *got.WaistCM, 0.01)
	require.NotNil(t, got.Visceral)
	assert.Equal(t, 15, *got.Visceral)
	assert.InDelta(t, 38.5, got.Tape["biceps"], 0.01)
	require.NotNil(t, got.Flagship)
	assert.InDelta(t, 7.4, *got.Flagship, 0.01)
}

func TestMeasurementsLimitAndOrder(t *testing.T) {
	root := t.TempDir()
	writer := store.NewWriter(store.NewPaths(root))
	for _, date := range []string{"2026-10-01", "2026-10-08", "2026-10-15"} {
		_, err := writer.WriteMeasurement(nlu.Measurement{Date: date, Person: nlu.Wife, WeightKG: floatPtrStore(60)})
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}
	reader := store.NewReader(store.NewPaths(root))
	values, err := reader.Measurements(nlu.Wife, 2)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Less(t, values[0].Date, values[1].Date)
}
