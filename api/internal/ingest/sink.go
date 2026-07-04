package ingest

import (
	"context"
	"time"

	"github.com/mable/mono/api/internal/store"
	"github.com/mable/mono/pipeline"
)

// eventSink implements pipeline.CollectSink[pipeline.Event]. The pipeline
// runtime drains the sink on a single goroutine, so appending to recs needs no
// lock. We buffer the whole (bounded) window and let the worker flush it in one
// transaction, which is far cheaper than a row-per-Push insert.
type eventSink struct {
	recs []store.EventRecord
}

// Push records one processed event together with its server-side capture
// latency (received_at -> now). It never blocks or drops: the bounded ingest
// channel and bounded window already cap how much can accumulate here.
func (s *eventSink) Push(_ context.Context, e pipeline.Event) error {
	s.recs = append(s.recs, store.EventRecord{
		Event:     e,
		CaptureMs: captureMs(e),
	})
	return nil
}

// captureMs computes how long the event spent between server receipt and this
// point, in milliseconds, using the in-band "_received_at" control property the
// handler stamped. Server receipt time is preferred over the client timestamp
// to stay robust against client clock skew.
func captureMs(e pipeline.Event) float64 {
	v, ok := e.Properties["_received_at"]
	if !ok {
		return 0
	}
	recv, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return 0
	}
	d := time.Since(recv)
	if d < 0 {
		return 0
	}
	return float64(d.Microseconds()) / 1000.0
}
