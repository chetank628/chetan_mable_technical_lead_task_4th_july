package store

import (
	"context"
	"time"

	"github.com/mable/mono/pipeline"
)

// InsertStageMetrics persists one window's per-stage metrics. run_id ties all
// rows of a single pipeline run together; window_ts is the wall-clock instant
// the window was flushed.
func (s *Store) InsertStageMetrics(ctx context.Context, runID string, windowTS time.Time, metrics []pipeline.StageMetric) error {
	if len(metrics) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO stage_metrics
	(run_id, window_ts, stage_name, items_in, items_out, dropped, errors,
	 batches, total_latency_ns, p50_ns, p99_ns, wall_ns, throughput)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, m := range metrics {
		if _, err := stmt.ExecContext(ctx,
			runID, windowTS.UTC(), m.Name, m.ItemsIn, m.ItemsOut, m.Dropped,
			m.Errors, m.Batches, m.TotalLatency.Nanoseconds(), m.P50.Nanoseconds(),
			m.P99.Nanoseconds(), m.Wall.Nanoseconds(), m.Throughput,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
