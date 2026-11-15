# Задачи фичи: LLM-извлечение тренировки

## Этап 1. Тесты (RED)
- [x] `training_llm_test.go`
- [x] service: primary/question/fallback
- [x] RED зафиксирован

## Этап 2. Реализация (GREEN)
- [x] `TrainingExtractor`: prompt + JSON schema + validate/normalize
- [x] `needs_more` → вопрос, без сохранения
- [x] merge с правилами (правила — база структуры)
- [x] service integration + `WithTrainingLLM`
- [x] app wiring: active LLM provider
- [x] live-проверка Ollama

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck`
- [x] combined-проверка с веткой `feat/week-summary` (natural parser): merge чистый, тесты зелёные
- [ ] Ручная проверка в Telegram после мержа

## Этап 4. PR
- [x] Коммит `feat/training-llm`, push, PR
