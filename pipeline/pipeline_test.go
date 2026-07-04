package pipeline

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestStruct is the fixed synthetic benchmark payload: 10 fields of mixed,
// pinned types so benchmark numbers are comparable across runs.
type TestStruct struct {
	ID        int64             // 64-bit identity
	Name      string            // variable-length text
	Score     float64           // 64-bit float
	Active    bool              // flag
	Tags      []string          // slice of strings
	CreatedAt time.Time         // timestamp
	Count     uint32            // 32-bit unsigned
	Ratio     float32           // 32-bit float
	Meta      map[string]string // small map
	Payload   []byte            // byte blob
}

// --- helpers ---------------------------------------------------------------

// sliceSink is a CollectSink that gathers elements under a mutex for assertions.
type sliceSink[T any] struct {
	mu    sync.Mutex
	items []T
}

func (s *sliceSink[T]) Push(_ context.Context, v T) error {
	s.mu.Lock()
	s.items = append(s.items, v)
	s.mu.Unlock()
	return nil
}

func (s *sliceSink[T]) snapshot() []T {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]T, len(s.items))
	copy(out, s.items)
	return out
}

// erroringStage returns an error for any batch containing failOn.
type erroringStage struct {
	name   string
	failOn int
}

func (s erroringStage) Name() string { return s.name }
func (s erroringStage) Process(_ context.Context, in []int) ([]int, int, error) {
	for _, v := range in {
		if v == s.failOn {
			return nil, 0, fmt.Errorf("boom on %d", v)
		}
	}
	return in, 0, nil
}

// panicStage panics for any batch containing panicOn.
type panicStage struct {
	name    string
	panicOn int
}

func (s panicStage) Name() string { return s.name }
func (s panicStage) Process(_ context.Context, in []int) ([]int, int, error) {
	for _, v := range in {
		if v == s.panicOn {
			panic(fmt.Sprintf("kaboom on %d", v))
		}
	}
	return in, 0, nil
}

func assertNoLeak(t *testing.T, base int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if runtime.NumGoroutine() <= base {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: have %d, want <= %d", runtime.NumGoroutine(), base)
}

// --- core stage tests ------------------------------------------------------

func TestMapFilter(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(8), WithWorkerCount(4)).
		Map("double", func(v int) int { return v * 2 }).
		Filter("evens>10", func(v int) bool { return v > 10 })

	in := make([]int, 0, 20)
	for i := 0; i < 20; i++ {
		in = append(in, i)
	}
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}

	got := sink.snapshot()
	sort.Ints(got)
	// doubled values > 10: i*2 for i in 6..19 -> 12..38
	if len(got) != 14 {
		t.Fatalf("want 14 items, got %d (%v)", len(got), got)
	}
	if got[0] != 12 || got[len(got)-1] != 38 {
		t.Fatalf("unexpected bounds: %v", got)
	}
}

func TestGenerateExplosion(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(16)).
		Generate("fanout", func(v int) []int { return []int{v, v, v, v, v} })

	in := make([]int, 100)
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}
	if n := len(sink.snapshot()); n != 500 {
		t.Fatalf("want 500, got %d", n)
	}
}

func TestReduceKeyedGlobal(t *testing.T) {
	ctx := context.Background()
	p := New[int](WithBatchSize(7), WithWorkerCount(4))
	in := make([]int, 0, 1000)
	for i := 0; i < 1000; i++ {
		in = append(in, i)
	}

	// Count by parity across the whole stream.
	counts, _, err := Reduce(ctx, p, FromSlice(ctx, in),
		func(v int) string {
			if v%2 == 0 {
				return "even"
			}
			return "odd"
		},
		func() int { return 0 },
		func(acc, _ int) int { return acc + 1 },
	)
	if err != nil {
		t.Fatal(err)
	}
	if counts["even"] != 500 || counts["odd"] != 500 {
		t.Fatalf("bad counts: %v", counts)
	}
}

func TestDeduplicateExact(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(10), WithWorkerCount(1)).
		Deduplicate("dedup", func(v int) any { return v }, 0)

	in := []int{1, 1, 2, 3, 3, 3, 4, 2, 1}
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}
	got := sink.snapshot()
	sort.Ints(got)
	want := []int{1, 2, 3, 4}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestDeduplicateBoundedEvictionFalseNegative(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	// capacity 2, single worker for deterministic ordering.
	p := New[int](WithBatchSize(1), WithWorkerCount(1)).
		Deduplicate("dedup", func(v int) any { return v }, 2)

	// 1,2 fill the LRU; 3 evicts 1; then 1 reappears -> false negative (passes).
	in := []int{1, 2, 3, 1}
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}
	if n := len(sink.snapshot()); n != 4 {
		t.Fatalf("expected 4 (false negative after eviction), got %d", n)
	}
}

