package store

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

var baseTime = time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)

// testClock is safe for concurrent use so race tests can inject it too.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestStore() (*Store, *testClock) {
	clock := &testClock{t: baseTime}
	s := New()
	s.now = clock.Now
	return s, clock
}

func backgroundContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestInsert(t *testing.T) {
	afterAdvance := baseTime.Add(time.Minute)

	tests := []struct {
		name            string
		seed            *Segment
		incoming        Segment
		cancelled       bool
		wantCreated     bool
		wantAsset       bool
		wantStored      *Segment
		wantLastUpdated time.Time
	}{
		{
			name:            "new segment created",
			incoming:        Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			wantCreated:     true,
			wantAsset:       true,
			wantStored:      &Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			wantLastUpdated: afterAdvance,
		},
		{
			name:            "duplicate index keeps first entry",
			seed:            &Segment{AssetID: "a", Index: 3, CDNURL: "u3", ReceivedAt: baseTime},
			incoming:        Segment{AssetID: "a", Index: 3, CDNURL: "changed", ReceivedAt: baseTime.Add(time.Hour)},
			wantCreated:     false,
			wantAsset:       true,
			wantStored:      &Segment{AssetID: "a", Index: 3, CDNURL: "u3", ReceivedAt: baseTime},
			wantLastUpdated: baseTime, // no bump: no mutation
		},
		{
			name:        "cancelled context stores nothing",
			incoming:    Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			cancelled:   true,
			wantCreated: false,
			wantAsset:   false,
		},
		{
			name:            "cancelled context leaves existing asset untouched",
			seed:            &Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			incoming:        Segment{AssetID: "a", Index: 1, CDNURL: "u1", ReceivedAt: baseTime},
			cancelled:       true,
			wantCreated:     false,
			wantAsset:       true,
			wantStored:      &Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			wantLastUpdated: baseTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, clock := newTestStore()
			bg := backgroundContext(t)

			if tt.seed != nil {
				if !s.Insert(bg, *tt.seed) {
					t.Fatal("seed Insert = false, want true")
				}
			}
			clock.Advance(time.Minute)

			ctx := bg
			if tt.cancelled {
				ctx = cancelledContext(t)
			}

			if got := s.Insert(ctx, tt.incoming); got != tt.wantCreated {
				t.Errorf("created = %v, want %v", got, tt.wantCreated)
			}

			m, ok := s.Get(bg, tt.incoming.AssetID)
			if ok != tt.wantAsset {
				t.Fatalf("Get ok = %v, want %v", ok, tt.wantAsset)
			}
			if !tt.wantAsset {
				return
			}
			if !m.LastUpdated.Equal(tt.wantLastUpdated) {
				t.Errorf("last_updated = %v, want %v", m.LastUpdated, tt.wantLastUpdated)
			}
			if len(m.Segments) != 1 {
				t.Fatalf("len(segments) = %d, want 1", len(m.Segments))
			}
			requireEqualSegment(t, m.Segments[0], *tt.wantStored)
		})
	}
}

