package telemetry

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"v11-server/modules/alerts"
)

// Module encapsulates telemetry routes and services.
type Module struct {
	repo           *Repository
	collector      *Collector
	alertStore     *alerts.AlertStore
	thresholds     alerts.Thresholds
	sampleInterval time.Duration
}

// NewModule creates a new telemetry Module.
func NewModule(db *sql.DB, collector *Collector, alertStore *alerts.AlertStore, thresholds alerts.Thresholds, sampleIntervals ...time.Duration) *Module {
	interval := time.Minute
	if len(sampleIntervals) > 0 && sampleIntervals[0] > 0 {
		interval = sampleIntervals[0]
	}
	return &Module{
		repo:           NewRepository(db),
		collector:      collector,
		alertStore:     alertStore,
		thresholds:     thresholds,
		sampleInterval: interval,
	}
}

// RegisterRoutes mounts telemetry endpoints on the router group.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/history", m.HandleHistory)
	rg.GET("/current", m.HandleCurrent)
}

// HandleHistory returns bounded time-series health samples.
// Query params:
//   - limit: integer, default 60, max 500 (response points)
//   - hours: integer, default 24, max 168 (7 days)
func (m *Module) HandleHistory(c *gin.Context) {
	limit := 60
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		} else if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit parameter"})
			return
		}
	}
	if limit > 500 {
		limit = 500
	}

	maxAge := 24 * time.Hour
	if hStr := c.Query("hours"); hStr != "" {
		if h, err := strconv.Atoi(hStr); err == nil && h > 0 {
			if h > 168 {
				h = 168
			}
			maxAge = time.Duration(h) * time.Hour
		} else if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid hours parameter"})
			return
		}
	}

	// Read a bounded raw window, then select evenly spaced points so a full
	// 24-hour chart does not collapse to only the newest 500 samples.
	rawLimit := limit * 10
	if rawLimit < limit {
		rawLimit = 5000
	}
	if rawLimit > 5000 {
		rawLimit = 5000
	}
	samples, err := m.repo.GetRecentSamples(c.Request.Context(), rawLimit, maxAge)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve telemetry history"})
		return
	}

	c.JSON(http.StatusOK, downsampleChronologically(samples, limit))
}

func downsampleChronologically(descending []HealthSample, limit int) []HealthSample {
	if len(descending) <= limit {
		result := make([]HealthSample, len(descending))
		copy(result, descending)
		return result
	}
	if limit <= 1 {
		return []HealthSample{descending[0]}
	}
	result := make([]HealthSample, 0, limit)
	last := len(descending) - 1
	for i := 0; i < limit; i++ {
		index := (i * last) / (limit - 1)
		result = append(result, descending[index])
	}
	return result
}

// HandleCurrent returns the most recent sample, collector status, and active alert conditions.
func (m *Module) HandleCurrent(c *gin.Context) {
	ctx := c.Request.Context()

	latest, err := m.repo.GetLatestSample(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve latest sample"})
		return
	}

	lastCollected, lastErr := m.collector.LastCollectionInfo()

	var conditions []alerts.Condition
	if latest != nil {
		alertSample := alerts.Sample{
			Timestamp:     latest.Timestamp,
			UptimeSeconds: latest.UptimeSeconds,
			MemAllocMB:    latest.MemoryAllocMB,
			MemSysMB:      latest.MemorySysMB,
			DiskTotalGB:   latest.DiskTotalGB,
			DiskFreeGB:    latest.DiskFreeGB,
			BatteryLevel:  latest.BatteryLevel,
			BatteryStatus: latest.BatteryStatus,
			ServiceOK:     true,
		}
		conditions = alerts.Evaluate(alertSample, m.thresholds)
	}

	c.JSON(http.StatusOK, gin.H{
		"latest_sample":           latest,
		"last_collected":          lastCollected,
		"last_error":              lastErr,
		"sample_interval_seconds": int64(m.sampleInterval.Seconds()),
		"conditions":              conditions,
	})
}
