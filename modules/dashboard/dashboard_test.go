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
