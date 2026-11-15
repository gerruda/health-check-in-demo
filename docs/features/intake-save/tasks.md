# Задачи фичи: intake → store

## Этап 0. Спека
- [x] `docs/features/intake-save/README.md`
- [x] Этот чеклист

## Этап 1. Тесты (RED)
- [x] `internal/intake/save_test.go` — сохранение чек-ина/питания/альбома,
      person, дата, ошибка, без saver, идемпотентность
- [x] `TestFormatReplySaved`
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `intake/save.go`: Saver, WithSaver/WithPersonResolver/WithClock,
      toCheckin/toNutrition, save()
- [x] `intake/service.go`: вызов save после Extract, ответ с путями/ошибкой
- [x] `intake/extract.go`: FormatReply(extracted, savedPaths...)
- [x] `app`: storeSaver поверх store.Writer, personResolver, wiring

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck` — чисто
- [x] Ручная проверка: файл в `данные_*/ежедневные_чекины/` после сообщения
- [ ] Telegram: чек-ин → «Сохранено: …» (после мержа, вручную) (после мержа, если возможно)

## Этап 4. PR
- [x] Коммит `feat/intake-save`, push, PR
