# Stan Backend Tech Lead — Coding Challenge

## Overview

Welcome to the Stan Backend Tech Lead Coding Challenge.

This challenge is designed to assess your ability to design, implement, and reason about a production-grade Go service in a streaming media context. It is expected to take **3–5 hours** to complete. You will be asked to walk through your solution in a follow-up technical session, so please be prepared to discuss every decision you made.

> **Note:** As with any infrastructure exercise, please deploy this as a standalone service — not as an endpoint bolted onto an existing app. The service should be accessible at a root-resolvable URL (e.g. `https://myservice.onrender.com`).

---

## Background

Stan's transcoder pipeline emits events whenever a video segment becomes available for playback. Downstream CDN origin servers poll a manifest service to discover which segments are ready to serve to players.

Your task is to build the **Streaming Availability Service** — the Go HTTP service that sits between the transcoder and the CDN.

---

## Functional Requirements

### `POST /ingest`

Accepts a JSON body describing a newly available segment. Stores it in memory.

**Request body:**

```json
{
  "asset_id": "show/thunderbirds/season/1/ep3",
  "segment_index": 4,
  "cdn_url": "https://cdn.example.com/thunderbirds/s1e3/seg4.ts",
  "received_at": "2024-05-01T10:00:00Z"
}
```

**Success response — `202 Accepted`:**

```json
{
  "status": "accepted"
}
```

**Error response — `400 Bad Request`** (malformed JSON or missing required fields):

```json
{
  "error": "Could not decode request: JSON parsing failed"
}
```

---

### `GET /manifest/{asset_id}`

Returns the full ordered segment list for a given asset.

**Success response — `200 OK`:**

```json
{
  "asset_id": "show/thunderbirds/season/1/ep3",
  "segments": [
    { "index": 0, "cdn_url": "https://cdn.example.com/thunderbirds/s1e3/seg0.ts" },
    { "index": 1, "cdn_url": "https://cdn.example.com/thunderbirds/s1e3/seg1.ts" },
    { "index": 2, "cdn_url": "https://cdn.example.com/thunderbirds/s1e3/seg2.ts" }
  ],
  "last_updated": "2024-05-01T10:00:05Z"
}
```

**Not found — `404 Not Found`:**

```json
{
  "error": "asset not found"
}
```

---

### `GET /health`

Returns basic service health. Should always return `200 OK` while the service is running.

```json
{
  "status": "ok",
  "uptime_seconds": 142
}
```

---

## Constraints

These constraints are intentional. They are designed to surface trade-offs, not to make your life difficult. Read them carefully.

1. **Standard library only.** Use only the Go standard library. You may use one third-party package if you include a written justification in your README explaining why the standard library was insufficient.

2. **No global mutex on the state store.** A single `sync.Mutex` wrapping the entire in-memory store is not acceptable. Choose a more granular approach and explain your choice in the README.

3. **Out-of-order delivery.** Segments may arrive in any order. The manifest must always return segments sorted ascending by `segment_index`.

4. **Idempotent ingestion.** Posting the same segment twice (same `asset_id` + `segment_index`) must not create duplicates. The second write should be a no-op or an update, but the manifest must remain clean.

5. **Graceful shutdown.** On `SIGTERM`, the service must finish processing all in-flight requests before exiting. The drain timeout should be configurable (e.g. via environment variable).

6. **No persistence is required.** In-memory state that resets on restart is acceptable. State your assumptions clearly.

---

## What to Submit

- **Source code** hosted on GitHub (public or private — if private, share access)
- **A deployed URL** (Render free tier is sufficient)
- **A `README.md`** that includes:
  - How to run locally
  - How to run the tests (`go test ./...` must pass; `go test -race ./...` must also pass)
  - **A short paragraph (5–10 sentences) on one design decision you made and why** — this is required, not optional

---

## What We Are Looking For

| Area | Signal |
|---|---|
| Concurrency correctness | Per-asset or sharded locking; safe concurrent reads and writes |
| Go idioms | `context.Context` propagation, `%w` error wrapping, `defer` hygiene |
| Correctness | Idempotent ingest, sorted manifest, correct HTTP status codes |
| Testing | `httptest`, `t.Run` subtests, race detector passing, at least one benchmark |
| Operational maturity | `/health` endpoint, structured logging, graceful shutdown with drain |
| README quality | Clear setup, honest trade-off discussion, known limitations called out |

