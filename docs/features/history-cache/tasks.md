# Задачи фичи: кэш истории + компактный LLM-контекст

## Этап 1. Аудит/замер
- [x] Подтвердить перечитывание файлов (`CheckinValues/NutritionValues/TrainingFiles`)
- [x] Бенчмарк до/после: 22.8 мс → 10.4 мкс на 365 файлах

## Этап 2. Тесты (RED)
- [x] `store/cache_test.go` (инвалидация, питание/тренировки, people, race)
- [x] `tools/checkin_test.go` (компактная сводка, без сырого markdown)

## Этап 3. Реализация (GREEN)
- [x] `store.Cache`, Reader options, snapshot-парсинг
- [x] Writer cache invalidation
- [x] Общий кэш в app (Reader/Writer/tool)
- [x] `tools.read_last_checkin` → компактная сводка

## Этап 4. Проверки/PR
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [x] docs/architecture/scaling-plan.md, спека, README, отчёт
- [x] Коммит `feat/history-cache`, push, PR
