# Задачи фичи: M2 — анализ зон и питания

## Этап 0. Спека
- [x] `docs/features/m2-analysis/README.md`
- [x] Этот чеклист

## Этап 1. Тесты (RED)
- [x] `internal/analysis/zones_test.go` — U3–U5, границы, жена, unknown
- [x] `internal/analysis/nutrition_test.go` — U6, цели атлета/жены, фазы, правка
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `internal/analysis/zones.go` — Zone/Thresholds/Input/Result/Evaluate
- [x] `internal/analysis/nutrition.go` — Targets/Input/CheckNutrition/Fix

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck` — чисто

## Этап 4. PR
- [x] Коммит `feat/m2-analysis`, push, PR
- [ ] После мержа m1-store — интеграция с reader/настройками и ответами
