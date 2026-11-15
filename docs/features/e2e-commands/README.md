# E2E-набор команд и форм

**Ветка:** `test/e2e-commands` · **Запуск:** `make e2e` (или
`go test -run TestE2E ./internal/app`).

Набор проходит через **реальный `router.Router`** и **реальные
`commandHandlers`/intake-формы**, но с временным `DATA_ROOT`, временными
настройками, fake Telegram Sender и fake LLM/Ask. Это регрессия «ни одна
команда не потерялась» без сети и без настоящего Ollama/Telegram.

## Покрытие

| Сценарий | Что проверяется |
|---|---|
| `/help`, `/start`, `/help@bot` | справка, `@username`-суффикс |
| `/settings` | цели, зоны, LLM, «Воскресный разбор: выкл» |
| `/set kcal`, `/set weekly_review`, `/set weekly_review_time` | применение и отображение |
| `/set` без аргументов, `/set reset` | usage fallback, сброс |
| `/set kcal 900`, `/set weekly_review_time 25:00`, `/set weekly_review maybe` | валидация |
| `/status`, `/week`, `/review` | реальные данные из store, анализ недели и стратегия |
| `/ask`, `/ask@bot`, `/ask` без вопроса | вопрос в агент, usage |
| свободный чат | LLM-ответ |
| `/checkin` | все поля формы, файл, повтор при невалидном вводе |
| `/food` | 4 макроса, файл, nutrition-coach |
| `/measure` | тело + лента, файл |
| `/training` (без текста) | форма, файл, RPE/длительность/пульс |
| `/training <текст>` | свободный отчёт, асинхронное сохранение |
| `/cancel` | активная форма и отсутствие формы |
| `/unknown` | вежливый fallback |
| жена (chat 2) | свои `/status`/`/settings`, профили не смешиваются |

## Принципы

- Один тест — один свежий `DATA_ROOT`/`SETTINGS_DIR`; никаких общих файлов.
- Сообщения асинхронного intake ждём с таймаутом, не `sleep` на глаз.
- Не мокаем то, что проверяем: writer/reader/forms/router настоящие; fake —
  только внешние границы (Telegram, LLM/Ask, downloader).
- Новый тест добавляем при добавлении команды/поля формы: это релизный щит.