---

## Example Ingestion Scenarios

The following scenarios represent the range of inputs your service will receive during testing. Your service must handle all of them correctly.

---

### Scenario 1 — Normal sequential ingest

Segments arriving in order for a single asset.

```json
{ "asset_id": "show/thunderbirds/season/1/ep1", "segment_index": 0, "cdn_url": "https://cdn.example.com/thunderbirds/s1e1/seg0.ts", "received_at": "2024-05-01T10:00:00Z" }
{ "asset_id": "show/thunderbirds/season/1/ep1", "segment_index": 1, "cdn_url": "https://cdn.example.com/thunderbirds/s1e1/seg1.ts", "received_at": "2024-05-01T10:00:01Z" }
{ "asset_id": "show/thunderbirds/season/1/ep1", "segment_index": 2, "cdn_url": "https://cdn.example.com/thunderbirds/s1e1/seg2.ts", "received_at": "2024-05-01T10:00:02Z" }
```

**Expected manifest:** segments ordered `[0, 1, 2]`.

---

### Scenario 2 — Out-of-order delivery

Segments arriving non-sequentially. Common during transcoder parallelisation.

```json
{ "asset_id": "show/scooby/season/1/ep4", "segment_index": 5, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg5.ts", "received_at": "2024-05-01T10:00:00Z" }
{ "asset_id": "show/scooby/season/1/ep4", "segment_index": 2, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg2.ts", "received_at": "2024-05-01T10:00:01Z" }
{ "asset_id": "show/scooby/season/1/ep4", "segment_index": 0, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg0.ts", "received_at": "2024-05-01T10:00:02Z" }
{ "asset_id": "show/scooby/season/1/ep4", "segment_index": 3, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg3.ts", "received_at": "2024-05-01T10:00:03Z" }
{ "asset_id": "show/scooby/season/1/ep4", "segment_index": 1, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg1.ts", "received_at": "2024-05-01T10:00:04Z" }
{ "asset_id": "show/scooby/season/1/ep4", "segment_index": 4, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg4.ts", "received_at": "2024-05-01T10:00:05Z" }
```

**Expected manifest:** segments ordered `[0, 1, 2, 3, 4, 5]` regardless of arrival order.

---

### Scenario 3 — Duplicate segment delivery

The transcoder re-sends segments on reconnect. The manifest must remain clean.

```json
{ "asset_id": "show/thetaste/season/1/ep2", "segment_index": 0, "cdn_url": "https://cdn.example.com/thetaste/s1e2/seg0.ts", "received_at": "2024-05-01T10:00:00Z" }
{ "asset_id": "show/thetaste/season/1/ep2", "segment_index": 1, "cdn_url": "https://cdn.example.com/thetaste/s1e2/seg1.ts", "received_at": "2024-05-01T10:00:01Z" }
{ "asset_id": "show/thetaste/season/1/ep2", "segment_index": 0, "cdn_url": "https://cdn.example.com/thetaste/s1e2/seg0.ts", "received_at": "2024-05-01T10:00:02Z" }
{ "asset_id": "show/thetaste/season/1/ep2", "segment_index": 1, "cdn_url": "https://cdn.example.com/thetaste/s1e2/seg1.ts", "received_at": "2024-05-01T10:00:03Z" }
{ "asset_id": "show/thetaste/season/1/ep2", "segment_index": 2, "cdn_url": "https://cdn.example.com/thetaste/s1e2/seg2.ts", "received_at": "2024-05-01T10:00:04Z" }
```

**Expected manifest:** exactly `[0, 1, 2]` — no duplicates.

---

### Scenario 4 — Multiple concurrent assets

Multiple unrelated assets ingesting simultaneously. Each must maintain its own independent manifest.

