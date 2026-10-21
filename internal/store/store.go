// Package store will hold the in-memory segment store.
//
// Shape decisions (see README):
//   - no global mutex on state (constraint 2): Store.mu guards only the
//     asset index map; each asset carries its own mutex so unrelated assets
//     never contend;
//   - segments keyed by index in a map[int]Segment: idempotent writes fall
//     out of the structure (constraint 4);
//   - Get returns a Manifest snapshot (sorted copy), never internal state;
//   - last_updated is the server clock and bumps only on actual mutation;
//   - no persistence (constraint 6): state resets on restart.
//
// Only the shape exists so far. All operations panic with TODOs.
package store

import (
	"context"
	"sync"
	"time"
)

// Segment is one ingested, playable media segment.
type Segment struct {
	AssetID    string
	Index      int
	CDNURL     string
	ReceivedAt time.Time
}

// Manifest is the store-owned view of an asset's segments. The handler maps
// this to wire JSON; the store never sees HTTP types.
type Manifest struct {
	AssetID     string
	Segments    []Segment // snapshot copy, sorted ascending by Index
	LastUpdated time.Time
}

// asset holds one asset's segments behind its own lock.
type asset struct {
	mu          sync.Mutex
	segments    map[int]Segment
	lastUpdated time.Time // server clock; bumps only on mutation
}

// Store is the in-memory segment store.
type Store struct {
	mu     sync.RWMutex // guards assets only and never held during segment reads/writes
	assets map[string]*asset
	now    func() time.Time // lets tests control last_updated deterministically.
}

// New returns a new initialised store
func New() *Store {
	// down the line this could have additional configurations and allow caller to have multiple stores
	return &Store{
		assets: make(map[string]*asset),
		now:    time.Now,
	}
}

// Insert stores seg only if (AssetID, Index) is new.
// Returns created=false if the index already exists (no mutation, so
// last_updated does not bump).
//
// TODO: implement; panics until then.
func (s *Store) Insert(ctx context.Context, seg Segment) (created bool) {
	panic("not implemented")
}

// InsertOrReplace stores seg, overwriting an existing (AssetID, Index).
// Returns replaced=true only if an existing entry actually changed;
// an identical resend is a no-op (no mutation, no last_updated bump).
// Bumps last_updated when it mutates.
//
// TODO: implement; panics until then.
func (s *Store) InsertOrReplace(ctx context.Context, seg Segment) (replaced bool) {
	panic("not implemented")
}

// Get returns the asset's manifest with segments sorted ascending by Index
// (constraint 3) and a snapshot copy safe for the caller to retain.
// ok=false when the asset has never been ingested.
//
// TODO: implement; panics until then.
func (s *Store) Get(ctx context.Context, assetID string) (Manifest, bool) {
	panic("not implemented")
}
