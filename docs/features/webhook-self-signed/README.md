# Фича: self-signed webhook по IP (без домена)

**Ветка:** `feat/webhook-self-signed` · **Дата:** 2026-11-14
**Связано:** `docs/features/webhook-mode/`, `docs/operations/deploy-vps.md`.

## Проблема

VPS имеет только IPv4, домена нет. Telegram webhook требует HTTPS с
сертификатом; публичные CA сертификаты на голый IP не выдают. Telegram
принимает self-signed сертификат, если загрузить его через `setWebhook`.

## Решение

- Прямой HTTPS-сервер бота на `0.0.0.0:8443` (Telegram разрешает только
  443/80/88/8443; 8844 не подходит).
- Self-signed сертификат с SAN `IP:<адрес>`.
- `setWebhook` с multipart-полем `certificate` (публичный сертификат).
- Приватный ключ остаётся на VPS (`600`), в Telegram не уходит.
- Команда генерации в боте:
  ```bash
  healthbot -gen-webhook-cert \
    -webhook-cert-out ... -webhook-key-out ... \
    -webhook-host 203.0.113.10 -webhook-cert-days 3650
  ```
- Caddy/домен не нужны; polling остаётся режимом по умолчанию.

## Конфиг

```dotenv
WEBHOOK_ENABLED=true
WEBHOOK_URL=https://203.0.113.10:8443/telegram/webhook
WEBHOOK_LISTEN=0.0.0.0:8443
WEBHOOK_SECRET=...
WEBHOOK_TLS_CERT=/etc/healthbot/webhook-cert.pem
WEBHOOK_TLS_KEY=/etc/healthbot/webhook-key.pem
```

Валидация:
- cert и key только вместе;
- файлы существуют и читаются, `tls.LoadX509KeyPair` проходит;
- URL https, порт из 443/80/88/8443;
- порт URL совпадает с `WEBHOOK_LISTEN`.

## Acceptance

| # | Критерий | Тест |
|---|---|---|
| AC1 | Сертификат с SAN IP, key 0600 | `TestGenerateIPCertificate` |
| AC2 | `-force` защищает от перезаписи | `TestGenerateRefusesOverwriteWithoutForce` |
| AC3 | `setWebhook` отправляет `certificate` multipart | `TestConfigureWebhookUploadsSelfSignedCertificate` |
| AC4 | Конфиг: cert/key вместе, порты совпадают, 8844 отклоняется | `TestWebhookTLSConfig` |
| AC5 | HTTPS `/healthz` и webhook работают без домена | ручная проверка `curl -k` + `getWebhookInfo` |
| AC6 | Polling/Caddy-режим не сломаны | существующие webhook-тесты |

## Риски

- Сертификат привязан к IP; при смене IP перегенерировать.
- Порт 8443 открыт в интернет; защита — `WEBHOOK_SECRET` и firewall.
- Браузер будет ругаться на self-signed; это ожидаемо, Telegram — нет.
- Ошибки `setWebhook` (сертификат/порт) логируются.