```json
{ "asset_id": "show/thunderbirds/season/1/ep1", "segment_index": 0, "cdn_url": "https://cdn.example.com/thunderbirds/s1e1/seg0.ts", "received_at": "2024-05-01T10:00:00Z" }
{ "asset_id": "show/scooby/season/1/ep4",       "segment_index": 0, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg0.ts",       "received_at": "2024-05-01T10:00:00Z" }
{ "asset_id": "show/theoriginals/season/1/ep1", "segment_index": 0, "cdn_url": "https://cdn.example.com/theoriginals/s1e1/seg0.ts", "received_at": "2024-05-01T10:00:00Z" }
{ "asset_id": "show/thunderbirds/season/1/ep1", "segment_index": 1, "cdn_url": "https://cdn.example.com/thunderbirds/s1e1/seg1.ts", "received_at": "2024-05-01T10:00:01Z" }
{ "asset_id": "show/scooby/season/1/ep4",       "segment_index": 1, "cdn_url": "https://cdn.example.com/scooby/s1e4/seg1.ts",       "received_at": "2024-05-01T10:00:01Z" }
{ "asset_id": "show/theoriginals/season/1/ep1", "segment_index": 1, "cdn_url": "https://cdn.example.com/theoriginals/s1e1/seg1.ts", "received_at": "2024-05-01T10:00:01Z" }
```

**Expected:** three independent manifests, each with `[0, 1]`, with no cross-contamination.

---

### Scenario 5 — Malformed request bodies

Your service must return `400 Bad Request` with a structured error body for all of these. Do not panic or return a 500.

**5a — Invalid JSON:**

```
POST /ingest
Content-Type: application/json

{ "asset_id": "show/thunderbirds", "segment_index": ]]]
```

**Expected response — `400`:**

```json
{
  "error": "Could not decode request: JSON parsing failed"
}
```

**5b — Missing required field (`cdn_url` omitted):**

```json
{
  "asset_id": "show/thunderbirds/season/1/ep1",
  "segment_index": 0,
  "received_at": "2024-05-01T10:00:00Z"
}
```

**Expected response — `400`:**

```json
{
  "error": "missing required field: cdn_url"
}
```

**5c — Wrong type (`segment_index` sent as string):**

```json
{
  "asset_id": "show/thunderbirds/season/1/ep1",
  "segment_index": "four",
  "cdn_url": "https://cdn.example.com/thunderbirds/s1e1/seg4.ts",
  "received_at": "2024-05-01T10:00:00Z"
}
```

**Expected response — `400`:**

```json
{
  "error": "Could not decode request: JSON parsing failed"
}
```

**5d — Empty body:**

```
POST /ingest
Content-Type: application/json

(empty)
```

**Expected response — `400`:**

```json
{
  "error": "Could not decode request: JSON parsing failed"
}
```

---

### Scenario 6 — Asset not found

A `GET /manifest` request for an asset that has never been ingested.

```
GET /manifest/show/nonexistent/season/1/ep99
```

**Expected response — `404`:**

```json
{
  "error": "asset not found"
}
```

---

### Scenario 7 — High-volume burst (load scenario)

This will be discussed in the follow-up session rather than tested automatically, but your README should address it.

Imagine 200 concurrent goroutines each posting segments for 500 different asset IDs simultaneously, while 50 concurrent goroutines are polling `/manifest` for random asset IDs.

- Where does your implementation break first?
- What would you change to support this load?
- What observability would you add before putting this in production?

You do not need to implement a solution to this — but you must be able to discuss it.

---

## FAQ

**What programming language should I use?**
Go. This is a Go backend tech lead role and the challenge must be submitted in Go.

**Do I need a database?**
No. In-memory state is expected and acceptable. State your assumptions about persistence clearly in your README.

**Can I use a framework like Gin or Chi?**
No. Standard library only, per the constraints above. The one-exception rule applies if genuinely justified.

**How will I be evaluated?**
Your submitted code will be reviewed before the follow-up session. In the session, you will be asked to run your tests live, walk through your design decisions, and answer extension questions about how your design scales and changes under new requirements.

**What if I run out of time?**
Leave a `TODO` comment in the code with a note explaining what you would have done and why. A well-reasoned TODO is a stronger signal than a rushed implementation.

---

## Useful Links

- [Render](https://render.com/) — free hosting for small Go services
- [GitHub](https://github.com/) — free private repositories
- [Go standard library](https://pkg.go.dev/std) — `net/http`, `sync`, `context`, `encoding/json`
- [Go race detector](https://go.dev/doc/articles/race_detector) — `go test -race ./...`
- [httptest package](https://pkg.go.dev/net/testing/httptest) — for handler testing without a live server

---

*This challenge was designed by the Stan Platform Engineering team. Questions about setup or deployment can be directed to your recruiting contact. Questions about the design of the service should be saved for the follow-up session — how you navigate ambiguity is part of what we are assessing.*
