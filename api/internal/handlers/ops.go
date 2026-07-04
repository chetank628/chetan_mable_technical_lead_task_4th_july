package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Health is a combined liveness/readiness probe. It returns 200 only when the
// database responds to a ping; otherwise 503 so an orchestrator can hold
// traffic until the service is actually ready.
func (h *Handler) Health(c *gin.Context) {
	dbOK := h.store.Ping(c.Request.Context()) == nil
	status := http.StatusOK
	if !dbOK {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"status": map[bool]string{true: "ok", false: "degraded"}[dbOK],
		"db":     dbOK,
	})
}

// Metrics exposes ingest counters in Prometheus text exposition format. We emit
// it by hand to avoid pulling in the Prometheus client dependency for a handful
// of gauges and counters.
func (h *Handler) Metrics(c *gin.Context) {
	s := h.worker.Stats()
	var b strings.Builder

	writeMetric(&b, "counter", "mable_events_submitted_total",
		"Events offered to the ingest worker.", s.Submitted)
	writeMetric(&b, "counter", "mable_events_accepted_total",
		"Events accepted into the ingest buffer.", s.Accepted)
	writeMetric(&b, "counter", "mable_events_dropped_total",
		"Events dropped because the ingest buffer was full.", s.Dropped)
	writeMetric(&b, "counter", "mable_events_persisted_total",
		"Events written to SQLite.", s.Persisted)
	writeMetric(&b, "counter", "mable_pipeline_windows_total",
		"Completed windowed pipeline runs.", s.Windows)
	writeMetric(&b, "counter", "mable_pipeline_errors_total",
		"Windowed pipeline runs that returned an error.", s.PipelineErrors)
	writeMetric(&b, "gauge", "mable_ingest_queue_depth",
		"Current number of events buffered in the ingest channel.", int64(s.QueueDepth))
	writeMetric(&b, "gauge", "mable_ingest_queue_capacity",
		"Capacity of the ingest channel.", int64(s.QueueCapacity))
	writeMetric(&b, "gauge", "mable_last_window_latency_ns",
		"Wall-clock duration of the most recent window flush.", s.LastWindowNs)

	c.Header("Content-Type", "text/plain; version=0.0.4")
	c.String(http.StatusOK, b.String())
}

func writeMetric(b *strings.Builder, typ, name, help string, value int64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s %s\n", name, typ)
	fmt.Fprintf(b, "%s %d\n", name, value)
}
