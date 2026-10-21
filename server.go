package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func drainTimeoutFromEnv() time.Duration {
	d := 30 * time.Second
	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			d = parsed
		}
	}
	if v := os.Getenv("DRAIN_TIMEOUT"); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			d = parsed
		}
	}
	return d
}

func serveAndShutdown(ctx context.Context, srv *http.Server, addr string, drain time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "error", err)
			return err
		}
		return nil
	case <-ctx.Done():
	}

	slog.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), drain)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed, forcing close", "error", err)
		if cerr := srv.Close(); cerr != nil {
			slog.Error("force close failed", "error", cerr)
			return cerr
		}
		return err
	}

	slog.Info("server stopped gracefully")
	return nil
}
