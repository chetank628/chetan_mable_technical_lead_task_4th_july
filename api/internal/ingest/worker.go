// Package ingest owns the long-running, windowed pipeline that turns submitted
// tracking events into persisted rows and per-stage metrics.
//
// Why windows? The pipeline library flushes its []StageMetric once per
// execute() call. A single perpetual run would only flush at shutdown, so
// per-stage metadata (the brief's requirement) would never reach the store
// while the service is up. We therefore run one bounded pipeline per window:
// the worker drains the shared ingest channel into a per-window source until
// the window fills (WINDOW_MAX_EVENTS) or expires (WINDOW_DURATION), then lets
// that run complete — flushing events and metrics — before starting the next.
// One logical ingest stream, intrinsic back-pressure, metrics every window.
package ingest

import (
	"context"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mable/mono/api/internal/config"
	"github.com/mable/mono/api/internal/store"
	"github.com/mable/mono/pipeline"
)

// Worker is the long-running ingest engine. Construct with New, start with
// Run, submit events with Submit, and stop with Close.
type Worker struct {
	cfg   config.Config
	store *store.Store
	ch    chan pipeline.Event

	// counters are read by the /metrics endpoint; all updated atomically.
	submitted     atomic.Int64
	accepted      atomic.Int64
	dropped       atomic.Int64
	persisted     atomic.Int64
	windows       atomic.Int64
	lastWindowNs  atomic.Int64
	pipelineErrs  atomic.Int64

	closeOnce sync.Once
	done      chan struct{}
}

// New creates an ingest Worker backed by st. The bounded channel capacity comes
// from cfg.IngestBufferDepth.
func New(cfg config.Config, st *store.Store) *Worker {
	return &Worker{
		cfg:   cfg,
		store: st,
		ch:    make(chan pipeline.Event, cfg.IngestBufferDepth),
		done:  make(chan struct{}),
	}
}

// Submit offers an event to the ingest channel without blocking the caller.
// It returns false if the buffer is full (the event is dropped and counted),
// honouring the "tracking must never block the UI" requirement: freshness over
// completeness under overload.
func (w *Worker) Submit(e pipeline.Event) bool {
	w.submitted.Add(1)
	select {
	case w.ch <- e:
		w.accepted.Add(1)
		return true
	default:
		w.dropped.Add(1)
		return false
	}
}

// Run drives the windowed pipeline until the context is cancelled or the ingest
// channel is closed by Close. It blocks, so callers should run it in its own
// goroutine. When it returns, all accepted events have been flushed.
func (w *Worker) Run(ctx context.Context) {
	defer close(w.done)
	for {
		more := w.runWindow(ctx)
		if !more {
			return
		}
	}
}

// runWindow executes exactly one bounded pipeline run. It returns false when the
// ingest stream is finished (channel closed or context cancelled) and the worker
// should stop.
func (w *Worker) runWindow(ctx context.Context) bool {
	source := make(chan pipeline.Event)
	sink := &eventSink{}
	p := buildPipeline(w.cfg)

	var (
		metrics []pipeline.StageMetric
		runErr  error
		runDone = make(chan struct{})
	)
	go func() {
		defer close(runDone)
		metrics, runErr = p.Collect(ctx, source, sink)
	}()

	start := time.Now()
	finished := w.feedWindow(ctx, source) // closes source before returning
	<-runDone

	// Persist the window's events in one transaction, then its stage metrics.
	w.flush(ctx, sink, metrics, start)
	if runErr != nil {
		w.pipelineErrs.Add(1)
		log.Printf("ingest: pipeline run error: %v", runErr)
	}
	return !finished
}

// feedWindow forwards events from the shared ingest channel into the per-window
// source until the window's size or time bound is hit. It always closes source.
// It returns true if the ingest stream is finished (shutdown), false if the
// window simply ended and another should follow.
func (w *Worker) feedWindow(ctx context.Context, source chan<- pipeline.Event) (finished bool) {
	defer close(source)

	timer := time.NewTimer(w.cfg.WindowDuration)
	defer timer.Stop()

	count := 0
	for {
		select {
		case e, ok := <-w.ch:
			if !ok {
				return true // channel closed => graceful shutdown
			}
			select {
			case source <- e:
			case <-ctx.Done():
				return true
			}
			count++
			if count >= w.cfg.WindowMaxEvents {
				return false
			}
		case <-timer.C:
			return false
		case <-ctx.Done():
			return true
		}
	}
}

// flush writes the window's collected events and per-stage metrics to the store.
// Empty windows are skipped entirely to avoid metric noise and pointless writes.
func (w *Worker) flush(ctx context.Context, sink *eventSink, metrics []pipeline.StageMetric, start time.Time) {
	if len(sink.recs) == 0 {
		return
	}
	// Use a background context for the final write so a cancelled request
	// context (shutdown) does not abort persistence of already-accepted data.
	writeCtx := context.Background()

	if err := w.store.InsertEvents(writeCtx, sink.recs); err != nil {
		log.Printf("ingest: insert events: %v", err)
	} else {
		w.persisted.Add(int64(len(sink.recs)))
	}

	runID := strconv.FormatInt(start.UnixNano(), 10)
	if err := w.store.InsertStageMetrics(writeCtx, runID, start, metrics); err != nil {
		log.Printf("ingest: insert stage metrics: %v", err)
	}

	w.windows.Add(1)
	w.lastWindowNs.Store(time.Since(start).Nanoseconds())
}

// Close stops accepting new events and signals the worker to drain and exit.
// It is safe to call multiple times. Callers should wait on Wait afterwards.
func (w *Worker) Close() {
	w.closeOnce.Do(func() { close(w.ch) })
}

// Wait blocks until Run has fully drained and returned.
func (w *Worker) Wait() { <-w.done }

// Snapshot is a point-in-time view of the worker's counters for /metrics.
type Snapshot struct {
	Submitted      int64
	Accepted       int64
	Dropped        int64
	Persisted      int64
	Windows        int64
	QueueDepth     int
	QueueCapacity  int
	LastWindowNs   int64
	PipelineErrors int64
}

// Stats returns the current counter snapshot.
func (w *Worker) Stats() Snapshot {
	return Snapshot{
		Submitted:      w.submitted.Load(),
		Accepted:       w.accepted.Load(),
		Dropped:        w.dropped.Load(),
		Persisted:      w.persisted.Load(),
		Windows:        w.windows.Load(),
		QueueDepth:     len(w.ch),
		QueueCapacity:  cap(w.ch),
		LastWindowNs:   w.lastWindowNs.Load(),
		PipelineErrors: w.pipelineErrs.Load(),
	}
}
