# Задачи фичи: команды M2

## Этап 1. Тесты (RED)
- [x] `internal/store/values_test.go` — CheckinValues/TrainingFiles
- [x] `internal/app/commands_internal_test.go` — Settings/Set/Reset/Status
- [x] `internal/router/commands_test.go` — разбор команд
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `store.Reader.CheckinValues`, `TrainingFiles`
- [x] `config.SettingsDir` (+ .env.example)
- [x] `router`: интерфейсы SettingsHandler/StatusHandler, options, dispatch, HelpText
- [x] `app.commandHandlers`: форматирование настроек, set/reset/status
- [x] `app` wiring: newCommandHandlers + router options

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [ ] Ручная проверка `/settings`, `/set`, `/status` в боте (после мержа)

## Этап 4. PR
- [x] Коммит `feat/m2-commands`, push, PR
