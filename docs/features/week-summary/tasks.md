# Задачи фичи: /week

## Этап 1. Тесты (RED)
- [x] `internal/store/week_values_test.go` (питание/вес/детали тренировок)
- [x] `internal/app/week_internal_test.go` (сводка/нет данных)
- [x] `internal/router/week_test.go`
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `Reader.NutritionValues`, `WeightKG`, детали `TrainingFile`
- [x] `app.commandHandlers.Week` + форматирование
- [x] router `WeekHandler`/`WithWeek`/`/week`, help
- [x] app wiring `WithWeek(commands)`

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [x] demo: сводка на тестовых данных
- [ ] Ручная проверка в Telegram после мержа

## Этап 4. PR
- [x] Коммит `feat/week-summary`, push, PR
