package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
		Addr:    addr,
		Handler: mux,
	}

	fmt.Printf("listening on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
