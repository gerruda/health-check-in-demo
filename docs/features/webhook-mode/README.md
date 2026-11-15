# Фича: webhook-режим (с переключателем)

**Ветка:** `feat/vps-webhook` · **Дата:** 2026-11-14

## Требования

- По умолчанию — long polling (локальная отладка, VPS без домена).
- `WEBHOOK_ENABLED=true` включает HTTP-сервер, `setWebhook` в Telegram и
  обработку обновлений через webhook.
- Webhook не должен блокировать HTTP-ответ на время LLM: update кладётся в
  очередь, Telegram получает 200 сразу; очередь ограничена, при переполнении —
  503 (Telegram повторит).
- Секрет `WEBHOOK_SECRET` проверяется по заголовку
  `X-Telegram-Bot-Api-Secret-Token`; тело ограничено 1 МиБ.
- Повторный `update_id` в течение 10 минут пропускается.
- При переходе на polling бот удаляет webhook (`deleteWebhook`).
- `/healthz` — для Caddy/systemd-проверки.

## Конфиг

`WEBHOOK_ENABLED`, `WEBHOOK_URL` (https), `WEBHOOK_LISTEN`, `WEBHOOK_SECRET`,
`WEBHOOK_MAX_CONNECTIONS` (1–100), `WEBHOOK_DROP_PENDING`.

## Acceptance

| # | Критерий | Тест |
|---|---|---|
| AC1 | Валидный update с секретом → 200 и очередь | `TestWebhookHandlerAcceptsValidUpdate` |
| AC2 | Неверный секрет → 403 | `TestWebhookHandlerRejectsWrongSecret` |
| AC3 | Битая JSON/GET → 400/405 | `TestWebhookHandlerRejectsBadJSONAndMethod` |
| AC4 | Полная очередь → 503 | `TestWebhookHandlerReturnsBusyWhenQueueFull` |
| AC5 | `/healthz` → 200 ok | `TestWebhookHealthz` |
| AC6 | Конфиг: https обязателен, секрет валиден | `TestWebhookConfig` |
| AC7 | Polling удаляет webhook перед getUpdates | код `runLongPolling` + deploy-док |

## Границы

TLS терминируется Caddy/nginx; бот слушает localhost. Webhook не заменяет
long polling для локального запуска без домена.
