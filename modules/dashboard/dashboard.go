package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed index.html
var indexHTML []byte

//go:embed tele-bridge.html
var teleBridgeHTML []byte

//go:embed schedule.html
var scheduleHTML []byte

//go:embed live_map.html
var liveMapHTML []byte

//go:embed endpoints.html
var endpointsHTML []byte

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
type Module struct {
	liveMapEnabled bool
}

// New creates a new dashboard Module.
func New() *Module {
	return &Module{}
}

// SetLiveMapEnabled controls whether Live Map links/cards are rendered in dashboard HTML.
func (m *Module) SetLiveMapEnabled(enabled bool) {
	m.liveMapEnabled = enabled
}

func (m *Module) renderPage(c *gin.Context, raw []byte) {
	out := raw
	if !m.liveMapEnabled {
		out = stripMarkedBlocks(raw, "<!-- LIVE_MAP_START -->", "<!-- LIVE_MAP_END -->")
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", out)
}

func stripMarkedBlocks(src []byte, startMarker, endMarker string) []byte {
	s := string(src)
	for {
		start := strings.Index(s, startMarker)
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], endMarker)
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+len(endMarker):]
	}
	return []byte(s)
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
	r.GET("/endpoints", m.HandleEndpoints)
	r.HEAD("/endpoints", m.HandleEndpoints)
	r.GET("/watchtower", func(c *gin.Context) { c.Redirect(http.StatusTemporaryRedirect, "/") })
	r.HEAD("/watchtower", func(c *gin.Context) { c.Redirect(http.StatusTemporaryRedirect, "/") })

	subFS, err := fs.Sub(staticFS, "static")
	if err == nil {
		r.StaticFS("/static", http.FS(subFS))
	}
}

// HandleDashboard serves the single-page responsive dashboard.
func (m *Module) HandleDashboard(c *gin.Context) {
	m.renderPage(c, indexHTML)
}

// HandleTeleBridge serves the dedicated standalone Telegram Bridge interface.
func (m *Module) HandleTeleBridge(c *gin.Context) {
	m.renderPage(c, teleBridgeHTML)
}

// HandleSchedule serves the focused personal reminder calendar.
func (m *Module) HandleSchedule(c *gin.Context) {
	m.renderPage(c, scheduleHTML)
}

// HandleLiveMap serves the responsive tracker viewer after token authorization.
func (m *Module) HandleLiveMap(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Data(http.StatusOK, "text/html; charset=utf-8", liveMapHTML)
}

// HandleEndpoints serves the dedicated Gatus health endpoints management interface.
func (m *Module) HandleEndpoints(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", endpointsHTML)
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
