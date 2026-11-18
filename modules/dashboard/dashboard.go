package dashboard

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed index.html
var indexHTML []byte

//go:embed tele-bridge.html
var teleBridgeHTML []byte

//go:embed schedule.html
var scheduleHTML []byte

//go:embed manifest.json
var manifestJSON []byte

//go:embed sw.js
var swJS []byte

//go:embed icon-192.png
var icon192PNG []byte

//go:embed icon-512.png
var icon512PNG []byte

//go:embed icon.svg
var iconSVG []byte

//go:embed static/*
var staticFS embed.FS

// Module handles serving the Pocket Sentinel status dashboard and PWA assets.
type Module struct{}

// New creates a new dashboard Module.
func New() *Module {
	return &Module{}
}

// RegisterRoutes mounts the dashboard and PWA endpoints on the root engine.
func (m *Module) RegisterRoutes(r *gin.Engine) {
	r.GET("/", m.HandleDashboard)
	r.HEAD("/", m.HandleDashboard)
	r.GET("/manifest.json", m.HandleManifest)
	r.GET("/sw.js", m.HandleServiceWorker)
	r.GET("/icon-192.png", m.HandleIcon192)
	r.GET("/icon-512.png", m.HandleIcon512)
	r.GET("/icon.svg", m.HandleIconSVG)

	// Dedicated Telegram Bridge Pages
	r.GET("/tele-bridge", m.HandleTeleBridge)
	r.HEAD("/tele-bridge", m.HandleTeleBridge)
	r.GET("/tele-brige", m.HandleTeleBridge)
	r.HEAD("/tele-brige", m.HandleTeleBridge)
	r.GET("/relay", m.HandleTeleBridge)
	r.HEAD("/relay", m.HandleTeleBridge)
	r.GET("/schedule", m.HandleSchedule)
	r.HEAD("/schedule", m.HandleSchedule)

	subFS, err := fs.Sub(staticFS, "static")
	if err == nil {
		r.StaticFS("/static", http.FS(subFS))
	}
}

// HandleDashboard serves the single-page responsive dashboard.
func (m *Module) HandleDashboard(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}

// HandleTeleBridge serves the dedicated standalone Telegram Bridge interface.
func (m *Module) HandleTeleBridge(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", teleBridgeHTML)
}

// HandleSchedule serves the focused personal reminder calendar.
func (m *Module) HandleSchedule(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", scheduleHTML)
}

// HandleManifest serves the web app manifest for PWA installation.
func (m *Module) HandleManifest(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/manifest+json; charset=utf-8", manifestJSON)
}

// HandleServiceWorker serves the service worker script.
func (m *Module) HandleServiceWorker(c *gin.Context) {
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Service-Worker-Allowed", "/")
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", swJS)
}

// HandleIcon192 serves the 192x192 PNG icon.
func (m *Module) HandleIcon192(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/png", icon192PNG)
}

// HandleIcon512 serves the 512x512 PNG icon.
func (m *Module) HandleIcon512(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/png", icon512PNG)
}

// HandleIconSVG serves the scalable SVG icon.
func (m *Module) HandleIconSVG(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/svg+xml", iconSVG)
}
