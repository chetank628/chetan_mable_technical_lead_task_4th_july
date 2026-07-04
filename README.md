# Mable Technical Lead Engineer — Technical Task

A full-stack event tracking platform: a Go **pipeline library**, a **tracking
API** (Gin + SQLite), a standalone **browser tracker** script, and a demo
**e-commerce SPA** that exercises the entire flow end-to-end.

## Repository structure

| Path | What |
|------|------|
| `pipeline/` | Generic concurrent streaming pipeline library (Go) |
| `api/` | Gin + SQLite tracking API that ingests events through the pipeline |
| `scripts/` | Standalone, dependency-free browser tracker (`tracker.js`) |
| `ecom/` | Demo e-commerce SPA (React Router 7, React 19, Tailwind v4) |

## Quick start (all three pieces)

```bash
# 1. API (terminal A)
cd api && go run .          # http://localhost:8080

# 2. Frontend (terminal B)
cd ecom && pnpm install && pnpm dev   # http://localhost:5173
```

The tracker is loaded via `<script src="/tracker.js">` in the SPA and ships
events to the API's `POST /api/events` endpoint.

---

# pipeline/

A generic, concurrent streaming pipeline library in Go. A `Pipeline[T]` is
generic over a single element type `T` and is composed of stages. Elements flow
through **bounded channels** in **dynamically-sized batches**, and each stage
fans out across a configurable pool of **worker goroutines**. Bounded channels
make back-pressure intrinsic, so memory stays bounded at any volume.

This is the reusable library both the standalone benchmark harness and the
tracking API consume to process events on the way to the analytics DB.

## Install

```bash
go get github.com/mable/mono/pipeline
```

Requires Go 1.21+ (uses generics).

## Quick start

```go
ctx := context.Background()

// Build a pipeline over your element type.
p := pipeline.New[Event](
    pipeline.WithBatchSize(1024),
    pipeline.WithWorkerCount(8),
).
    Filter("tracked", func(e Event) bool { return e.EventType != "" }).
    Stage(pipeline.NewChurnEnrich()).                                   // custom stage
    Deduplicate("dedup", func(e Event) any { return e.SessionID }, 100_000)

// Terminate with a keyed global Reduce sink (counts per event type).
counts, metrics, err := pipeline.Reduce(ctx, p, source,
    func(e Event) string { return e.EventType }, // key
    func() int { return 0 },                      // init accumulator
    func(acc int, _ Event) int { return acc + 1 }, // fold
)
```

`source` is a `<-chan T`. Use `pipeline.FromSlice(ctx, items)` to feed a slice,
or supply your own channel (e.g. from an HTTP ingest handler).

## Stages

| Stage | Signature | Notes |
|-------|-----------|-------|
| `Map` | `func(T) T` | 1:1 transform. |
| `Filter` | `func(T) bool` | `false` drops the element (counted). |
| `Generate` | `func(T) []T` | 1:N; emits zero-or-more produced `T`. |
| `If` | `pred, then, els *Pipeline[T]` | Routes each `T` into one of two sub-pipelines; outputs merge back downstream. |
| `Deduplicate` | `key func(T) any, capacity int` | Drops repeats by key via a bounded LRU. Place between `Filter` and the sink. |
| `Stage` | `Stage[T]` | Add any custom stage (the extension protocol). |

### Sinks (terminal)

- **`Reduce[T,K,R]`** — folds the whole stream into a keyed `map[K]R`. It is
  **global** (accumulates across all batches), not per-batch, because analytics
  aggregates are only meaningful over the whole stream. It is a package function
  (not a method) because it introduces the second/third type parameters `K, R`.
  The sink is drained on a single goroutine, which makes the accumulator
  race-free without locks while still benefiting from upstream fan-out.
- **`Collect`** — drains into a caller-provided bounded `CollectSink[T]`.
  `ChannelSink[T]` is a ready-made implementation: with `Block` a full channel
  applies back-pressure upstream; with `DropOnFull` it sheds and counts the
  element.

## Configuration

All hyper-parameters are functional options on `New`:

