# Фича: читаемый Telegram-формат и голос «Ритма»

**Ветка:** `feat/telegram-formatting` · **Дата:** 2026-11-14
**Связанные пункты:** FR-8 (тон/подача), UX сообщений.

## 1. Проблема

Бот отправлял LLM/команды как plain text. Telegram не рендерит Markdown-таблицы
и `**...**`: в чате появлялись «сырые» `|` и звёздочки. Сводки были сухими.

## 2. Что сделано

- `internal/telegramfmt`: Markdown → **Telegram HTML** (`parse_mode=HTML`):
  - заголовки → `<b>`, `**жирный**`/`__жирный__` → `<b>`;
  - списки `-`/`*` → `•`, нумерованные сохраняются;
  - inline-code и fenced code → `<code>`/`<pre>`;
  - ссылки `[text](https://...)` → `<a>`;
  - **таблицы** → `header` + пункты `• <b>первая колонка</b> — остальные`;
  - HTML экранируется (`<`, `>`, `&`), инъекции не проходят;
  - `.report`-файлы остаются Markdown-каноном — конвертация только для чата.
- Форматтер подключён во **все** точки отправки: `router.reply`, intake-`sendText`,
  воскресный планировщик. `parse_mode` всегда `HTML`.
- Голос «Ритма»: короткие фразы от себя в `/status`, `/week`, `/review`,
  разборе тренировки и питания (`internal/persona/flavor.go`), с учётом зоны/данных.
  В системный промпт добавлена одна живая фраза в конце, без мотивации.

## 3. Acceptance

| # | Критерий | Проверка |
|---|---|---|
| AC1 | Таблица из ответа LLM не содержит `|`, читается пунктами | `TestFormatTableBecomesBullets` |
| AC2 | Заголовки/жирный/списки/код/ссылки | `TestFormatHeadingsListsAndCode`, `TestFormatEscapesHTMLAndLinks` |
| AC3 | HTML-инъекция экранируется | `TestFormatEscapesHTMLAndLinks` |
| AC4 | Роутер отправляет уже отформатированный текст | `TestRouterFormatsMarkdownAnswer` |
| AC5 | Живые фразы в статусе/неделе/тренировке/питании | `TestStatusWithCheckins`, `TestWeekSummary`, `TestNutrition...`, `flavor_test` |
| AC6 | `make check`/`make e2e`/`-race`/`govulncheck`; спека/README/отчёт | CI |

## 4. Границы

- Telegram не поддерживает таблицы — они сознательно превращаются в списки.
- Markdown-файлы (отчёты, база знаний) не конвертируются и остаются каноном.
- Длинные сообщения >4096 символов — отдельный backlog (чанкинг/Fork).
