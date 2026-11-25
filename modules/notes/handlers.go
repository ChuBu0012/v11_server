package notes

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"v11-server/shared/ai"
)

// ScheduleBridge converts a note text into a scheduled reminder.
type ScheduleBridge func(ctx context.Context, rawText string) (title string, startsAt string, err error)

// Module serves Pocket Notes REST endpoints.
type Module struct {
	store          *Store
	aiEngine       *ai.Engine
	scheduleBridge ScheduleBridge
}

// NewModule constructs a new notes Module.
func NewModule(store *Store, aiEngine *ai.Engine, scheduleBridge ScheduleBridge) *Module {
	return &Module{
		store:          store,
		aiEngine:       aiEngine,
		scheduleBridge: scheduleBridge,
	}
}

// RegisterRoutes mounts /api/notes endpoints onto a Gin engine.
func (m *Module) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/notes")
	{
		api.GET("", m.list)
		api.POST("", m.create)
		api.PUT("/:id", m.update)
		api.POST("/:id/pin", m.togglePin)
		api.POST("/:id/archive", m.toggleArchive)
		api.POST("/:id/auto-tag", m.autoTag)
		api.POST("/:id/to-schedule", m.toSchedule)
		api.DELETE("/:id", m.delete)
	}
}

func (m *Module) list(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	archived := c.Query("archived") == "1" || strings.EqualFold(c.Query("archived"), "true")
	items, err := m.store.List(c.Request.Context(), q, archived, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดโน้ตไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (m *Module) create(c *gin.Context) {
	var input CreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		return
	}
	note, err := m.store.Create(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, note)
}

func (m *Module) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input UpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		return
	}
	note, err := m.store.Update(c.Request.Context(), id, input)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโน้ตที่ระบุ"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, note)
}

func (m *Module) togglePin(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	note, err := m.store.TogglePin(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโน้ตที่ระบุ"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, note)
}

func (m *Module) toggleArchive(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	note, err := m.store.ToggleArchive(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโน้ตที่ระบุ"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, note)
}

func (m *Module) autoTag(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	note, err := m.store.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโน้ตที่ระบุ"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rawBody := notePlainText(note)
	suggested := SuggestTags(c.Request.Context(), m.aiEngine, rawBody)
	if suggested == "" {
		suggested = "ทั่วไป"
	}
	combined := suggested
	if note.Tags != "" {
		combined = note.Tags + "," + suggested
	}
	updated, err := m.store.Update(c.Request.Context(), id, UpdateInput{Tags: &combined})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (m *Module) toSchedule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if m.scheduleBridge == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ระบบตารางนัดหมายยังไม่พร้อมใช้งาน"})
		return
	}
	note, err := m.store.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโน้ตที่ระบุ"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	raw := notePlainText(note)
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "โน้ตว่างเปล่าไม่สามารถตั้งเตือนได้"})
		return
	}
	title, startsAt, err := m.scheduleBridge(c.Request.Context(), raw)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"title":     title,
		"starts_at": startsAt,
	})
}

func (m *Module) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	err := m.store.Delete(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโน้ตที่ระบุ"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสโน้ตไม่ถูกต้อง"})
		return 0, false
	}
	return id, true
}

func notePlainText(n Note) string {
	var parts []string
	if n.Title != "" {
		parts = append(parts, n.Title)
	}
	if n.Content != "" {
		parts = append(parts, n.Content)
	}
	for _, it := range n.Items {
		if it.Text != "" {
			parts = append(parts, it.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// SuggestTags returns 1-3 short comma-separated tags using AI with deterministic keyword fallback.
func SuggestTags(ctx context.Context, engine *ai.Engine, text string) string {
	if engine != nil && engine.HasAPIKey() {
		res, err := engine.Generate(
			ctx,
			"ให้แท็กสั้นๆ 1-2 คำ ภาษาไทยหรืออังกฤษ คั่นด้วยเครื่องหมายจุลภาค (,) เท่านั้น ไม่ต้องใส่ # และไม่ต้องอธิบายเพิ่ม",
			text,
		)
		if err == nil && strings.TrimSpace(res) != "" {
			cleaned := normalizeTags(res)
			if cleaned != "" {
				return cleaned
			}
		}
	}

	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "ซื้อ") || strings.Contains(lower, "ของ") || strings.Contains(lower, "ตลาด"):
		return "ซื้อของ"
	case strings.Contains(lower, "ประชุม") || strings.Contains(lower, "งาน") || strings.Contains(lower, "deploy") || strings.Contains(lower, "โค้ด"):
		return "งาน"
	case strings.Contains(lower, "จ่าย") || strings.Contains(lower, "บิล") || strings.Contains(lower, "เงิน") || strings.Contains(lower, "โอน"):
		return "การเงิน"
	default:
		return "ส่วนตัว"
	}
}
