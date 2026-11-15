// Package config загружает настройки healthbot из переменных окружения и
// файла .env (ТЗ §5, §9): токен и chat_id, пути DATA_ROOT/LOG_DIR и параметры
// LLM-провайдеров (FR-6). Пакет не знает про Telegram и файловую БЗ.
package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Режимы активного LLM-провайдера (FR-6).
const (
	ModeOllama = "ollama"
	ModeAPI    = "api"
)

// Config — конфигурация бота. Дефолты совпадают с ТЗ §9.
type Config struct {
	BotToken      string `env:"BOT_TOKEN"`
	AthleteChatID int64  `env:"TELEGRAM_CHAT_ID_ATHLETE" envDefault:"0"`
	WifeChatID    int64  `env:"TELEGRAM_CHAT_ID_WIFE" envDefault:"0"`
	DataRoot      string `env:"DATA_ROOT" envDefault:"../.."`
	LogDir        string `env:"LOG_DIR" envDefault:"logs"`
	SettingsDir   string `env:"SETTINGS_DIR" envDefault:"настройки"`

	// BotProxyURL — HTTP/SOCKS5-прокси для Telegram API (обход блокировок из РФ).
	// Пусто — прямое подключение. Пример: http://127.0.0.1:12334
	BotProxyURL string `env:"BOT_PROXY_URL"`

	LLMMode     string `env:"LLM_MODE" envDefault:"ollama"`
	OllamaURL   string `env:"OLLAMA_URL" envDefault:"http://127.0.0.1:11434"`
	OllamaModel string `env:"OLLAMA_MODEL" envDefault:"qwen2.5:3b"`
	LLMAPIURL   string `env:"LLM_API_URL"`
	LLMAPIKey   string `env:"LLM_API_KEY"`
	LLMModel    string `env:"LLM_MODEL"`
	LLMFallback string `env:"LLM_FALLBACK"`

	// Автоматический воскресный разбор (FR-3.7).
	SundayReviewEnabled bool   `env:"SUNDAY_REVIEW_ENABLED" envDefault:"true"`
	SundayReviewTime    string `env:"SUNDAY_REVIEW_TIME" envDefault:"09:00"`
	SundayReviewTZ      string `env:"SUNDAY_REVIEW_TZ" envDefault:"Europe/Moscow"`

	// Векторный поиск по базе знаний (embeddings). Ollama /api/embed.
	EmbeddingEnabled bool   `env:"EMBEDDING_ENABLED" envDefault:"true"`
	EmbeddingURL     string `env:"EMBEDDING_URL"`
	EmbeddingModel   string `env:"EMBEDDING_MODEL" envDefault:"nomic-embed-text:latest"`

	// Режим получения обновлений: long polling (по умолчанию) или webhook.
	// Webhook требует публичный HTTPS URL и reverse proxy с TLS.
	WebhookEnabled        bool   `env:"WEBHOOK_ENABLED" envDefault:"false"`
	WebhookURL            string `env:"WEBHOOK_URL"`
	WebhookListen         string `env:"WEBHOOK_LISTEN" envDefault:"127.0.0.1:8080"`
	WebhookSecret         string `env:"WEBHOOK_SECRET"`
	WebhookMaxConnections int    `env:"WEBHOOK_MAX_CONNECTIONS" envDefault:"1"`
	WebhookDropPending    bool   `env:"WEBHOOK_DROP_PENDING" envDefault:"false"`
	// Прямой TLS для self-signed сертификата (без Caddy/домена).
	WebhookTLSCert string `env:"WEBHOOK_TLS_CERT"`
	WebhookTLSKey  string `env:"WEBHOOK_TLS_KEY"`

	// Prometheus-метрики (по умолчанию выключены; на VPS включает оператор).
	MetricsEnabled bool   `env:"METRICS_ENABLED" envDefault:"false"`
	MetricsListen  string `env:"METRICS_LISTEN" envDefault:"127.0.0.1:9090"`
	MetricsPath    string `env:"METRICS_PATH" envDefault:"/metrics"`

	// Vision (скриншоты): отдельно от чата (FR-6.8.1). Пусто = как чат.
	LLMVisionMode   string `env:"LLM_VISION_MODE"`
	LLMVisionModel  string `env:"LLM_VISION_MODEL"`
	LLMVisionAPIURL string `env:"LLM_VISION_API_URL"`
	LLMVisionAPIKey string `env:"LLM_VISION_API_KEY"`
}

