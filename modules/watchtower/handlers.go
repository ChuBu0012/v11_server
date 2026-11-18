package watchtower

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Module encapsulates Watchtower handlers and service for app integration.
type Module struct {
	handler *Handler
	svc     *Service
}

// NewModule creates a new Watchtower Module.
func NewModule(svc *Service) *Module {
	return &Module{
		handler: NewHandler(svc),
		svc:     svc,
	}
}

// RegisterRoutes registers Watchtower API routes onto a Gin Engine.
func (m *Module) RegisterRoutes(r *gin.Engine) {
	m.handler.RegisterRoutes(r)
}

// RegisterRoutesGroup registers Watchtower API routes onto a Gin RouterGroup.
func (m *Module) RegisterRoutesGroup(rg *gin.RouterGroup) {
	m.handler.RegisterRoutesGroup(rg)
}

// Service returns the underlying Watchtower Service.
func (m *Module) Service() *Service {
	return m.svc
}

// Handler serves HTTP endpoints for Personal Watchtower.
type Handler struct {
	svc *Service
}

// NewHandler creates a new Handler instance.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers Watchtower API routes onto a Gin Engine.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	group := r.Group("/api/watchtower")
	h.RegisterRoutesGroup(group)
}

// RegisterRoutesGroup registers Watchtower API routes onto a Gin RouterGroup.
func (h *Handler) RegisterRoutesGroup(rg *gin.RouterGroup) {
	// Public read-only endpoints
	rg.GET("/summary", h.summary)
	rg.GET("/watchers", h.listWatchers)
	rg.GET("/watchers/:id/checks", h.listChecks)

	// PIN-protected mutation endpoints
	rg.POST("/watchers", h.authorize(), h.createWatcher)
	rg.PATCH("/watchers/:id", h.authorize(), h.updateWatcher)
	rg.POST("/watchers/:id/check", h.authorize(), h.checkNow)
	rg.POST("/watchers/:id/pause", h.authorize(), h.pauseWatcher)
	rg.POST("/watchers/:id/resume", h.authorize(), h.resumeWatcher)
	rg.DELETE("/watchers/:id", h.authorize(), h.deleteWatcher)
}

// authorize middleware validates X-Watchtower-PIN header using ConstantTimeCompare.
func (h *Handler) authorize() gin.HandlerFunc {
	return func(c *gin.Context) {
		configuredPIN := h.svc.PIN()
		if configuredPIN == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "Watchtower mutations unavailable: WATCHTOWER_PIN not configured",
			})
			return
		}

		provided := c.GetHeader("X-Watchtower-PIN")
		if provided == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				provided = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}
		if provided == "" {
			provided = c.Query("pin")
		}

		if len(provided) != len(configuredPIN) ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(configuredPIN)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "PIN ไม่ถูกต้อง หรือไม่ได้รับอนุญาต",
			})
			return
		}

		c.Next()
	}
}

func (h *Handler) summary(c *gin.Context) {
	sum, err := h.svc.Store().GetSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve summary: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, sum)
}

func (h *Handler) listWatchers(c *gin.Context) {
	items, err := h.svc.Store().ListWatchers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list watchers: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) listChecks(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watcher ID"})
		return
	}

	limit := 50
	if lStr := c.Query("limit"); lStr != "" {
		if parsedLimit, err := strconv.Atoi(lStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	checks, err := h.svc.Store().ListChecks(c.Request.Context(), id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list checks: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": checks})
}

func (h *Handler) createWatcher(c *gin.Context) {
	var input CreateWatcherInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	w, err := h.svc.Store().CreateWatcher(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, ErrMaxWatchersReached) || errors.Is(err, ErrInvalidName) ||
			errors.Is(err, ErrInvalidStatus) || errors.Is(err, ErrForbiddenIP) ||
			errors.Is(err, ErrInvalidScheme) || errors.Is(err, ErrLocalhost) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Immediately check the new watcher (spec §21: checked immediately upon creation)
	_, _ = h.svc.CheckWatcher(c.Request.Context(), w.ID, false)

	// Refresh watcher with latest probe state
	latest, err := h.svc.Store().GetWatcher(c.Request.Context(), w.ID)
	if err == nil {
		w = latest
	}

	c.JSON(http.StatusCreated, w)
}

func (h *Handler) updateWatcher(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watcher ID"})
		return
	}

	var input UpdateWatcherInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	existing, err := h.svc.Store().GetWatcher(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrWatcherNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Watcher not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	urlChanged := input.URL != nil && *input.URL != existing.URL
	statusChanged := input.ExpectedStatus != nil && *input.ExpectedStatus != existing.ExpectedStatus

	updated, err := h.svc.Store().UpdateWatcher(c.Request.Context(), id, input)
	if err != nil {
		if errors.Is(err, ErrInvalidName) || errors.Is(err, ErrInvalidStatus) ||
			errors.Is(err, ErrForbiddenIP) || errors.Is(err, ErrInvalidScheme) ||
			errors.Is(err, ErrLocalhost) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Spec §72: Editing URL or expected status resets the watcher to unknown baseline,
	// clears active outage metadata, and immediately performs a diagnostic check.
	// Configuration edits never generate recovery messages.
	if urlChanged || statusChanged {
		_ = h.svc.Store().ResetWatcherBaseline(c.Request.Context(), id)
		_, _ = h.svc.CheckWatcher(c.Request.Context(), id, true) // diagnostic check
		if refreshed, err := h.svc.Store().GetWatcher(c.Request.Context(), id); err == nil {
			updated = refreshed
		}
	}

	c.JSON(http.StatusOK, updated)
}

func (h *Handler) checkNow(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watcher ID"})
		return
	}

	res, err := h.svc.CheckWatcher(c.Request.Context(), id, true)
	if err != nil {
		if errors.Is(err, ErrWatcherNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Watcher not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *Handler) pauseWatcher(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watcher ID"})
		return
	}

	if err := h.svc.Store().PauseWatcher(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrWatcherNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Watcher not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "state": StatePaused})
}

func (h *Handler) resumeWatcher(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watcher ID"})
		return
	}

	if err := h.svc.Store().ResumeWatcher(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrWatcherNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Watcher not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Immediately check target upon resume
	_, _ = h.svc.CheckWatcher(c.Request.Context(), id, false)

	latest, err := h.svc.Store().GetWatcher(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"id": id, "state": StateUnknown})
		return
	}

	c.JSON(http.StatusOK, latest)
}

func (h *Handler) deleteWatcher(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watcher ID"})
		return
	}

	if err := h.svc.Store().DeleteWatcher(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrWatcherNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Watcher not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
