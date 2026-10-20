// Package store will hold the in-memory segment store, using per-asset
// locking (constraint 2: no global mutex on the whole store).
//
// Everything here is a stub: the ingest handler validates requests and
// forwards them via Add; the real state lands together with the manifest
// endpoint.
package store

import (
	"context"
	"time"
)

// Segment is one ingested, playable media segment.
type Segment struct {
	AssetID    string
	Index      int
	CDNURL     string
	ReceivedAt time.Time
}

// Store holds per-asset segments in memory. No persistence (constraint 6):
// state resets on restart.
type Store struct{}

// New returns a store. Safe to call today so the service can boot; the
// state it manages is not implemented yet.
//
// TODO: initialise per-asset (or sharded) state here.
func New() *Store {
	return &Store{}
}

// Add stores a segment. Must be idempotent on (AssetID, Index) — constraint 4.
//
// TODO: implement; panics until then.
func (s *Store) Add(ctx context.Context, seg Segment) {
	panic("not implemented")
}

// Get returns the segments for an asset, sorted ascending by Index —
// constraint 3 (out-of-order delivery).
//
// TODO: implement; panics until then.
func (s *Store) Get(ctx context.Context, assetID string) ([]Segment, bool) {
	panic("not implemented")
}
