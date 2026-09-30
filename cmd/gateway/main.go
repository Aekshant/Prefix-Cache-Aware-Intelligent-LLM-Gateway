// Command gateway runs the prefix-cache-aware LLM gateway.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"llm-gateway/internal/config"
	"llm-gateway/internal/gateway"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gw := gateway.New(cfg)
	go gw.RunJanitor(ctx, cfg.JanitorInterval)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", gw.HandleChat)
	mux.HandleFunc("/healthz", gateway.HandleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("gateway listening",
			"addr", cfg.ListenAddr,
			"workers", len(cfg.Workers),
			"cache_ttl", cfg.CacheTTL,
			"max_active_conns", cfg.MaxActiveConns,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}
}
