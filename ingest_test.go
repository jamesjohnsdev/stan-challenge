package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jamesjohnsdev/stan-challenge/internal/store"
)

// fakeStore stands in for the real store (still a stub) so the handler's
// validation and forwarding behaviour can be asserted in isolation.
// TODO: adjust once store implemented
type fakeStore struct {
	added []store.Segment
}

// TODO: adjust once store implemented
func (f *fakeStore) Add(_ context.Context, seg store.Segment) {
	f.added = append(f.added, seg)
}

func TestIngest(t *testing.T) {
	segTime, err := time.Parse(time.RFC3339, "2024-05-01T10:00:00Z")
	if err != nil {
		t.Fatalf("parse fixture time: %v", err)
	}

	tests := []struct {
		name              string
		body              string
		wantStatus        int
		wantError         string
		wantAdded         *store.Segment
		receivedAtFromNow bool
	}{
		{
			name:       "valid request accepted",
			body:       `{"asset_id":"show/thunderbirds/season/1/ep3","segment_index":4,"cdn_url":"https://cdn.example.com/thunderbirds/s1e3/seg4.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusAccepted,
			wantAdded: &store.Segment{
				AssetID:    "show/thunderbirds/season/1/ep3",
				Index:      4,
				CDNURL:     "https://cdn.example.com/thunderbirds/s1e3/seg4.ts",
				ReceivedAt: segTime,
			},
		},
		{
			name:       "invalid json",
			body:       `{ "asset_id": "show/thunderbirds", "segment_index": ]]]`,
			wantStatus: http.StatusBadRequest,
			wantError:  "Could not decode request: JSON parsing failed",
		},
		{
			name:       "missing cdn_url",
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":0,"received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "missing required field: cdn_url",
		},
		{
			name:       "segment_index wrong type",
			body:       `{"asset_id":"show/thunderbirds/season/1/ep1","segment_index":"four","cdn_url":"https://cdn.example.com/thunderbirds/s1e1/seg4.ts","received_at":"2024-05-01T10:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "Could not decode request: JSON parsing failed",
		},
		{
			name:       "empty body",
			body:       ``,
			wantStatus: http.StatusBadRequest,
			wantError:  "Could not decode request: JSON parsing failed",
		},
		{
			name:       "missing asset_id",
			body:       `{"segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "missing required field: asset_id",
		},
		{
			name:       "missing segment_index",
			body:       `{"asset_id":"show/x","cdn_url":"https://cdn.example.com/seg0.ts"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "missing required field: segment_index",
		},
		{
			name:       "negative segment_index",
			body:       `{"asset_id":"show/x","segment_index":-1,"cdn_url":"https://cdn.example.com/seg0.ts"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid field: segment_index",
		},
		{
			name:       "received_at not rfc3339",
			body:       `{"asset_id":"show/x","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"yesterday"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "Could not decode request: JSON parsing failed",
		},
		{
			name:       "trailing content after object",
			body:       `{"asset_id":"show/x","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts"}{"asset_id":"show/y"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "Could not decode request: JSON parsing failed",
		},
		{
			name:       "unknown fields tolerated",
			body:       `{"asset_id":"show/x","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts","received_at":"2024-05-01T10:00:00Z","future_field":true}`,
			wantStatus: http.StatusAccepted,
			wantAdded: &store.Segment{
				AssetID:    "show/x",
				Index:      0,
				CDNURL:     "https://cdn.example.com/seg0.ts",
				ReceivedAt: segTime,
			},
		},
		{
			name:              "received_at omitted defaults to server clock",
			body:              `{"asset_id":"show/x","segment_index":0,"cdn_url":"https://cdn.example.com/seg0.ts"}`,
			wantStatus:        http.StatusAccepted,
			wantAdded:         &store.Segment{AssetID: "show/x", Index: 0, CDNURL: "https://cdn.example.com/seg0.ts"},
			receivedAtFromNow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{}
			mux := registerRoutes(time.Now(), st)

			req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			switch tt.wantStatus {
			case http.StatusAccepted:
				var body struct {
					Status string `json:"status"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body.Status != "accepted" {
					t.Errorf("status = %q, want %q", body.Status, "accepted")
				}
			case http.StatusBadRequest:
				var body struct {
					Error string `json:"error"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body.Error != tt.wantError {
					t.Errorf("error = %q, want %q", body.Error, tt.wantError)
				}
			}

			if tt.wantAdded == nil {
				if len(st.added) != 0 {
					t.Errorf("store.Add called %d times, want 0", len(st.added))
				}
				return
			}

			if len(st.added) != 1 {
				t.Fatalf("store.Add called %d times, want 1", len(st.added))
			}
			got := st.added[0]
			if got.AssetID != tt.wantAdded.AssetID {
				t.Errorf("AssetID = %q, want %q", got.AssetID, tt.wantAdded.AssetID)
			}
			if got.Index != tt.wantAdded.Index {
				t.Errorf("Index = %d, want %d", got.Index, tt.wantAdded.Index)
			}
			if got.CDNURL != tt.wantAdded.CDNURL {
				t.Errorf("CDNURL = %q, want %q", got.CDNURL, tt.wantAdded.CDNURL)
			}
			if tt.receivedAtFromNow {
				if d := time.Since(got.ReceivedAt); d > time.Minute || d < 0 {
					t.Errorf("ReceivedAt = %v, want server clock (delta %v)", got.ReceivedAt, d)
				}
			} else if !got.ReceivedAt.Equal(tt.wantAdded.ReceivedAt) {
				t.Errorf("ReceivedAt = %v, want %v", got.ReceivedAt, tt.wantAdded.ReceivedAt)
			}
		})
	}
}

// Idemppotency is not enforced in the handler but in the store
// handler should accept & forward resends
func TestIngestDuplicateRequestsBothAccepted(t *testing.T) {
	st := &fakeStore{}
	mux := registerRoutes(time.Now(), st)

	body := `{"asset_id":"show/thetaste/season/1/ep2","segment_index":0,"cdn_url":"https://cdn.example.com/thetaste/s1e2/seg0.ts","received_at":"2024-05-01T10:00:00Z"}`

	for i := range 2 {
		req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("attempt %d: status = %d, want %d", i, rec.Code, http.StatusAccepted)
		}
	}

	if len(st.added) != 2 {
		t.Errorf("store.Add called %d times, want 2 (dedup deferred to store)", len(st.added))
	}
}
