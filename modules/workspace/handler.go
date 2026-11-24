package workspace

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Module wraps workspace HTTP handlers.
type Module struct {
	service *Service
}

// NewModule constructs a workspace Module.
func NewModule(service *Service) *Module {
	return &Module{service: service}
}

// RegisterRoutes registers notes and sheets REST endpoints.
func (m *Module) RegisterRoutes(r *gin.Engine) {
	notes := r.Group("/api/notes")
	{
		notes.GET("", m.handleListNotes)
		notes.POST("", m.handleCreateNote)
		notes.GET("/:id", m.handleGetNote)
		notes.PUT("/:id", m.handleUpdateNote)
		notes.DELETE("/:id", m.handleDeleteNote)
		notes.POST("/:id/ai", m.handleNoteAI)
	}

	sheets := r.Group("/api/sheets")
	{
		sheets.GET("", m.handleListSheets)
		sheets.POST("", m.handleCreateSheet)
		sheets.GET("/:id", m.handleGetSheet)
		sheets.PUT("/:id", m.handleUpdateSheet)
		sheets.DELETE("/:id", m.handleDeleteSheet)
		sheets.POST("/:id/ai", m.handleSheetAI)
	}
}

// Service returns the underlying workspace service.
func (m *Module) Service() *Service {
	return m.service
}

// Notes Handlers

func (m *Module) handleListNotes(c *gin.Context) {
	tag := c.Query("tag")
	search := c.Query("search")
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))

	filter := NoteFilter{
		Tag:    tag,
		Search: search,
		Limit:  limit,
		Offset: offset,
	}

	notes, err := m.service.ListNotes(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, notes)
}

func (m *Module) handleCreateNote(c *gin.Context) {
	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	note, err := m.service.CreateNote(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, ErrNoteTooLarge) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, note)
}

func (m *Module) handleGetNote(c *gin.Context) {
	id := c.Param("id")
	note, err := m.service.GetNote(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, note)
}

func (m *Module) handleUpdateNote(c *gin.Context) {
	id := c.Param("id")
	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	note, err := m.service.UpdateNote(c.Request.Context(), id, &req)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
			return
		}
		if errors.Is(err, ErrNoteTooLarge) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, note)
}

func (m *Module) handleDeleteNote(c *gin.Context) {
	id := c.Param("id")
	err := m.service.DeleteNote(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "id": id})
}

func (m *Module) handleNoteAI(c *gin.Context) {
	id := c.Param("id")
	var req NoteAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	res, err := m.service.RunNoteAI(c.Request.Context(), id, &req)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// Sheets Handlers

func (m *Module) handleListSheets(c *gin.Context) {
	sheets, err := m.service.ListSheets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sheets)
}

func (m *Module) handleCreateSheet(c *gin.Context) {
	var req CreateSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	sheet, err := m.service.CreateSheet(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, ErrSheetDimensionsExceed) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, sheet)
}

func (m *Module) handleGetSheet(c *gin.Context) {
	id := c.Param("id")
	sheet, err := m.service.GetSheet(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrSheetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sheet)
}

func (m *Module) handleUpdateSheet(c *gin.Context) {
	id := c.Param("id")
	var req UpdateSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	sheet, err := m.service.UpdateSheet(c.Request.Context(), id, &req)
	if err != nil {
		if errors.Is(err, ErrSheetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
			return
		}
		if errors.Is(err, ErrSheetDimensionsExceed) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sheet)
}

func (m *Module) handleDeleteSheet(c *gin.Context) {
	id := c.Param("id")
	err := m.service.DeleteSheet(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrSheetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "id": id})
}

func (m *Module) handleSheetAI(c *gin.Context) {
	id := c.Param("id")
	var req SheetAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	res, err := m.service.RunSheetAI(c.Request.Context(), id, &req)
	if err != nil {
		if errors.Is(err, ErrSheetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
