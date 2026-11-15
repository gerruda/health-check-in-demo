# Задачи фичи: приём тренировок + /status

## Этап 1. Тесты (RED)
- [x] `internal/intake/training_test.go` (эвристика/парсер)
- [x] `TestServiceSavesTraining`, `TestServiceTrainingWithRealStore`
- [x] `internal/router/training_test.go`
- [x] `/status`: «Ориентир», без моделей
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `intake/training.go`: LooksLikeTraining/ParseTraining/FormatTrainingReply
- [x] `intake/training_service.go`: SubmitTraining/processTraining
- [x] `Saver.SaveTraining` + app storeSaver
- [x] router: маршрут, `/training`, HelpText
- [x] `/status`: убраны LLM/Vision, добавлен «Ориентир»

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [ ] Ручная проверка в Telegram после мержа

## Этап 4. PR
- [x] Коммит `feat/training-intake`, push, PR
