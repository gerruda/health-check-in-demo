# Деплой на VPS (1 ГБ RAM / 1 vCPU)

**Сценарий:** бот на VPS, чат и vision — через API (`LLM_MODE=api`), Ollama на
VPS **не поднимаем**. Embeddings либо выключены (лексический поиск), либо
вынесены на домашнюю Ollama через VPN.

> 1 ГБ/1 vCPU хватает для Go-бота + Caddy. Ollama с `qwen2.5:3b`/vision требует
> ~3–4 ГБ RAM и 2+ vCPU — на этой машине будет OOM/таймауты.

## 0. Что понадобится

- VPS Debian/Ubuntu, SSH-ключ, `sudo`.
- Домен (для webhook + TLS), DNS A → IP VPS. Для long polling домен не нужен.
- API-ключ OpenAI-совместимого провайдера (чат + при наличии vision).
- Данные проекта: `данные_пользователя/`, `данные_жены/`, `база_знаний/`,
  `настройки/` (лучше чистый бэкап, без тестовых чек-инов).
- Остановленный локальный экземпляр бота (иначе Telegram 409).

## 1. Сборка

Локально в репозитории:

```bash
make build-linux                 # bin/healthbot-linux-amd64
# для ARM VPS: make build-linux GOARCH=arm64
file bin/healthbot-linux-amd64   # ELF, statically linked
```

## 2. Пользователь и каталоги на VPS

```bash
ssh user@VPS
sudo adduser --system --group --home /opt/healthbot healthbot
sudo mkdir -p /opt/healthbot/{bin,data,logs}
sudo chown -R healthbot:healthbot /opt/healthbot
```

Перенос с локальной машины:

```bash
scp bin/healthbot-linux-amd64 user@VPS:/tmp/healthbot
ssh user@VPS 'sudo install -m 755 /tmp/healthbot /opt/healthbot/bin/healthbot'

rsync -av данные_пользователя данные_жены база_знаний настройки \
  user@VPS:/tmp/health-data/
ssh user@VPS 'sudo mv /tmp/health-data/* /opt/healthbot/data/ && sudo chown -R healthbot:healthbot /opt/healthbot/data'
```

## 3. `.env` на сервере

```bash
sudo nano /opt/healthbot/.env
sudo chmod 600 /opt/healthbot/.env
sudo chown healthbot:healthbot /opt/healthbot/.env
```

Минимально:

```dotenv
BOT_TOKEN=...
TELEGRAM_CHAT_ID_ATHLETE=...
TELEGRAM_CHAT_ID_WIFE=...
DATA_ROOT=/opt/healthbot/data
LOG_DIR=/opt/healthbot/logs
SETTINGS_DIR=/opt/healthbot/data/настройки
SUNDAY_REVIEW_TZ=Europe/Moscow

# API для чата/извлечения
LLM_MODE=api
LLM_API_URL=https://api.timeweb.ai/v1
LLM_API_KEY=...
LLM_MODEL=<текстовая модель>
LLM_FALLBACK=

# Vision: если модель умеет картинки; иначе оставить пустым = vision выключен
LLM_VISION_MODE=api
LLM_VISION_MODEL=<vision-модель>
# LLM_VISION_API_URL/KEY пусты -> наследуются из LLM_API_URL/KEY

# 1 ГБ: embeddings на VPS не запускаем
EMBEDDING_ENABLED=false
```

Если есть домашняя Ollama через WireGuard/Tailscale — можно включить только
embeddings (чат/vision остаются API):

```dotenv
EMBEDDING_ENABLED=true
EMBEDDING_URL=http://10.8.0.1:11434
EMBEDDING_MODEL=nomic-embed-text:latest
```

Ollama дома должна слушать VPN-интерфейс, не публичный `0.0.0.0`. Проверка:

```bash
curl http://10.8.0.1:11434/api/tags
```

## 4. Режим получения обновлений: polling, webhook по IP или webhook за Caddy

**Long polling (по умолчанию)** — проще, не требует домена/входящих портов:

```dotenv
WEBHOOK_ENABLED=false
```

### Вариант self-signed по IP (без домена, рекомендуемый для этого VPS)

Telegram принимает self-signed сертификат, если загрузить его через `setWebhook`.
Сертификат генерирует сам бот:

```bash
cd /root/health-check-in
./bin/healthbot-linux-amd64 -gen-webhook-cert \
  -webhook-cert-out /etc/healthbot/webhook-cert.pem \
  -webhook-key-out /etc/healthbot/webhook-key.pem \
  -webhook-host 203.0.113.10 \
  -webhook-cert-days 3650
```

Права: `key` — `600`, `cert` — `644`. Порт — **8443** (Telegram разрешает
только 443, 80, 88, 8443; произвольный 8844 не подойдёт).

```dotenv
WEBHOOK_ENABLED=true
WEBHOOK_URL=https://203.0.113.10:8443/telegram/webhook
WEBHOOK_LISTEN=0.0.0.0:8443
WEBHOOK_SECRET=...
WEBHOOK_TLS_CERT=/etc/healthbot/webhook-cert.pem
WEBHOOK_TLS_KEY=/etc/healthbot/webhook-key.pem
```

