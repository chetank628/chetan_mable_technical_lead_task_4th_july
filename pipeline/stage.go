package pipeline

import (
	"container/list"
	"context"
	"fmt"
	"sync"
)

// Stage is the single extension point of the library. Any type implementing
// Stage can be added to a pipeline via Pipeline.Stage, with no changes to the
// core. A stage consumes a batch and emits a batch (0..N elements out per
// element in), reporting how many elements it intentionally dropped.
//
// Process MAY be called concurrently from multiple worker goroutines, so
// implementations that hold mutable state must synchronise it themselves
// (see dedupStage for an example). The built-in func-based stages are pure and
// therefore trivially safe.
type Stage[T any] interface {
	// Name identifies the stage in metrics and error messages.
	Name() string
	// Process transforms an inbound batch into an outbound batch. dropped is the
	// number of inbound elements deliberately discarded. A non-nil err is
	// handled per the pipeline's ErrorPolicy.
	Process(ctx context.Context, in []T) (out []T, dropped int, err error)
}

// safeProcess invokes a stage and converts any panic into a stage error,
// counting the whole batch as dropped. This keeps a misbehaving user stage from
// crashing a worker goroutine.
func safeProcess[T any](ctx context.Context, s Stage[T], in []T) (out []T, dropped int, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = nil
			dropped = len(in)
			err = fmt.Errorf("panic in stage %q: %v", s.Name(), r)
		}
	}()
	return s.Process(ctx, in)
}

// applyStages runs a batch through an ordered list of stages synchronously,
// threading the output of one into the input of the next. Used by If to run its
// sub-pipelines inline within a worker.
func applyStages[T any](ctx context.Context, stages []Stage[T], in []T) (out []T, dropped int, err error) {
	cur := in
	for _, s := range stages {
		o, d, e := safeProcess(ctx, s, cur)
		dropped += d
		if e != nil {
			return nil, dropped, e
		}
		cur = o
	}
	return cur, dropped, nil
}

// --- Map -------------------------------------------------------------------

type mapStage[T any] struct {
	name string
	fn   func(T) T
}

func (s mapStage[T]) Name() string { return s.name }

func (s mapStage[T]) Process(_ context.Context, in []T) ([]T, int, error) {
	out := make([]T, len(in))
	for i, v := range in {
		out[i] = s.fn(v)
	}
	return out, 0, nil
}

// --- Filter ----------------------------------------------------------------

type filterStage[T any] struct {
	name string
	pred func(T) bool
}

func (s filterStage[T]) Name() string { return s.name }

func (s filterStage[T]) Process(_ context.Context, in []T) ([]T, int, error) {
	out := make([]T, 0, len(in))
	dropped := 0
	for _, v := range in {
		if s.pred(v) {
			out = append(out, v)
		} else {
			dropped++
		}
	}
	return out, dropped, nil
}

// --- Generate --------------------------------------------------------------

type generateStage[T any] struct {
	name string
	fn   func(T) []T
}

func (s generateStage[T]) Name() string { return s.name }

// Process emits zero-or-more newly produced T per input element. If the caller
// wants the original element to pass through, fn should include it in its
// return slice.
func (s generateStage[T]) Process(_ context.Context, in []T) ([]T, int, error) {
	out := make([]T, 0, len(in))
	for _, v := range in {
		out = append(out, s.fn(v)...)
	}
	return out, 0, nil
}

// --- Deduplicate -----------------------------------------------------------

// dedupStage drops elements whose key has been seen before. It keeps a bounded
// LRU "seen" set so memory stays bounded at any volume. The trade-off: once the
// LRU evicts a key, a later repeat of that key is a false negative (it passes
// through). Set capacity to 0 for an unbounded (exact) seen set.
type dedupStage[T any] struct {
	name  string
	key   func(T) any
	cap   int
	mu    sync.Mutex
	seen  map[any]*list.Element
	order *list.List // front = most recently seen
}

func newDedupStage[T any](name string, key func(T) any, capacity int) *dedupStage[T] {
	return &dedupStage[T]{
		name:  name,
		key:   key,
		cap:   capacity,
		seen:  make(map[any]*list.Element),
		order: list.New(),
	}
}

func (s *dedupStage[T]) Name() string { return s.name }

func (s *dedupStage[T]) Process(_ context.Context, in []T) ([]T, int, error) {
	out := make([]T, 0, len(in))
	dropped := 0
	// Deduplicate is a global synchronization point: all workers serialise on
	// this mutex. This is by design — exactness across the whole stream
	// requires shared state.
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range in {
		k := s.key(v)
		if el, ok := s.seen[k]; ok {
			s.order.MoveToFront(el)
			dropped++
			continue
		}
		out = append(out, v)
		s.seen[k] = s.order.PushFront(k)
		if s.cap > 0 && s.order.Len() > s.cap {
			oldest := s.order.Back()
			if oldest != nil {
				s.order.Remove(oldest)
				delete(s.seen, oldest.Value)
			}
		}
	}
	return out, dropped, nil
}
