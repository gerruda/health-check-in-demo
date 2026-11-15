# Задачи фичи: SQLite-индекс

## Этап 1. Схема/парность
- [x] `historydb.Open/migrate`, Rebuild из Markdown
- [x] `CheckinValues/NutritionValues/TrainingSessions` (limit/order)
- [x] тесты парности/изоляции профилей

## Этап 2. Write-through/интеграция
- [x] Upsert чек-ина/питания (COALESCE) и тренировки (merge payload)
- [x] `store.Reader.WithIndex` (fallback Markdown), `Writer.WithIndexUpdater`
- [x] app: `DATA_ROOT/.healthbot/history.db`, rebuild при старте, graceful fallback
- [x] training report upsert
- [x] tests partial training/limit

## Этап 3. Проверки/PR
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [x] Обновить scaling-plan/спеку/README/отчёт
- [x] Коммит `feat/sqlite-index`, push, PR
