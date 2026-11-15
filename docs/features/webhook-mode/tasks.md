# Задачи: webhook + деплой
- [x] config: WEBHOOK_* + валидация (https, secret, listen, path)
- [x] HTTP-сервер: secret, body limit, queue, 503, healthz
- [x] runWebhook: setWebhook, dedup update_id, graceful shutdown
- [x] runLongPolling: deleteWebhook перед polling
- [x] тесты handler/config
- [x] deploy/healthbot.service, deploy/Caddyfile, make build-linux
- [x] docs/operations/deploy-vps.md (1 ГБ, API, embeddings)
- [x] спека/README/отчёт
- [x] PR
