// Package pipeline is a generic, concurrent streaming pipeline library.
//
// A Pipeline[T] is generic over a single element type T and is composed of
// stages. Elements flow through bounded channels in dynamically-sized batches;
// each stage fans out across a configurable pool of worker goroutines. Bounded
// channels make back-pressure intrinsic, so memory stays bounded at any volume.
//
// Typing model: the pipeline is homogeneous in T (a stage is func(T) -> T-ish).
// The only place a second type appears is the Reduce sink (Reduce[T,K,R]). This
// trades the ability to change element type mid-stream for compile-time safety
// and an allocation-free hot path. Cross-type transforms are modelled by making
// T a struct/sum that carries variants.
//
// Ordering is NOT preserved across a stage's fan-out workers. Stages that need
// determinism (Reduce keyed aggregation) are order-independent by construction.
package pipeline

import (
	"context"
	"sync"
	"time"
)

// Pipeline is a generic streaming pipeline over element type T. Build it with
// New and the fluent stage methods, then terminate it with Collect or Reduce.
type Pipeline[T any] struct {
	cfg    Config
	stages []Stage[T]

	mu      sync.Mutex
	metrics []StageMetric
}

// New creates a pipeline with the given options applied over the defaults.
func New[T any](opts ...Option) *Pipeline[T] {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	cfg.normalize()
	return &Pipeline[T]{cfg: cfg}
}

// Map appends a stage that transforms each T into a T.
func (p *Pipeline[T]) Map(name string, fn func(T) T) *Pipeline[T] {
	return p.Stage(mapStage[T]{name: name, fn: fn})
}

// Filter appends a stage that drops elements for which pred returns false.
func (p *Pipeline[T]) Filter(name string, pred func(T) bool) *Pipeline[T] {
	return p.Stage(filterStage[T]{name: name, pred: pred})
}

// Generate appends a 1->N stage that emits zero-or-more produced T per element.
func (p *Pipeline[T]) Generate(name string, fn func(T) []T) *Pipeline[T] {
	return p.Stage(generateStage[T]{name: name, fn: fn})
}

// Deduplicate appends a stage that drops repeat elements by a caller-supplied
// key. capacity bounds the LRU seen-set (0 = unbounded/exact). The key's
// concrete return type must be comparable; a non-comparable key panics, which
// the runtime recovers into a stage error.
//
// Position this between Filter and Reduce so only surviving, relevant events are
// considered for de-duplication.
func (p *Pipeline[T]) Deduplicate(name string, key func(T) any, capacity int) *Pipeline[T] {
	return p.Stage(newDedupStage[T](name, key, capacity))
}

// If appends a routing stage: each element goes to the then or els sub-pipeline
// based on pred, and both outputs merge back downstream. Only the linear stages
// of the sub-pipelines are used.
func (p *Pipeline[T]) If(name string, pred func(T) bool, then, els *Pipeline[T]) *Pipeline[T] {
	var thenStages, elseStages []Stage[T]
	if then != nil {
		thenStages = then.stages
	}
	if els != nil {
		elseStages = els.stages
	}
	return p.Stage(ifStage[T]{name: name, pred: pred, then: thenStages, els: elseStages})
}

// Stage appends any custom Stage[T]. This is the open extension protocol:
// implement Stage[T] and add it here without touching the core.
func (p *Pipeline[T]) Stage(s Stage[T]) *Pipeline[T] {
	p.stages = append(p.stages, s)
	return p
}

// Metrics returns the per-stage metadata snapshot from the most recent run.
func (p *Pipeline[T]) Metrics() []StageMetric {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]StageMetric, len(p.metrics))
	copy(out, p.metrics)
	return out
}

