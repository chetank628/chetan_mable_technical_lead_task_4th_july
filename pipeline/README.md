# pipeline

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
