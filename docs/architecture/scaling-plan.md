# План масштабирования: Markdown-канон + SQLite-индекс, мультипользовательность

**Дата:** 2026-11-12 · **Статус:** шаг 1 (кэш) и шаг 2 (SQLite read-model) реализованы; шаг 3 — locks/users — впереди

## 1. Принципы

1. **Markdown — канон.** Человекочитаемые файлы в `данные_<person>/` остаются
   источником правды; их можно читать/бэкапить/править руками.
2. **SQLite — производный read-model.** Ускоряет тренды/выборки, не заменяет
   канон; при потере индекса — полная перестройка из Markdown.
3. **Один писатель на пользователя.** Атомарная запись markdown + advisory-lock
   на профиль; конкурирующие записи сериализуются.
4. **LLM получает только компактный контекст.** Структурированные метрики
   считаются кодом; в промпт — сводка (≤1.5k символов) + узкое окно, никогда
   не вся история.
5. **Изоляция пользователей.** Пути/настройки/кэш/индекс по user_id; никакого
   смешивания данных атлета и жены, и будущих пользователей.

## 2. Текущее состояние (шаг 1)

- `internal/store.Reader` кэширует разобранные чек-ины/питание/тренировки в памяти.
- Инвалидация: отпечаток каталога (mtime+size) + явная инвалидация Writer'ом.
- `read_last_checkin` отдаёт LLM компактную сводку (последний чек-ин, HRV_7, зоны,
  последние тренировки), а не сырой файл.
- Замер: 365 файлов — 22.8 мс → 10.4 мкс на запрос тренда.
- SQLite read-model: `DATA_ROOT/.healthbot/history.db`; rebuild из Markdown при
  старте, write-through после записи, Reader fallback на Markdown.

## 3. Целевая схема

```
Telegram → router → app
                    ├─ writer: lock(profile) → markdown tmp→rename → index upsert (tx)
                    ├─ reader: index query (WAL) → fallback markdown parse
                    └─ LLM: compact context builder (метрики кода, ≤1.5k)
```

### SQLite (WAL, busy_timeout)
```sql
users(id INTEGER PK, slug TEXT UNIQUE, telegram_chat_id INTEGER UNIQUE,
      person TEXT, status TEXT, created_at TEXT)
checkins(user_id, date, sleep_h, hrv, rhr, mood, weight_kg, alcohol,
         PRIMARY KEY(user_id,date))
nutrition(user_id, date, kcal, protein_g, fat_g, carbs_g,
          PRIMARY KEY(user_id,date))
trainings(id PK, user_id, date, kind, done, duration_min, rpe, payload_json)
measurements(user_id, date, weight, body_fat, muscle, visceral, waist, ...)
settings(user_id PK, yaml TEXT, version INTEGER, updated_at)
index_meta(user_id PK, markdown_version INTEGER, built_at TEXT)
```

- Индекс обновляется **после** успешного markdown-коммита в той же операции;
  при ошибке индекса — markdown уже канон, фоновая переиндексация.
- `healthbot -reindex` — полная перестройка; `-reindex --user` — один профиль.
- Версия схемы — `schema_migrations`; миграции только вперёд.
- Чтение трендов — SQL (`AVG`, `COUNT`, окна); падение индекса → fallback на
  Markdown-парсер (медленно, но корректно).

### Конкурентная запись
- Внутри процесса — `sync.Map`/mutex по профилю.
- Между процессами — `flock` на `данные_<slug>/.lock` (или SQLite `BEGIN IMMEDIATE`
  как второй рубеж). Lock берётся на write-операцию, не на всё время жизни бота.
- Merge markdown остаётся идемпотентным; повторная запись не дублирует строки.
- Ошибка lock → мягкий отказ/повтор, не потеря данных.

### Мультипользовательность
- `users` — реестр; allowlist превращается в источник регистрации.
- Пути: `данные_<slug>/`; slug из user_id/username, уникален; settings отдельно.
- Авторизация: chat_id → user; чужие запросы игнорируются; все запросы к Reader/
  Writer принимают user_id, а не глобальный person.
- Квоты/лимиты: размер истории, частота LLM-вызовов; изоляция кэшей.
- Онбординг: `/start` → регистрация (инвайт/allowlist), выбор профиля.
- Совместимость: текущие athlete/wife становятся первыми двумя пользователями
  (`slug=athlete|wife`), данные не перемещаются — маппинг в users.

## 4. Этапы

| Этап | Содержание | Критерий готовности |
|---|---|---|
| **1 (done)** | In-memory кэш + компактный tool-контекст | benchmark, race, тесты |
| **2 (done)** | `store.Index` + SQLite read-model (`internal/historydb`); write-through; rebuild при старте; fallback Markdown | индекс ≡ Markdown на тестах; тренды из SQL |
| 3 | Per-user locks; миграция users; изоляция настроек/путей | конкурентные write-тесты; нет смешивания |
| 4 | Мультипользовательский онбординг, лимиты, фоновый reindex | 3+ пользователя в тестах; нагрузочный тест |
| 5 | Метрики/логи индекса, бэкапы, восстановление | rebuild из бэкапа Markdown |

## 5. Тесты/метрики

- Кэш: инвалидация, конкурентность (`-race`), изоляция профилей.
- Индекс: `rebuild(markdown) == index`; upsert после записи; fallback при
  повреждении; WAL/busy_timeout под конкурентными читателями.
- LLM-контекст: размер сводки ≤1.5k символов; история не попадает в промпт.
- Нагрузка: 1/10/100 пользователей; p95 `/status`, `/week`; отсутствие дублей
  при параллельной записи.

## 6. Риски

- Кэш по mtime каталога не увидит правку файла «на месте» (не через Writer) —
  для внешних правок предусмотреть `-reindex`/явный invalidate.
- SQLite и Markdown могут разойтись при сбое между шагами — Markdown канон,
  индекс перестраиваем; health-check в фоне.
- Мультипользовательность меняет авторизацию/пути — делать за фича-флагом,
  с миграцией тестов и обратной совместимостью athlete/wife.
