package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mable/mono/pipeline"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestUserLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	u, err := st.CreateUser(ctx, "a@b.com", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 || u.Email != "a@b.com" {
		t.Fatalf("unexpected user: %+v", u)
	}

	if _, err := st.CreateUser(ctx, "a@b.com", "hash2"); err != ErrUserExists {
		t.Fatalf("want ErrUserExists, got %v", err)
	}

	got, err := st.GetUserByEmail(ctx, "a@b.com")
	if err != nil || got.ID != u.ID {
		t.Fatalf("get by email: %v %+v", err, got)
	}

	if _, err := st.GetUserByEmail(ctx, "missing@b.com"); err != ErrUserNotFound {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestInsertEventsAndStats(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	base := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	recs := []EventRecord{
		{Event: ev("PageView", base, map[string]string{"a": "1", "_received_at": base.Format(time.RFC3339Nano)}), CaptureMs: 10},
		{Event: ev("PageView", base.Add(time.Minute), map[string]string{"a": "1", "b": "2"}), CaptureMs: 20},
		{Event: ev("Purchase", base.Add(2*time.Minute), map[string]string{}), CaptureMs: 30},
	}
	if err := st.InsertEvents(ctx, recs); err != nil {
		t.Fatalf("insert events: %v", err)
	}

	stats, err := st.Stats(ctx, base.Add(-time.Hour), base.Add(time.Hour), "hour")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalEvents != 3 {
		t.Fatalf("total events = %d, want 3", stats.TotalEvents)
	}
	if stats.AvgCaptureMs != 20 {
		t.Fatalf("avg capture = %v, want 20", stats.AvgCaptureMs)
	}
	// avg params: (1 + 2 + 0) / 3 = 1
	if stats.AvgEventParams != 1 {
		t.Fatalf("avg params = %v, want 1", stats.AvgEventParams)
	}
	if stats.TypeCounts["PageView"] != 2 || stats.TypeCounts["Purchase"] != 1 {
		t.Fatalf("type counts = %+v", stats.TypeCounts)
	}
}

func TestInsertStageMetrics(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	metrics := []pipeline.StageMetric{
		{Name: "consent", ItemsIn: 10, ItemsOut: 8, Dropped: 2, Batches: 1, P50: 5, P99: 9, Throughput: 100},
		{Name: "dedup", ItemsIn: 8, ItemsOut: 7, Dropped: 1, Batches: 1, P50: 3, P99: 4, Throughput: 90},
	}
	if err := st.InsertStageMetrics(ctx, "run1", now, metrics); err != nil {
		t.Fatalf("insert metrics: %v", err)
	}

	stats, err := st.Stats(ctx, now.Add(-time.Hour), now.Add(time.Hour), "hour")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats.StageRollups) != 2 {
		t.Fatalf("rollups = %d, want 2", len(stats.StageRollups))
	}
	byName := map[string]int64{}
	for _, r := range stats.StageRollups {
		byName[r.StageName] = r.Dropped
	}
	if byName["consent"] != 2 || byName["dedup"] != 1 {
		t.Fatalf("unexpected rollups: %+v", stats.StageRollups)
	}
}

func ev(typ string, ts time.Time, props map[string]string) pipeline.Event {
	return pipeline.Event{
		EventType:  typ,
		UserID:     "u1",
		SessionID:  "s1",
		Timestamp:  ts,
		Properties: props,
	}
}
