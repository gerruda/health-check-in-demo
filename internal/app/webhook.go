package app

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/mymmrac/telego"

	"healthbot/internal/config"
	"healthbot/internal/logging"
	"healthbot/internal/metrics"
	"healthbot/internal/router"
)

const (
	webhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token" // #nosec G101 -- имя HTTP-заголовка, не секрет
	webhookMaxBodyBytes = 1 << 20
	webhookQueueSize    = 128
)

// runWebhook поднимает локальный HTTP-сервер, регистрирует webhook в Telegram
// и обрабатывает обновления последовательно, как long polling.
func runWebhook(ctx context.Context, cfg config.Config, bot *telego.Bot, handler *router.Router, logger *slog.Logger) error {
	updates := make(chan telego.Update, webhookQueueSize)
	mux := newWebhookHTTPHandler(cfg.WebhookPath(), cfg.WebhookSecret, updates, logger)

	listener, err := net.Listen("tcp", cfg.WebhookListen)
	if err != nil {
		return fmt.Errorf("webhook: слушать %q: %w", cfg.WebhookListen, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	tlsEnabled := cfg.WebhookTLSEnabled()
	if tlsEnabled {
		if _, loadErr := tls.LoadX509KeyPair(cfg.WebhookTLSCert, cfg.WebhookTLSKey); loadErr != nil {
			_ = listener.Close()
			return fmt.Errorf("webhook: TLS cert/key: %w", loadErr)
		}
	}

	serveErr := make(chan error, 1)
	go func() {
		var serveErrValue error
		if tlsEnabled {
			serveErrValue = server.ServeTLS(listener, cfg.WebhookTLSCert, cfg.WebhookTLSKey)
		} else {
			serveErrValue = server.Serve(listener)
		}
		if serveErrValue != nil && !errors.Is(serveErrValue, http.ErrServerClosed) {
			serveErr <- serveErrValue
		}
	}()

	setCtx, cancelSet := context.WithTimeout(ctx, 20*time.Second)
	err = configureWebhook(setCtx, bot, cfg)
	cancelSet()
	if err != nil {
		_ = server.Close()
		return fmt.Errorf("webhook: setWebhook: %w", err)
	}

	logger.Info(logging.Msg("app:runWebhook", "webhook запущен"),
		"url", cfg.WebhookURL,
		"listen", cfg.WebhookListen,
		"path", cfg.WebhookPath(),
		"max_connections", cfg.WebhookMaxConnections,
		"tls", tlsEnabled,
		"secret_set", cfg.WebhookSecret != "")
	if cfg.WebhookSecret == "" {
		logger.Warn(logging.Msg("app:runWebhook", "WEBHOOK_SECRET пуст — рекомендуй задать секрет"))
	}

	seen := make(map[int]time.Time)
	cleanupSeen := func(now time.Time) {
		if len(seen) < 1000 {
			return
		}
		for updateID, at := range seen {
			if now.Sub(at) > 10*time.Minute {
				delete(seen, updateID)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = server.Shutdown(shutdownCtx)
			cancel()
			logger.Info(logging.Msg("app:runWebhook", "webhook остановлен, бот завершает работу"))
			return nil
		case serveErrValue := <-serveErr:
			return fmt.Errorf("webhook: сервер: %w", serveErrValue)
		case update := <-updates:
			metrics.Default().SetWebhookQueue(len(updates))
			now := time.Now()
			cleanupSeen(now)
			if at, ok := seen[update.UpdateID]; ok && now.Sub(at) < 10*time.Minute {
				logger.Warn(logging.Msg("app:runWebhook", "повторный update пропущен"), "update_id", update.UpdateID)
				continue
			}
			seen[update.UpdateID] = now
			handleSafely(ctx, handler, logger, update)
		}
	}
}

// configureWebhook регистрирует webhook; для self-signed TLS сертификат
// загружается в Telegram параметром certificate.
func configureWebhook(ctx context.Context, bot *telego.Bot, cfg config.Config) error {
	params := &telego.SetWebhookParams{
		URL:                cfg.WebhookURL,
		SecretToken:        cfg.WebhookSecret,
		MaxConnections:     cfg.WebhookMaxConnections,
		AllowedUpdates:     []string{"message"},
		DropPendingUpdates: cfg.WebhookDropPending,
	}
	if cfg.WebhookTLSEnabled() {
		certFile, err := os.Open(cfg.WebhookTLSCert)
		if err != nil {
			return fmt.Errorf("open certificate: %w", err)
		}
		defer func() { _ = certFile.Close() }()
		params.Certificate = &telego.InputFile{File: certFile}
	}
	return bot.SetWebhook(ctx, params)
}

// newWebhookHTTPHandler создаёт HTTP-обработчик: проверка секрета, лимит тела,
// декодирование update и постановка в очередь. Telegram получает 200 только
// после успешной постановки — долгий LLM не блокирует HTTP-ответ.
func newWebhookHTTPHandler(path, secret string, updates chan<- telego.Update, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			metrics.Default().ObserveWebhookRequest("405")
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if secret != "" {
			got := request.Header.Get(webhookSecretHeader)
			if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				metrics.Default().ObserveWebhookRequest("403")
				http.Error(writer, "forbidden", http.StatusForbidden)
				return
			}
		}
		body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, webhookMaxBodyBytes))
		if err != nil {
			metrics.Default().ObserveWebhookRequest("400")
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		var update telego.Update
		if err = json.Unmarshal(body, &update); err != nil || update.UpdateID <= 0 {
			logger.Warn(logging.Msg("app:webhook", "некорректный update"), "error", err)
			metrics.Default().ObserveWebhookRequest("400")
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		select {
		case updates <- update:
			metrics.Default().SetWebhookQueue(len(updates))
			metrics.Default().ObserveWebhookRequest("200")
			writer.WriteHeader(http.StatusOK)
		default:
			logger.Warn(logging.Msg("app:webhook", "очередь webhook переполнена"), "update_id", update.UpdateID)
			metrics.Default().ObserveWebhookRequest("503")
			http.Error(writer, "busy", http.StatusServiceUnavailable)
		}
	})
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok"))
	})
	return mux
}
