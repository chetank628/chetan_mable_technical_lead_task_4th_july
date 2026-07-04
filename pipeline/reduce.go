package pipeline

import (
	"context"
	"errors"
	"sync/atomic"
)

// Sink is a terminal stage that drains the stream. Unlike Stage it does not emit
// downstream. The runtime drains the sink on a single goroutine, which makes the
// accumulator race-free without locks while still benefiting from upstream
// fan-out.
type Sink[T any] interface {
	Name() string
	// Consume folds one batch into the sink, returning how many elements were
	// dropped (e.g. a DropOnFull Collect sink) and any error.
	Consume(ctx context.Context, in []T) (dropped int, err error)
}

// --- Reduce ----------------------------------------------------------------

type reduceSink[T any, K comparable, R any] struct {
	name    string
	keyFn   func(T) K
	initFn  func() R
	reducer func(acc R, v T) R
	acc     map[K]R
}

func (s *reduceSink[T, K, R]) Name() string { return s.name }

func (s *reduceSink[T, K, R]) Consume(_ context.Context, in []T) (int, error) {
	for _, v := range in {
		k := s.keyFn(v)
		cur, ok := s.acc[k]
		if !ok {
			cur = s.initFn()
		}
		s.acc[k] = s.reducer(cur, v)
	}
	return 0, nil
}

// Reduce terminates a pipeline by folding the whole stream into a keyed map.
// It is global (accumulates across every batch), not per-batch, because
// analytics aggregates (counts/sums per key) are only meaningful over the whole
// stream. Reduce is a sink: it is not chainable into further T stages, which is
// why it is a package function (it introduces the second type parameters K, R
// that a method on Pipeline[T] cannot).
//
// For an un-keyed global fold, supply a key function that returns a constant.
func Reduce[T any, K comparable, R any](
	ctx context.Context,
	p *Pipeline[T],
	source <-chan T,
	key func(T) K,
	init func() R,
	reduce func(acc R, v T) R,
) (map[K]R, []StageMetric, error) {
	s := &reduceSink[T, K, R]{
		name:    "reduce",
		keyFn:   key,
		initFn:  init,
		reducer: reduce,
		acc:     make(map[K]R),
	}
	metrics, err := p.execute(ctx, source, s)
	return s.acc, metrics, err
}

// --- Collect ---------------------------------------------------------------

// ErrDropped is returned by a CollectSink.Push to signal the element was
// intentionally dropped (e.g. a full DropOnFull sink). It is counted as a drop,
// not a pipeline error.
var ErrDropped = errors.New("pipeline: element dropped by collect sink")

// CollectSink is a caller-provided bounded destination for the stream. Its
// back-pressure behaviour is defined by the implementation: a blocking Push
// propagates back-pressure upstream; a Push that returns ErrDropped sheds load.
type CollectSink[T any] interface {
	Push(ctx context.Context, v T) error
}

type collectSink[T any] struct {
	name string
	sink CollectSink[T]
}

func (s collectSink[T]) Name() string { return s.name }

func (s collectSink[T]) Consume(ctx context.Context, in []T) (int, error) {
	dropped := 0
	for _, v := range in {
		if err := s.sink.Push(ctx, v); err != nil {
			if errors.Is(err, ErrDropped) {
				dropped++
				continue
			}
			return dropped, err
		}
	}
	return dropped, nil
}

// Collect terminates a pipeline by draining the stream into a caller-provided
// bounded sink. Back-pressure is the sink's responsibility (see ChannelSink).
func (p *Pipeline[T]) Collect(ctx context.Context, source <-chan T, sink CollectSink[T]) ([]StageMetric, error) {
	return p.execute(ctx, source, collectSink[T]{name: "collect", sink: sink})
}

// ChannelSink is a ready-made bounded CollectSink backed by a channel. With the
// Block policy a full channel applies back-pressure upstream; with DropOnFull a
// full channel sheds the element and counts it. The caller owns Out and, under
// the Block policy, must drain it concurrently to avoid deadlock.
type ChannelSink[T any] struct {
	Out     chan T
	Policy  CollectPolicy
	dropped int64
}

// Push implements CollectSink.
func (s *ChannelSink[T]) Push(ctx context.Context, v T) error {
	if s.Policy == DropOnFull {
		select {
		case s.Out <- v:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			atomic.AddInt64(&s.dropped, 1)
			return ErrDropped
		}
	}
	select {
	case s.Out <- v:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Dropped reports how many elements this sink shed under DropOnFull.
func (s *ChannelSink[T]) Dropped() int64 { return atomic.LoadInt64(&s.dropped) }
