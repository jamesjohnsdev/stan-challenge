package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		start      func() time.Time
		wantStatus int
		wantBody   bool
		uptimeMin  int64
		uptimeMax  int64
	}{
		{
			name:       "returns 200 with status ok",
			method:     http.MethodGet,
			start:      func() time.Time { return time.Now().Add(-142 * time.Second) },
			wantStatus: http.StatusOK,
			wantBody:   true,
			uptimeMin:  141,
			uptimeMax:  143,
		},
		{
			name:       "uptime is non-negative at service start",
			method:     http.MethodGet,
			start:      func() time.Time { return time.Now() },
			wantStatus: http.StatusOK,
			wantBody:   true,
			uptimeMin:  0,
			uptimeMax:  5,
		},
		{
			name:       "rejects non-GET methods",
			method:     http.MethodPost,
			start:      time.Now,
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := registerRoutes(tt.start(), &fakeStore{})
			req := httptest.NewRequest(tt.method, "/health", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if !tt.wantBody {
				return
			}

			var body struct {
				Status        string `json:"status"`
				UptimeSeconds int64  `json:"uptime_seconds"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Status != "ok" {
				t.Errorf("status = %q, want %q", body.Status, "ok")
			}
			if body.UptimeSeconds < tt.uptimeMin || body.UptimeSeconds > tt.uptimeMax {
				t.Errorf("uptime_seconds = %d, want [%d, %d]", body.UptimeSeconds, tt.uptimeMin, tt.uptimeMax)
			}
		})
	}
}
