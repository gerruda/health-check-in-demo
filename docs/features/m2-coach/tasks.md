# Задачи фичи: M2 — HRV_7 и ответы коуча

## Этап 0. Спека
- [x] `docs/features/m2-coach/README.md`
- [x] Этот чеклист
- [x] Ветка стеком от `feat/m2-analysis` (использует Zone/NutritionResult)

## Этап 1. Тесты (RED)
- [x] `internal/analysis/hrv_test.go`
- [x] `internal/analysis/coach_test.go`
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `internal/analysis/hrv.go` — Direction/HRVTrend/TrendHRV
- [x] `internal/analysis/coach.go` — CheckinReply, NutritionReply

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck` — чисто
- [x] Лимиты строк ответов (≤10 чек-ин, ≤8 питание)

## Этап 4. PR
- [x] Коммит `feat/m2-coach`, push
- [x] PR с базой `feat/m2-analysis` (стек); после мержа анализа — ретаргет на main