// FromSlice returns a channel fed by items on a background goroutine. The feeder
// respects ctx cancellation so it never leaks.
func FromSlice[T any](ctx context.Context, items []T) <-chan T {
	ch := make(chan T)
	go func() {
		defer close(ch)
		for _, v := range items {
			select {
			case ch <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// execute wires up the batcher, stage runners, and sink, then runs to
// completion. It is the single orchestration path shared by Collect and Reduce.
func (p *Pipeline[T]) execute(ctx context.Context, source <-chan T, sink Sink[T]) ([]StageMetric, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		errOnce sync.Once
		runErr  error
	)
	setErr := func(err error) {
		errOnce.Do(func() { runErr = err })
	}

	buf := p.cfg.ChannelBufferDepth

	// Batcher feeds the first channel with dynamically-sized batches.
	head := make(chan []T, buf)
	go batcher(ctx, source, head, p.cfg.BatchSize, p.cfg.BatchTimeout)

	// Each linear stage gets a runner reading prev, writing next.
	var (
		wg      sync.WaitGroup
		metrics = make([]StageMetric, len(p.stages)+1)
		prev    = head
	)
	for i, st := range p.stages {
		next := make(chan []T, buf)
		wg.Add(1)
		go func(idx int, stage Stage[T], in <-chan []T, out chan []T) {
			defer wg.Done()
			metrics[idx] = runStage(ctx, cancel, stage, in, out, p.cfg, setErr)
		}(i, st, prev, next)
		prev = next
	}

	// Drain the sink on this goroutine (single-threaded => race-free accumulator).
	sinkMetric := drainSink(ctx, cancel, sink, prev, p.cfg, setErr)
	metrics[len(p.stages)] = sinkMetric

	wg.Wait()

	p.mu.Lock()
	p.metrics = metrics
	p.mu.Unlock()

	if p.cfg.MetricsSink != nil {
		if err := p.cfg.MetricsSink.Record(ctx, metrics); err != nil && runErr == nil {
			runErr = err
		}
	}
	return metrics, runErr
}

// batcher accumulates source elements into batches of up to size, flushing a
// partial batch after timeout (if > 0) so low-volume streams do not stall.
func batcher[T any](ctx context.Context, source <-chan T, out chan<- []T, size int, timeout time.Duration) {
	defer close(out)

	batch := make([]T, 0, size)
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		select {
		case out <- batch:
			batch = make([]T, 0, size)
			return true
		case <-ctx.Done():
			return false
		}
	}

	var (
		timer   *time.Timer
		timerCh <-chan time.Time
	)
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timerCh = timer.C
		defer timer.Stop()
	}

	for {
		select {
		case v, ok := <-source:
			if !ok {
				flush()
				return
			}
			batch = append(batch, v)
			if len(batch) >= size {
				if !flush() {
					return
				}
				if timer != nil {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(timeout)
				}
			}
		case <-timerCh:
			if !flush() {
				return
			}
			timer.Reset(timeout)
		case <-ctx.Done():
			return
		}
	}
}

// runStage runs a single stage with a fan-out worker pool, accumulating
// per-worker stats that are merged into one StageMetric at the end.
func runStage[T any](
	ctx context.Context,
	cancel context.CancelFunc,
	stage Stage[T],
	in <-chan []T,
	out chan []T,
	cfg Config,
	setErr func(error),
) StageMetric {
	defer close(out)
	start := time.Now()

	workers := cfg.WorkerCount
	stats := make([]workerStat, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ws := &stats[id]
			for batch := range in {
				t0 := time.Now()
				outBatch, dropped, err := safeProcess(ctx, stage, batch)
				ws.latencies = append(ws.latencies, time.Since(t0))
				ws.batches++
				ws.in += int64(len(batch))
				ws.dropped += int64(dropped)

				if err != nil {
					ws.errs++
					if cfg.OnError == FailFast {
						setErr(err)
						cancel()
						return
					}
					continue // SkipAndCount: drop this batch's output.
				}

				if len(outBatch) > 0 {
					ws.out += int64(len(outBatch))
					select {
					case out <- outBatch:
					case <-ctx.Done():
						return
					}
				}
			}
		}(w)
	}
	wg.Wait()

	return mergeStageMetric(stage.Name(), stats, time.Since(start))
}

// drainSink consumes the final channel into the sink on the calling goroutine.
func drainSink[T any](
	ctx context.Context,
	cancel context.CancelFunc,
	sink Sink[T],
	in <-chan []T,
	cfg Config,
	setErr func(error),
) StageMetric {
	start := time.Now()
	ws := workerStat{}

	for batch := range in {
		t0 := time.Now()
		dropped, err := sink.Consume(ctx, batch)
		ws.latencies = append(ws.latencies, time.Since(t0))
		ws.batches++
		ws.in += int64(len(batch))
		ws.dropped += int64(dropped)
		ws.out += int64(len(batch) - dropped)

		if err != nil {
			ws.errs++
			if cfg.OnError == FailFast {
				setErr(err)
				cancel()
				// Keep draining so upstream goroutines unblock and exit.
			}
		}
	}

	return mergeStageMetric(sink.Name(), []workerStat{ws}, time.Since(start))
}
