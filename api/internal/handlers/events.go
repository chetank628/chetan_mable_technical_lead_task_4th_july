package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mable/mono/api/internal/model"
)

// Ingest accepts a single tracking event or a JSON array of them, stamps the
// server-derived fields, and submits each to the ingest worker without
// blocking. It returns 202 immediately so a tracking beacon never stalls the
// UI. Oversized bodies yield 413; malformed JSON yields 400.
func (h *Handler) Ingest(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.cfg.MaxBodyBytes)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "payload too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read body"})
		return
	}

	events, err := decodeEvents(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed JSON"})
		return
	}
	if len(events) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no events"})
		return
	}

	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	now := time.Now().UTC()

	accepted, dropped := 0, 0
	for _, in := range events {
		if h.worker.Submit(in.ToEvent(ip, ua, now)) {
			accepted++
		} else {
			dropped++
		}
	}

	c.JSON(http.StatusAccepted, gin.H{
		"accepted": accepted,
		"dropped":  dropped,
	})
}

// decodeEvents parses the body as either a single IngestEvent object or an
// array of them, returning a uniform slice.
func decodeEvents(raw []byte) ([]model.IngestEvent, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("empty body")
	}
	if trimmed[0] == '[' {
		var arr []model.IngestEvent
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	var one model.IngestEvent
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return nil, err
	}
	return []model.IngestEvent{one}, nil
}
