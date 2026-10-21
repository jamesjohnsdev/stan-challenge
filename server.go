package main

import (
	"context"
	"errors"
	"fmt"
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
		fmt.Printf("listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			return err
		}
		return nil
	case <-ctx.Done():
	}

	fmt.Fprintln(os.Stderr, "shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), drain)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "graceful shutdown failed: %v, forcing close\n", err)
		if cerr := srv.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "force close failed: %v\n", cerr)
			return cerr
		}
		return err
	}

	fmt.Fprintln(os.Stderr, "server stopped gracefully")
	return nil
}
