# Задачи фичи: LLM-провайдеры + чат + инструменты

Статусы: `[ ]` — не сделано, `[~]` — в работе, `[x]` — готово (тесты зелёные).

## Этап 0. Спека
- [x] Файл фичи `docs/features/llm-providers/README.md` (AC, решения, тест-матрица)
- [x] Этот чеклист задач
- [x] ТЗ v1.4: FR-6.1 `Chat`, FR-6.7 (чат + инструменты), §13 U18–U23/I11–I12/E6, §5 `agent.go`/`tools`
- [x] `.env.example` и дефолт модели: `qwen2.5:3b`

## Этап 1. Тесты (RED) — выполнено
- [x] `internal/llm/ollama_test.go` — chat, tool_calls, ошибки, extract
- [x] `internal/llm/openai_test.go` — chat, tool_calls, 401
- [x] `internal/llm/provider_test.go` — выбор провайдера, fallback
- [x] `internal/llm/agent_test.go` — tool-loop, лимит шагов, ошибки
- [x] `internal/tools/checkin_test.go` — последний чек-ин, шаблон, нет данных, person
- [x] `internal/router/chat_test.go` — ответ в чат, fallback при ошибке LLM
- [x] `internal/app/ask_test.go` — CLI-путь `-ask` на httptest-Ollama
- [x] Зафиксировать RED: `go test ./...` падает (пакетов/функций ещё нет)

## Этап 2. Реализация (GREEN) — выполнено
- [x] `internal/llm/provider.go` — типы (`Message`, `ToolCall`, `Tool`, `Request`),
      интерфейс `Provider` (`Chat`, `Extract`), `FromOptions`, `Fallback`
- [x] `internal/llm/ollama.go` — `/api/chat`, native tools, `format` для Extract,
      таймаут 20 с, логи `[llm:ollama]` (без текста)
- [x] `internal/llm/openai.go` — `/chat/completions`, Bearer, tools, логи `[llm:api]`
- [x] `internal/llm/agent.go` — tool-loop (≤4 шага), executor, system prompt
- [x] `internal/tools/checkin.go` — инструмент `read_last_checkin` (athlete/wife)
- [x] `internal/router/chat.go` — свободный текст → `Chatter`, таймаут, fallback-текст
- [x] `internal/app` — сборка провайдера/агента из конфига, передача в роутер
- [x] `internal/config` — дефолт `OLLAMA_MODEL=qwen2.5:3b`
- [x] `cmd/healthbot -ask` — смоук без Telegram

## Этап 3. Проверки — выполнено
- [x] `make check` — fmt/vet/lint/test зелёные, 0 issues
- [x] `go test -race ./...`
- [x] `govulncheck ./...` — чисто
- [x] Живой смок Ollama: `-ask` простой ответ
- [x] Живой смок инструмента: `-ask "Какой HRV в последнем чек-ине?"` → tool call → ответ
- [ ] `make run` + сообщение в Telegram — ответ модели (финальный E2E, вручную)
- [x] README/отчёт обновлены (история 0.2.0)

## Этап 4. PR
- [x] Коммиты в ветке `feat/llm-providers`
- [x] Push ветки; PR открыть по ссылке из ответа агента (CI проверит)
- [ ] Ревью: спека ↔ тесты ↔ код без расхождений
