# Задачи фичи: отчёт + уточнение

## Этап 1. Парсинг
- [x] Комбинированный чек-ин+тренировка: чсс/вариабельность/сон
- [x] «по 15 раз на руку и на удар», удары джеб/кросс/хук, подъем по лестнице
- [x] `TestParseCombinedUserMessage`

## Этап 2. Отчёт
- [x] `store.Writer.WriteTrainingReport` (атомарная замена)
- [x] `app.TrainingReport` (утро/таблица/кардио/статистика/анализ/прогрессия)
- [x] `TestTrainingReportTemplate`, live combined

## Этап 3. Уточнение
- [x] pending RPE/длительность/пульс, ответ обновляет отчёт
- [x] правила имеют приоритет, LLM не выдумывает RPE/время
- [x] router: pending до маршрутизации; тесты

## Этап 4. Проверки/PR
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [x] Коммит `feat/training-report`, push, PR
