package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jamesjohnsdev/stan-challenge/internal/store"
)

func main() {
	mux := registerRoutes(time.Now(), store.New())

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Printf("listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	fmt.Fprintln(os.Stderr, "shutdown signal received")

	drainTimeout := 30 * time.Second
	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			drainTimeout = d
		}
	}
	if v := os.Getenv("DRAIN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			drainTimeout = d
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "graceful shutdown failed: %v, forcing close\n", err)
		if err := srv.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "force close failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "server stopped gracefully")
}

type serviceStore interface {
	ingestStore
	manifestStore
}

func registerRoutes(startTime time.Time, st serviceStore) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /ingest", ingestHandler(st))
	mux.Handle("GET /manifest/{asset_id...}", manifestHandler(st))
	mux.Handle("GET /health", healthHandler(startTime))
	return mux
}

func handleManifest(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

type errorResponse struct {
	Error string `json:"error"`
}

type statusResponse struct {
	Status string `json:"status"`
}

type healthResponse struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

func healthHandler(startTime time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:        "ok",
			UptimeSeconds: int64(time.Since(startTime).Seconds()),
		})
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "write response: %v\n", err)
	}
}
