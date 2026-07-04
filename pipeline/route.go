package pipeline

import "context"

// ifStage routes each element into one of two sub-pipelines based on a
// predicate, then merges both outputs back into the downstream stream. The
// sub-pipelines are run inline (synchronously) within the worker that owns the
// batch, so ordering between the two branches is not preserved and each branch
// benefits from the outer stage's fan-out.
//
// Only the linear stage list of each sub-pipeline is used; sinks are terminal
// and cannot appear inside a sub-pipeline. Nested If stages are supported
// because an ifStage is itself a Stage.
type ifStage[T any] struct {
	name string
	pred func(T) bool
	then []Stage[T]
	els  []Stage[T]
}

func (s ifStage[T]) Name() string { return s.name }

func (s ifStage[T]) Process(ctx context.Context, in []T) ([]T, int, error) {
	thenIn := make([]T, 0, len(in))
	elseIn := make([]T, 0, len(in))
	for _, v := range in {
		if s.pred(v) {
			thenIn = append(thenIn, v)
		} else {
			elseIn = append(elseIn, v)
		}
	}

	thenOut, thenDrop, err := applyStages(ctx, s.then, thenIn)
	if err != nil {
		return nil, thenDrop, err
	}
	elseOut, elseDrop, err := applyStages(ctx, s.els, elseIn)
	if err != nil {
		return nil, thenDrop + elseDrop, err
	}

	out := make([]T, 0, len(thenOut)+len(elseOut))
	out = append(out, thenOut...)
	out = append(out, elseOut...)
	return out, thenDrop + elseDrop, nil
}
