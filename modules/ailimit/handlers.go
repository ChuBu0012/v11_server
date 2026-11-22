package ailimit

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Module encapsulates the AI Limit service, repository, and routes.
type Module struct {
	service   *Service
	collector *Collector
}

// NewModule constructs the ailimit Module.
func NewModule(service *Service, collector *Collector) *Module {
	return &Module{
		service:   service,
		collector: collector,
	}
}

// RegisterRoutes mounts the AI limit endpoints on the Gin router group.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", m.HandleGetLimits)
	rg.POST("/refresh", m.HandleRefresh)
}

// HandleGetLimits returns the current quota buckets, insights, and history.
func (m *Module) HandleGetLimits(c *gin.Context) {
	includeHistory := true
	if h := c.Query("history"); h == "false" || h == "0" {
		includeHistory = false
	}

	hours := 24
	if hs := c.Query("hours"); hs != "" {
		if parsed, err := strconv.Atoi(hs); err == nil && parsed > 0 {
			hours = parsed
		}
	}

	resp, err := m.service.GetStatus(c.Request.Context(), includeHistory, hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to retrieve AI limits",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// HandleRefresh triggers an on-demand collection pass and returns updated status.
func (m *Module) HandleRefresh(c *gin.Context) {
	resp, err := m.service.Collect(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to refresh AI limits",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Service returns the underlying service instance for bot or other module integration.
func (m *Module) Service() *Service {
	return m.service
}

// Collector returns the collector worker instance.
func (m *Module) Collector() *Collector {
	return m.collector
}
