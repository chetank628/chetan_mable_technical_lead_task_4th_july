// Package model holds the request/response DTOs exchanged over the HTTP API.
// The streaming element type is pipeline.Event (reused directly as the
// pipeline's T), so this package only defines the wire shapes around it.
package model

import (
	"time"

	"github.com/mable/mono/pipeline"
)

// IngestEvent is the wire shape accepted by POST /api/events. It mirrors the
// tracking payload a browser beacon sends. Server-derived fields (IP,
// UserAgent, ReceivedAt) are filled in by the handler, never trusted from the
// client.
type IngestEvent struct {
	EventType  string            `json:"event_type"`
	UserID     string            `json:"user_id"`
	SessionID  string            `json:"session_id"`
	Timestamp  time.Time         `json:"timestamp"`
	URL        string            `json:"url"`
	Referrer   string            `json:"referrer"`
	UserAgent  string            `json:"user_agent"`
	Amount     float64           `json:"amount,omitempty"`
	Currency   string            `json:"currency,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`

	// Consent gates ingestion: events without consent are dropped at the first
	// pipeline stage (PII/consent gate from the brief).
	Consent bool `json:"consent"`
}

// ToEvent converts the wire DTO into the pipeline's Event, attaching the
// server-derived fields the client is not allowed to set.
func (in IngestEvent) ToEvent(ip, userAgent string, receivedAt time.Time) pipeline.Event {
	ua := in.UserAgent
	if ua == "" {
		ua = userAgent
	}
	props := in.Properties
	if props == nil {
		props = map[string]string{}
	}
	// Carry consent and server receipt time through the Properties map so the
	// homogeneous Pipeline[Event] can act on them without changing T.
	props["_consent"] = boolStr(in.Consent)
	props["_received_at"] = receivedAt.UTC().Format(time.RFC3339Nano)

	return pipeline.Event{
		EventType:  in.EventType,
		UserID:     in.UserID,
		SessionID:  in.SessionID,
		Timestamp:  in.Timestamp,
		URL:        in.URL,
		Referrer:   in.Referrer,
		UserAgent:  ua,
		IP:         ip,
		Amount:     in.Amount,
		Currency:   in.Currency,
		Properties: props,
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// --- Auth DTOs -------------------------------------------------------------

// Credentials is the body for signup and login.
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserResponse is the public view of an authenticated user.
type UserResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Stats DTOs ------------------------------------------------------------

// Stats is the computed analytics payload returned by GET /api/stats.
type Stats struct {
	AvgCaptureMs    float64           `json:"avg_capture_ms"`
	AvgEventParams  float64           `json:"avg_event_params"`
	TotalEvents     int64             `json:"total_events"`
	EventsOverTime  []TimeBucket      `json:"events_over_time"`
	TypeCounts      map[string]int64  `json:"type_counts"`
	TypeOverTime    []TypeTimeBucket  `json:"type_over_time"`
	StageRollups    []StageRollup     `json:"stage_rollups"`
}

// TimeBucket is a count of events within a time bucket (UTC, truncated).
type TimeBucket struct {
	Bucket time.Time `json:"bucket"`
	Count  int64     `json:"count"`
}

// TypeTimeBucket is a per-event-type count within a time bucket.
type TypeTimeBucket struct {
	Bucket    time.Time `json:"bucket"`
	EventType string    `json:"event_type"`
	Count     int64     `json:"count"`
}

// StageRollup aggregates per-stage pipeline metadata across all windows.
type StageRollup struct {
	StageName     string  `json:"stage_name"`
	Windows       int64   `json:"windows"`
	ItemsIn       int64   `json:"items_in"`
	ItemsOut      int64   `json:"items_out"`
	Dropped       int64   `json:"dropped"`
	Errors        int64   `json:"errors"`
	Batches       int64   `json:"batches"`
	AvgP50Ns      float64 `json:"avg_p50_ns"`
	AvgP99Ns      float64 `json:"avg_p99_ns"`
	AvgThroughput float64 `json:"avg_throughput"`
}
