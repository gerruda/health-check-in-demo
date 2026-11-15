# Фича: Prometheus-метрики и Grafana

**Ветка:** `feat/prometheus-metrics` · **Дата:** 2026-11-14

## Требования

- Метрики **выключены по умолчанию**: `METRICS_ENABLED=false`.
- При включении — отдельный listener `METRICS_LISTEN` (обычно
  `127.0.0.1:9090`), путь `METRICS_PATH=/metrics`.
- Не публиковать метрики в интернет; Prometheus/Grafana — на VPS или через
  SSH-туннель.
- Собираем: обновления, длительность, очередь webhook, HTTP-статусы webhook,
  LLM-запросы/латентность/токены, fallback, ошибки, сохранения, Go/process.
- Grafana dashboard — `deploy/grafana-dashboard.json`.

## Конфиг

```dotenv
METRICS_ENABLED=true
METRICS_LISTEN=127.0.0.1:9090
METRICS_PATH=/metrics
```

## Метрики

- `healthbot_updates_total{command,status}`
- `healthbot_update_duration_seconds{command}`
- `healthbot_webhook_queue_length`
- `healthbot_webhook_requests_total{status}`
- `healthbot_last_update_timestamp_seconds`
- `healthbot_llm_requests_total{provider,model,kind,status}`
- `healthbot_llm_request_duration_seconds{provider,model,kind}`
- `healthbot_llm_tokens_total{provider,model,direction}`
- `healthbot_llm_fallback_total{from,to}`
- `healthbot_errors_total{component}`
- `healthbot_saves_total{kind,status}`
- стандартные `go_*`, `process_*`

Токены сейчас учитываются для chat-вызовов (OpenAI `usage`, Ollama
`prompt_eval_count`/`eval_count`); для Extract/Vision — отдельный backlog.

## Acceptance

- `TestMetricsExposition` — имена/лейблы в `/metrics`;
- `TestMetricsConfig` — включение/валидация;
- ручная: `curl -s 127.0.0.1:9090/metrics | grep healthbot_`;
- Grafana dashboard импортируется, панели не пустые.

## Ограничения 1 ГБ

Bot + Prometheus + Grafana в 1 ГБ — тесно. В `docker-compose.monitoring.yml`
заданы лимиты (180M/220M), retention 7d; при OOM вынести мониторинг на другой
хост или добавить swap.
