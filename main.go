package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest", handleIngest)
	mux.HandleFunc("GET /manifest/{asset_id...}", handleManifest)
	mux.HandleFunc("GET /health", handleHealth)

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	fmt.Printf("listening on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func handleIngest(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

func handleManifest(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	panic("not implemented")
}