func TestInsertOrReplace(t *testing.T) {
	afterAdvance := baseTime.Add(time.Minute)

	tests := []struct {
		name            string
		seed            *Segment
		incoming        Segment
		cancelled       bool
		wantReplaced    bool
		wantAsset       bool
		wantStored      *Segment
		wantLastUpdated time.Time
	}{
		{
			name:            "new segment stores without replacing",
			incoming:        Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			wantReplaced:    false,
			wantAsset:       true,
			wantStored:      &Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			wantLastUpdated: afterAdvance,
		},
		{
			name:            "changed cdn_url replaces entry and bumps last_updated",
			seed:            &Segment{AssetID: "a", Index: 3, CDNURL: "u3", ReceivedAt: baseTime},
			incoming:        Segment{AssetID: "a", Index: 3, CDNURL: "u3-v2", ReceivedAt: baseTime.Add(time.Hour)},
			wantReplaced:    true,
			wantAsset:       true,
			wantStored:      &Segment{AssetID: "a", Index: 3, CDNURL: "u3-v2", ReceivedAt: baseTime.Add(time.Hour)},
			wantLastUpdated: afterAdvance,
		},
		{
			name:            "same cdn_url is a no-op even when received_at differs",
			seed:            &Segment{AssetID: "a", Index: 3, CDNURL: "u3", ReceivedAt: baseTime},
			incoming:        Segment{AssetID: "a", Index: 3, CDNURL: "u3", ReceivedAt: baseTime.Add(time.Hour)},
			wantReplaced:    false,
			wantAsset:       true,
			wantStored:      &Segment{AssetID: "a", Index: 3, CDNURL: "u3", ReceivedAt: baseTime},
			wantLastUpdated: baseTime,
		},
		{
			name:            "cancelled context leaves existing asset untouched",
			seed:            &Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			incoming:        Segment{AssetID: "a", Index: 1, CDNURL: "u1", ReceivedAt: baseTime},
			cancelled:       true,
			wantReplaced:    false,
			wantAsset:       true,
			wantStored:      &Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			wantLastUpdated: baseTime,
		},
		{
			name:         "cancelled context stores nothing",
			incoming:     Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime},
			cancelled:    true,
			wantReplaced: false,
			wantAsset:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, clock := newTestStore()
			bg := backgroundContext(t)

			if tt.seed != nil {
				if !s.Insert(bg, *tt.seed) {
					t.Fatal("seed Insert = false, want true")
				}
			}
			clock.Advance(time.Minute)

			ctx := bg
			if tt.cancelled {
				ctx = cancelledContext(t)
			}

			if got := s.InsertOrReplace(ctx, tt.incoming); got != tt.wantReplaced {
				t.Errorf("replaced = %v, want %v", got, tt.wantReplaced)
			}

			m, ok := s.Get(bg, tt.incoming.AssetID)
			if ok != tt.wantAsset {
				t.Fatalf("Get ok = %v, want %v", ok, tt.wantAsset)
			}
			if !tt.wantAsset {
				return
			}
			if !m.LastUpdated.Equal(tt.wantLastUpdated) {
				t.Errorf("last_updated = %v, want %v", m.LastUpdated, tt.wantLastUpdated)
			}
			if len(m.Segments) != 1 {
				t.Fatalf("len(segments) = %d, want 1", len(m.Segments))
			}
			requireEqualSegment(t, m.Segments[0], *tt.wantStored)
		})
	}
}