| Option | Default | Meaning |
|--------|---------|---------|
| `WithBatchSize(n)` | 256 | Max elements per batch (also flushed by timeout). |
| `WithWorkerCount(n)` | `NumCPU()` | Fan-out goroutines per stage. |
| `WithChannelBufferDepth(n)` | 8 | Inter-stage channel buffer (in batches); 0 = unbuffered. |
| `WithBatchTimeout(d)` | 5ms | Flush a partial batch after `d`; `<=0` disables. |
| `WithErrorPolicy(p)` | `SkipAndCount` | `SkipAndCount` or `FailFast`. |
| `WithMetricsSink(s)` | nil | Receives the metrics snapshot at end of each run. |

Invalid values are clamped (e.g. `WorkerCount<1 -> 1`), so degenerate configs
still run correctly.

## Per-stage metadata

The brief requires per-stage metadata — not one timing for the whole pipeline —
including **errors and drops**, not just latency. After (or during) a run,
`Pipeline.Metrics()` returns a `[]StageMetric`, one entry per stage plus the
sink:

```go
type StageMetric struct {
    Name         string
    ItemsIn      int64
    ItemsOut     int64
    Dropped      int64
    Errors       int64
    Batches      int64
    TotalLatency time.Duration
    P50, P99     time.Duration
    Wall         time.Duration
    Throughput   float64 // items/sec
}
```

Set `WithMetricsSink` to forward this snapshot to an external store (e.g.
ClickHouse) — the seam the API caller uses to ingest pipeline metadata
alongside tracking events.

## Adding a stage (extension protocol)

Implement the single-method `Stage[T]` interface and add it with `.Stage(...)`.
No core change required. `ChurnEnrich` in `examples_stage.go` is a worked
example that enriches every `Purchase` event with a churn-probability score:

```go
type ChurnEnrich struct{ StageName string }

func (c ChurnEnrich) Name() string { return c.StageName }

func (c ChurnEnrich) Process(_ context.Context, in []Event) ([]Event, int, error) {
    out := make([]Event, len(in))
    for i, e := range in {
        if e.EventType == "Purchase" {
            e.ChurnProbability = churnScore(e)
        }
        out[i] = e
    }
    return out, 0, nil // out, dropped, err
}

p.Stage(pipeline.NewChurnEnrich())
```

`Process` may be called concurrently across workers; stages that hold mutable
state must synchronise it (see `dedupStage`). A panic inside a stage is
recovered, converted to a stage error, and counted — it never crashes a worker.

## Design highlights

- **Typing model:** homogeneous `Pipeline[T]`; the only second type appears in
  `Reduce[T,K,R]`. Compile-time safety and an allocation-free hot path, at the
  cost of changing element type mid-stream (model variants inside `T`).
- **Concurrency:** bounded channels + per-stage worker pools, batched hand-off.
  Back-pressure is intrinsic; memory is bounded at any volume.
- **Ordering:** not preserved across a stage's fan-out workers (documented).
  Order-sensitive logic (keyed Reduce) is order-independent by construction.
- **Deduplicate:** bounded LRU seen-set → bounded memory. Trade-off: after
  eviction a repeated key is a false negative. `capacity = 0` is exact/unbounded.

## Tests

```bash
go test -race ./...   # race-clean, includes goroutine-leak assertions
go vet ./...
```

Covered edge cases: empty input, single element, filter-drops-everything,
generate explosion, stage error (skip vs fail-fast), recovered panic, context
cancellation (no goroutine leak), bounded-dedup false negative, collect
block vs drop-on-full back-pressure, batch-timeout flush, and the degenerate
config (unbuffered channels, 1 worker, batch size 1).

## Benchmarks

```bash
go test -run=NONE -bench=. -benchmem
```

Benchmarks run at **10 / 1k / 100k / 1M** events for two payloads: the fixed
`TestStruct` (10 mixed pinned fields) and the sample Mable event loaded from
`testdata/sample_event.json`. Default cap is **1M events** to stay laptop-safe.