func TestIfRouting(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	then := New[int]().Map("x100", func(v int) int { return v * 100 })
	els := New[int]().Map("neg", func(v int) int { return -v })
	p := New[int](WithBatchSize(8)).
		If("split", func(v int) bool { return v%2 == 0 }, then, els)

	in := []int{1, 2, 3, 4}
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}
	got := sink.snapshot()
	sort.Ints(got)
	// evens 2,4 -> 200,400 ; odds 1,3 -> -1,-3
	want := []int{-3, -1, 200, 400}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

// --- edge cases ------------------------------------------------------------

func TestEmptyInput(t *testing.T) {
	ctx := context.Background()
	p := New[int]()
	counts, metrics, err := Reduce(ctx, p, FromSlice[int](ctx, nil),
		func(int) int { return 0 }, func() int { return 0 }, func(a, _ int) int { return a + 1 })
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 0 {
		t.Fatalf("expected empty result, got %v", counts)
	}
	if metrics[len(metrics)-1].ItemsIn != 0 {
		t.Fatalf("expected zero sink ItemsIn, got %d", metrics[len(metrics)-1].ItemsIn)
	}
}

func TestSingleElement(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int]().Map("inc", func(v int) int { return v + 1 })
	if _, err := p.Collect(ctx, FromSlice(ctx, []int{41}), sink); err != nil {
		t.Fatal(err)
	}
	got := sink.snapshot()
	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("want [42], got %v", got)
	}
}

func TestFilterDropsEverything(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int]().Filter("none", func(int) bool { return false })
	if _, err := p.Collect(ctx, FromSlice(ctx, []int{1, 2, 3}), sink); err != nil {
		t.Fatal(err)
	}
	if n := len(sink.snapshot()); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestErrorSkipAndCount(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(1), WithWorkerCount(2)).
		Stage(erroringStage{name: "maybe_err", failOn: 5})
	in := []int{1, 2, 5, 7}
	metrics, err := p.Collect(ctx, FromSlice(ctx, in), sink)
	if err != nil {
		t.Fatalf("SkipAndCount should not return error, got %v", err)
	}
	if n := len(sink.snapshot()); n != 3 {
		t.Fatalf("want 3 survivors, got %d", n)
	}
	if metrics[0].Errors != 1 {
		t.Fatalf("want 1 error counted, got %d", metrics[0].Errors)
	}
}

func TestErrorFailFast(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(1), WithWorkerCount(2), WithErrorPolicy(FailFast)).
		Stage(erroringStage{name: "maybe_err", failOn: 3})
	in := make([]int, 0, 1000)
	for i := 0; i < 1000; i++ {
		in = append(in, i)
	}
	_, err := p.Collect(ctx, FromSlice(ctx, in), sink)
	if err == nil {
		t.Fatal("FailFast should return an error")
	}
}

func TestPanicRecovered(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(1), WithWorkerCount(2)).
		Stage(panicStage{name: "boom", panicOn: 9})
	in := []int{8, 9, 10}
	metrics, err := p.Collect(ctx, FromSlice(ctx, in), sink)
	if err != nil {
		t.Fatalf("panic should be recovered under SkipAndCount, got %v", err)
	}
	if n := len(sink.snapshot()); n != 2 {
		t.Fatalf("want 2 survivors, got %d", n)
	}
	if metrics[0].Errors != 1 {
		t.Fatalf("want 1 recovered-panic error, got %d", metrics[0].Errors)
	}
}

func TestContextCancellationNoLeak(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())

	// Infinite source that respects ctx.
	src := make(chan int)
	go func() {
		defer close(src)
		i := 0
		for {
			select {
			case src <- i:
				i++
			case <-ctx.Done():
				return
			}
		}
	}()

	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(4), WithWorkerCount(3)).
		Map("slow", func(v int) int { return v })

	done := make(chan struct{})
	go func() {
		_, _ = p.Collect(ctx, src, sink)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not shut down after cancel")
	}
	assertNoLeak(t, base)
}

func TestBatchTimeoutLowVolume(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	// Large batch size but tiny timeout: a trickle must still flow promptly.
	p := New[int](WithBatchSize(10000), WithBatchTimeout(10*time.Millisecond), WithWorkerCount(1)).
		Map("id", func(v int) int { return v })

	src := make(chan int)
	go func() {
		defer close(src)
		for i := 0; i < 3; i++ {
			src <- i
			time.Sleep(15 * time.Millisecond)
		}
	}()

	start := time.Now()
	if _, err := p.Collect(ctx, src, sink); err != nil {
		t.Fatal(err)
	}
	if n := len(sink.snapshot()); n != 3 {
		t.Fatalf("want 3, got %d", n)
	}
	// Should not have waited for a full 10000-element batch.
	if time.Since(start) > time.Second {
		t.Fatalf("batch timeout did not flush promptly: %v", time.Since(start))
	}
}

func TestDegenerateConfig(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	// Unbuffered channels, single worker, batch size clamped to 1.
	p := New[int](WithChannelBufferDepth(0), WithWorkerCount(0), WithBatchSize(0)).
		Map("inc", func(v int) int { return v + 1 })
	in := []int{1, 2, 3}
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}
	got := sink.snapshot()
	sort.Ints(got)
	if fmt.Sprint(got) != fmt.Sprint([]int{2, 3, 4}) {
		t.Fatalf("got %v", got)
	}
}