func TestGet(t *testing.T) {
	t.Run("unknown asset returns not found", func(t *testing.T) {
		s, _ := newTestStore()

		m, ok := s.Get(backgroundContext(t), "show/nonexistent")
		if ok {
			t.Fatalf("ok = true, want false; manifest = %+v", m)
		}
		if len(m.Segments) != 0 {
			t.Errorf("segments = %v, want empty/nil slice not required but asset missing means zero manifest", m.Segments)
		}
	})

	t.Run("returns segments sorted ascending regardless of arrival order", func(t *testing.T) {
		s, clock := newTestStore()
		ctx := backgroundContext(t)

		for _, idx := range []int{5, 2, 0, 3, 1, 4} {
			s.InsertOrReplace(ctx, Segment{
				AssetID:    "show/scooby/season/1/ep4",
				Index:      idx,
				CDNURL:     fmt.Sprintf("cdn/seg%d.ts", idx),
				ReceivedAt: baseTime,
			})
		}

		m, ok := s.Get(ctx, "show/scooby/season/1/ep4")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if m.AssetID != "show/scooby/season/1/ep4" {
			t.Errorf("asset_id = %q", m.AssetID)
		}
		wantIndexes := []int{0, 1, 2, 3, 4, 5}
		var gotIndexes []int
		for _, seg := range m.Segments {
			gotIndexes = append(gotIndexes, seg.Index)
			wantURL := fmt.Sprintf("cdn/seg%d.ts", seg.Index)
			if seg.CDNURL != wantURL {
				t.Errorf("segment %d url = %q, want %q", seg.Index, seg.CDNURL, wantURL)
			}
		}
		if !slices.Equal(gotIndexes, wantIndexes) {
			t.Errorf("indexes = %v, want %v", gotIndexes, wantIndexes)
		}
		if !m.LastUpdated.Equal(clock.Now()) {
			t.Errorf("last_updated = %v, want %v", m.LastUpdated, clock.Now())
		}
	})

	t.Run("returned manifest is a snapshot", func(t *testing.T) {
		s, _ := newTestStore()
		ctx := backgroundContext(t)

		for i := range 2 {
			s.InsertOrReplace(ctx, Segment{AssetID: "a", Index: i, CDNURL: fmt.Sprintf("u%d", i), ReceivedAt: baseTime})
		}
		first, ok := s.Get(ctx, "a")
		if !ok {
			t.Fatal("ok = false, want true")
		}

		s.InsertOrReplace(ctx, Segment{AssetID: "a", Index: 2, CDNURL: "u2", ReceivedAt: baseTime})

		if len(first.Segments) != 2 {
			t.Errorf("earlier manifest grew to %d segments, want it frozen at 2", len(first.Segments))
		}
	})

	t.Run("cancelled context returns not found", func(t *testing.T) {
		s, _ := newTestStore()
		ctx := backgroundContext(t)

		s.InsertOrReplace(ctx, Segment{AssetID: "a", Index: 0, CDNURL: "u0", ReceivedAt: baseTime})

		if _, ok := s.Get(cancelledContext(t), "a"); ok {
			t.Error("ok = true on cancelled context, want false")
		}
		if _, ok := s.Get(ctx, "a"); !ok {
			t.Error("ok = false on live context, want true")
		}
	})

	t.Run("assets are independent", func(t *testing.T) {
		s, _ := newTestStore()
		ctx := backgroundContext(t)

		for _, id := range []string{"show/thunderbirds/season/1/ep1", "show/scooby/season/1/ep4"} {
			for i := range 2 {
				s.InsertOrReplace(ctx, Segment{AssetID: id, Index: i, CDNURL: id + "/seg", ReceivedAt: baseTime})
			}
		}

		for _, id := range []string{"show/thunderbirds/season/1/ep1", "show/scooby/season/1/ep4"} {
			m, ok := s.Get(ctx, id)
			if !ok {
				t.Fatalf("%s not found", id)
			}
			if len(m.Segments) != 2 {
				t.Errorf("%s has %d segments, want 2", id, len(m.Segments))
			}
			for _, seg := range m.Segments {
				if seg.CDNURL != id+"/seg" {
					t.Errorf("%s segment %d url = %q: cross-contamination", id, seg.Index, seg.CDNURL)
				}
			}
		}
	})
}

func TestSortedSnapshot(t *testing.T) {
	tests := []struct {
		name        string
		in          map[int]Segment
		wantIndexes []int
	}{
		{
			name:        "empty map returns non-nil empty slice",
			in:          map[int]Segment{},
			wantIndexes: []int{},
		},
		{
			name:        "single segment",
			in:          map[int]Segment{7: {Index: 7}},
			wantIndexes: []int{7},
		},
		{
			name: "unsorted keys returned ascending",
			in: map[int]Segment{
				5: {Index: 5}, 2: {Index: 2}, 9: {Index: 9},
				0: {Index: 0}, 4: {Index: 4},
			},
			wantIndexes: []int{0, 2, 4, 5, 9},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortedSnapshot(tt.in)

			if got == nil {
				t.Fatal("result is nil, want non-nil slice (JSON must marshal as [])")
			}
			var gotIndexes []int
			for _, seg := range got {
				gotIndexes = append(gotIndexes, seg.Index)
			}
			if !slices.Equal(gotIndexes, tt.wantIndexes) {
				t.Errorf("indexes = %v, want %v", gotIndexes, tt.wantIndexes)
			}
		})
	}
}

