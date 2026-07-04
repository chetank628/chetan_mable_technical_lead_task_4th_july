package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// benchVolumes are the mandated event volumes. The 1M cap keeps the run
// laptop-safe; push higher only for the optional streamed-to-disk experiment.
var benchVolumes = []int{10, 1_000, 100_000, 1_000_000}

func makeTestStructs(n int) []TestStruct {
	out := make([]TestStruct, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		out[i] = TestStruct{
			ID:        int64(i),
			Name:      "item-" + strconv.Itoa(i),
			Score:     float64(i) * 1.5,
			Active:    i%2 == 0,
			Tags:      []string{"a", "b"},
			CreatedAt: now,
			Count:     uint32(i % 1000),
			Ratio:     float32(i%7) / 7.0,
			Meta:      map[string]string{"k": "v"},
			Payload:   []byte("payload"),
		}
	}
	return out
}

func loadSampleEvent(tb testing.TB) Event {
	tb.Helper()
	raw, err := os.ReadFile("testdata/sample_event.json")
	if err != nil {
		tb.Fatalf("read sample_event.json: %v", err)
	}
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		tb.Fatalf("unmarshal sample_event.json: %v", err)
	}
	return e
}

func makeEvents(tb testing.TB, n int) []Event {
	tmpl := loadSampleEvent(tb)
	types := []string{"PageView", "Click", "AddToCart", "Checkout", "Purchase"}
	out := make([]Event, n)
	for i := 0; i < n; i++ {
		e := tmpl
		e.EventType = types[i%len(types)]
		e.UserID = "u_" + strconv.Itoa(i%10000)
		e.SessionID = "s_" + strconv.Itoa(i%5000)
		out[i] = e
	}
	return out
}

// testStructPipeline builds a representative pipeline: Map -> Filter ->
// Deduplicate -> Reduce (count by Count bucket).
func benchTestStructPipeline() *Pipeline[TestStruct] {
	return New[TestStruct](WithBatchSize(1024)).
		Map("score", func(t TestStruct) TestStruct { t.Score *= 1.01; return t }).
		Filter("active", func(t TestStruct) bool { return t.Active }).
		Deduplicate("by_id", func(t TestStruct) any { return t.ID }, 200_000)
}

// benchEventPipeline mirrors the API ingest path: Filter -> ChurnEnrich ->
// Deduplicate -> Reduce (count by event type).
func benchEventPipeline() *Pipeline[Event] {
	tracked := map[string]bool{
		"PageView": true, "Click": true, "AddToCart": true,
		"Checkout": true, "PaymentInfoAdded": true, "Purchase": true, "Lead": true,
	}
	return New[Event](WithBatchSize(1024)).
		Filter("tracked", func(e Event) bool { return tracked[e.EventType] }).
		Stage(NewChurnEnrich()).
		Deduplicate("by_session_type", func(e Event) any { return e.SessionID + "|" + e.EventType }, 200_000)
}

func BenchmarkTestStruct(b *testing.B) {
	ctx := context.Background()
	for _, n := range benchVolumes {
		data := makeTestStructs(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p := benchTestStructPipeline()
				_, _, err := Reduce(ctx, p, FromSlice(ctx, data),
					func(t TestStruct) uint32 { return t.Count % 10 },
					func() int { return 0 },
					func(acc int, _ TestStruct) int { return acc + 1 },
				)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(n)*float64(b.N)/b.Elapsed().Seconds(), "events/sec")
		})
	}
}

func BenchmarkMableEvent(b *testing.B) {
	ctx := context.Background()
	for _, n := range benchVolumes {
		data := makeEvents(b, n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p := benchEventPipeline()
				_, _, err := Reduce(ctx, p, FromSlice(ctx, data),
					func(e Event) string { return e.EventType },
					func() int { return 0 },
					func(acc int, _ Event) int { return acc + 1 },
				)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(n)*float64(b.N)/b.Elapsed().Seconds(), "events/sec")
		})
	}
}
