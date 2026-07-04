package pipeline

import (
	"context"
	"sort"
	"time"
)

// StageMetric is the per-stage metadata snapshot produced by a run. The brief
// requires per-stage (not per-pipeline) metadata including errors and drops,
// not just latency.
type StageMetric struct {
	// Name is the stage's reported name.
	Name string
	// ItemsIn is the number of elements that entered the stage.
	ItemsIn int64
	// ItemsOut is the number of elements the stage emitted downstream.
	ItemsOut int64
	// Dropped is the number of elements intentionally dropped (e.g. by Filter,
	// Deduplicate, or a DropOnFull Collect sink).
	Dropped int64
	// Errors is the number of batches that failed (stage error or recovered
	// panic) under the SkipAndCount policy.
	Errors int64
	// Batches is the number of batches processed by the stage.
	Batches int64
	// TotalLatency is the summed wall time spent inside the stage's Process
	// calls across all workers.
	TotalLatency time.Duration
	// P50 and P99 are the per-batch processing latency percentiles.
	P50 time.Duration
	P99 time.Duration
	// Wall is the wall-clock duration the stage runner was active.
	Wall time.Duration
	// Throughput is ItemsOut divided by Wall in items/second.
	Throughput float64
}

// MetricsSink is the seam for forwarding metrics to an external store (e.g. the
// API caller pushing into ClickHouse). It is intentionally tiny so callers can
// implement it without depending on pipeline internals.
type MetricsSink interface {
	Record(ctx context.Context, metrics []StageMetric) error
}

// MetricsSinkFunc adapts a plain function to the MetricsSink interface.
type MetricsSinkFunc func(ctx context.Context, metrics []StageMetric) error

// Record implements MetricsSink.
func (f MetricsSinkFunc) Record(ctx context.Context, metrics []StageMetric) error {
	return f(ctx, metrics)
}

// workerStat is per-worker accumulation, kept worker-local during the run and
// merged once at the end so the hot path needs no locks or atomics.
type workerStat struct {
	in        int64
	out       int64
	dropped   int64
	errs      int64
	batches   int64
	latencies []time.Duration
}

// mergeStageMetric folds per-worker stats and the runner's wall time into a
// single StageMetric, computing latency percentiles from the merged samples.
func mergeStageMetric(name string, workers []workerStat, wall time.Duration) StageMetric {
	m := StageMetric{Name: name, Wall: wall}
	var lats []time.Duration
	for i := range workers {
		w := &workers[i]
		m.ItemsIn += w.in
		m.ItemsOut += w.out
		m.Dropped += w.dropped
		m.Errors += w.errs
		m.Batches += w.batches
		lats = append(lats, w.latencies...)
	}
	for _, l := range lats {
		m.TotalLatency += l
	}
	m.P50 = percentile(lats, 0.50)
	m.P99 = percentile(lats, 0.99)
	if wall > 0 {
		m.Throughput = float64(m.ItemsOut) / wall.Seconds()
	}
	return m
}

// percentile returns the p-quantile (0..1) of the samples using
// nearest-rank. It sorts a copy so the caller's slice is untouched.
func percentile(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(p * float64(len(sorted)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
