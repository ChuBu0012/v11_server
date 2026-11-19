package dashboard

import (
	"net/http"
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
	if !strings.Contains(body, "Hardware &amp; Runtime Metrics") && !strings.Contains(body, "Hardware & Runtime Metrics") {
		t.Errorf("Expected dashboard body to contain hardware metrics section")
	}
	if !strings.Contains(body, "Personal Watchtower") {
		t.Errorf("Expected dashboard body to contain 'Personal Watchtower'")
	}
	if !strings.Contains(body, "vivo V11") || !strings.Contains(body, "observer") && !strings.Contains(body, "Observer") {
		t.Errorf("Expected dashboard body to contain V11 observer perspective badge")
	}
	if !strings.Contains(body, "watchtowerSummaryComponent") {
		t.Errorf("Expected dashboard body to contain 'watchtowerSummaryComponent'")
	}

	// Secret scan: Ensure no bot tokens or sensitive placeholders leaked into HTML
	forbidden := []string{"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "NGROK_AUTHTOKEN", "BEGIN PRIVATE KEY"}
	for _, term := range forbidden {
		if strings.Contains(body, term) {
			t.Errorf("Security risk: found '%s' in dashboard HTML", term)
		}
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
	if !strings.Contains(swBody, "/watchtower") {
		t.Errorf("Expected sw.js to cache '/watchtower'")
	}
	if !strings.Contains(swBody, "/static/js/components/watchtower.js") {
		t.Errorf("Expected sw.js to cache '/static/js/components/watchtower.js'")
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

func TestWatchtowerEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mod := New()
	r := gin.New()
	mod.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/watchtower", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 for /watchtower, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Expected Content-Type to contain 'text/html', got '%s'", ct)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Personal Watchtower") {
		t.Errorf("Expected /watchtower body to contain 'Personal Watchtower'")
	}
	if !strings.Contains(body, "vivo V11") || !strings.Contains(body, "observer") && !strings.Contains(body, "Observer") {
		t.Errorf("Expected /watchtower body to contain V11 perspective badge")
	}
	if !strings.Contains(body, "watchtowerComponent") {
		t.Errorf("Expected /watchtower body to contain 'watchtowerComponent'")
	}
	if !strings.Contains(body, "watcher-chart-") {
		t.Errorf("Expected /watchtower body to contain watcher chart canvas")
	}
	if !strings.Contains(body, "Check now") {
		t.Errorf("Expected /watchtower body to contain 'Check now' button")
	}

	// HEAD request verification
	reqHead := httptest.NewRequest(http.MethodHead, "/watchtower", nil)
	rrHead := httptest.NewRecorder()
	r.ServeHTTP(rrHead, reqHead)
	if rrHead.Code != http.StatusOK {
		t.Errorf("Expected 200 for HEAD /watchtower, got %d", rrHead.Code)
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
