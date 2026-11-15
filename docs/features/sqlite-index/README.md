# Фича: SQLite read-model истории (этап 2 плана масштабирования)

**Ветка:** `feat/sqlite-index` · **Статус:** готово к PR · **Дата:** 2026-11-12
**Связанные пункты:** FR-2.9, план [`architecture/scaling-plan.md`](../../architecture/scaling-plan.md).

## 1. Что сделано

- Пакет `internal/historydb` (чистый Go-драйвер `modernc.org/sqlite`):
  - схема `checkins(person,date,sleep,hrv,rhr,mood,weight,kcal,protein,fat,carbs)`
    и `trainings(person,date,kind,done,duration,rpe,calories,payload_json)`;
  - `Rebuild(ctx, dataRoot)` — полная перестройка из **Markdown-канона**;
  - write-through: `UpsertCheckin/Nutrition/Training` после записи Markdown
    (частичные записи не теряют поля/упражнения — COALESCE/merge payload);
  - запросы `CheckinValues`, `NutritionValues`, `TrainingSessions` с limit/порядком.
- `store.Reader` с `WithIndex` — читает из SQLite; **при ошибке падает обратно
  на Markdown/cache**; `store.Writer` с `WithIndexUpdater` — write-through.
- App при старте открывает `DATA_ROOT/.healthbot/history.db`, перестраивает
  индекс из Markdown и логирует длительность; при сбое индекса бот продолжает
  на Markdown (NFR-2). Индекс используют `/status`, `/week`, coach-ответы,
  training report.

## 2. Acceptance criteria

| # | Критерий | Проверка |
|---|---|---|
| AC1 | Rebuild(markdown) → запросы дают те же значения, профили не смешиваются | `TestRebuildFromMarkdown` |
| AC2 | Write-through после чек-ина/питания/тренировки | `TestUpsertWriteThrough` |
| AC3 | Частичная запись тренировки не теряет упражнения | `TestUpsertTrainingPreservesExercises` |
| AC4 | Limit/порядок дат | `TestLimitAndOrder` |
| AC5 | Reader использует индекс, при ошибке — Markdown fallback | код + app wiring |
| AC6 | `make check`/`-race`/`govulncheck`; спека/README/отчёт; PR | CI |

## 3. Ограничения / следующие этапы

- Перестройка при старте — простой и надёжный вариант; дальше — инкрементальный
  sync/reindex CLI и фоновый health-check.
- `users`-реестр и per-user `flock` — этап 3 (сейчас два профиля athlete/wife,
  один процесс writer'а).
- Внешние правки Markdown не видны до рестарта/`Rebuild` (Markdown — канон,
  индекс производный).
- SQLite WAL включён, `busy_timeout=5000`, `MaxOpenConns=1`; при нескольких
  процессах нужен per-user lock (план).
