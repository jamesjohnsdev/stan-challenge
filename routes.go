package main

import (
	"net/http"
	"time"
)

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
