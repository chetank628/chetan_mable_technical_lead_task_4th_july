package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mable/mono/pipeline"
)

// EventRecord is a pipeline.Event paired with the server-side capture latency
// (received_at -> persisted) that the analytics layer reports on.
type EventRecord struct {
	Event     pipeline.Event
	CaptureMs float64
}

// InsertEvents persists a batch of processed events in a single transaction.
// It is called from the ingest worker's collect sink, which the pipeline drains
// on a single goroutine, so no additional locking is required here.
func (s *Store) InsertEvents(ctx context.Context, recs []EventRecord) error {
	if len(recs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO events
	(event_type, user_id, session_id, ts, url, referrer, user_agent, ip,
	 amount, currency, properties, churn_probability, received_at, capture_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, r := range recs {
		e := r.Event
		props := propsJSON(e.Properties)
		receivedAt := receivedAtOf(e)
		if _, err := stmt.ExecContext(ctx,
			e.EventType, e.UserID, e.SessionID, e.Timestamp.UTC(), e.URL, e.Referrer,
			e.UserAgent, e.IP, e.Amount, e.Currency, props, e.ChurnProbability,
			receivedAt.UTC(), r.CaptureMs,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// propsJSON serialises the user-facing properties, stripping the internal
// underscore-prefixed control keys the pipeline used in-band.
func propsJSON(props map[string]string) string {
	if len(props) == 0 {
		return "{}"
	}
	clean := make(map[string]string, len(props))
	for k, v := range props {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		clean[k] = v
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// receivedAtOf extracts the server receipt time the handler stamped into the
// event, falling back to the client timestamp (then now) if absent.
func receivedAtOf(e pipeline.Event) time.Time {
	if v, ok := e.Properties["_received_at"]; ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t
		}
	}
	if !e.Timestamp.IsZero() {
		return e.Timestamp
	}
	return time.Now()
}
