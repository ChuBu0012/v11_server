package endpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestModule(t *testing.T) (*Module, string) {
	gin.SetMode(gin.TestMode)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `web:
  port: 8081
  address: "0.0.0.0"

storage:
  type: sqlite
  path: "` + tmpDir + `/gatus.db"

endpoints:
  - name: "Existing Service"
    group: "Core Services"
    url: "http://localhost:8080/health"
    interval: 30s
    conditions:
      - "[STATUS] == 200"
`
	if err := os.WriteFile(configPath, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	mod := New(configPath)
	return mod, configPath
}

func TestHandleGetEndpoints(t *testing.T) {
	mod, _ := setupTestModule(t)

	r := gin.New()
	rg := r.Group("/api")
	mod.RegisterRoutes(rg)

	req := httptest.NewRequest(http.MethodGet, "/api/endpoints", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var resp struct {
		Status    string         `json:"status"`
		GatusURL  string         `json:"gatus_url"`
		Endpoints []EndpointItem `json:"endpoints"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Expected status 'ok', got %q", resp.Status)
	}
	if len(resp.Endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(resp.Endpoints))
	}
	if resp.Endpoints[0].Name != "Existing Service" {
		t.Errorf("Expected endpoint name 'Existing Service', got %q", resp.Endpoints[0].Name)
	}
}

func TestHandleAddEndpoint(t *testing.T) {
	mod, configPath := setupTestModule(t)

	r := gin.New()
	rg := r.Group("/api")
	mod.RegisterRoutes(rg)

	// 1. Add valid endpoint
	payload := `{"name": "New API", "url": "api.example.com/status", "group": "Websites", "interval": "60s"}`
	req := httptest.NewRequest(http.MethodPost, "/api/endpoints", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for valid add, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	// Verify file content has new endpoint with https:// prefix
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "New API") || !strings.Contains(content, "https://api.example.com/status") {
		t.Errorf("Updated config does not contain new endpoint:\n%s", content)
	}

	// 2. Reject duplicate name
	reqDup := httptest.NewRequest(http.MethodPost, "/api/endpoints", bytes.NewBufferString(payload))
	reqDup.Header.Set("Content-Type", "application/json")
	rrDup := httptest.NewRecorder()
	r.ServeHTTP(rrDup, reqDup)

	if rrDup.Code != http.StatusConflict {
		t.Errorf("Expected status 409 for duplicate, got %d", rrDup.Code)
	}

	// 3. Reject empty name or url
	invalidPayload := `{"name": "", "url": ""}`
	reqInv := httptest.NewRequest(http.MethodPost, "/api/endpoints", bytes.NewBufferString(invalidPayload))
	reqInv.Header.Set("Content-Type", "application/json")
	rrInv := httptest.NewRecorder()
	r.ServeHTTP(rrInv, reqInv)

	if rrInv.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty fields, got %d", rrInv.Code)
	}
}

func TestHandleDeleteEndpoint(t *testing.T) {
	mod, configPath := setupTestModule(t)

	r := gin.New()
	rg := r.Group("/api")
	mod.RegisterRoutes(rg)

	// 1. Delete existing endpoint
	req := httptest.NewRequest(http.MethodDelete, "/api/endpoints/Existing%20Service", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for delete, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	data, _ := os.ReadFile(configPath)
	if strings.Contains(string(data), "Existing Service") {
		t.Errorf("Config still contains deleted endpoint: %s", string(data))
	}

	// 2. Delete non-existent endpoint
	reqNon := httptest.NewRequest(http.MethodDelete, "/api/endpoints/NonExistent", nil)
	rrNon := httptest.NewRecorder()
	r.ServeHTTP(rrNon, reqNon)

	if rrNon.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for non-existent endpoint, got %d", rrNon.Code)
	}
}
