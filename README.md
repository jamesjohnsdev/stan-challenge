# Stan Backend Tech Lead Challenge

HTTP service that tracks available video segments between a transcoder and CDN origin servers.

## Requirements

- Go 1.27+
- Standard library only

## Run locally

```bash
go run .

# Alternatively, build first
go build -o stan-challenge .
./stan-challenge
```

Service listens on `:8080` by default. Use `PORT` to change it:

```bash
PORT=3000 go run .
```

## Live Demo

This project is available at `https://stan-challenge-u8p3.onrender.com/`. This is not a production ready deployment, and is likely to be inactive if no recent requests have been made.

You should allow for an additional 50 seconds for startup after the first request is sent.

## Endpoints

- `POST /ingest` - Store a segment
- `GET /manifest/{asset_id}` - Get ordered segments for an asset
- `GET /health` - Health check

## Run tests

```bash
go test ./...
go test -race ./...
```

## Configuration

The following environment variables can be used for configuration.

- `PORT` - Server port (default: `8080`)
- `SHUTDOWN_TIMEOUT` or `DRAIN_TIMEOUT` - Graceful shutdown timeout parsed with `time.ParseDuration` (default: `30s`). Example: `SHUTDOWN_TIMEOUT=10s go run .`

## Design decision

I chose per-asset locking instead of a global mutex on the store. `Store.mu` only protects the assets map for lookups and creation, while each asset has its own `sync.Mutex` protecting its segment map and `last_updated`. This cuts contention between unrelated assets, satisfies the no-global-mutex constraint, and still allows safe concurrent reads and writes. Segments are stored in a `map[int]Segment` per asset so duplicate writes for the same index are naturally avoided. On `Get`, keys are collected and sorted with `slices.Sort` before returning a defensive copy so manifests are always ordered by `segment_index` regardless of arrival order. I put deduplication logic in the store (`InsertOrReplace`) to keep the handler thin and maintain clear separation of concerns.

## Implementation details

- **Idempotency**: `InsertOrReplace` skips identical resends (same CDN URL) without bumping `last_updated`. Different CDN URLs for the same index replace the entry and bump the timestamp. The ingest handler always returns `202 Accepted` on valid requests.
- **Validation**: The ingest handler rejects trailing JSON after the object, requires `asset_id`, `segment_index`, and `cdn_url`, checks `segment_index >= 0`, validates `received_at` as RFC3339 when present, and tolerates unknown fields for forward compatibility.
- **Context**: The store respects `context.Context` cancellation and handlers pass `r.Context()` through.
- **Graceful shutdown**: Uses `signal.NotifyContext` for `SIGTERM`/`SIGINT`, `http.Server.Shutdown` with a configurable drain timeout, and structured JSON logs via `log/slog`. HTTP timeouts are configured on the server (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`).
- **Snapshots**: `Get` copies the segment map before sorting and returns a snapshot, so callers cannot mutate internal state.
- **Go idioms**: Uses `context.Context`, `slices.Sort`, defensive copies, proper error handling, and `defer` for cleanup.

## Assumptions

- No persistence. State resets on restart.
- When `received_at` is missing, server time (`time.Now().UTC()`) is used. When present, it must be RFC3339.
- Duplicate `(asset_id, segment_index)` requests are idempotent. Identical resends do not bump `last_updated`.
- Unknown JSON fields are tolerated. Trailing content is rejected.
- `asset_id`, `segment_index`, and `cdn_url` are required. `segment_index` must be >= 0.

## Known limitations

- **Unbounded memory**: State grows without eviction or TTL.
- **Single node only**: No shared state across instances.
- **No auth/rate limiting**: No access control or request throttling.
- **No persistence**: Data is lost on restart.

## High volume burst (Scenario 7)

With 200 goroutines writing to 500 assets and 50 goroutines reading manifests, the first bottleneck would be memory due to unbounded maps and no eviction. Each `Get` does a full map copy to build a sorted snapshot, creating allocation churn and GC pressure. Per-asset locking prevents cross-asset contention, but hot assets would serialize access and increase lock contention. Asset creation also contends on `Store.mu`.

To support this load, I would enforce memory bounds with LRU/TTL eviction, reduce copy cost with copy-on-write or pre-sorted structures, use RW locks or sharding to reduce contention, and apply backpressure on ingest. For observability, I'd add request metrics (latency, status, in-flight), store metrics (asset count, segment count, lock wait time), runtime metrics (heap, goroutines, GC), structured logs with request IDs, and alerts on error rates and p99 latency.
