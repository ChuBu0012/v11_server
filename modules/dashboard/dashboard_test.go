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
