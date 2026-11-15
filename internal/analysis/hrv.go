package analysis

import "math"

// Direction — направление 7-дневного тренда HRV.
type Direction string

// Направления тренда.
const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
	DirectionFlat Direction = "flat"
)

// Name возвращает русское название направления.
func (d Direction) Name() string {
	switch d {
	case DirectionUp:
		return "рост"
	case DirectionDown:
		return "снижение"
	default:
		return "стабильно"
	}
}

// Arrow возвращает стрелку направления для ответа в чат.
func (d Direction) Arrow() string {
	switch d {
	case DirectionUp:
		return "↑"
	case DirectionDown:
		return "↓"
	default:
		return "→"
	}
}

// HRVTrend — статистика HRV за окно (обычно 7 дней).
type HRVTrend struct {
	Average   float64
	Direction Direction
	Count     int
	Min       int
	Max       int
}

// noiseThresholdMS — шумовой порог направления: один день не считается трендом.
const noiseThresholdMS = 1.0

// TrendHRV считает среднее, направление и min/max по значениям HRV.
// Направление: последнее значение vs среднее ±1 мс; менее 2 значений — flat.
func TrendHRV(values []int) HRVTrend {
	if len(values) == 0 {
		return HRVTrend{Direction: DirectionFlat}
	}

	sum := 0
	minimum, maximum := values[0], values[0]
	for _, value := range values {
		sum += value
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
	}
	average := float64(sum) / float64(len(values))

	direction := DirectionFlat
	if len(values) >= 2 {
		last := float64(values[len(values)-1])
		switch {
		case last > average+noiseThresholdMS:
			direction = DirectionUp
		case last < average-noiseThresholdMS:
			direction = DirectionDown
		}
	}

	return HRVTrend{
		Average:   math.Round(average*10) / 10,
		Direction: direction,
		Count:     len(values),
		Min:       minimum,
		Max:       maximum,
	}
}
