# Задачи фичи: характер «Ритм»

## Этап 1. Спека/решения
- [x] `docs/features/persona-rhythm/README.md` (рерайт и напряжения)
- [x] Ветка `feat/persona-rhythm`

## Этап 2. Тесты (RED)
- [x] `internal/persona/persona_test.go`
- [x] Проверки голоса в `analysis`, `intake`, `app`, `router`

## Этап 3. Реализация (GREEN)
- [x] `internal/persona`: Name/SystemPrompt/CalmEmoji/BannedEmoji/BannedWords
- [x] `llm`: промпт из persona
- [x] `analysis`: «🌿 Доброе утро», «Ориентир», «Сигнал», «корректировка»
- [x] `intake`: «Отметил данные», «Всё верно?»
- [x] `app`: «Отметил», «Вернул дефолты»; `router`: HelpText/неизвестная команда

## Этап 4. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [x] Спека v1.9 (FR-8), README, отчёт
- [ ] Ручная проверка тона в Telegram (после мержа)

## Этап 5. PR
- [x] Коммит `feat/persona-rhythm`, push, PR
