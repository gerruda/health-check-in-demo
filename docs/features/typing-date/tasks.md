# Задачи фичи: typing и дата

## Этап 1. Тесты (RED)
- [x] `internal/typing/typing_test.go`
- [x] `internal/router/typing_test.go`
- [x] `internal/intake/typing_test.go`
- [x] `TestFormatReplyDateRU`
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `internal/typing/typing.go` (первый action сразу + тикер)
- [x] `router.WithTyping` + запуск в `answerWithChat`
- [x] `intake.WithTyping` + запуск в `processImages/processText`
- [x] `app`: sendChatAction(typing) через бота/прокси в wiring
- [x] `intake.FormatReply`: дата ДД.ММ.ГГГГ

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [ ] Живой запуск бота: индикатор в Telegram (ручная проверка пользователем после мержа)

## Этап 4. PR
- [x] Коммит `feat/typing-date`, push, PR
