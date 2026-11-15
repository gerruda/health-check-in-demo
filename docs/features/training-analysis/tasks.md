# Задачи фичи: разбор тренировки

## Этап 1. Тесты (RED)
- [x] `analysis/progression_test.go` (U8/U9/volume)
- [x] `store/training_history_test.go`
- [x] app TrainingReply / intake coach
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `analysis.NextSession` + `TrainingVolume`
- [x] `store.Reader.TrainingSessions` (парсинг файла)
- [x] `intake.TrainingCoach` + `WithTrainingCoach`
- [x] `app.commandHandlers.TrainingReply` (объём/следующая/боль)
- [x] app wiring

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [ ] Ручная проверка в Telegram после мержа
- [x] Разбор тренировки из vision: coach в buildReply

## Этап 4. PR
- [x] Коммит `feat/training-analysis`, push, PR
