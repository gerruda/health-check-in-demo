# Фича: команды M2 — `/set`, `/settings`, `/status`

**Ветка:** `feat/m2-commands` · **Статус:** готово к PR (проверки зелёные) · **Дата:** 2026-11-12
**Связанные пункты ТЗ:** FR-5 (команды), FR-7.1 (показ настроек), FR-3.2 (зоны), §13 (U13, U14, статус).

## 1. Зачем

Настройки (FR-7) и анализ (M2) уже есть, но пользователь не может ими
воспользоваться из чата. Нужны команды:

- `/set <ключ> <значение>` и `/set reset` — цели/пороги из чата;
- `/settings` — текущие значения, `*` = переопределено, плюс активный LLM;
- `/status` — HRV_7 (из реальных файлов), зона, цели, последняя тренировка, LLM.

Также добавляется парсинг истории чек-инов (`store.Reader.CheckinValues`) и
тренировок (`TrainingFiles`) — задел для `/week` и coach-ответов.

## 2. Acceptance criteria

| # | Критерий | Проверка |
|---|---|---|
| AC1 | `/settings` показывает цели/зоны: дефолты контракта и переопределения с `*`, активный LLM/vision | `app/commands_internal_test.go` |
| AC2 | `/set kcal 2400` → `settings.Apply`, ответ «Готово»; невалидное значение → понятная ошибка (U14) | app + router tests |
| AC3 | `/set reset` → `settings.Reset` | tests |
| AC4 | `/status`: HRV_7 (среднее/направление/зоны по 7 дням из файлов), зона по последнему чек-ину с учётом настроек, цели, последняя тренировка, LLM | tests |
| AC5 | `store.Reader.CheckinValues`: парсит сон/HRV/пульс/настроение из сгенерированных файлов, limit, сортировка | `store/values_test.go` |
| AC6 | `store.Reader.TrainingFiles`: список тренировок (дата/тип), сортировка | tests |
| AC7 | Роутер: разбор команд, ошибки хендлеров → текст в чат, `/help` обновлён, неизвестная команда — подсказка | `router/commands_test.go` |
| AC8 | Человек по `chat_id`: атлет/жена (настройки не смешиваются) | app tests |
| AC9 | `make check`/`-race`; спека/README/отчёт; PR | CI |

## 3. Архитектура

- Router объявляет интерфейсы `SettingsHandler` и `StatusHandler` (без импорта
  app/settings); `WithSettings`/`WithStatus`.
- `app.commandHandlers` реализует: `Settings`, `Set`, `Reset`, `Status`;
  использует `settings.Store`, `store.Reader`, `analysis` (зоны/HRV_7), конфиг.
- Источник базы зоны: `hrv_base` из настроек (ручная) или 7-дневное среднее
  (fallback — дефолт атлета 33 / жены 34). Пороги RHR/margins — из настроек.
- Зона по последнему чек-ину: HRV/RHR/сон/настроение из файла.
- Ответы — короткие, на «ты», без давления; ошибки настроек — понятным текстом.

## 4. Матрица тестов

| Тест | Что проверяет |
|---|---|
| `TestReaderCheckinValues` | парсинг сон 7:33/HRV/RHR/настроение, сортировка, limit |
| `TestReaderTrainingFiles` | даты/типы тренировок, сортировка, шаблоны игнорируются |
| `TestRouterSettingsCommand` | `/settings` → handler → ответ |
| `TestRouterSetCommand` | `/set kcal 2400`, `/set reset`, без аргументов |
| `TestRouterStatusCommand` | `/status` → handler |
| `TestRouterHelpListsCommands` | `/help` содержит `/set`, `/status` |
| `TestSettingsDefaultsAndOverrides` | дефолты, `*`, LLM |
| `TestSetInvalidValue` | 900 → ошибка «диапазон» |
| `TestStatusWithCheckins` | HRV_7, 🟢/🟡/🔴, цели, тренировка |
| `TestStatusNoCheckins` | «нет данных», без паники |

## 5. Что НЕ входит

- `/week` (сводка недели) и `/article` — следующими инкрементами.
- История чата/FSM-формы (`/checkin` по шагам) — M4.
- Автоматический пересчёт coach-ответов на сохранение чек-ина — следующим PR.
