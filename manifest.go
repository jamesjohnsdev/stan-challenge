package main

import (
	"context"
	"net/http"
	"time"

	"github.com/jamesjohnsdev/stan-challenge/internal/store"
)

// manifestStore is the slice of the store needed by the manifest handler.
type manifestStore interface {
	Get(ctx context.Context, assetID string) (store.Manifest, bool)
}

type manifestResponse struct {
	AssetID     string        `json:"asset_id"`
	Segments    []segmentItem `json:"segments"`
	LastUpdated time.Time     `json:"last_updated"`
}

type segmentItem struct {
	Index  int    `json:"index"`
	CDNURL string `json:"cdn_url"`
}

func manifestHandler(st manifestStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assetID := r.PathValue("asset_id")
		if assetID == "" {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "asset not found"})
			return
		}

		m, ok := st.Get(r.Context(), assetID)
		if !ok {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "asset not found"})
			return
		}

		resp := manifestResponse{
			AssetID:     m.AssetID,
			Segments:    make([]segmentItem, 0, len(m.Segments)),
			LastUpdated: m.LastUpdated.UTC(),
		}
		for _, seg := range m.Segments {
			resp.Segments = append(resp.Segments, segmentItem{
				Index:  seg.Index,
				CDNURL: seg.CDNURL,
			})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
