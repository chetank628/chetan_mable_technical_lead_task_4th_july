package pipeline

import (
	"context"
	"hash/fnv"
	"time"
)

// Event is a Mable-style tracking event. It lives in the library as the demo
// payload used by the example stage and the sample_event.json benchmark. Real
// callers would define their own T; this type exists to make the extension demo
// and benchmarks self-contained.
type Event struct {
	EventType  string            `json:"event_type"` // e.g. "PageView", "AddToCart", "Purchase"
	UserID     string            `json:"user_id"`
	SessionID  string            `json:"session_id"`
	Timestamp  time.Time         `json:"timestamp"`
	URL        string            `json:"url"`
	Referrer   string            `json:"referrer"`
	UserAgent  string            `json:"user_agent"`
	IP         string            `json:"ip"`
	Amount     float64           `json:"amount,omitempty"`   // populated for Purchase
	Currency   string            `json:"currency,omitempty"` // populated for Purchase
	Properties map[string]string `json:"properties,omitempty"`

	// ChurnProbability is enriched by ChurnEnrich for Purchase events.
	ChurnProbability float64 `json:"churn_probability,omitempty"`
}

// ChurnEnrich is an EXAMPLE custom stage proving the extension protocol: it
// implements Stage[Event] and is added via Pipeline.Stage with no change to the
// core. It enriches every Purchase event with a churn-probability score and
// passes all other events through untouched.
//
// The score here is a deterministic placeholder (a real implementation would
// call a model). It demonstrates that a stage can be stateless, pure, and
// trivially composable.
type ChurnEnrich struct {
	StageName string
}

// NewChurnEnrich returns a ChurnEnrich stage with a default name.
func NewChurnEnrich() ChurnEnrich {
	return ChurnEnrich{StageName: "churn_enrich"}
}

// Name implements Stage.
func (c ChurnEnrich) Name() string {
	if c.StageName == "" {
		return "churn_enrich"
	}
	return c.StageName
}

// Process implements Stage[Event].
func (c ChurnEnrich) Process(_ context.Context, in []Event) ([]Event, int, error) {
	out := make([]Event, len(in))
	for i, e := range in {
		if e.EventType == "Purchase" {
			e.ChurnProbability = churnScore(e)
		}
		out[i] = e
	}
	return out, 0, nil
}

// churnScore is a deterministic stand-in for a churn model: it blends a
// stable hash of the user with the purchase amount into a [0,1) probability.
func churnScore(e Event) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(e.UserID))
	base := float64(h.Sum32()%1000) / 1000.0 // [0,1)

	// Larger purchases slightly lower modelled churn.
	adj := e.Amount / 1000.0
	score := base - adj
	if score < 0 {
		score = 0
	}
	if score >= 1 {
		score = 0.999
	}
	return score
}
