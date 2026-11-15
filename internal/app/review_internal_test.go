package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func floatPtrReview(value float64) *float64 { return &value }
func intPtrReview(value int) *int           { return &value }

func TestReviewShowsDeltasAndActions(t *testing.T) {
	handlers, cfg := newTestHandlers(t)
	// фиксируем «сегодня», иначе тест зависит от реальной даты и недельного окна
	handlers.now = func() time.Time { return time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC) }
	writer := store.NewWriter(store.NewPaths(cfg.DataRoot))
	_, err := writer.WriteMeasurement(nlu.Measurement{
		Date: "2026-11-01", Person: nlu.Athlete,
		WeightKG: floatPtrReview(103.5), WaistCM: floatPtrReview(112), Visceral: intPtrReview(16),
	})
	require.NoError(t, err)
	_, err = writer.WriteMeasurement(nlu.Measurement{
		Date: "2026-11-08", Person: nlu.Athlete,
		WeightKG: floatPtrReview(102.8), WaistCM: floatPtrReview(110.5), Visceral: intPtrReview(15),
	})
	require.NoError(t, err)
	hrv := 32
	_, err = writer.WriteCheckin(nlu.Checkin{Date: "2026-11-08", Person: nlu.Athlete, HRV: &hrv})
	require.NoError(t, err)

	review, err := handlers.Review(context.Background(), 1)
	require.NoError(t, err)
	assert.Contains(t, review, "Воскресный разбор")
	assert.Contains(t, review, "вес -0.7 кг")
	assert.Contains(t, review, "талия -1.5 см")
	assert.Contains(t, review, "HRV: 32.0")
	assert.Contains(t, review, "Стратегия на следующую неделю")
}
