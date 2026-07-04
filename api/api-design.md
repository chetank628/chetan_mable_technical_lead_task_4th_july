# Mable Tracking API — Design

A Go (Gin) service that ingests tracking events, streams every event through the
existing `pipeline.Pipeline[Event]` library, persists pipeline output and
per-stage metadata to SQLite, serves cookie-based JWT auth, and computes the
required analytics.

## 1. Architecture

```
        ┌─────────┐   POST /api/events     ┌──────────────────────────────┐
        │   SPA   │ ─────────────────────▶ │ Gin handler (202 immediately) │
        │ (ecom)  │ ◀───────────────────── │  - validate, size-limit       │
        └─────────┘   GET /api/stats       │  - stamp ip/ua/received_at    │
             ▲                              │  - Submit() non-blocking       │
             │ JSON                         └───────────────┬───────────────┘
             │                                              │ bounded channel
             │                                              ▼
        ┌────┴────────────┐         ┌──────────────────────────────────────┐
        │  /api/stats     │◀────────│        ingest.Worker (1 goroutine)     │
        │  /metrics       │  reads  │  windowed runs of pipeline.Pipeline    │
        │  /health        │         │  Filter→Map→Filter→Map→Churn→Dedup     │
        └────┬────────────┘         │  Collect ─▶ eventSink (buffers window) │
             │                      └───────────────┬──────────────────────┘
             │ read-only (WAL)                      │ per-window flush
             ▼                                      ▼
        ┌─────────────────────────────────────────────────────────────────┐
        │                          SQLite (WAL)                            │
        │   users        events        stage_metrics                       │
        └─────────────────────────────────────────────────────────────────┘
```

The HTTP layer never touches the pipeline directly: it only `Submit`s onto a
bounded channel. One background worker owns the pipeline and the database
writes, so the write path is single-threaded (race-free accumulation) while
read-only stats queries run concurrently under WAL.

## 2. Key decision: the windowed pipeline

**Problem.** The library flushes its `[]StageMetric` exactly once per
`execute()` call (i.e. per `Collect`/`Reduce`). The brief requires *per-stage*
metadata (errors, drops, latency percentiles) to be observable while the service
runs. A single perpetual run would only ever flush metrics at shutdown.

**Chosen solution — windowed runs.** The worker drains the shared ingest channel
into one **bounded pipeline run per window**. A window closes when it reaches
`WINDOW_MAX_EVENTS` *or* `WINDOW_DURATION`, whichever comes first. When the run
completes it flushes (a) the window's events in one transaction and (b) the
per-stage metrics for that window into `stage_metrics`. Then the next window
starts. This yields:

- one *logical* ingest stream with intrinsic back-pressure (bounded channel),
- per-stage metadata in the store every window (sub-second by default), and
- bounded memory: at most one window's events are in flight.

**Rejected alternatives.**

- *Perpetual single run.* Simplest wiring, but metrics never flush until
  shutdown, failing the per-stage-metadata requirement. Also makes back-pressure
  observability and incremental persistence awkward.
- *Per-request run.* Spinning a pipeline per HTTP request gives instant metrics
  but destroys batching/fan-out economics, resets the dedup LRU every request,
  and adds goroutine churn under load — the opposite of a streaming design.

**Trade-off accepted.** The dedup LRU is per-window, so a replayed beacon that
straddles two windows is not de-duplicated. Replays almost always arrive close
together, so a reasonably sized window catches them; the dedup key also includes
the event minute to bound semantic duplicates.

## 3. Pipeline wiring (same library as the benchmark)

The API mirrors `benchEventPipeline` from the library's own bench so the API and
the benchmark are two callers of one library. Stage order (in
`internal/ingest/pipeline.go`):

1. `Filter("consent")` — drop events without the consent flag (PII/consent gate).
2. `Map("normalize")` — trim, canonicalise `event_type` to PascalCase, default a
   zero timestamp to now, clamp oversized URL/properties.
3. `Filter("tracked")` — whitelist known event types.
4. `Map("enrich_geo")` — synthetic geo/timezone from IP, device/browser from UA
   (no external calls).
5. `Stage(NewChurnEnrich())` — the library's example custom stage; demonstrates
   the open extension protocol on `Purchase` events.
6. `Deduplicate("dedup")` — bounded LRU over `session|type|minute`.
7. `Collect` into `eventSink` → SQLite.

> Note: normalisation is placed *before* the `tracked` filter and `ChurnEnrich`
> so those stages compare against a single canonical type spelling. This is a
> deliberate, correctness-driven deviation from a naive `tracked`-first order.

Control data (consent flag, server `received_at`) is threaded through the
event's `Properties` map under underscore-prefixed keys, so the pipeline stays
homogeneous in `Event` without changing `T`. These keys are stripped before the
event is persisted.

## 4. Typing model defence

`Pipeline[T]` is homogeneous: a stage is `func(T) -> T`-ish. This trades the
ability to change element type mid-stream for compile-time safety and an
allocation-free hot path. The only second type appears in the `Reduce[T,K,R]`
sink, which is why analytics aggregates that *do* change type (counts/sums by
key) are modelled as a sink, not a chainable stage. The API reuses
`pipeline.Event` directly as `T` and carries cross-cutting fields inside the
struct (and its `Properties`), exactly the pattern the library recommends.

