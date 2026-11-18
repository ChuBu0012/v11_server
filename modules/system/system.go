package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"v11-server/shared/hostmem"
)

type SystemStatus struct {
	Service       string                 `json:"service"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	UptimeHuman   string                 `json:"uptime_human"`
	GoVersion     string                 `json:"go_version"`
	OS            string                 `json:"os"`
	Arch          string                 `json:"arch"`
	NumCPU        int                    `json:"num_cpu"`
	Memory        map[string]interface{} `json:"memory"`
	HostMemory    *hostmem.Info          `json:"host_memory,omitempty"`
	Disk          map[string]interface{} `json:"disk"`
	Timestamp     time.Time              `json:"timestamp"`
}

type Module struct {
	startTime time.Time
}

func New(startTime time.Time) *Module {
	return &Module{startTime: startTime}
}

func (m *Module) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", m.HandleHealth)

	api := r.Group("/api")
	{
		api.GET("/system", m.HandleSystem)
		api.GET("/device/battery", m.HandleBattery)
	}
}

func (m *Module) HandleHealth(c *gin.Context) {
	uptime := time.Since(m.startTime)
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"service":        "v11-pocket-server",
		"uptime_seconds": int64(uptime.Seconds()),
		"uptime_human":   uptime.Round(time.Second).String(),
		"timestamp":      time.Now().UTC(),
	})
}

func (m *Module) HandleSystem(c *gin.Context) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := time.Since(m.startTime)
	diskInfo := getDiskInfo()

	var hostMem *hostmem.Info
	if hInfo, err := hostmem.Read(); err == nil && hInfo.TotalMB > 0 {
		hostMem = &hInfo
	}

	status := SystemStatus{
		Service:       "v11-pocket-server",
		UptimeSeconds: int64(uptime.Seconds()),
		UptimeHuman:   uptime.Round(time.Second).String(),
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		Memory: map[string]interface{}{
			"alloc_mb":       float64(mem.Alloc) / 1024 / 1024,
			"total_alloc_mb": float64(mem.TotalAlloc) / 1024 / 1024,
			"sys_mb":         float64(mem.Sys) / 1024 / 1024,
			"num_gc":         mem.NumGC,
			"heap_alloc_mb":  float64(mem.HeapAlloc) / 1024 / 1024,
			"heap_sys_mb":    float64(mem.HeapSys) / 1024 / 1024,
		},
		HostMemory: hostMem,
		Disk:       diskInfo,
		Timestamp:  time.Now().UTC(),
	}

	c.JSON(http.StatusOK, status)
}

func (m *Module) HandleBattery(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	cmdPath := "termux-battery-status"
	termuxBin := "/data/data/com.termux/files/usr/bin/termux-battery-status"
	if _, err := os.Stat(termuxBin); err == nil {
		cmdPath = termuxBin
	}

	cmd := exec.CommandContext(ctx, cmdPath)
	cmd.Env = append(os.Environ(), "PATH=/data/data/com.termux/files/usr/bin:/system/bin:"+os.Getenv("PATH"))
	out, err := cmd.Output()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"error":     fmt.Sprintf("termux-battery-status unavailable or failed: %v", err),
			"timestamp": time.Now().UTC(),
		})
		return
	}

	var batteryData map[string]interface{}
	if err := json.Unmarshal(out, &batteryData); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"available":  true,
			"raw_output": string(out),
			"timestamp":  time.Now().UTC(),
		})
		return
	}

	batteryData["available"] = true
	batteryData["queried_at"] = time.Now().UTC()
	c.JSON(http.StatusOK, batteryData)
}

func getDiskInfo() map[string]interface{} {
	targetPath := "/data"
	if _, err := os.Stat(targetPath); err != nil {
		targetPath = "."
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(targetPath, &stat); err != nil {
		return map[string]interface{}{
			"path":  targetPath,
			"error": err.Error(),
		}
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	return map[string]interface{}{
		"path":     targetPath,
		"total_gb": fmt.Sprintf("%.2f GB", float64(totalBytes)/(1024*1024*1024)),
		"free_gb":  fmt.Sprintf("%.2f GB", float64(freeBytes)/(1024*1024*1024)),
		"used_gb":  fmt.Sprintf("%.2f GB", float64(usedBytes)/(1024*1024*1024)),
	}
}