func TestCollectDropOnFull(t *testing.T) {
	ctx := context.Background()
	cs := &ChannelSink[int]{Out: make(chan int, 2), Policy: DropOnFull}
	// Never drain Out: only 2 elements fit, the rest are dropped.
	p := New[int](WithBatchSize(1), WithWorkerCount(1))
	in := make([]int, 100)
	metrics, err := p.Collect(ctx, FromSlice(ctx, in), cs)
	if err != nil {
		t.Fatal(err)
	}
	if cs.Dropped() < 90 {
		t.Fatalf("expected most elements dropped, dropped=%d", cs.Dropped())
	}
	sinkMetric := metrics[len(metrics)-1]
	if sinkMetric.Dropped != cs.Dropped() {
		t.Fatalf("metric drop %d != sink drop %d", sinkMetric.Dropped, cs.Dropped())
	}
}

func TestCollectBlockBackpressure(t *testing.T) {
	ctx := context.Background()
	out := make(chan int, 4)
	cs := &ChannelSink[int]{Out: out, Policy: Block}

	var got []int
	var mu sync.Mutex
	drained := make(chan struct{})
	go func() {
		for v := range out {
			mu.Lock()
			got = append(got, v)
			mu.Unlock()
		}
		close(drained)
	}()

	p := New[int](WithBatchSize(8), WithWorkerCount(2))
	in := make([]int, 500)
	for i := range in {
		in[i] = i
	}
	if _, err := p.Collect(ctx, FromSlice(ctx, in), cs); err != nil {
		t.Fatal(err)
	}
	close(out)
	<-drained
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 500 {
		t.Fatalf("block policy must deliver all, got %d", n)
	}
}

func TestMetricsContent(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(10)).
		Map("double", func(v int) int { return v * 2 }).
		Filter("keep>20", func(v int) bool { return v > 20 })

	in := make([]int, 0, 30)
	for i := 0; i < 30; i++ {
		in = append(in, i)
	}
	metrics, err := p.Collect(ctx, FromSlice(ctx, in), sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 stage metrics (map, filter, sink), got %d", len(metrics))
	}
	mapM, filterM := metrics[0], metrics[1]
	if mapM.ItemsIn != 30 || mapM.ItemsOut != 30 {
		t.Fatalf("map metrics wrong: %+v", mapM)
	}
	// values >20 after doubling: i*2>20 => i>=11 => 19 items
	if filterM.ItemsOut != 19 || filterM.Dropped != 11 {
		t.Fatalf("filter metrics wrong: out=%d dropped=%d", filterM.ItemsOut, filterM.Dropped)
	}
	if mapM.Throughput <= 0 {
		t.Fatalf("expected positive throughput, got %f", mapM.Throughput)
	}

	// Pipeline.Metrics() returns the same snapshot.
	if len(p.Metrics()) != 3 {
		t.Fatalf("Metrics() snapshot mismatch")
	}
}

func TestMetricsSinkSeam(t *testing.T) {
	ctx := context.Background()
	var captured []StageMetric
	ms := MetricsSinkFunc(func(_ context.Context, m []StageMetric) error {
		captured = m
		return nil
	})
	sink := &sliceSink[int]{}
	p := New[int](WithMetricsSink(ms)).Map("id", func(v int) int { return v })
	if _, err := p.Collect(ctx, FromSlice(ctx, []int{1, 2, 3}), sink); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 2 {
		t.Fatalf("metrics sink did not receive snapshot, got %d", len(captured))
	}
}

func TestChurnEnrichExtension(t *testing.T) {
	ctx := context.Background()
	sink := &sliceSink[Event]{}
	p := New[Event](WithBatchSize(4)).Stage(NewChurnEnrich())
	in := []Event{
		{EventType: "Purchase", UserID: "a", Amount: 50},
		{EventType: "PageView", UserID: "b"},
		{EventType: "Purchase", UserID: "c", Amount: 5000},
	}
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}
	for _, e := range sink.snapshot() {
		switch e.EventType {
		case "Purchase":
			if e.ChurnProbability < 0 || e.ChurnProbability >= 1 {
				t.Fatalf("churn prob out of range: %f", e.ChurnProbability)
			}
		default:
			if e.ChurnProbability != 0 {
				t.Fatalf("non-purchase should not be enriched: %+v", e)
			}
		}
	}
}

func TestNoLeakOnNormalCompletion(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx := context.Background()
	sink := &sliceSink[int]{}
	p := New[int](WithBatchSize(16), WithWorkerCount(4)).
		Map("a", func(v int) int { return v + 1 }).
		Filter("b", func(v int) bool { return v%2 == 0 })
	in := make([]int, 5000)
	if _, err := p.Collect(ctx, FromSlice(ctx, in), sink); err != nil {
		t.Fatal(err)
	}
	assertNoLeak(t, base)
}
