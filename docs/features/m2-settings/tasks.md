# Задачи фичи: M2 — настройки FR-7

## Этап 0. Спека
- [x] `docs/features/m2-settings/README.md`
- [x] Этот чеклист

## Этап 1. Тесты (RED)
- [x] `internal/settings/settings_test.go` — Load/Apply/Reset, U13/U14,
      диапазоны, cross-field, история, person, tmp
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `internal/settings/settings.go` — Settings/Change, Store, Load/Apply/Reset,
      ParseKey, валидация диапазонов, история
- [x] `internal/settings/store.go` — путь, YAML load/save, атомарная запись,
      дефолтные margins

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck` — чисто
- [x] Ручная проверка YAML-файла (история, значения)

## Этап 4. PR
- [x] Коммит `feat/m2-settings`, push, PR
- [ ] После мержа photo-intake — `/set`/`/settings` в роутере (отдельный PR)
