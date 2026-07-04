package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Stats returns the computed analytics over a time range. Query params:
//   - since, until: RFC3339 timestamps (default: last 24h .. now)
//   - granularity:  minute|hour|day (default: hour)
func (h *Handler) Stats(c *gin.Context) {
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)
	until := now

	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t.UTC()
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'since' (want RFC3339)"})
			return
		}
	}
	if v := c.Query("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			until = t.UTC()
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'until' (want RFC3339)"})
			return
		}
	}
	if until.Before(since) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'until' must be after 'since'"})
		return
	}

	granularity := c.DefaultQuery("granularity", "hour")

	stats, err := h.store.Stats(c.Request.Context(), since, until, granularity)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute stats"})
		return
	}
	c.JSON(http.StatusOK, stats)
}
