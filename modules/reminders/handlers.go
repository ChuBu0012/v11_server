package reminders

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Module serves the schedule API. Writes are intentionally public (no PIN);
// the owner chose convenience over a PIN for this personal server.
type Module struct {
	store         *Store
	telegramReady bool
}

func NewModule(store *Store, telegramReady bool) *Module {
	return &Module{store: store, telegramReady: telegramReady}
}
func (m *Module) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/schedule/status", m.status)
	r.GET("/api/schedule/reminders", m.list)
	r.GET("/api/schedule/upcoming", m.upcoming)
	r.GET("/api/schedule/history", m.history)
	r.POST("/api/schedule/reminders", m.create)
	r.PUT("/api/schedule/reminders/:id", m.update)
	r.POST("/api/schedule/reminders/:id/pause", m.pause)
	r.POST("/api/schedule/reminders/:id/resume", m.resume)
	r.DELETE("/api/schedule/reminders/:id", m.delete)
	r.POST("/api/schedule/complete", m.complete)
}

func (m *Module) status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"telegram_ready": m.telegramReady, "timezone": timezone})
}

func (m *Module) list(c *gin.Context) {
	items, err := m.store.List(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": "โหลดรายการไม่สำเร็จ"})
		return
	}
	c.JSON(200, gin.H{"items": items, "timezone": timezone})
}
func (m *Module) upcoming(c *gin.Context) {
	items, err := m.store.Upcoming(c.Request.Context(), time.Now())
	if err != nil {
		c.JSON(500, gin.H{"error": "โหลดรายการถัดไปไม่สำเร็จ"})
		return
	}
	c.JSON(200, gin.H{"items": items, "timezone": timezone})
}
func (m *Module) history(c *gin.Context) {
	items, err := m.store.History(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": "โหลดประวัติไม่สำเร็จ"})
		return
	}
	c.JSON(200, gin.H{"items": items})
}
func (m *Module) create(c *gin.Context) {
	var input Input
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "ข้อมูลการเตือนไม่ถูกต้อง"})
		return
	}
	item, err := m.store.Create(c.Request.Context(), input)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, item)
}
func (m *Module) update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "รหัสรายการไม่ถูกต้อง"})
		return
	}
	var input Input
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "ข้อมูลการเตือนไม่ถูกต้อง"})
		return
	}
	if err = m.store.Update(c.Request.Context(), id, input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.Status(204)
}
func (m *Module) pause(c *gin.Context)  { m.setPaused(c, true) }
func (m *Module) resume(c *gin.Context) { m.setPaused(c, false) }
func (m *Module) setPaused(c *gin.Context, paused bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "รหัสรายการไม่ถูกต้อง"})
		return
	}
	if err = m.store.SetPaused(c.Request.Context(), id, paused); err != nil {
		c.JSON(404, gin.H{"error": "ไม่พบรายการประจำสัปดาห์"})
		return
	}
	c.Status(204)
}
func (m *Module) delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "รหัสรายการไม่ถูกต้อง"})
		return
	}
	if err = m.store.Delete(c.Request.Context(), id); err != nil {
		c.JSON(500, gin.H{"error": "ลบรายการไม่สำเร็จ"})
		return
	}
	c.Status(204)
}
func (m *Module) complete(c *gin.Context) {
	var input struct {
		ReminderID int64  `json:"reminder_id"`
		EventAt    string `json:"event_at"`
	}
	if c.ShouldBindJSON(&input) != nil || input.ReminderID < 1 {
		c.JSON(400, gin.H{"error": "ข้อมูลไม่ถูกต้อง"})
		return
	}
	event, err := time.Parse(time.RFC3339, input.EventAt)
	if err != nil {
		c.JSON(400, gin.H{"error": "เวลาไม่ถูกต้อง"})
		return
	}
	if err = m.store.Complete(c.Request.Context(), input.ReminderID, event); err != nil {
		c.JSON(500, gin.H{"error": "ทำเครื่องหมายไม่สำเร็จ"})
		return
	}
	c.Status(204)
}
