package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"healthbot/internal/nlu"
)

func TestParseReportTrainingKeepsMachineFields(t *testing.T) {
	content := `# Уличная тренировка: 13.11.2026

## Утренние показатели
- нет данных чек-ина

## Разминка/заминка
- Суставная разминка

## Основные упражнения (3 подхода)
| Упражнение | Подходы | Повторения | RPE | Примечания |
|------------|---------|------------|-----|------------|
| подтягивания узким хватом | 3 | 3 | 7 | — |
| отжимания на брусьях | 3 | 10 | 7 | — |

## Кардио-дополнение
- подъем по лестнице на 32 этаж

## Общая статистика
- Длительность: 45 мин
- Калории: 195 ккал
- Средний пульс: 123
- RPE: 7
- Общий объём: 3 упражнения × 3 подхода = 9 подходов

## Боль
- плечо: 3/10

## Самочувствие
- тяжело
`
	date := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	training := parseTrainingSession(date, "ulichnaya", nlu.Athlete, content)

	require.NotNil(t, training.DurationMin)
	assert.Equal(t, 45, *training.DurationMin)
	require.NotNil(t, training.RPESession)
	require.NotNil(t, training.AvgHR)
	assert.Equal(t, 123, *training.AvgHR)
	require.NotNil(t, training.Calories)
	assert.Equal(t, 195, *training.Calories)
	assert.Equal(t, "подъем по лестнице на 32 этаж", training.Cardio)
	require.NotNil(t, training.RPESession)
	assert.Equal(t, 7, *training.RPESession)
	assert.Equal(t, 3, training.Pain["плечо"])
	assert.Equal(t, "тяжело", training.Felt)
	require.Len(t, training.Exercises, 3) // разминка + 2 основных
	assert.Equal(t, "Суставная разминка", training.Exercises[0].Name)
	assert.Equal(t, "разминка", training.Exercises[0].Note)
	assert.Equal(t, "подтягивания узким хватом", training.Exercises[1].Name)
	require.NotNil(t, training.Exercises[1].Sets)
	assert.Equal(t, 3, *training.Exercises[1].Sets)
	require.NotNil(t, training.Exercises[1].Reps)
	assert.Equal(t, 3, *training.Exercises[1].Reps)
}