**Method:** each iteration builds a fresh pipeline (`Filter → enrich/transform →
Deduplicate → keyed global Reduce`) and runs the whole input slice through it.
`events/sec` is reported as a custom metric. Input slices are built outside the
timed loop. We judge methodology, not absolute throughput.

**Machine:** Apple M1, 8 cores, 8 GB RAM, macOS, Go 1.21 (`darwin/arm64`).

Representative results (`-benchtime=3x`; throughput varies with batch/worker
config and machine):

| Payload | n | ns/op | events/sec | allocs/op |
|---------|---|-------|------------|-----------|
| TestStruct | 10 | 102,181 | ~98k | 164 |
| TestStruct | 1,000 | 662,014 | ~1.5M | 1,023 |
| TestStruct | 100,000 | 29.4M | ~3.4M | 101,840 |
| TestStruct | 1,000,000 | 345M | ~2.9M | 1,009,915 |
| Mable event | 10 | 82,625 | ~121k | 160 |
| Mable event | 1,000 | 677,736 | ~1.5M | 3,162 |
| Mable event | 100,000 | 34.4M | ~2.9M | 205,746 |
| Mable event | 1,000,000 | 317M | ~3.2M | 2,009,341 |

## File layout

| File | Contents |
|------|----------|
| `pipeline.go` | `Pipeline[T]`, builder DSL, runtime (batcher, stage runners, sink drain). |
| `stage.go` | `Stage[T]` interface + Map/Filter/Generate/Deduplicate. |
| `route.go` | `If[T]` routing + sub-pipeline merge. |
| `reduce.go` | `Reduce[T,K,R]` keyed global sink + `Collect`/`ChannelSink`. |
| `metrics.go` | `StageMetric`, merge/percentiles, `MetricsSink` seam. |
| `config.go` | Functional options + defaults. |
| `examples_stage.go` | `Event` type + `ChurnEnrich` example custom stage. |
| `*_test.go` | Unit + edge-case tests, benchmarks, `TestStruct`. |
| `testdata/sample_event.json` | Real, valid Mable event JSON. |

---

# api/

A Gin + SQLite Go service that ingests tracking events through the
`github.com/mable/mono/pipeline` library on a long-running **windowed** pipeline,
persists events and per-stage pipeline metadata, serves cookie-based JWT auth,
and computes analytics.

See [`api-design.md`](./api/api-design.md) for the full design rationale.

## Run

```bash
cd api
go run .
# listens on :8080 (dev mode, SQLite file ./mable.db)
```

### Configuration (env vars)

| Var | Default | Purpose |
| --- | --- | --- |
| `APP_ENV` | `dev` | `dev`/`test`/`prod`; gates cookie Secure/SameSite and JWT-secret requirement |
| `ADDR` | `:8080` | HTTP listen address |
| `DB_PATH` | `mable.db` | SQLite file (`:memory:` for tests) |
| `JWT_SECRET` | dev fallback | **Required in prod** |
| `JWT_TTL` | `24h` | Session lifetime |
| `CORS_ORIGIN` | `http://localhost:5173` | Exact SPA origin (no `*` with credentials) |
| `MAX_BODY_BYTES` | `1048576` | Ingest body size cap (413 over) |
| `INGEST_BUFFER_DEPTH` | `8192` | Bounded ingest channel capacity |
| `WINDOW_DURATION` | `500ms` | Max time per pipeline window |
| `WINDOW_MAX_EVENTS` | `2048` | Max events per pipeline window |
| `PIPELINE_BATCH_SIZE` | `256` | `pipeline.WithBatchSize` |
| `PIPELINE_WORKERS` | `0` (NumCPU) | `pipeline.WithWorkerCount` |
| `PIPELINE_CHANNEL_BUFFER` | `8` | `pipeline.WithChannelBufferDepth` |
| `PIPELINE_BATCH_TIMEOUT` | `5ms` | `pipeline.WithBatchTimeout` |
| `DEDUP_CAPACITY` | `200000` | Dedup LRU size |

