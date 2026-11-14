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
	repo       *Repository
	collector  *Collector
	alertStore *alerts.AlertStore
	thresholds alerts.Thresholds
}

// NewModule creates a new telemetry Module.
func NewModule(db *sql.DB, collector *Collector, alertStore *alerts.AlertStore, thresholds alerts.Thresholds) *Module {
	return &Module{
		repo:       NewRepository(db),
		collector:  collector,
		alertStore: alertStore,
		thresholds: thresholds,
	}
}

// RegisterRoutes mounts telemetry endpoints on the router group.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/history", m.HandleHistory)
	rg.GET("/current", m.HandleCurrent)
}

// HandleHistory returns bounded time-series health samples.
// Query params:
//   - limit: integer, default 60, max 500
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

	samples, err := m.repo.GetRecentSamples(c.Request.Context(), limit, maxAge)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve telemetry history"})
		return
	}

	c.JSON(http.StatusOK, samples)
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
		"latest_sample":  latest,
		"last_collected": lastCollected,
		"last_error":     lastErr,
		"conditions":     conditions,
	})
}
