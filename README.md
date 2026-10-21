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
- `SHUTDOWN_TIMEOUT` or `DRAIN_TIMEOUT` - Graceful shutdown timeout, parsed with `time.ParseDuration` (default: `30s`). Use a time suffix e.g. 10s, 500ms etc.

## Design decision

I used per-asset locking instead of a global mutex on the store. `Store.mu` only protects the assets map for lookups and creation. Each asset has its own `sync.Mutex` for its segment map and `last_updated`. This cuts contention between unrelated assets and matches constraint 2.

Segments are stored in a `map[int]Segment` per asset, which makes duplicate writes for the same index a no-op. On `Get`, keys are collected and sorted with `slices.Sort` before returning a copy, so out of order arrivals still produce an ascending manifest. `last_updated` only changes on actual mutations. `Get` copies the map and returns a snapshot, so the caller cannot mutate internal state. The store passes `context.Context` checks through and the handler uses `r.Context()`.

For idempotency, the logic is in the store. `InsertOrReplace` skips when an identical entry already exists (same CDN URL), and only updates if different. This keeps the handler thin and it still returns `202 Accepted` on valid requests. The handler also rejects trailing JSON after the object, enforces required fields, checks `segment_index >= 0`, validates `received_at` as RFC3339 when present, and tolerates unknown fields for forward compatibility.

The server uses `net/http` timeouts (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`), `signal.NotifyContext` for `SIGTERM` and `SIGINT`, and `http.Server.Shutdown` with the configurable drain timeout. Logs go to stdout as JSON with `log/slog`.

## Assumptions

- No persistence. State resets on restart.
- When `received_at` is missing, server time (`time.Now().UTC()`) is used.
- When present, `received_at` must be RFC3339.
- Duplicate `(asset_id, segment_index)` requests are idempotent. Identical resends do not bump `last_updated`.
- Unknown JSON fields are tolerated. Trailing content is rejected.
- `asset_id`, `segment_index`, and `cdn_url` are required. `segment_index` must be >= 0.

## High volume burst thoughts (Scenario 7)

With 200 goroutines writing to 500 assets and 50 goroutines reading manifests, memory is likely to become the main issue first. The maps keep growing, and completed assets are never removed. Each Get copies the entire segment map to create a snapshot, so heavy reads cause extra allocations and put more pressure on the GC. Locking is fine at the per-asset level because different assets do not block each other. However, multiple goroutines accessing the same asset will still compete for its mutex. Store.mu only protects the index map, which is good, but creating an asset still requires that lock.

To handle more load, I'd look at sharding locks, using sync.RWMutex where it makes sense, and adding a size limit with LRU or TTL eviction for old assets. I'd also try to avoid copying the whole map on every read, possibly by using immutable snapshots with copy-on-write or versioned data. If writes come in bursts, batching or backpressure could help. A more structured store or sharded maps might also be worth considering, along with horizontal scaling if shared state becomes necessary.

Before going to production, I'd add metrics for request latency, status codes and in-flight requests, as well as store metrics like asset count, segment count and lock wait time. I'd also monitor heap usage, goroutine count and GC activity. Structured logs with trace or request IDs would help with debugging, and health checks could report when the service is struggling. Rate limiting and timeouts are already handled at the HTTP level, but I'd also set up alerts for rising error rates and high p99 latency.
