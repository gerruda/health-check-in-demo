# Фича: in-memory кэш истории + компактная сводка для LLM

**Ветка:** `feat/history-cache` · **Статус:** готово к PR · **Дата:** 2026-11-12
**Связанные пункты ТЗ:** FR-2.9 (кэш/индекс), FR-6.7.4 (контекст LLM), NFR-2/3, масштабирование.

## 1. Проблема (подтверждена)

`/status`, `/week`, коуч-ответ на чек-ин и LLM-инструмент на каждый запрос:
- заново читали каталоги и **все** файлы истории;
- парсили regex'ами сон/HRV/пульс/питание/тренировки;
- отдавали LLM **сырой** чек-ин (до 4000 символов), а не структурированную сводку.

Замер на 365 чек-инах: `CheckinValues(7)` **22.8 мс/op, 9895 allocs** без кэша;
с кэшем — **10.4 мкс/op, 9 allocs** (~2000×). Это и есть узкое место при росте истории.

## 2. Что сделано

### In-memory кэш (`internal/store`)
- `Cache` с ключами `person:checkins` / `person:trainings`; хранит **разобранные**
  `DailyValues`, `DailyNutrition`, `TrainingFile`.
- Валидность — по отпечатку каталога `mtime+size`: атомарная запись (`tmp→rename`)
  меняет mtime, кэш сам инвалидируется. `Writer` с общим кэшем инвалидирует
  `person` явно после записи.
- Один проход по файлам собирает и метрики, и питание (`checkinSnapshot`);
  `CheckinValues`/`NutritionValues` отдают копии срезов — кэш нельзя испортить.
- `Reader` по умолчанию имеет свой кэш; `WithCache` — общий с Writer/tool;
  `WithoutCache` — для бенчмарков.
- Кэш потокобезопасен (`RWMutex`), тест под `-race` с конкурентными чтениями/записями.

### Компактная сводка для LLM (`internal/tools`)
`read_last_checkin` больше не отдаёт markdown. Он считает в коде:
- последний чек-ин (сон/HRV/пульс/настроение/вес);
- `analysis.TrendHRV` + счётчики зон за 7 дней;
- последние 3 тренировки;
- лимит **1500 символов**; вся история в промпт не попадает.

## 3. Acceptance criteria

| # | Критерий | Проверка |
|---|---|---|
| AC1 | Повторные тренд-запросы не перечитывают/не перепарсивают файлы | benchmark 10 мкс vs 22.8 мс |
| AC2 | Запись/merge инвалидирует кэш (явно и по mtime) | `TestCacheInvalidatesAfterWrites` |
| AC3 | Питание и тренировки кэшируются отдельно | `TestCacheInvalidatesNutritionAndTrainings` |
| AC4 | Профили не смешиваются; кэш потокобезопасен | `TestCacheDoesNotSharePeople`, `-race` |
| AC5 | LLM получает компактную сводку, не сырой файл | `TestLastCheckinCompactSummary` (<1500) |
| AC6 | `make check`/`-race`/`govulncheck`; спека/README/план; PR | CI |

## 4. Что дальше (архитектурный план)

См. [`docs/architecture/scaling-plan.md`](../../architecture/scaling-plan.md):
Markdown остаётся каноном, SQLite — производный read-model; per-user lock;
WAL/транзакции; rebuild; мультипользовательский реестр. In-memory кэш — шаг 1,
SQLite-индекс — следующий этап.
