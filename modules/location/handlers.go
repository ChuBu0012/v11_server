package location

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxLocationRequestBytes   = 64 * 1024
	maximumFutureLocationSkew = 2 * time.Minute
)

type Module struct {
	store        *Store
	service      *Service
	pin          string
	linkResolver *LinkResolver
}

func NewModule(store *Store, service *Service, pin string) *Module {
	return &Module{store: store, service: service, pin: pin, linkResolver: NewLinkResolver()}
}

// ViewerToken returns the active viewer token string.
func (m *Module) ViewerToken() string {
	if m == nil || m.store == nil {
		return ""
	}
	_, viewer := m.store.Tokens()
	return viewer
}

func (m *Module) RegisterRoutes(r *gin.Engine) {
	r.POST("/api/location/report", m.report)
	r.GET("/api/location/live", m.live)
	r.GET("/api/location/geofences", m.requirePIN(), m.listGeofences)
	r.POST("/api/location/geofences", m.requirePIN(), m.createGeofence)
	r.PUT("/api/location/geofences/:id", m.requirePIN(), m.updateGeofence)
	r.DELETE("/api/location/geofences/:id", m.requirePIN(), m.deleteGeofence)
	r.PATCH("/api/location/settings", m.requirePIN(), m.updateSettings)
	r.POST("/api/location/resolve-link", m.requirePIN(), m.resolveLink)
}

func (m *Module) ViewerPage(page gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, viewerToken := m.store.Tokens()
		if !constantTimeEqual(c.Param("token"), viewerToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "ลิงก์แผนที่ไม่ถูกต้อง"})
			return
		}
		if page == nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		page(c)
	}
}

func (m *Module) report(c *gin.Context) {
	secretToken, _ := m.store.Tokens()
	providedToken := c.GetHeader("X-Location-Token")
	if providedToken == "" {
		providedToken = c.Query("token")
	}
	if !constantTimeEqual(providedToken, secretToken) {
		log.Printf("[location] rejected unauthenticated report")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "location token ไม่ถูกต้อง"})
		return
	}
	device := strings.TrimSpace(c.Query("device"))
	if !validDevice(device) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "device ต้องเป็น me หรือ friend"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxLocationRequestBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "OwnTracks payload ว่างหรือมีขนาดเกินกำหนด"})
		return
	}
	var report LocationReport
	if err := json.Unmarshal(body, &report); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "OwnTracks JSON ไม่ถูกต้อง"})
		return
	}
	if report.Type != "location" {
		c.Data(http.StatusOK, "application/json; charset=utf-8", []byte("[]"))
		return
	}
	battery := -1
	if report.Battery != nil {
		battery = *report.Battery
	}
	recordedAt := time.Unix(report.Timestamp, 0).UTC()
	if report.Timestamp <= 0 || recordedAt.After(time.Now().UTC().Add(maximumFutureLocationSkew)) || !ValidCoordinates(report.Latitude, report.Longitude) ||
		!finite(report.Accuracy) || report.Accuracy <= 0 || battery < -1 || battery > 100 ||
		report.BatteryStatus < 0 || report.BatteryStatus > 3 || !finite(report.Speed) ||
		!finite(report.Altitude) || !finite(report.Course) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "พิกัดหรือข้อมูล OwnTracks ไม่ถูกต้อง"})
		return
	}
	point := LocationPoint{
		DeviceID: device, Latitude: report.Latitude, Longitude: report.Longitude,
		Accuracy: report.Accuracy, Speed: report.Speed, Altitude: report.Altitude,
		Course: report.Course, Battery: battery, BatteryStatus: report.BatteryStatus,
		RecordedAt: recordedAt,
	}
	if _, err := m.service.ProcessPoint(c.Request.Context(), point); err != nil {
		log.Printf("[location] report processing failed for device=%s: %v", device, err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "บันทึกข้อมูลตำแหน่งไม่สำเร็จ"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", []byte("[]"))
}

func (m *Module) live(c *gin.Context) {
	_, viewerToken := m.store.Tokens()
	provided := c.GetHeader("X-Location-Viewer-Token")
	if provided == "" {
		provided = c.Query("token")
	}
	if !constantTimeEqual(provided, viewerToken) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "viewer token ไม่ถูกต้อง"})
		return
	}
	response, err := m.service.Live(c.Request.Context())
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "โหลดข้อมูลตำแหน่งไม่สำเร็จ"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, response)
}

func (m *Module) listGeofences(c *gin.Context) {
	zones, err := m.store.ListGeofences(c.Request.Context())
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "โหลด geofence ไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, zones)
}

func (m *Module) createGeofence(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var input GeofenceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "ข้อมูล geofence ไม่ถูกต้อง"})
		return
	}
	zone, err := m.store.CreateGeofence(c.Request.Context(), input)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, zone)
}

func (m *Module) updateGeofence(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "geofence id ไม่ถูกต้อง"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var input GeofenceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "ข้อมูล geofence ไม่ถูกต้อง"})
		return
	}
	zone, found, err := m.service.UpdateGeofence(c.Request.Context(), id, input)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "ไม่พบ geofence"})
		return
	}
	c.JSON(http.StatusOK, zone)
}

func (m *Module) deleteGeofence(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "geofence id ไม่ถูกต้อง"})
		return
	}
	deleted, err := m.store.DeleteGeofence(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "ลบ geofence ไม่สำเร็จ"})
		return
	}
	if !deleted {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "ไม่พบ geofence"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (m *Module) updateSettings(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var input SettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "ข้อมูล settings ไม่ถูกต้อง"})
		return
	}
	settings, err := m.store.UpdateSettings(c.Request.Context(), input)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "บันทึก settings ไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (m *Module) requirePIN() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m.pin == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "ยังไม่ได้ตั้งค่า PIN"})
			return
		}
		provided := c.GetHeader("X-Relay-PIN")
		if provided == "" {
			provided = c.GetHeader("X-PIN")
		}
		if provided == "" {
			if auth := c.GetHeader("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				provided = strings.TrimSpace(auth[7:])
			}
		}
		if provided == "" {
			provided = c.Query("pin")
		}
		if !constantTimeEqual(provided, m.pin) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "PIN ไม่ถูกต้อง"})
			return
		}
		c.Next()
	}
}

func constantTimeEqual(provided, expected string) bool {
	return len(provided) == len(expected) && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (m *Module) resolveLink(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2*1024)
	var input struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.URL) == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "รองรับเฉพาะลิงก์ Google Maps หรือพิกัด lat, lng"})
		return
	}
	lat, lng, err := m.linkResolver.Resolve(c.Request.Context(), input.URL)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"latitude": lat, "longitude": lng})
	case errors.Is(err, ErrLinkNoCoordinates):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": "ลิงก์นี้ไม่มีพิกัด ลองกดค้างบนแผนที่แทน"})
	case errors.Is(err, ErrLinkNotSupported):
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "รองรับเฉพาะลิงก์ Google Maps หรือพิกัด lat, lng"})
	default:
		log.Printf("[location] resolve link failed: %v", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "เปิดลิงก์ไม่สำเร็จ ลองใหม่อีกครั้ง"})
	}
}
