package analysis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"healthbot/internal/analysis"
)

func TestTrendHRVEmpty(t *testing.T) {
	trend := analysis.TrendHRV(nil)
	assert.Zero(t, trend.Count)
	assert.Zero(t, trend.Average)
	assert.Equal(t, analysis.DirectionFlat, trend.Direction)
}

func TestTrendHRVUp(t *testing.T) {
	trend := analysis.TrendHRV([]int{30, 31, 32, 33, 34, 35, 36})

	assert.Equal(t, 7, trend.Count)
	assert.InDelta(t, 33.0, trend.Average, 0.01)
	assert.Equal(t, analysis.DirectionUp, trend.Direction)
	assert.Equal(t, 30, trend.Min)
	assert.Equal(t, 36, trend.Max)
	assert.Equal(t, "↑", trend.Direction.Arrow())
}

func TestTrendHRVDown(t *testing.T) {
	trend := analysis.TrendHRV([]int{36, 35, 34, 33, 32, 31, 30})

	assert.InDelta(t, 33.0, trend.Average, 0.01)
	assert.Equal(t, analysis.DirectionDown, trend.Direction)
	assert.Equal(t, "↓", trend.Direction.Arrow())
}

func TestTrendHRVFlatNoise(t *testing.T) {
	trend := analysis.TrendHRV([]int{33, 32, 34, 33, 33, 34, 33})
	assert.Equal(t, analysis.DirectionFlat, trend.Direction)
	assert.Equal(t, "→", trend.Direction.Arrow())

	single := analysis.TrendHRV([]int{35})
	assert.Equal(t, analysis.DirectionFlat, single.Direction, "один день — не тренд")
	assert.Equal(t, 1, single.Count)
}

func TestTrendHRVStats(t *testing.T) {
	trend := analysis.TrendHRV([]int{31, 33, 35})
	assert.Equal(t, 3, trend.Count)
	assert.InDelta(t, 33.0, trend.Average, 0.01)
	assert.Equal(t, 31, trend.Min)
	assert.Equal(t, 35, trend.Max)
}
