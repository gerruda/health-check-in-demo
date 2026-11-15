# Задачи фичи: photo intake (vision)

## Этап 0. Спека
- [x] `docs/features/photo-intake/README.md` (AC, архитектура, тест-матрица)
- [x] Этот чеклист
- [ ] ТЗ v1.5: FR-6.8, §9 (env), §13 (U24–U29/I13–I16/E8), §5 (`internal/vision`)
- [ ] `.env.example` + `.env`: `LLM_VISION_MODE=ollama`, `LLM_VISION_MODEL=qwen2.5vl:3b`

## Этап 1. Тесты (RED)
- [x] `internal/config/config_test.go` — vision defaults/separate/invalid
- [x] `internal/llm/vision_test.go` — Ollama/OpenAI ExtractVision, images/format
- [x] `internal/llm/provider_test.go` — VisionFromOptions
- [x] `internal/intake/extract_test.go` — питание/готовность/текст-чек-ин/дата/ошибки
- [x] `internal/intake/service_test.go` — альбом/одиночное/раздельные/текст/ошибка
- [x] `internal/router/intake_test.go` — photo/document/текст-чек-ин/чат/no-vision
- [x] `internal/app/image_test.go` — CLI `-image` c mock vision
- [x] RED зафиксирован (`go test ./...` падает)

## Этап 2. Реализация (GREEN)
- [x] `internal/config` — vision-поля, резолверы, валидация
- [x] `internal/llm` — `Provider.ExtractVision`, Ollama images/format, OpenAI image_url,
      fallback; `VisionFromOptions`
- [ ] `internal/vision/extract.go` — промпт (+today), схема, нормализация, валидация
- [ ] `internal/vision/service.go` — альбомы (debounce 1.2с), downloader, сериализация,
      ответы, formatReply
- [x] `internal/router` — `PhotoIntake`, `WithPhotoIntake`, обработка photo/document
- [x] `internal/app` — downloader (`GetFile` + HTTP через прокси), wiring, `ScanImage`
- [x] `cmd/healthbot -image`/`-checkin`, Makefile `image`/`checkin`

## Этап 3. Проверки
- [x] `make check`, `go test -race ./...`, `govulncheck` — чисто
- [x] Живой смоук: готовность (`-image readiness.jpg`)
- [x] Живой смоук: питание + готовность одним альбомом (`-image a.jpg,b.jpg`)
- [ ] Telegram: одиночный скриншот + альбом с подписью (вручную, после запуска)
- [x] README/отчёт/память обновлены (0.3.0)

## Этап 4. PR
- [x] Коммиты в `feat/photo-intake`
- [x] Push, PR (ссылка на README фичи), CI проверит
- [ ] Ревью: спека ↔ тесты ↔ код
