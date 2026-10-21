package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jamesjohnsdev/stan-challenge/internal/store"
)

// manifestStore is the slice of the store needed by manifest handler
type manifestTestStore interface {
	Get(ctx context.Context, assetID string) (store.Manifest, bool)
	InsertOrReplace(ctx context.Context, seg store.Segment) bool
}

func TestManifest(t *testing.T) {
	baseTime := time.Date(2024, 5, 1, 10, 0, 5, 0, time.UTC)

	tests := []struct {
		name       string
		path       string
		seed       []store.Segment
		lastUpdate time.Time
		wantStatus int
		wantBody   string
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "asset not found returns 404",
			path:       "/manifest/show/nonexistent/season/1/ep99",
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"asset not found"}`,
		},
		{
			name: "returns ordered segments",
			path: "/manifest/show/thunderbirds/season/1/ep3",
			seed: []store.Segment{
				{AssetID: "show/thunderbirds/season/1/ep3", Index: 0, CDNURL: "https://cdn.example.com/thunderbirds/s1e3/seg0.ts", ReceivedAt: baseTime},
				{AssetID: "show/thunderbirds/season/1/ep3", Index: 1, CDNURL: "https://cdn.example.com/thunderbirds/s1e3/seg1.ts", ReceivedAt: baseTime},
				{AssetID: "show/thunderbirds/season/1/ep3", Index: 2, CDNURL: "https://cdn.example.com/thunderbirds/s1e3/seg2.ts", ReceivedAt: baseTime},
			},
			lastUpdate: baseTime,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp struct {
					AssetID     string `json:"asset_id"`
					Segments    []struct {
						Index  int    `json:"index"`
						CDNURL string `json:"cdn_url"`
					} `json:"segments"`
					LastUpdated string `json:"last_updated"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if resp.AssetID != "show/thunderbirds/season/1/ep3" {
					t.Errorf("asset_id = %s", resp.AssetID)
				}
				if len(resp.Segments) != 3 {
					t.Fatalf("len segments = %d", len(resp.Segments))
				}
				for i := 0; i < 3; i++ {
					if resp.Segments[i].Index != i {
						t.Errorf("segment[%d].index = %d, want %d", i, resp.Segments[i].Index, i)
					}
				}
				if resp.LastUpdated == "" {
					t.Error("last_updated missing")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeManifestStore{
				manifests: map[string]store.Manifest{},
			}
			if len(tt.seed) > 0 {
				st.manifests[tt.seed[0].AssetID] = store.Manifest{
					AssetID:     tt.seed[0].AssetID,
					Segments:    tt.seed,
					LastUpdated: tt.lastUpdate,
				}
			}

			mux := registerRoutes(time.Now(), st)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.wantBody != "" {
				if rec.Body.String() != tt.wantBody+"\n" && rec.Body.String() != tt.wantBody {
					t.Errorf("body = %s, want %s", rec.Body.String(), tt.wantBody)
				}
				return
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec.Body.Bytes())
			}
		})
	}
}

type fakeManifestStore struct {
	manifests map[string]store.Manifest
}

func (f *fakeManifestStore) InsertOrReplace(ctx context.Context, seg store.Segment) bool {
	return false
}

func (f *fakeManifestStore) Get(ctx context.Context, assetID string) (store.Manifest, bool) {
	m, ok := f.manifests[assetID]
	return m, ok
}
