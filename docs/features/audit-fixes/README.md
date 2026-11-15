# Фича: фиксы по аудиту кода (bugs.md)

**Ветка:** `fix/audit-bugs` · **Дата:** 2026-11-14
**Источник:** [`docs/review/bugs.md`](../../review/bugs.md).

## 1. Контекст

Аудит выявил 7 логических ошибок, не ловящихся `make check`/`-race`. Задача —
покрыть каждую спецификацией и регрессионным тестом, затем исправить одним PR.
Канон данных — Markdown; SQLite — производный read-model.

## 2. Требования и acceptance criteria

| ID | Требование | Acceptance (тест) |
|---|---|---|
| BUG-1 | `Reading.TrainingSessions` должен возвращать упражнения/RPE/длительность/калории/кардио/боль **после** `WriteTrainingReport`; отчёт не должен уничтожать машинно-читаемый канон | `TestTrainingReportRoundTrip`, `TestParseReportTraining` |
| BUG-2 | Инвалидация кэша не зависит от грубого mtime каталога: отпечаток учитывает файлы (имя/размер/mtime) | `TestDirStampChangesAfterFileEdit` |
| BUG-3 | Тренировка со скриншота без даты не сохраняется молча «на сегодня»: бот спрашивает дату и сохраняет после ответа | `TestVisionTrainingWithoutDateAsksDate` |
| BUG-4 | Ошибка разбора питания сообщает про питание, а не про чек-ин | `TestNutritionErrorUsesNutritionHint` |
| BUG-5 | Санитизация `/ask` сохраняет структуру ответа (переносы/списки), удаляя только ссылки/DOI/PMID | `TestSanitizeKeepsLineBreaks` |
| BUG-6 | Ошибка чтения отдельного файла логируется и не маскируется под «нет данных» | `TestReaderLogsSkippedFile` |
| BUG-7 | Эвристика «осмысленный вопрос» одна для router и app | `TestTextutilLooksLikeQuestion` + использование в обоих |

## 3. Не входит

- Переход на SQLite как единственный источник (остаётся Markdown-канон).
- Векторный/гибридный поиск, LLM-качество, UI-косметика — отдельные backlog.

## 4. Проверки

`make check`, `make e2e`, `go test -race -count=1 ./...`, `govulncheck`.
