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
package store

import (
	"context"
	"slices"
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
	mu     sync.RWMutex // guards assets only — never held during segment reads/writes
	assets map[string]*asset
	now    func() time.Time // lets tests control last_updated deterministically
}

// New returns an empty store, safe to call today so the service can boot.
func New() *Store {
	return &Store{
		assets: make(map[string]*asset),
		now:    time.Now,
	}
}

func sortedSnapshot(m map[int]Segment) []Segment {
	if len(m) == 0 {
		return []Segment{}
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	out := make([]Segment, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

// Insert stores seg only if (AssetID, Index) is new.
// Returns created=false if the index already exists (no mutation, so
// last_updated does not bump). Respects context cancellation.
func (s *Store) Insert(ctx context.Context, seg Segment) bool {
	if err := ctx.Err(); err != nil {
		return false
	}

	a := s.getOrCreateAsset(seg.AssetID)
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false
	}
	if _, exists := a.segments[seg.Index]; exists {
		return false
	}
	if a.segments == nil {
		a.segments = make(map[int]Segment)
	}
	a.segments[seg.Index] = seg
	a.lastUpdated = s.now()
	return true
}

// InsertOrReplace stores seg, overwriting an existing (AssetID, Index).
// Returns replaced=true only if an existing entry actually changed;
// an identical resend (same cdn_url) is a no-op (no mutation, no last_updated bump).
// Bumps last_updated when it mutates. Respects context cancellation.
func (s *Store) InsertOrReplace(ctx context.Context, seg Segment) bool {
	if err := ctx.Err(); err != nil {
		return false
	}

	a := s.getOrCreateAsset(seg.AssetID)
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false
	}
	if a.segments == nil {
		a.segments = make(map[int]Segment)
	}
	existing, exists := a.segments[seg.Index]
	if exists && existing.CDNURL == seg.CDNURL {
		return false
	}
	a.segments[seg.Index] = seg
	a.lastUpdated = s.now()
	return exists
}

// Get returns the asset's manifest with segments sorted ascending by Index
// and a snapshot copy safe for the caller to retain.
// ok=false when the asset has never been ingested or context is cancelled.
func (s *Store) Get(ctx context.Context, assetID string) (Manifest, bool) {
	if err := ctx.Err(); err != nil {
		return Manifest{}, false
	}

	s.mu.RLock()
	a, ok := s.assets[assetID]
	s.mu.RUnlock()
	if !ok {
		return Manifest{}, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Manifest{}, false
	}
	if len(a.segments) == 0 {
		return Manifest{}, false
	}

	cp := make(map[int]Segment, len(a.segments))
	for k, v := range a.segments {
		cp[k] = v
	}
	return Manifest{
		AssetID:     assetID,
		Segments:    sortedSnapshot(cp),
		LastUpdated: a.lastUpdated,
	}, true
}

func (s *Store) getOrCreateAsset(id string) *asset {
	s.mu.RLock()
	a, ok := s.assets[id]
	s.mu.RUnlock()
	if ok {
		return a
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.assets[id]; ok {
		return a
	}
	a = &asset{segments: make(map[int]Segment)}
	s.assets[id] = a
	return a
}
