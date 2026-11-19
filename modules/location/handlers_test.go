package location

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"v11-server/shared/db"
)

func newLocationTestRouter(t *testing.T) (*gin.Engine, *Module, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := db.InitDB(filepath.Join(t.TempDir(), "handlers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	secret, viewer := strings.Repeat("a", 32), strings.Repeat("b", 32)
	store, err := NewStore(database, Credentials{SecretToken: secret, ViewerToken: viewer, SecretExplicit: true, ViewerExplicit: true})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil)
	module := NewModule(store, service, "1804")
	router := gin.New()
	module.RegisterRoutes(router)
	router.GET("/live/:token", module.ViewerPage(func(c *gin.Context) { c.String(http.StatusOK, "viewer page") }))
	return router, module, secret, viewer
}

func TestReportEndpointReturnsOwnTracksEmptyAck(t *testing.T) {
	router, _, secret, _ := newLocationTestRouter(t)
	payload := fmt.Sprintf(`{"_type":"location","lat":13.7462,"lon":100.5348,"acc":15,"tst":%d,"batt":45,"bs":1,"vel":24,"alt":18,"cog":180}`, time.Now().Unix())
	req := httptest.NewRequest(http.MethodPost, "/api/location/report?device=me&token="+secret, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("report response = %d %q, want 200 []", response.Code, response.Body.String())
	}
}

func TestReportEndpointRejectsBadCredentialsAndPayload(t *testing.T) {
	router, _, secret, _ := newLocationTestRouter(t)
	tests := []struct {
		name   string
		url    string
		body   string
		status int
	}{
		{name: "bad token", url: "/api/location/report?device=me&token=wrong", body: `{}`, status: http.StatusUnauthorized},
		{name: "unknown device", url: "/api/location/report?device=other&token=" + secret, body: `{}`, status: http.StatusBadRequest},
		{name: "invalid coordinates", url: "/api/location/report?device=me&token=" + secret, body: `{"_type":"location","lat":91,"lon":0,"acc":5,"tst":1700000000}`, status: http.StatusBadRequest},
		{name: "zero accuracy", url: "/api/location/report?device=me&token=" + secret, body: `{"_type":"location","lat":0,"lon":0,"acc":0,"tst":1700000000}`, status: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader(tc.body))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}

func TestReportEndpointRejectsFutureTimestamp(t *testing.T) {
	router, _, secret, _ := newLocationTestRouter(t)
	payload := fmt.Sprintf(`{"_type":"location","lat":13.7462,"lon":100.5348,"acc":15,"tst":%d,"batt":45,"bs":1}`, time.Now().Add(10*time.Minute).Unix())
	req := httptest.NewRequest(http.MethodPost, "/api/location/report?device=me&token="+secret, strings.NewReader(payload))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("future timestamp status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
}

func TestLocationLiveEndpointIsTokenGuardedAndOmitsCredentials(t *testing.T) {
	router, module, secret, viewer := newLocationTestRouter(t)
	if _, err := module.service.ProcessPoint(t.Context(), samplePoint(DeviceMe, time.Now().UTC(), 13.7)); err != nil {
		t.Fatal(err)
	}
	if _, err := module.service.ProcessPoint(t.Context(), samplePoint(DeviceFriend, time.Now().UTC(), 13.7001)); err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/location/live", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized live status = %d, want 401", unauthorized.Code)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/location/live?token="+viewer, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("live status = %d body=%s", response.Code, response.Body.String())
	}
	var live LiveResponse
	if err := json.Unmarshal(response.Body.Bytes(), &live); err != nil {
		t.Fatal(err)
	}
	if live.Me == nil || live.Friend == nil || live.DistanceMeters == nil || len(live.Breadcrumbs[DeviceMe]) != 1 {
		t.Fatalf("live response missing device telemetry: %+v", live)
	}
	if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), viewer) {
		t.Fatalf("live response leaked credentials: %s", response.Body.String())
	}
}

func TestLocationManagementAPIsRequirePIN(t *testing.T) {
	router, _, _, viewer := newLocationTestRouter(t)
	post := func(pin string, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if pin != "" {
			req.Header.Set("X-Relay-PIN", pin)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if response := post("wrong", http.MethodGet, "/api/location/geofences", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong PIN status = %d, want 401", response.Code)
	}
	created := post("1804", http.MethodPost, "/api/location/geofences", `{"name":"บ้าน","latitude":13.7,"longitude":100.5,"radius_meters":150}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"notify_enter":true`) {
		t.Fatalf("create geofence response = %d %s", created.Code, created.Body.String())
	}
	var zone Geofence
	if err := json.Unmarshal(created.Body.Bytes(), &zone); err != nil {
		t.Fatal(err)
	}
	if listed := post("1804", http.MethodGet, "/api/location/geofences", ""); listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "บ้าน") {
		t.Fatalf("list geofences = %d %s", listed.Code, listed.Body.String())
	}
	patched := post("1804", http.MethodPatch, "/api/location/settings", `{"notify_battery":false}`)
	if patched.Code != http.StatusOK || !strings.Contains(patched.Body.String(), `"notify_battery":false`) {
		t.Fatalf("settings patch = %d %s", patched.Code, patched.Body.String())
	}
	deleted := post("1804", http.MethodDelete, fmt.Sprintf("/api/location/geofences/%d", zone.ID), "")
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete geofence = %d %s", deleted.Code, deleted.Body.String())
	}

	if page := post("", http.MethodGet, "/live/invalid", ""); page.Code != http.StatusUnauthorized {
		t.Fatalf("invalid viewer token status = %d, want 401", page.Code)
	}
	if page := post("", http.MethodGet, "/live/"+viewer, ""); page.Code != http.StatusOK || page.Body.String() != "viewer page" {
		t.Fatalf("viewer page response = %d %q", page.Code, page.Body.String())
	}
}
