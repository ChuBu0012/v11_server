package endpoints

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
)

const (
	DefaultConfigPath  = "/data/data/com.termux/files/home/gatus/config.yaml"
	DefaultTailscaleIP = "100.92.194.87"
	DefaultGatusPort   = "8081"
)

// EndpointItem represents an endpoint's current state and configuration.
type EndpointItem struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	URL         string `json:"url"`
	Interval    string `json:"interval"`
	Alive       bool   `json:"alive"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
	LastChecked string `json:"last_checked,omitempty"`
}

// AddEndpointRequest is the payload for adding an endpoint.
type AddEndpointRequest struct {
	Name     string `json:"name" binding:"required"`
	URL      string `json:"url" binding:"required"`
	Group    string `json:"group"`
	Interval string `json:"interval"`
}

// Module manages Gatus endpoints via config.yaml and PM2 reload.
type Module struct {
	mu         sync.Mutex
	configPath string
	gatusURL   string
	httpClient *http.Client
}

// New creates a new endpoints Module.
func New(configPath string) *Module {
	if configPath == "" {
		configPath = os.Getenv("GATUS_CONFIG_PATH")
		if configPath == "" {
			if _, err := os.Stat(DefaultConfigPath); err == nil {
				configPath = DefaultConfigPath
			} else {
				configPath = "gatus/config.yaml"
			}
		}
	}

	gatusURL := os.Getenv("GATUS_PUBLIC_URL")
	if gatusURL == "" {
		gatusURL = "http://100.92.194.87:8081/"
	}

	return &Module{
		configPath: configPath,
		gatusURL:   gatusURL,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// RegisterRoutes registers endpoints API routes onto a Gin RouterGroup.
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/endpoints", m.HandleGetEndpoints)
	rg.POST("/endpoints", m.HandleAddEndpoint)
	rg.DELETE("/endpoints/:name", m.HandleDeleteEndpoint)
	rg.POST("/endpoints/delete", m.HandleDeleteEndpoint)
}

// HandleGetEndpoints returns the configured endpoints combined with live Gatus statuses.
func (m *Module) HandleGetEndpoints(c *gin.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items, err := m.readEndpointsFromConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("[endpoints] Error reading config: %v", err)
	}

	// Try fetching live statuses from local Gatus
	statuses := m.fetchGatusStatuses()
	for i := range items {
		if stat, ok := statuses[items[i].Name]; ok {
			items[i].Alive = stat.Alive
			items[i].DurationMS = stat.DurationMS
			items[i].LastChecked = stat.LastChecked
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"gatus_url": m.gatusURL,
		"endpoints": items,
	})
}

// HandleAddEndpoint validates and appends a new endpoint into config.yaml and reloads Gatus.
func (m *Module) HandleAddEndpoint(c *gin.Context) {
	var req AddEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "กรุณาระบุชื่อและ URL ของ Endpoint ให้ครบถ้วน"})
		return
	}

	name := strings.TrimSpace(req.Name)
	rawURL := strings.TrimSpace(req.URL)
	group := strings.TrimSpace(req.Group)
	interval := strings.TrimSpace(req.Interval)

	if len(name) == 0 || len(name) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "ชื่อ Endpoint ต้องมีความยาวระหว่าง 1 ถึง 80 ตัวอักษร"})
		return
	}

	// Format and validate URL
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "tcp://") {
		rawURL = "https://" + rawURL
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "รูปแบบ URL ไม่ถูกต้อง"})
		return
	}

	if group == "" {
		group = "Websites"
	}
	if interval == "" {
		interval = "60s"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := m.readRawConfig()
	if err != nil {
		root = make(map[string]interface{})
	}

	var endpointsList []interface{}
	if rawList, ok := root["endpoints"].([]interface{}); ok {
		endpointsList = rawList
	}

	// Check duplicates by name
	for _, ep := range endpointsList {
		if epMap, ok := ep.(map[string]interface{}); ok {
			if existingName, _ := epMap["name"].(string); strings.EqualFold(existingName, name) {
				c.JSON(http.StatusConflict, gin.H{"status": "error", "message": fmt.Sprintf("มี Endpoint ชื่อ '%s' อยู่แล้วในระบบ", name)})
				return
			}
		}
	}

	// Build condition based on protocol
	conditions := []string{"[STATUS] == 200"}
	if strings.HasPrefix(rawURL, "tcp://") {
		conditions = []string{"[CONNECTED] == true"}
	}

	newEntry := map[string]interface{}{
		"name":       name,
		"group":      group,
		"url":        rawURL,
		"interval":   interval,
		"conditions": conditions,
	}

	endpointsList = append(endpointsList, newEntry)
	root["endpoints"] = endpointsList

	if err := m.writeRawConfig(root); err != nil {
		log.Printf("[endpoints] Failed to write config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "ไม่สามารถบันทึกไฟล์ config ได้"})
		return
	}

	// Trigger PM2 reload asynchronously
	go m.restartGatus()

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("เพิ่ม '%s' เรียบร้อยแล้ว กำลังเริ่มการตรวจสอบ", name),
		"endpoint": EndpointItem{
			Name:     name,
			Group:    group,
			URL:      rawURL,
			Interval: interval,
		},
	})
}

// HandleDeleteEndpoint removes an endpoint by name from config.yaml and reloads Gatus.
func (m *Module) HandleDeleteEndpoint(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			name = strings.TrimSpace(req.Name)
		}
	}

	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "กรุณาระบุชื่อ Endpoint ที่ต้องการลบ"})
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := m.readRawConfig()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "ไม่พบไฟล์คอนฟิก"})
		return
	}

	rawList, ok := root["endpoints"].([]interface{})
	if !ok || len(rawList) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "ไม่พบรายการ Endpoint ในคอนฟิก"})
		return
	}

	found := false
	filtered := make([]interface{}, 0, len(rawList))
	for _, ep := range rawList {
		if epMap, ok := ep.(map[string]interface{}); ok {
			if existingName, _ := epMap["name"].(string); strings.EqualFold(existingName, name) {
				found = true
				continue
			}
		}
		filtered = append(filtered, ep)
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": fmt.Sprintf("ไม่พบ Endpoint ชื่อ '%s'", name)})
		return
	}

	root["endpoints"] = filtered
	if err := m.writeRawConfig(root); err != nil {
		log.Printf("[endpoints] Failed to write config after delete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "ไม่สามารถบันทึกไฟล์คอนฟิกหลังลบได้"})
		return
	}

	// Trigger PM2 reload asynchronously
	go m.restartGatus()

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("ลบ '%s' เรียบร้อยแล้ว", name),
	})
}

func (m *Module) readRawConfig() (map[string]interface{}, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, err
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func (m *Module) writeRawConfig(root map[string]interface{}) error {
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", m.configPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, m.configPath)
}

func (m *Module) readEndpointsFromConfig() ([]EndpointItem, error) {
	root, err := m.readRawConfig()
	if err != nil {
		return nil, err
	}

	rawList, ok := root["endpoints"].([]interface{})
	if !ok {
		return nil, nil
	}

	items := make([]EndpointItem, 0, len(rawList))
	for _, ep := range rawList {
		if epMap, ok := ep.(map[string]interface{}); ok {
			name, _ := epMap["name"].(string)
			group, _ := epMap["group"].(string)
			urlStr, _ := epMap["url"].(string)
			interval, _ := epMap["interval"].(string)
			if name != "" && urlStr != "" {
				items = append(items, EndpointItem{
					Name:     name,
					Group:    group,
					URL:      urlStr,
					Interval: interval,
				})
			}
		}
	}
	return items, nil
}

type liveStatus struct {
	Alive       bool
	DurationMS  int64
	LastChecked string
}

func (m *Module) fetchGatusStatuses() map[string]liveStatus {
	result := make(map[string]liveStatus)
	// Local Gatus runs on localhost:8081
	resp, err := m.httpClient.Get("http://127.0.0.1:8081/api/v1/endpoints/statuses")
	if err != nil || resp.StatusCode != http.StatusOK {
		return result
	}
	defer resp.Body.Close()

	var rawList []struct {
		Name    string `json:"name"`
		Results []struct {
			Success   bool   `json:"success"`
			Duration  int64  `json:"duration"` // in nanoseconds
			Timestamp string `json:"timestamp"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
		return result
	}

	for _, item := range rawList {
		if len(item.Results) > 0 {
			latest := item.Results[len(item.Results)-1]
			durationMS := latest.Duration / 1_000_000
			result[item.Name] = liveStatus{
				Alive:       latest.Success,
				DurationMS:  durationMS,
				LastChecked: latest.Timestamp,
			}
		}
	}
	return result
}

func (m *Module) restartGatus() {
	var cmd *exec.Cmd
	termuxPM2 := "/data/data/com.termux/files/usr/bin/pm2"
	if _, err := os.Stat(termuxPM2); err == nil {
		cmd = exec.Command("node", termuxPM2, "restart", "gatus")
	} else if pm2Bin, err := exec.LookPath("pm2"); err == nil {
		cmd = exec.Command(pm2Bin, "restart", "gatus")
	} else {
		log.Printf("[endpoints] pm2 binary not found, skipping restart")
		return
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[endpoints] Warning: pm2 restart gatus failed: %v (output: %s)", err, string(out))
	} else {
		log.Printf("[endpoints] ✅ Gatus restarted successfully via PM2")
	}
}
