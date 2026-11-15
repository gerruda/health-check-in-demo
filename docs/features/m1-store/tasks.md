# Задачи фичи: M1 — файловое хранилище

## Этап 0. Спека
- [x] `docs/features/m1-store/README.md` (AC, решения, матрица тестов)
- [x] Этот чеклист

## Этап 1. Тесты (RED)
- [x] `internal/nlu/models_test.go` — Person, json-теги
- [x] `internal/store/paths_test.go` — U10, типы, traversal
- [x] `internal/store/writer_test.go` — создание, merge/I2, идемпотентность,
      тренировка/пропуск/питание, отсутствие tmp
- [x] `internal/store/reader_test.go` — даты/latest/чтение
- [x] RED: `go test ./...` падает (пакетов нет)

## Этап 2. Реализация (GREEN)
- [x] `internal/nlu/models.go` — структуры из ТЗ §6
- [x] `internal/store/paths.go` — пути/имена, валидация person/kind/date
- [x] `internal/store/writer.go` — рендер чек-ина/тренировки/питания,
      markdown-merge секций, атомарная запись
- [x] `internal/store/reader.go` — список дат, последняя, чтение

## Этап 3. Проверки
- [x] `make check` (fmt/vet/lint/test) — 0 issues
- [x] `go test -race ./...`, `govulncheck ./...`
- [x] Ручная проверка: записать тестовые файлы во временный `DATA_ROOT`,
      прочитать глазами (формат как в контракте)

## Этап 4. PR
- [x] Коммит в `feat/m1-store`, push, PR с описанием
- [ ] После мержа photo-intake — интеграция: intake → writer (отдельный PR)