## Endpoints

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| `POST` | `/api/events` | no | Ingest one event or an array; returns `202` |
| `POST` | `/api/auth/signup` | no | Create account, set session cookie |
| `POST` | `/api/auth/login` | no | Login, set session cookie |
| `POST` | `/api/auth/logout` | no | Clear session cookie |
| `GET` | `/api/auth/me` | cookie | Current user |
| `GET` | `/api/stats` | cookie | Analytics (`?since=&until=&granularity=`) |
| `GET` | `/health` | no | Liveness/readiness (DB ping) |
| `GET` | `/metrics` | no | Prometheus-format ingest counters |

## Quick demo

```bash
# ingest (no auth) — consent gate requires consent:true
curl -s -XPOST localhost:8080/api/events -H 'Content-Type: application/json' \
  -d '{"event_type":"Purchase","session_id":"s1","user_id":"u1","amount":42,"currency":"usd","consent":true}'

# signup -> stores cookie in jar
curl -s -c jar.txt -XPOST localhost:8080/api/auth/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"supersecret"}'

# stats (uses cookie)
curl -s -b jar.txt 'localhost:8080/api/stats?granularity=minute'

# ops
curl -s localhost:8080/health
curl -s localhost:8080/metrics
```

## Test

```bash
cd api
go test -race ./...
```

---

# scripts/ — Mable Tracker (`tracker.js`)

A standalone, dependency-free, framework-agnostic web tracking script. It
collects browser-side analytics events and ships them to the Mable ingest API
(`POST /api/events`) using resilient, non-blocking transport.

It works on any site — load it with a `<script src>` tag, no build step.

## Quick start

```html
<script src="/tracker.js"></script>
<script>
  window.mable.init({
    endpoint: "http://localhost:8080/api/events",
    consent: false, // start with consent off; call setConsent(true) on accept
    debug: true,    // optional: console.debug tracing
  });
</script>
```

### Data layer (load-order safe)

You can queue commands before the script loads, Google-Tag-Manager style:

```html
<script>
  window.mableDataLayer = window.mableDataLayer || [];
  window.mableDataLayer.push(["identify", "user-123"]);
  window.mableDataLayer.push(["track", "Lead", { source: "hero" }]);
</script>
<script src="/tracker.js"></script>
<script>
  window.mable.init({ endpoint: "http://localhost:8080/api/events" });
</script>
```

`init()` drains the queue, then future `push()` calls execute immediately.

## API

| Method | Description |
| --- | --- |
| `init({ endpoint, consent?, batchSize?, flushIntervalMs?, debug?, userId? })` | Initialize. Patches history for SPA page views, binds click + lifecycle listeners, replays any persisted retry queue. |
| `identify(userId)` | Associate subsequent events with a user id. |
| `setConsent(granted: boolean)` | Grant/revoke consent. On grant, buffered + persisted events are flushed. |
| `track(type, props)` | Track a whitelisted event. Non-whitelisted types are dropped. |
| `pageView()` | Manually emit a `PageView`. |
| `addToCart(props)` / `checkout(props)` / `paymentInfoAdded(props)` / `purchase(props)` / `lead(props)` / `click(props)` | Convenience helpers for the canonical commerce events. |

### Whitelisted event types

The backend ingests only these (others are dropped at the API filter stage):

`PageView, Click, AddToCart, Checkout, PaymentInfoAdded, Purchase, Lead`

### Special property keys

- `amount` → promoted to the top-level `amount` field (number).
- `currency` → promoted to the top-level `currency` field (string).
- Everything else in `props` is **coerced to a string** and placed in
  `properties` (the API requires `map[string]string`). Objects/arrays are
  JSON-stringified automatically.

```js
window.mable.purchase({
  amount: 129.99,
  currency: "USD",
  order_id: "ORD-1001",
  items: [{ id: 42, qty: 2 }], // becomes a JSON string in properties.items
});
```

## Auto-tracked events

- **PageView** — on initial load, and on SPA route changes. The script patches
  `history.pushState`/`replaceState` and listens to `popstate`, so client-side
  navigation in React Router / Remix emits page views correctly. Duplicate
  consecutive paths are de-duped.
