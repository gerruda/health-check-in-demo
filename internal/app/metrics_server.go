package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"healthbot/internal/config"
	"healthbot/internal/logging"
	"healthbot/internal/metrics"
)

// startMetricsServer поднимает /metrics на отдельном listener (обычно localhost).
func startMetricsServer(ctx context.Context, cfg config.Config, logger *slog.Logger) (func(), error) {
	if !cfg.MetricsEnabled {
		return func() {}, nil
	}
	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, metrics.Handler())
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok"))
	})
	listener, err := net.Listen("tcp", cfg.MetricsListen)
	if err != nil {
		return nil, fmt.Errorf("metrics: слушать %q: %w", cfg.MetricsListen, err)
	}
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error(logging.Msg("app:metrics", "metrics-сервер остановился"), "error", serveErr)
		}
	}()
	logger.Info(logging.Msg("app:metrics", "Prometheus /metrics запущен"),
		"listen", cfg.MetricsListen, "path", cfg.MetricsPath)

	stop := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}
	_ = ctx
	return stop, nil
}
