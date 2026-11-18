package digest

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Module wraps digest service and HTTP routing.
type Module struct {
	service *Service
}

// New creates a new digest Module.
func New(service *Service) *Module {
	return &Module{service: service}
}

// RegisterRoutes registers endpoints under /api/digest.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/status", m.handleStatus)
	rg.POST("/toggle", m.handleToggle)
	rg.POST("/run", m.handleRun)
	rg.GET("/history", m.handleHistory)
}

func (m *Module) handleStatus(c *gin.Context) {
	settings, err := m.service.store.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}

	latest, err := m.service.store.GetLatestRun(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load latest digest"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"settings":   settings,
		"latest_run": latest,
	})
}

func (m *Module) handleToggle(c *gin.Context) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		// Toggle current state if not explicitly specified
		settings, err := m.service.store.GetSettings(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read settings"})
			return
		}
		newVal := !settings.Enabled
		body.Enabled = &newVal
	}

	if err := m.service.store.SetEnabled(c.Request.Context(), *body.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}

	settings, _ := m.service.store.GetSettings(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"message":  "Settings updated",
		"settings": settings,
	})
}

func (m *Module) handleRun(c *gin.Context) {
	// Force run regardless of enabled setting
	record, err := m.service.GenerateDigest(c.Request.Context(), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "Failed to generate digest",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Digest generated successfully",
		"digest":  record,
	})
}

func (m *Module) handleHistory(c *gin.Context) {
	history, err := m.service.store.GetHistory(c.Request.Context(), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load history"})
		return
	}
	c.JSON(http.StatusOK, history)
}
