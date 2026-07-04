package store

import (
	"context"
	"fmt"
	"time"

	"github.com/mable/mono/api/internal/model"
)

// bucketFormats maps a caller-supplied granularity to a SQLite strftime format.
// Truncating ts to the bucket start keeps "events over time" series compact.
var bucketFormats = map[string]string{
	"minute": "%Y-%m-%dT%H:%M:00Z",
	"hour":   "%Y-%m-%dT%H:00:00Z",
	"day":    "%Y-%m-%dT00:00:00Z",
}

// Stats computes the analytics payload over events with ts in [since, until].
// granularity is one of "minute", "hour", "day" (defaults to "hour").
func (s *Store) Stats(ctx context.Context, since, until time.Time, granularity string) (model.Stats, error) {
	format, ok := bucketFormats[granularity]
	if !ok {
		format = bucketFormats["hour"]
	}

	var out model.Stats

	// Scalars: total, average server capture latency, average #properties.
	row := s.db.QueryRowContext(ctx, `
SELECT
	COUNT(*),
	COALESCE(AVG(capture_ms), 0),
	COALESCE(AVG((SELECT COUNT(*) FROM json_each(events.properties))), 0)
FROM events
WHERE ts BETWEEN ? AND ?`, since.UTC(), until.UTC())
	if err := row.Scan(&out.TotalEvents, &out.AvgCaptureMs, &out.AvgEventParams); err != nil {
		return model.Stats{}, fmt.Errorf("scalar stats: %w", err)
	}

	var err error
	if out.EventsOverTime, err = s.eventsOverTime(ctx, since, until, format); err != nil {
		return model.Stats{}, err
	}
	if out.TypeCounts, err = s.typeCounts(ctx, since, until); err != nil {
		return model.Stats{}, err
	}
	if out.TypeOverTime, err = s.typeOverTime(ctx, since, until, format); err != nil {
		return model.Stats{}, err
	}
	if out.StageRollups, err = s.stageRollups(ctx, since, until); err != nil {
		return model.Stats{}, err
	}
	return out, nil
}

func (s *Store) eventsOverTime(ctx context.Context, since, until time.Time, format string) ([]model.TimeBucket, error) {
	// ts is persisted via Go's time.Time text encoding ("2006-01-02 15:04:05
	// ..."); SQLite's date functions only parse the leading 19-char
	// "YYYY-MM-DD HH:MM:SS" prefix, so we slice it before bucketing.
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT strftime('%s', substr(ts, 1, 19)) AS bucket, COUNT(*)
FROM events
WHERE ts BETWEEN ? AND ?
GROUP BY bucket
ORDER BY bucket`, format), since.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("events over time: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.TimeBucket
	for rows.Next() {
		var bucketStr string
		var count int64
		if err := rows.Scan(&bucketStr, &count); err != nil {
			return nil, err
		}
		out = append(out, model.TimeBucket{Bucket: parseBucket(bucketStr), Count: count})
	}
	return out, rows.Err()
}

func (s *Store) typeCounts(ctx context.Context, since, until time.Time) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT event_type, COUNT(*)
FROM events
WHERE ts BETWEEN ? AND ?
GROUP BY event_type
ORDER BY event_type`, since.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("type counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]int64)
	for rows.Next() {
		var t string
		var c int64
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		out[t] = c
	}
	return out, rows.Err()
}

func (s *Store) typeOverTime(ctx context.Context, since, until time.Time, format string) ([]model.TypeTimeBucket, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT strftime('%s', substr(ts, 1, 19)) AS bucket, event_type, COUNT(*)
FROM events
WHERE ts BETWEEN ? AND ?
GROUP BY bucket, event_type
ORDER BY bucket, event_type`, format), since.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("type over time: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.TypeTimeBucket
	for rows.Next() {
		var bucketStr, eventType string
		var count int64
		if err := rows.Scan(&bucketStr, &eventType, &count); err != nil {
			return nil, err
		}
		out = append(out, model.TypeTimeBucket{
			Bucket:    parseBucket(bucketStr),
			EventType: eventType,
			Count:     count,
		})
	}
	return out, rows.Err()
}

func (s *Store) stageRollups(ctx context.Context, since, until time.Time) ([]model.StageRollup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
	stage_name,
	COUNT(*)            AS windows,
	SUM(items_in),
	SUM(items_out),
	SUM(dropped),
	SUM(errors),
	SUM(batches),
	COALESCE(AVG(p50_ns), 0),
	COALESCE(AVG(p99_ns), 0),
	COALESCE(AVG(throughput), 0)
FROM stage_metrics
WHERE window_ts BETWEEN ? AND ?
GROUP BY stage_name
ORDER BY stage_name`, since.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("stage rollups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.StageRollup
	for rows.Next() {
		var r model.StageRollup
		if err := rows.Scan(
			&r.StageName, &r.Windows, &r.ItemsIn, &r.ItemsOut, &r.Dropped,
			&r.Errors, &r.Batches, &r.AvgP50Ns, &r.AvgP99Ns, &r.AvgThroughput,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// parseBucket converts a strftime bucket label back into a time.Time. On parse
// failure it returns the zero time rather than erroring the whole query.
func parseBucket(s string) time.Time {
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t
	}
	return time.Time{}
}