- **Click** — delegated listener for elements with a `data-track` attribute:

  ```html
  <button data-track="AddToCart" data-track-id="42" data-track-name="Sneaker">
    Add to cart
  </button>
  ```

  All `data-track-*` attributes are captured into `properties`. If the
  `data-track` value is itself a whitelisted type (e.g. `AddToCart`), that type
  is used; otherwise it falls back to `Click`.

## Transport & resilience

- **Non-blocking**: every public method is wrapped in `try/catch` and never
  throws into the host UI.
- **Batching**: events are buffered and flushed when `batchSize` (default 10)
  is reached or after `flushIntervalMs` (default 5s).
- **Unload-safe**: flushes via `navigator.sendBeacon` on `visibilitychange`
  (hidden) and `pagehide`; falls back to `fetch(..., { keepalive: true })`.
- **Credentialed**: fetches use `credentials: "include"` to match the API's
  CORS + cookie auth.
- **Offline retry**: failed sends (network down / 5xx) are persisted to
  `localStorage` and replayed on the `online` event and on next `init()`. The
  persisted queue is byte-capped (oldest dropped) to avoid unbounded growth.
- **Non-retryable handling**: `400` (malformed) and `413` (too large) responses
  are dropped rather than retried.
- **Consent gating**: with consent off, events are buffered/held and never
  transmitted; the backend also drops any event missing `consent: true`.

## Browser vs. server data split

| Field | Source | Why |
| --- | --- | --- |
| `user_agent` | Browser (`navigator.userAgent`) | Available client-side. |
| `language`, `timezone`, `screen`, `viewport` | Browser | Only the client knows these. |
| `referrer`, `url` | Browser | Page context. |
| `session_id` | Browser (`sessionStorage`) | Per-tab session. |
| `user_id` | Browser (via `identify()`) | App-supplied identity. |
| `timestamp` | Browser (`Date.toISOString()`, RFC3339) | Event time. |
| **IP address** | **Server** (`c.ClientIP()`) | Never trust a client-sent IP. |
| **Server receipt time** | **Server** (`_received_at`) | Authoritative ingest time. |
| **Geo (country/region)** | **Server** (derived from IP) | Requires server-side IP lookup. |

The browser deliberately does **not** send an IP or geo — those are derived
server-side from the connection, which is the only trustworthy source.

## Wire shape (matches the Go API `IngestEvent`)

```json
{
  "event_type": "AddToCart",
  "user_id": "user-123",
  "session_id": "b1f3...",
  "timestamp": "2026-06-28T12:34:56.789Z",
  "url": "http://localhost:5173/product/42",
  "referrer": "http://localhost:5173/",
  "user_agent": "Mozilla/5.0 ...",
  "amount": 0,
  "currency": "",
  "properties": { "id": "42", "name": "Sneaker", "timezone": "Europe/London" },
  "consent": true
}
```

The API accepts either a single object or an array (the tracker always sends an
array batch) and responds `202 { "accepted": n, "dropped": m }`.

---

# ecom/ — Mable Shop

A demo e-commerce SPA built with **React Router 7 (Remix) in SPA mode**, React
19, Zustand, Tailwind v4 and Vite. It exercises the Mable tracking pipeline
end-to-end: auth via the Go API, a consent-gated tracker, and a live analytics
dashboard reading `/api/stats`.

## Stack

- **React Router 7** (framework mode, `ssr: false`) on **Vite** — serves on
  `:5173` to match the API's CORS allow-list.
- **React 19**, **TypeScript**, **Zustand** (state), **Tailwind CSS v4**,
  **React Icons**.
- **pnpm** for dependency management.

## Prerequisites

- Node 20+, pnpm 8+
- The Go API running on `:8080` (see `../api`).

## Run locally (all three pieces)

```bash
# 1. API (terminal A)
cd api && go run .

# 2. Frontend (terminal B)
cd ecom && pnpm install && pnpm dev      # http://localhost:5173
```