// Load читает .env (если он есть в текущем каталоге), затем переменные
// окружения, применяет дефолты и проверяет конфигурацию.
//
// Наличие BOT_TOKEN здесь НЕ проверяется: это делает RequireBotToken, чтобы
// режим --dry-run работал без реального токена (CI smoke, ТЗ §12).
func Load() (Config, error) {
	if err := LoadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("разбор переменных окружения: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate проверяет конфигурацию, кроме наличия токена.
func (c Config) Validate() error {
	var errs []error

	switch c.LLMMode {
	case ModeOllama, ModeAPI:
	default:
		errs = append(errs, fmt.Errorf("LLM_MODE=%q: допустимо %q или %q", c.LLMMode, ModeOllama, ModeAPI))
	}

	if c.LLMFallback != "" {
		if c.LLMFallback != ModeOllama && c.LLMFallback != ModeAPI {
			errs = append(errs, fmt.Errorf("LLM_FALLBACK=%q: допустимо %q, %q или пусто", c.LLMFallback, ModeOllama, ModeAPI))
		}
		if c.LLMFallback == c.LLMMode {
			errs = append(errs, fmt.Errorf("LLM_FALLBACK совпадает с активным LLM_MODE=%q: fallback должен отличаться", c.LLMMode))
		}
	}

	switch c.LLMMode {
	case ModeAPI:
		if strings.TrimSpace(c.LLMAPIURL) == "" {
			errs = append(errs, errors.New("LLM_MODE=api: не задан LLM_API_URL (OpenAI-совместимый endpoint)"))
		}
		if strings.TrimSpace(c.LLMModel) == "" {
			errs = append(errs, errors.New("LLM_MODE=api: не задан LLM_MODEL"))
		}
		if strings.TrimSpace(c.LLMAPIKey) == "" {
			errs = append(errs, errors.New("LLM_MODE=api: не задан LLM_API_KEY (ключ хранится только в .env)"))
		}
	case ModeOllama:
		if strings.TrimSpace(c.OllamaURL) == "" {
			errs = append(errs, errors.New("LLM_MODE=ollama: не задан OLLAMA_URL"))
		}
		if strings.TrimSpace(c.OllamaModel) == "" {
			errs = append(errs, errors.New("LLM_MODE=ollama: не задан OLLAMA_MODEL"))
		}
	}

	if strings.TrimSpace(c.DataRoot) == "" {
		errs = append(errs, errors.New("DATA_ROOT пуст: укажи каталог health/ с данными"))
	}

	if c.SundayReviewEnabled {
		if _, err := time.Parse("15:04", c.SundayReviewTime); err != nil {
			errs = append(errs, fmt.Errorf("SUNDAY_REVIEW_TIME=%q: ожидается HH:MM", c.SundayReviewTime))
		}
		if _, err := time.LoadLocation(c.SundayReviewTZ); err != nil {
			errs = append(errs, fmt.Errorf("SUNDAY_REVIEW_TZ=%q: неизвестная зона", c.SundayReviewTZ))
		}
	}

	if _, err := c.TelegramProxy(); err != nil {
		errs = append(errs, err)
	}

	if c.WebhookEnabled {
		parsed, err := url.Parse(strings.TrimSpace(c.WebhookURL))
		if err != nil || parsed.Host == "" {
			errs = append(errs, fmt.Errorf("WEBHOOK_URL=%q: ожидается абсолютный HTTPS URL", c.WebhookURL))
		} else {
			if parsed.Scheme != "https" {
				errs = append(errs, fmt.Errorf("WEBHOOK_URL=%q: Telegram требует https", c.WebhookURL))
			}
			if parsed.RawQuery != "" || parsed.Fragment != "" {
				errs = append(errs, fmt.Errorf("WEBHOOK_URL=%q: не должен содержать query/fragment", c.WebhookURL))
			}
			if parsed.Path == "/healthz" {
				errs = append(errs, errors.New("WEBHOOK_URL: путь /healthz занят health-check-ом"))
			}
		}
		if strings.TrimSpace(c.WebhookListen) == "" {
			errs = append(errs, errors.New("WEBHOOK_ENABLED=true: не задан WEBHOOK_LISTEN"))
		} else if _, _, err := net.SplitHostPort(c.WebhookListen); err != nil {
			errs = append(errs, fmt.Errorf("WEBHOOK_LISTEN=%q: ожидается host:port: %w", c.WebhookListen, err))
		}
		if c.WebhookMaxConnections < 1 || c.WebhookMaxConnections > 100 {
			errs = append(errs, fmt.Errorf("WEBHOOK_MAX_CONNECTIONS=%d: допустимо 1–100", c.WebhookMaxConnections))
		}
		if c.WebhookSecret != "" && !webhookSecretPattern.MatchString(c.WebhookSecret) {
			errs = append(errs, errors.New("WEBHOOK_SECRET: допустимы A-Z, a-z, 0-9, _ и - (1–256 символов)"))
		}
		if tlsRequested := c.WebhookTLSCert != "" || c.WebhookTLSKey != ""; tlsRequested {
			if c.WebhookTLSCert == "" || c.WebhookTLSKey == "" {
				errs = append(errs, errors.New("WEBHOOK_TLS_CERT и WEBHOOK_TLS_KEY должны быть заданы вместе"))
			} else {
				if _, err := os.Stat(c.WebhookTLSCert); err != nil {
					errs = append(errs, fmt.Errorf("WEBHOOK_TLS_CERT=%q: %w", c.WebhookTLSCert, err))
				}
				if _, err := os.Stat(c.WebhookTLSKey); err != nil {
					errs = append(errs, fmt.Errorf("WEBHOOK_TLS_KEY=%q: %w", c.WebhookTLSKey, err))
				}
				if _, err := tls.LoadX509KeyPair(c.WebhookTLSCert, c.WebhookTLSKey); err != nil {
					errs = append(errs, fmt.Errorf("WEBHOOK_TLS_CERT/KEY: %w", err))
				}
				urlPort, urlErr := c.WebhookURLPort()
				listenPort, listenErr := c.WebhookListenPort()
				if urlErr != nil {
					errs = append(errs, urlErr)
				} else if listenErr != nil {
					errs = append(errs, listenErr)
				} else {
					if !allowedWebhookPorts[urlPort] {
						errs = append(errs, fmt.Errorf("WEBHOOK_URL: порт %d не поддерживается Telegram (443, 80, 88, 8443)", urlPort))
					}
					if urlPort != listenPort {
						errs = append(errs, fmt.Errorf("порт WEBHOOK_URL (%d) не совпадает с WEBHOOK_LISTEN (%d)", urlPort, listenPort))
					}
				}
			}
		}
	}

	if c.MetricsEnabled {
		if _, _, err := net.SplitHostPort(c.MetricsListen); err != nil {
			errs = append(errs, fmt.Errorf("METRICS_LISTEN=%q: ожидается host:port: %w", c.MetricsListen, err))
		}
		if !strings.HasPrefix(c.MetricsPath, "/") || c.MetricsPath == "/" {
			errs = append(errs, fmt.Errorf("METRICS_PATH=%q: ожидается путь вида /metrics", c.MetricsPath))
		}
		if c.MetricsPath == "/healthz" {
			errs = append(errs, errors.New("METRICS_PATH=/healthz занят health-check-ом"))
		}
	}

	if c.VisionEnabled() {
		switch c.VisionMode() {
		case ModeOllama, ModeAPI:
		default:
			errs = append(errs, fmt.Errorf("LLM_VISION_MODE=%q: допустимо %q или %q (или пусто = как LLM_MODE)", c.LLMVisionMode, ModeOllama, ModeAPI))
		}
		switch c.VisionMode() {
		case ModeAPI:
			if c.VisionAPIURL() == "" {
				errs = append(errs, errors.New("LLM_VISION_MODE=api: не задан LLM_VISION_API_URL (или LLM_API_URL)"))
			}
			if c.VisionModel() == "" {
				errs = append(errs, errors.New("LLM_VISION_MODE=api: не задана LLM_VISION_MODEL"))
			}
		case ModeOllama:
			if c.VisionModel() == "" {
				errs = append(errs, errors.New("LLM_VISION_MODE=ollama: не задана LLM_VISION_MODEL (или OLLAMA_MODEL)"))
			}
		}
	}

	return errors.Join(errs...)
}

var (
	webhookSecretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
	allowedWebhookPorts  = map[int]bool{443: true, 80: true, 88: true, 8443: true}
)

// WebhookURLPort возвращает порт публичного URL (по умолчанию 443).
func (c Config) WebhookURLPort() (int, error) {
	parsed, err := url.Parse(strings.TrimSpace(c.WebhookURL))
	if err != nil || parsed.Host == "" {
		return 0, fmt.Errorf("WEBHOOK_URL=%q: некорректный URL", c.WebhookURL)
	}
	port := parsed.Port()
	if port == "" {
		return 443, nil
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("WEBHOOK_URL=%q: некорректный порт", c.WebhookURL)
	}
	return value, nil
}

// WebhookListenPort возвращает порт локального listener.
func (c Config) WebhookListenPort() (int, error) {
	_, port, err := net.SplitHostPort(c.WebhookListen)
	if err != nil {
		return 0, fmt.Errorf("WEBHOOK_LISTEN=%q: %w", c.WebhookListen, err)
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("WEBHOOK_LISTEN=%q: некорректный порт", c.WebhookListen)
	}
	return value, nil
}

// WebhookTLSEnabled сообщает, что webhook обслуживается напрямую по HTTPS.
func (c Config) WebhookTLSEnabled() bool {
	return c.WebhookTLSCert != "" && c.WebhookTLSKey != ""
}

// WebhookPath возвращает путь, который слушает локальный HTTP-сервер webhook.
func (c Config) WebhookPath() string {
	parsed, err := url.Parse(strings.TrimSpace(c.WebhookURL))
	if err != nil || parsed.Path == "" {
		return "/"
	}
	return parsed.Path
}

// TelegramProxy возвращает URL прокси для Telegram API или nil, если прокси не
// задан (прямое подключение). Допустимые схемы: http, https, socks5.
func (c Config) TelegramProxy() (*url.URL, error) {
	raw := strings.TrimSpace(c.BotProxyURL)
	if raw == "" {
		return nil, nil
	}

	proxyURL, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("BOT_PROXY_URL=%q: некорректный URL прокси: %w", raw, err)
	}
	switch proxyURL.Scheme {
	case "http", "https", "socks5":
	default:
		return nil, fmt.Errorf("BOT_PROXY_URL=%q: схема %q не поддерживается (нужно http, https или socks5)", raw, proxyURL.Scheme)
	}
	if proxyURL.Host == "" {
		return nil, fmt.Errorf("BOT_PROXY_URL=%q: не указан хост:порт прокси", raw)
	}
	return proxyURL, nil
}

// VisionEnabled сообщает, настроен ли приём скриншотов (FR-6.8.1).
func (c Config) VisionEnabled() bool {
	return strings.TrimSpace(c.LLMVisionMode) != "" ||
		strings.TrimSpace(c.LLMVisionModel) != "" ||
		strings.TrimSpace(c.LLMVisionAPIURL) != ""
}

// VisionMode возвращает режим vision-провайдера (по умолчанию — как чат).
func (c Config) VisionMode() string {
	if mode := strings.TrimSpace(c.LLMVisionMode); mode != "" {
		return mode
	}
	return c.LLMMode
}

// VisionModel возвращает модель для скриншотов (по умолчанию — как чат-модель).
func (c Config) VisionModel() string {
	if model := strings.TrimSpace(c.LLMVisionModel); model != "" {
		return model
	}
	if c.VisionMode() == ModeAPI {
		return c.LLMModel
	}
	return c.OllamaModel
}

// VisionAPIURL возвращает URL vision-API (по умолчанию — общий LLM_API_URL).
func (c Config) VisionAPIURL() string {
	if url := strings.TrimSpace(c.LLMVisionAPIURL); url != "" {
		return url
	}
	return c.LLMAPIURL
}

// VisionAPIKey возвращает ключ vision-API (по умолчанию — общий LLM_API_KEY).
func (c Config) VisionAPIKey() string {
	if key := strings.TrimSpace(c.LLMVisionAPIKey); key != "" {
		return key
	}
	return c.LLMAPIKey
}

// RequireBotToken проверяет, что BOT_TOKEN задан. Токен берётся только из
// окружения/.env и никогда не выводится в чат или логи (NFR-1).
func (c Config) RequireBotToken() error {
	if strings.TrimSpace(c.BotToken) == "" {
		return errors.New("BOT_TOKEN не задан: скопируй .env.example в .env и укажи токен из BotFather (токен задаётся только там)")
	}
	return nil
}

// LogValue реализует slog.LogValuer: делает конфигурацию безопасной для
// логирования — токен и ключ API не попадают в логи (NFR-1). Прокси пишется
// без userinfo (логин/пароль прокси, если есть, не утекают).
func (c Config) LogValue() slog.Value {
	proxy := ""
	if u, err := c.TelegramProxy(); err == nil && u != nil {
		proxy = u.Scheme + "://" + u.Host
	}
	return slog.GroupValue(
		slog.String("llm_mode", c.LLMMode),
		slog.String("llm_fallback", c.LLMFallback),
		slog.String("ollama_url", c.OllamaURL),
		slog.String("ollama_model", c.OllamaModel),
		slog.String("llm_api_url", c.LLMAPIURL),
		slog.String("llm_model", c.LLMModel),
		slog.Bool("vision_enabled", c.VisionEnabled()),
		slog.String("vision_mode", c.VisionMode()),
		slog.String("vision_model", c.VisionModel()),
		slog.String("data_root", c.DataRoot),
		slog.String("log_dir", c.LogDir),
		slog.String("bot_proxy", proxy),
		slog.Bool("webhook_enabled", c.WebhookEnabled),
		slog.String("webhook_url", c.WebhookURL),
		slog.String("webhook_listen", c.WebhookListen),
		slog.Bool("webhook_tls", c.WebhookTLSEnabled()),
		slog.Bool("metrics_enabled", c.MetricsEnabled),
		slog.String("metrics_listen", c.MetricsListen),
		slog.Int64("athlete_chat_id", c.AthleteChatID),
		slog.Int64("wife_chat_id", c.WifeChatID),
		slog.Bool("bot_token_set", c.BotToken != ""),
		slog.Bool("llm_api_key_set", c.LLMAPIKey != ""),
	)
}