For the API's analytics we deliberately compute aggregates in **SQL** rather than
with `Reduce`: the data must be queryable over arbitrary time ranges after the
fact, which is a database concern, while `Reduce` is an in-memory per-run fold.
`Reduce` remains the right tool for the benchmark's throughput counting.

## 5. Auth & cross-origin model

- Passwords hashed with **bcrypt** (default cost).
- On signup/login a **HS256 JWT** is issued and stored in an **HttpOnly**,
  `SameSite`, `Path=/` cookie. In prod the cookie is `Secure` + `SameSite=None`
  (cross-site SPA); locally it is non-Secure + `SameSite=Lax`.
- The JWT is **never** exposed to JavaScript, defeating XSS token theft.
- CORS is locked to the exact SPA origin with `Allow-Credentials: true`; a
  wildcard origin with credentials is rejected at config load in prod (browsers
  reject it anyway).
- Login returns a generic `401` for both unknown email and wrong password (no
  user enumeration), and runs a dummy bcrypt compare for unknown users to blunt
  timing side-channels. A missing `JWT_SECRET` in prod is a boot failure.

## 6. Back-pressure & overload

The ingest channel is bounded (`INGEST_BUFFER_DEPTH`). `Submit` is
**non-blocking**: if the buffer is full the event is dropped and counted
(`mable_events_dropped_total`), never blocking the HTTP handler — tracking must
not stall the UI (freshness over completeness). Within a window the library's
bounded inter-stage channels provide intrinsic back-pressure between stages.

## 7. Storage: SQLite trade-offs

SQLite was chosen for **zero-infra embedding** (single file, no server) which
suits a take-home and small/medium volumes. WAL mode + a single writer
connection give concurrent reads with the single-goroutine writer.

- *vs ClickHouse / a columnar OLAP store:* ClickHouse is the right destination
  at real analytics scale (the library even names it as the `MetricsSink`
  target). The `store` package is a thin seam — swapping the implementation for
  ClickHouse/Postgres is the documented scaling path. SQLite's single-writer
  limit and lack of columnar compression are the accepted trade-offs here.

Schema: `users`, `events` (indexed on `ts` and `(event_type, ts)`),
`stage_metrics` (indexed on `window_ts`). Pragmas: `journal_mode=WAL`,
`synchronous=NORMAL`, `busy_timeout=5000`.

CGO-free `modernc.org/sqlite` is used so the binary builds and tests run without
a C toolchain.

## 8. Analytics (`GET /api/stats`)

Computed in SQL over `[since, until]` with `granularity` minute|hour|day:

- **avg event capture time** — `AVG(capture_ms)` (server `received_at` → persist).
- **avg event params** — `AVG(json_array length of properties)` via `json_each`.
- **events over time** — count per time bucket.
- **per-type counts over time** — count per `(bucket, event_type)`, plus a flat
  `type_counts` map.
- **per-stage rollups** — `stage_metrics` aggregated by stage across windows
  (items in/out, dropped, errors, avg p50/p99, avg throughput).

## 9. Failure modes handled

- **Oversized/malformed payloads** — `MaxBytesReader` → `413`; bad JSON → `400`;
  per-event validation lives in the pipeline `Map`/`Filter` stages (counted as
  drops, never crashes).
- **Stage panic** — the library recovers panics into stage errors; under
  `SkipAndCount` the stream survives and the error is counted in `stage_metrics`
  and surfaced via `/metrics`.
- **Empty/low-volume windows** — `BatchTimeout` + the window timer prevent
  stalls; empty windows are skipped to avoid metric noise.
- **DB write failure** — logged and counted; the worker keeps running so a
  transient error does not kill ingestion.
- **Graceful shutdown** — SIGINT/SIGTERM → stop HTTP, close ingest channel,
  flush the final window (on a background context so shutdown cancellation does
  not abort persistence of already-accepted data), close DB.
- **Clock skew** — capture-time stats use server `received_at`; the client
  timestamp is stored separately as `ts`.

## 10. Scaling path

1. Swap the `store` implementation from SQLite to Postgres/ClickHouse (interface
   already isolates SQL).
2. Forward `stage_metrics` to Prometheus/ClickHouse via the library's
   `WithMetricsSink` seam instead of (or in addition to) the DB.
3. Horizontal scale: multiple API instances behind a load balancer, each with
   its own worker, all writing to the shared analytics store; ingest channel
   becomes a real broker (Kafka/NATS) if cross-instance ordering/durability is
   needed.

## 11. PII / consent

Events without consent are dropped at the first stage and never persisted. Only
whitelisted event types are stored. IP is retained for synthetic geo enrichment;
in a real deployment it would be hashed/truncated per policy. The internal
control keys (`_consent`, `_received_at`) are stripped before persistence.

## 12. How AI tooling was used

AI assistance (Cascade) was used to scaffold the package layout, draft the
boilerplate (config parsing, SQL CRUD, Gin handlers, tests) and this write-up,
working from the agreed plan. The pipeline-integration design (windowed runs to
reconcile the once-per-run metrics flush), the stage ordering correction, the
back-pressure/drop policy, and the auth/CORS model were the human-reviewed
decisions; generated code was verified with `go build`, `go vet`, and
`go test -race`.