The standalone tracker is served from `ecom/public/tracker.js` (a copy of
`../scripts/tracker.js`) and loaded via a `<script src>` tag in `app/root.tsx`,
exactly as any third-party site would embed it.

### Environment

`.env` (already present):

```
VITE_API_BASE=http://localhost:8080
VITE_TRACKER_ENDPOINT=http://localhost:8080/api/events
```

## Verifying the full journey

1. Open `http://localhost:5173` and **Accept tracking** in the consent banner.
2. **Sign up** (`/signup`) — email with `@`, password ≥ 8 chars.
3. **Browse** the product grid (DummyJSON), search/filter, open a product.
4. **Add to cart**, adjust quantities in `/cart`.
5. **Checkout** (`/checkout`) — multi-step shipping → payment → review → place
   order → success → redirect home.
6. Open **Stats** (`/dashboard`) to see events land via `GET /api/stats`
   (cookie-gated), or check raw pipeline metrics at
   `http://localhost:8080/metrics`.

Events emitted along the way: `PageView` (auto, on every route change),
`Lead` (signup/login), `AddToCart`, `Checkout`, `PaymentInfoAdded`, `Purchase`.

## Architecture

```
app/
  root.tsx              # HTML shell, loads /tracker.js, root error boundary
  routes.ts             # route config (SPA)
  routes/
    _shell.tsx          # header + consent banner + toaster + session restore
    home.tsx            # product library (search/filter/add-to-cart)
    product.tsx         # product detail
    cart.tsx            # quantities, remove, totals
    checkout.tsx        # multi-step, auth-gated, tracks each step
    dashboard.tsx       # /api/stats analytics
    login.tsx / signup.tsx
  stores/
    authStore.ts        # user from /auth/me (never the JWT)
    cartStore.ts        # persisted to localStorage (survives auth/refresh)
    uiStore.ts          # consent (persisted) + toasts
  lib/
    api.ts              # credentialed API client + DummyJSON catalog
    tracker.ts          # typed wrapper around window.mable
    types.ts            # shared DTOs (mirror the Go API)
  components/           # Header, ConsentBanner, Toaster, states
  hooks/useRequireAuth  # protected-route redirect
```

## How the contracts are honoured

- **Auth**: every fetch uses `credentials: "include"`; the JWT lives only in the
  `HttpOnly` `mable_session` cookie and is never read by JS. Session is restored
  on load via `GET /auth/me`.
- **Properties are strings**: the tracker coerces every `properties` value to a
  string (objects → JSON), matching the API's `map[string]string`.
- **Consent**: nothing is transmitted with `consent:true` until the user
  accepts; the backend also drops events lacking consent.
- **Tracked types only**: `PageView, Click, AddToCart, Checkout,
  PaymentInfoAdded, Purchase, Lead`.

## Edge cases covered

- **API down / network failure** — tracker queues to `localStorage` and replays
  on `online`; UI shows error states with retry, never blocks.
- **Oversized/invalid payloads** — `413`/`400` responses are dropped (not
  retried) by the tracker; the API client surfaces a readable message.
- **Consent not granted** — no events sent; held in the tracker queue.
- **Session expiry / 401** — protected routes redirect to `/login?next=…`; the
  **cart is preserved** (separate localStorage store).
- **Refresh mid-checkout** — session re-restored via `/auth/me`, cart restored
  from localStorage.
- **Duplicate rapid clicks** — server-side dedup; add-to-cart is idempotent per
  product (increments quantity).
- **Empty cart** — checkout is blocked with an empty state; empty product API
  result shows an empty state.
- **SPA route changes** — the tracker patches `history.pushState`/`popstate`,
  so client-side navigation emits `PageView` correctly.

## Scripts

| Command | Description |
| --- | --- |
| `pnpm dev` | Start the dev server on `:5173`. |
| `pnpm build` | Production build (SPA → `build/client/`). |
| `pnpm start` | Preview the production build on `:5173`. |
| `pnpm typecheck` | Run typegen + `tsc`. |

> Scope: build + run locally. Cloud deployment is out of scope this round.