// TestConcurrentIngestAndRead hammers the store the way scenario 7 describes:
// many writers across many assets (including the same asset) while readers
// poll manifests. Must pass under -race.
func TestConcurrentIngestAndRead(t *testing.T) {
	s, _ := newTestStore()
	ctx := backgroundContext(t)

	const (
		assets       = 32
		segsPerAsset = 16
		readers      = 8
		readLoops    = 100
	)

	var wg sync.WaitGroup

	// One goroutine per (asset, segment) so writes contend on the same asset.
	for a := range assets {
		for idx := range segsPerAsset {
			wg.Add(1)
			go func(a, idx int) {
				defer wg.Done()
				seg := Segment{
					AssetID:    fmt.Sprintf("asset/%d", a),
					Index:      idx,
					CDNURL:     fmt.Sprintf("cdn/%d/%d.ts", a, idx),
					ReceivedAt: time.Now(),
				}
				if idx%2 == 0 {
					s.Insert(ctx, seg)
				} else {
					s.InsertOrReplace(ctx, seg)
				}
			}(a, idx)
		}
	}

	for r := range readers {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := range readLoops {
				if m, ok := s.Get(ctx, fmt.Sprintf("asset/%d", (r+i)%assets)); ok {
					_ = m.Segments
				}
				if _, ok := s.Get(ctx, "asset/missing"); ok {
					t.Error("missing asset found")
				}
			}
		}(r)
	}

	wg.Wait()

	for a := range assets {
		id := fmt.Sprintf("asset/%d", a)
		m, ok := s.Get(ctx, id)
		if !ok {
			t.Fatalf("%s not found", id)
		}
		if len(m.Segments) != segsPerAsset {
			t.Fatalf("%s has %d segments, want %d", id, len(m.Segments), segsPerAsset)
		}
		for i, seg := range m.Segments {
			if seg.Index != i {
				t.Errorf("%s segment at pos %d has index %d: not sorted", id, i, seg.Index)
			}
			wantURL := fmt.Sprintf("cdn/%d/%d.ts", a, i)
			if seg.CDNURL != wantURL {
				t.Errorf("%s segment %d url = %q, want %q: cross-contamination", id, i, seg.CDNURL, wantURL)
			}
		}
	}
}

// BenchmarkInsert measures the duplicate hot path: after warmup every write
// hits an existing index (the idempotent no-op branch), which is what a
// transcoder reconnect storm looks like.
func BenchmarkInsert(b *testing.B) {
	s := New()
	ctx := context.Background()
	seg := Segment{AssetID: "bench", ReceivedAt: time.Now()}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		seg.Index = i % 8192
		seg.CDNURL = "cdn/seg.ts"
		s.Insert(ctx, seg)
	}
}

// BenchmarkInsertOrReplace measures the mutating replace path: the cdn_url
// differs on every call, so each iteration takes the write branch.
func BenchmarkInsertOrReplace(b *testing.B) {
	s := New()
	ctx := context.Background()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.InsertOrReplace(ctx, Segment{
			AssetID:    fmt.Sprintf("asset/%d", i%64),
			Index:      (i >> 6) % 256,
			CDNURL:     fmt.Sprintf("cdn/%d.ts", i),
			ReceivedAt: time.Now(),
		})
	}
}

// BenchmarkGet measures manifest reads over a store with 64 assets x 256
// segments, concurrent the way CDN origins poll.
func BenchmarkGet(b *testing.B) {
	s := New()
	ctx := context.Background()
	for a := range 64 {
		for idx := range 256 {
			s.InsertOrReplace(ctx, Segment{
				AssetID:    fmt.Sprintf("asset/%d", a),
				Index:      idx,
				CDNURL:     fmt.Sprintf("cdn/%d/%d.ts", a, idx),
				ReceivedAt: time.Now(),
			})
		}
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Get(ctx, fmt.Sprintf("asset/%d", i%64))
			i++
		}
	})
}

func requireEqualSegment(t *testing.T, got, want Segment) {
	t.Helper()
	if got.AssetID != want.AssetID {
		t.Errorf("AssetID = %q, want %q", got.AssetID, want.AssetID)
	}
	if got.Index != want.Index {
		t.Errorf("Index = %d, want %d", got.Index, want.Index)
	}
	if got.CDNURL != want.CDNURL {
		t.Errorf("CDNURL = %q, want %q", got.CDNURL, want.CDNURL)
	}
	if !got.ReceivedAt.Equal(want.ReceivedAt) {
		t.Errorf("ReceivedAt = %v, want %v", got.ReceivedAt, want.ReceivedAt)
	}
}
