package sheets

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// AIEngine represents an AI text generator.
type AIEngine interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	HasAPIKey() bool
}

// Handler handles HTTP requests for sheets.
type Handler struct {
	store    *Store
	aiEngine AIEngine
}

// NewHandler creates a new sheets Handler.
func NewHandler(store *Store, aiEngine AIEngine) *Handler {
	return &Handler{
		store:    store,
		aiEngine: aiEngine,
	}
}

// RegisterRoutes registers sheets routes under /api/sheets.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	group := rg.Group("/sheets")
	{
		group.GET("", h.List)
		group.POST("", h.Create)
		group.GET("/:id", h.Get)
		group.PUT("/:id", h.Update)
		group.DELETE("/:id", h.Delete)
		group.POST("/:id/ai", h.AIAssist)
	}
}

// List returns all sheets.
func (h *Handler) List(c *gin.Context) {
	sheets, err := h.store.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sheets: " + err.Error()})
		return
	}
	if sheets == nil {
		sheets = []*Sheet{}
	}
	c.JSON(http.StatusOK, sheets)
}

// Create creates a new sheet.
func (h *Handler) Create(c *gin.Context) {
	var req CreateSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	sheet, err := h.store.Create(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create sheet: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, sheet)
}

// Get returns a single sheet by ID.
func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")
	sheet, err := h.store.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get sheet: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, sheet)
}

// Update updates a sheet's content or title.
func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")
	var req UpdateSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	sheet, err := h.store.Update(c.Request.Context(), id, req)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update sheet: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, sheet)
}

// Delete deletes a sheet.
func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := h.store.Delete(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete sheet: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "id": id})
}

// AIAssist provides intelligent sheet generation, formula help, or table analysis.
func (h *Handler) AIAssist(c *gin.Context) {
	if h.aiEngine == nil || !h.aiEngine.HasAPIKey() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI engine is not configured"})
		return
	}

	id := c.Param("id")
	sheet, err := h.store.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
		return
	}

	var req AIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	systemPrompt := "You are a spreadsheet AI expert running on Pocket Sentinel. " +
		"Assist the user with spreadsheet formulas, table data structure, or data analysis. " +
		"Answer concisely and directly in Thai. If the user asks for a formula, provide the exact formula like =SUM(...) or =A1*B1."

	userPrompt := "Current Sheet Title: " + sheet.Title + "\nUser Request: " + req.Prompt
	if len(sheet.DataJSON) > 2 && len(sheet.DataJSON) < 4000 {
		userPrompt += "\nSheet Data (JSON snippet): " + sheet.DataJSON
	}

	start := time.Now()
	res, err := h.aiEngine.Generate(c.Request.Context(), systemPrompt, userPrompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI generation failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, AIResponse{
		Action:     req.Action,
		Result:     res,
		Provider:   "fast-ai",
		DurationMS: time.Since(start).Milliseconds(),
	})
}
