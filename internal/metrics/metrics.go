// Package metrics — Prometheus-метрики healthbot.
package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "healthbot"

// TokenUsage — расход токенов одного LLM-вызова.
type TokenUsage struct {
	Prompt     int
	Completion int
	Total      int
}

// Metrics — набор коллекторов и реестр.
type Metrics struct {
	registry *prometheus.Registry

	updatesTotal    *prometheus.CounterVec
	updateDuration  *prometheus.HistogramVec
	webhookQueue    prometheus.Gauge
	webhookRequests *prometheus.CounterVec
	llmRequests     *prometheus.CounterVec
	llmDuration     *prometheus.HistogramVec
	llmTokens       *prometheus.CounterVec
	llmFallback     *prometheus.CounterVec
	savesTotal      *prometheus.CounterVec
	errorsTotal     *prometheus.CounterVec
	lastUpdate      prometheus.Gauge
}

// New создаёт изолированный набор метрик (удобно для тестов).
func New() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry: registry,
		updatesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "updates_total", Help: "Обработанные Telegram-обновления.",
		}, []string{"command", "status"}),
		updateDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "update_duration_seconds", Help: "Длительность обработки обновления.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
		}, []string{"command"}),
		webhookQueue: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "webhook_queue_length", Help: "Обновления в очереди webhook.",
		}),
		webhookRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "webhook_requests_total", Help: "HTTP-запросы webhook.",
		}, []string{"status"}),
		llmRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "llm_requests_total", Help: "Вызовы LLM.",
		}, []string{"provider", "model", "kind", "status"}),
		llmDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "llm_request_duration_seconds", Help: "Длительность вызова LLM.",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 45, 60, 120},
		}, []string{"provider", "model", "kind"}),
		llmTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "llm_tokens_total", Help: "Токены LLM по направлению.",
		}, []string{"provider", "model", "direction"}),
		llmFallback: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "llm_fallback_total", Help: "Переключения на резервный LLM.",
		}, []string{"from", "to"}),
		savesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "saves_total", Help: "Сохранения данных.",
		}, []string{"kind", "status"}),
		errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "errors_total", Help: "Ошибки по компонентам.",
		}, []string{"component"}),
		lastUpdate: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "last_update_timestamp_seconds", Help: "Unix-время последнего update.",
		}),
	}
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.updatesTotal, m.updateDuration, m.webhookQueue, m.webhookRequests,
		m.llmRequests, m.llmDuration, m.llmTokens, m.llmFallback,
		m.savesTotal, m.errorsTotal, m.lastUpdate,
	)
	return m
}

var (
	defaultOnce    sync.Once
	defaultMetrics *Metrics
)

// Default возвращает процесс-синглтон.
func Default() *Metrics {
	defaultOnce.Do(func() { defaultMetrics = New() })
	return defaultMetrics
}

// Handler отдаёт /metrics этого набора.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Handler отдаёт /metrics процесса.
func Handler() http.Handler { return Default().Handler() }

// ObserveUpdate учитывает обновление и его длительность.
func (m *Metrics) ObserveUpdate(command, status string, duration time.Duration) {
	m.updatesTotal.WithLabelValues(command, status).Inc()
	m.updateDuration.WithLabelValues(command).Observe(duration.Seconds())
}

// ObserveWebhookRequest учитывает HTTP-запрос Telegram.
func (m *Metrics) ObserveWebhookRequest(status string) {
	m.webhookRequests.WithLabelValues(status).Inc()
}

// SetWebhookQueue выставляет текущую длину очереди.
func (m *Metrics) SetWebhookQueue(length int) { m.webhookQueue.Set(float64(length)) }

// ObserveLLM учитывает вызов модели.
func (m *Metrics) ObserveLLM(provider, model, kind, status string, duration time.Duration, usage *TokenUsage) {
	m.llmRequests.WithLabelValues(provider, model, kind, status).Inc()
	m.llmDuration.WithLabelValues(provider, model, kind).Observe(duration.Seconds())
	if usage == nil {
		return
	}
	m.llmTokens.WithLabelValues(provider, model, "prompt").Add(float64(usage.Prompt))
	m.llmTokens.WithLabelValues(provider, model, "completion").Add(float64(usage.Completion))
	m.llmTokens.WithLabelValues(provider, model, "total").Add(float64(usage.Total))
}

// ObserveFallback фиксирует переключение провайдера.
func (m *Metrics) ObserveFallback(from, to string) { m.llmFallback.WithLabelValues(from, to).Inc() }

// ObserveSave фиксирует сохранение данных.
func (m *Metrics) ObserveSave(kind, status string) { m.savesTotal.WithLabelValues(kind, status).Inc() }

// IncError увеличивает счётчик ошибок компонента.
func (m *Metrics) IncError(component string) { m.errorsTotal.WithLabelValues(component).Inc() }

// SetLastUpdate обновляет время последнего обработанного update.
func (m *Metrics) SetLastUpdate(at time.Time) { m.lastUpdate.Set(float64(at.Unix())) }
