package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	mux := registerRoutes(time.Now())

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

func registerRoutes(startTime time.Time) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest", handleIngest)
	mux.HandleFunc("GET /manifest/{asset_id...}", handleManifest)
	mux.Handle("GET /health", healthHandler(startTime))
	return mux
}

func handleIngest(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

func handleManifest(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
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
