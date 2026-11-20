package dashboard

import (
	"net/http"
	"regexp"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDashboardEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Expected Content-Type to contain 'text/html', got '%s'", ct)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Pocket Sentinel") {
		t.Errorf("Expected dashboard body to contain 'Pocket Sentinel'")
	}
	if !strings.Contains(body, "สถานะตอนนี้") || !strings.Contains(body, "metric-grid") {
		t.Errorf("Expected dashboard body to contain the current device status section")
	}
	if strings.Contains(body, "Personal Watchtower") || strings.Contains(body, "watchtowerSummaryComponent") || strings.Contains(body, "Monitor") {
		t.Errorf("Expected retired Monitor and Watchtower interfaces to be absent from the dashboard")
	}
	if !strings.Contains(body, "href=\"/live\"") {
		t.Errorf("Expected dashboard body to contain nav link to '/live'")
	}
	if !strings.Contains(body, "chart-insight") || !strings.Contains(body, "task-nav") {
		t.Errorf("Expected dashboard body to include telemetry insight and task navigation")
	}

	// Secret scan: Ensure no bot tokens or sensitive placeholders leaked into HTML
	forbidden := []string{"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "NGROK_AUTHTOKEN", "BEGIN PRIVATE KEY"}
	for _, term := range forbidden {
		if strings.Contains(body, term) {
			t.Errorf("Security risk: found '%s' in dashboard HTML", term)
		}
	}
}

func TestLiveMapPageIsEmbeddedAndResponsive(t *testing.T) {
	mod := New()
	r := gin.New()
	r.GET("/live-page", mod.HandleLiveMap)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/live-page", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("live map status = %d", response.Code)
	}
	for _, want := range []string{"id=\"live-map-canvas\"", "live-map.css", "live-map.js", "viewport-fit=cover", "theme-toggle-btn", "task-nav"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("embedded live map page missing %q", want)
		}
	}
	for _, want := range []string{`class="map-sheet`, "friendHeadline()", `class="manage-sheet`, `class="place-sheet`, "map-live-btn",
		"navigateToFriend()", "copyFriendCoords()", "applyLink()", "theme-toggle-btn"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("live map page missing layout A element %q", want)
		}
	}
	for _, gone := range []string{"map-controls-group", "live-summary", "picker-step-list", "picker-suggestion-chips", `class="map-status"`, "picker-reticle"} {
		if strings.Contains(response.Body.String(), gone) {
			t.Errorf("live map page still contains retired %q", gone)
		}
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Error("live map response must not be cached and should use strict-origin-when-cross-origin")
	}
}

func TestPWARoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)

	// 1. Manifest
	reqM := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	rrM := httptest.NewRecorder()
	r.ServeHTTP(rrM, reqM)
	if rrM.Code != http.StatusOK {
		t.Fatalf("Expected 200 for manifest.json, got %d", rrM.Code)
	}
	if !strings.Contains(rrM.Header().Get("Content-Type"), "json") {
		t.Errorf("Expected JSON content-type for manifest, got %s", rrM.Header().Get("Content-Type"))
	}
	if !strings.Contains(rrM.Body.String(), "Pocket Sentinel") {
		t.Errorf("Expected manifest body to contain 'Pocket Sentinel'")
	}

	// 2. Service Worker
	reqSW := httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	rrSW := httptest.NewRecorder()
	r.ServeHTTP(rrSW, reqSW)
	if rrSW.Code != http.StatusOK {
		t.Fatalf("Expected 200 for sw.js, got %d", rrSW.Code)
	}
	if !strings.Contains(rrSW.Header().Get("Content-Type"), "javascript") {
		t.Errorf("Expected javascript content-type for sw.js, got %s", rrSW.Header().Get("Content-Type"))
	}
	if rrSW.Header().Get("Service-Worker-Allowed") != "/" {
		t.Errorf("Expected Service-Worker-Allowed header to be '/'")
	}
	swBody := rrSW.Body.String()
	if !strings.Contains(swBody, "sentinel-shell-v") {
		t.Errorf("Expected sw.js to contain cache name 'sentinel-shell-v*', got body: %s", swBody)
	}
	if strings.Contains(swBody, "/watchtower") || strings.Contains(swBody, "/static/js/components/watchtower.js") || strings.Contains(swBody, "/static/js/components/monitor.js") {
		t.Errorf("Expected sw.js to exclude retired Monitor and Watchtower assets")
	}

	// 3. Icons
	for _, path := range []string{"/icon-192.png", "/icon-512.png", "/icon.svg"} {
		reqI := httptest.NewRequest(http.MethodGet, path, nil)
		rrI := httptest.NewRecorder()
		r.ServeHTTP(rrI, reqI)
		if rrI.Code != http.StatusOK {
			t.Errorf("Expected 200 for %s, got %d", path, rrI.Code)
		}
		if rrI.Body.Len() == 0 {
			t.Errorf("Expected non-empty body for %s", path)
		}
	}
}

func TestStaticRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)

	staticFiles := []string{
		"/static/css/styles.css",
		"/static/js/app.js",
		"/static/js/components/telemetry.js",
		"/static/js/components/charts.js",
		"/static/js/components/three-core.js",
		"/static/js/components/digest.js",
		"/static/js/components/events.js",
		"/static/js/components/watchtower.js",
	}

	for _, path := range staticFiles {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 for %s, got %d", path, rr.Code)
		}
		if rr.Body.Len() == 0 {
			t.Errorf("Expected non-empty body for %s", path)
		}
	}
}

func TestTeleBridgeEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)

	paths := []string{"/tele-bridge", "/tele-brige", "/relay"}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected 200 for %s, got %d", p, rr.Code)
		}

		body := rr.Body.String()
		if !strings.Contains(body, "Telegram Bridge") {
			t.Errorf("Expected body of %s to contain 'Telegram Bridge'", p)
		}
		if !strings.Contains(body, "relayComponent") {
			t.Errorf("Expected body of %s to contain 'relayComponent'", p)
		}
		if !strings.Contains(body, "proofread-btn") {
			t.Errorf("Expected body of %s to contain 'proofread-btn'", p)
		}
		if !strings.Contains(body, "proofread-diff-card") {
			t.Errorf("Expected body of %s to contain 'proofread-diff-card'", p)
		}
	}
}

func TestWatchtowerEndpointRedirectsToDashboard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/watchtower", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect || rr.Header().Get("Location") != "/" {
		t.Fatalf("/watchtower should redirect to dashboard, status=%d location=%q", rr.Code, rr.Header().Get("Location"))
	}

	// HEAD request verification
	reqHead := httptest.NewRequest(http.MethodHead, "/watchtower", nil)
	rrHead := httptest.NewRecorder()
	r.ServeHTTP(rrHead, reqHead)
	if rrHead.Code != http.StatusTemporaryRedirect {
		t.Errorf("Expected redirect for HEAD /watchtower, got %d", rrHead.Code)
	}
}

func TestScheduleNLEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/schedule", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 for /schedule, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "nl-prompt-input") {
		t.Errorf("Expected /schedule to contain 'nl-prompt-input'")
	}
	if !strings.Contains(body, "nl-parse-btn") {
		t.Errorf("Expected /schedule to contain 'nl-parse-btn'")
	}
	if !strings.Contains(body, "nl-preview-card") {
		t.Errorf("Expected /schedule to contain 'nl-preview-card'")
	}
	if !strings.Contains(body, "nl-ambiguity-alert") {
		t.Errorf("Expected /schedule to contain 'nl-ambiguity-alert'")
	}
	if !strings.Contains(body, "nl-confirm-save-btn") {
		t.Errorf("Expected /schedule to contain 'nl-confirm-save-btn'")
	}
	if !strings.Contains(body, "nl-open-modal-btn") {
		t.Errorf("Expected /schedule to contain 'nl-open-modal-btn'")
	}
}

func fetchPage(t *testing.T, path string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)
	r.GET("/live-page", mod.HandleLiveMap)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d", path, rr.Code)
	}
	return rr.Body.String()
}

func TestPagesDropIconFontAndShareNav(t *testing.T) {
	for _, path := range []string{"/", "/schedule", "/relay", "/live-page"} {
		body := fetchPage(t, path)
		if strings.Contains(body, "remixicon") {
			t.Errorf("%s still loads the remixicon font", path)
		}
		wants := []string{`class="task-nav"`, `<header class="topbar"`}
		if path == "/live-page" {
			wants = wants[:1] // the map uses a floating status chip instead of a topbar
		}
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing shared markup %q", path, want)
			}
		}
	}
}

func TestScheduleUsesMonthPlusDayLayout(t *testing.T) {
	body := fetchPage(t, "/schedule")
	for _, want := range []string{`id="day-heading"`, `class="fab"`, `id="month-grid"`, `id="agenda-list"`,
		"nl-prompt-input", "nl-parse-btn", "nl-preview-card", "nl-ambiguity-alert", "nl-confirm-save-btn", "nl-open-modal-btn", `id="nl-fallback-note"`} {
		if !strings.Contains(body, want) {
			t.Errorf("schedule page missing %q", want)
		}
	}
	for _, gone := range []string{"agenda-view-button", "month-view-button", "schedule-next-title", `id="nl-or"`} {
		if strings.Contains(body, gone) {
			t.Errorf("schedule page still contains retired %q", gone)
		}
	}
}

func TestShellAssetsAllExist(t *testing.T) {
	sw := fetchPage(t, "/sw.js")
	assets := regexp.MustCompile(`'(/static/[^']+)'`).FindAllStringSubmatch(sw, -1)
	if len(assets) == 0 {
		t.Fatal("no static shell assets found in sw.js")
	}
	for _, m := range assets {
		if body := fetchPage(t, m[1]); body == "" {
			t.Errorf("shell asset %s is empty", m[1])
		}
	}
	for _, want := range []string{"/static/css/dashboard.css", "/static/css/relay.css"} {
		if !strings.Contains(sw, want) {
			t.Errorf("sw.js shell cache missing %s", want)
		}
	}
}
