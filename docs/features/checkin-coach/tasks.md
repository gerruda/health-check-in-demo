# Задачи фичи: коуч-ответ на чек-ин

## Этап 1. Тесты (RED)
- [x] `internal/intake/coach_test.go`
- [x] `TestCheckinReplyCoach` в app
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `intake`: Coach, WithCoach, buildReply
- [x] `app.commandHandlers.CheckinReply`: зона/HRV_7/ориентир
- [x] wiring: buildIntake(coach), команды до intake
- [x] fallback при ошибке/питании-only

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [x] demo: зона/HRV_7/«Сохранено»
- [ ] Ручная проверка в Telegram после мержа

## Этап 4. PR
- [x] Коммит `feat/checkin-coach`, push, PR
