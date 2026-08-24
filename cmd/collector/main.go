package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/carlosz/kaizenops/internal/config"
	kaizengithub "github.com/carlosz/kaizenops/internal/github"
	"github.com/carlosz/kaizenops/internal/ingest"
	"github.com/carlosz/kaizenops/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("collector exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("opening storage: %w", err)
	}
	defer store.Close()

	if err := storage.Migrate(ctx, store.Pool(), "migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	pool := ingest.NewPool(ctx, 8, 256, func(err error) {
		slog.Error("processing pipeline event", "err", err)
	})

	handler := &kaizengithub.Handler{
		Secret: cfg.GitHubWebhookSecret,
		Salt:   cfg.PseudonymSalt,
		Sink:   store,
		Enqueue: func(r *http.Request, task ingest.Task) error {
			return pool.Submit(r.Context(), task)
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/webhooks/github", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("collector listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down collector")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("running HTTP server: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down HTTP server: %w", err)
	}

	pool.Shutdown()

	return nil
}
