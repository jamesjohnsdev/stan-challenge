package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/jamesjohnsdev/stan-challenge/internal/store"
)

// ingestStore is the slice of the store this handler needs. Defined on the
// consumer side so tests can substitute a fake; *store.Store satisfies it.
//
// The handler calls InsertOrReplace (update branch of constraint 4) and
// ignores the bool: a re-send is still 202 Accepted either way.
type ingestStore interface {
	InsertOrReplace(ctx context.Context, seg store.Segment) bool
}

// Validation policy (decisions, see README):
//   - required: asset_id, segment_index, cdn_url — spec only demonstrates
//     cdn_url missing, but the manifest is meaningless without them;
//   - received_at is optional and defaults to the server clock;
//   - received_at must be RFC3339 when present;
//   - unknown JSON fields are tolerated (forward-compatible), but trailing
//     content after the top-level object is rejected;
//   - idempotency on (asset_id, segment_index) is the store's job — this
//     handler validates and forwards every request, including re-sends.
type ingestRequest struct {
	AssetID      string `json:"asset_id"`
	SegmentIndex *int   `json:"segment_index"`
	CDNURL       string `json:"cdn_url"`
	ReceivedAt   string `json:"received_at"`
}

const decodeErrorMessage = "Could not decode request: JSON parsing failed"

func ingestHandler(st ingestStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ingestRequest
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: decodeErrorMessage})
			return
		}
		if err := dec.Decode(&json.RawMessage{}); err != io.EOF {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: decodeErrorMessage})
			return
		}

		if req.AssetID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing required field: asset_id"})
			return
		}
		if req.SegmentIndex == nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing required field: segment_index"})
			return
		}
		if req.CDNURL == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing required field: cdn_url"})
			return
		}
		if *req.SegmentIndex < 0 {
			// Spec does not cover invalid (vs missing) values; kept in the
			// same structured-error shape.
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid field: segment_index"})
			return
		}

		receivedAt := time.Now().UTC()
		if req.ReceivedAt != "" {
			t, err := time.Parse(time.RFC3339, req.ReceivedAt)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: decodeErrorMessage})
				return
			}
			receivedAt = t
		}

		st.InsertOrReplace(r.Context(), store.Segment{
			AssetID:    req.AssetID,
			Index:      *req.SegmentIndex,
			CDNURL:     req.CDNURL,
			ReceivedAt: receivedAt,
		})

		writeJSON(w, http.StatusAccepted, statusResponse{Status: "accepted"})
	}
}