Открыть `8443/tcp` в firewall Timeweb. Caddy не нужен. Проверка:

```bash
curl -k https://203.0.113.10:8443/healthz   # ok
```

### Вариант webhook за Caddy/nginx (если появится домен)

```dotenv
WEBHOOK_ENABLED=true
WEBHOOK_URL=https://bot.example.com/telegram/webhook
WEBHOOK_LISTEN=127.0.0.1:8080
WEBHOOK_SECRET=<openssl rand -hex 24>
WEBHOOK_MAX_CONNECTIONS=1
WEBHOOK_DROP_PENDING=false
```

- `WEBHOOK_URL` — публичный HTTPS-адрес; путь из него слушает бот.
- `WEBHOOK_LISTEN` — локальный адрес; наружу не открывать.
- Секрет проверяется по заголовку `X-Telegram-Bot-Api-Secret-Token`.
- При переключении на polling бот сам вызывает `deleteWebhook`.

## 5. systemd

С локальной машины перенеси unit-файл:

```bash
scp deploy/healthbot.service user@VPS:/tmp/
```

На VPS:

```bash
sudo cp /tmp/healthbot.service /etc/systemd/system/healthbot.service
sudo systemctl daemon-reload
sudo systemctl enable --now healthbot
systemctl status healthbot
journalctl -u healthbot -f
```

Проверка конфига до старта:

```bash
cd /opt/healthbot
sudo -u healthbot env $(grep -v '^#' .env | xargs) /opt/healthbot/bin/healthbot -dry-run
```

## 6. Caddy (только если используешь домен/Caddy)

```bash
scp deploy/Caddyfile user@VPS:/tmp/    # с локальной машины
# на VPS:
sudo apt install -y caddy               # или официальный репозиторий Caddy
sudo cp /tmp/Caddyfile /etc/caddy/Caddyfile
sudo nano /etc/caddy/Caddyfile     # заменить bot.example.com
sudo systemctl reload caddy
curl https://bot.example.com/healthz   # ok
```

Caddy сам получит TLS-сертификат; DNS A должен указывать на VPS. Firewall:
`22`, `80`, `443`. Порт `8080` — только localhost.

Для nginx — тот же принцип: TLS termination + `proxy_pass http://127.0.0.1:8080`.

## 7. Проверка

```bash
journalctl -u healthbot -n 50 --no-pager
# ожидаемо:
#   webhook запущен (или long polling запущен)
#   бот запущен на аккаунте @...
curl -fsS https://bot.example.com/healthz
```

В Telegram: `/status`, `/week`, тренировка, скриншот, `/ask`. Проверить, что
второй экземпляр не запущен (иначе 409 / потеря сообщений).

Сменить webhook на polling: `WEBHOOK_ENABLED=false` → `systemctl restart healthbot`.
Обратно: заполнить webhook-переменные → restart.

## 7. Метрики Prometheus/Grafana (опционально)

В `.env` бота:

```dotenv
METRICS_ENABLED=true
METRICS_LISTEN=127.0.0.1:9090
METRICS_PATH=/metrics
```

Проверка на VPS:

```bash
curl -s http://127.0.0.1:9090/metrics | grep '^healthbot_' | head
```

Метрики отключены по умолчанию (`METRICS_ENABLED=false`) — фичу можно
выключить без изменения кода.

Prometheus + Grafana на VPS:

```bash
# локально
scp deploy/prometheus.yml deploy/docker-compose.monitoring.yml \
    deploy/grafana-dashboard.json user@VPS:/opt/healthbot/monitoring/
# на VPS
cd /opt/healthbot/monitoring
docker compose -f docker-compose.monitoring.yml up -d
```

Grafana: `http://127.0.0.1:3000` (не наружу). Доступ через SSH-туннель:

```bash
ssh -L 3000:127.0.0.1:3000 root@203.0.113.10
# затем http://127.0.0.1:3000
```

Добавь datasource `Prometheus` → `http://127.0.0.1:9090`, импортируй
`grafana-dashboard.json`.

**1 ГБ RAM:** bot + Prometheus + Grafana — тесно. В compose заданы лимиты
(180M/220M), retention 7d. Если начнётся OOM — вынеси мониторинг на другой хост
или добавь swap. Мониторинг не должен ронять бота.

## 8. Бэкап и эксплуатация

- Бэкап — каталог `/opt/healthbot/data` (Markdown-канон; `.healthbot` можно
  пересобрать). Пример cron:
  ```cron
  0 4 * * * rsync -a /opt/healthbot/data/ backup@home:/backup/healthbot/
  ```
- Логи: `journalctl -u healthbot` или `LOG_DIR` + logrotate; следить за диском.
- `MemoryMax=300M` достаточно для API-режима; при OOM поднять до `512M`.
- Обновление: собрать новый бинарь, `install`, `systemctl restart`; данные не
  трогать.
- Секреты — только в `.env` (`600`), не в git и не в чате.
- Один бот = один процесс на токен; локальный экземпляр остановить.
