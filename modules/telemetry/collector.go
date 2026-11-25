package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"v11-server/modules/alerts"
	"v11-server/shared/hostmem"
)

// SystemMetricsProvider supplies system runtime and disk statistics.
type SystemMetricsProvider interface {
	GetMetrics() (uptimeSeconds int64, allocMB, sysMB, diskTotalGB, diskFreeGB float64, err error)
}

// BatteryProvider supplies device battery metrics.
type BatteryProvider interface {
	GetBattery(ctx context.Context) (level *float64, status *string, err error)
}

// HostMemoryProvider supplies host device RAM statistics.
type HostMemoryProvider interface {
	GetHostMemory() (totalMB, usedMB, freeMB *float64, err error)
}

// AlertProcessor receives collected samples for threshold evaluation.
type AlertProcessor interface {
	ProcessSample(sample alerts.Sample) error
}

// Collector coordinates periodic health sampling and alert processing.
type Collector struct {
	repo            *Repository
	sysProvider     SystemMetricsProvider
	batProvider     BatteryProvider
	hostMemProvider HostMemoryProvider
	alertProc       AlertProcessor
	retentionAge    time.Duration
	retentionRows   int
	mu              sync.RWMutex
	lastCollected   time.Time
	lastError       string
}

// CollectorOption configures the Collector.
type CollectorOption func(*Collector)

func WithSystemProvider(p SystemMetricsProvider) CollectorOption {
	return func(c *Collector) { c.sysProvider = p }
}

func WithBatteryProvider(p BatteryProvider) CollectorOption {
	return func(c *Collector) { c.batProvider = p }
}

func WithHostMemoryProvider(p HostMemoryProvider) CollectorOption {
	return func(c *Collector) { c.hostMemProvider = p }
}

func WithAlertProcessor(p AlertProcessor) CollectorOption {
	return func(c *Collector) { c.alertProc = p }
}

func WithRetention(maxAge time.Duration, maxRows int) CollectorOption {
	return func(c *Collector) {
		c.retentionAge = maxAge
		c.retentionRows = maxRows
	}
}

// NewCollector creates a collector with defaults for vivo V11 / Termux.
func NewCollector(repo *Repository, startTime time.Time, opts ...CollectorOption) *Collector {
	c := &Collector{
		repo:            repo,
		sysProvider:     NewDefaultSystemProvider(startTime),
		batProvider:     &DefaultBatteryProvider{},
		hostMemProvider: &DefaultHostMemoryProvider{},
		retentionAge:    72 * time.Hour,
		retentionRows:   4320,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CollectOnce executes a single sampling, persistence, and alert cycle.
func (c *Collector) CollectOnce(ctx context.Context) (*HealthSample, error) {
	now := time.Now().UTC()

	uptime, allocMB, sysMB, diskTotal, diskFree, err := c.sysProvider.GetMetrics()
	if err != nil {
		c.mu.Lock()
		c.lastError = fmt.Sprintf("system metrics failed: %v", err)
		c.mu.Unlock()
		return nil, err
	}

	batLevel, batStatus, batErr := c.batProvider.GetBattery(ctx)
	if batErr != nil {
		// Degraded mode: battery unavailable does not fail the sample
		batLevel = nil
		batStatus = nil
	}

	var hostTotalMB, hostUsedMB, hostFreeMB *float64
	if c.hostMemProvider != nil {
		hTotal, hUsed, hFree, hErr := c.hostMemProvider.GetHostMemory()
		if hErr == nil {
			hostTotalMB = hTotal
			hostUsedMB = hUsed
			hostFreeMB = hFree
		}
	}

	sample := HealthSample{
		Timestamp:         now,
		UptimeSeconds:     uptime,
		MemoryAllocMB:     allocMB,
		MemorySysMB:       sysMB,
		DiskTotalGB:       diskTotal,
		DiskFreeGB:        diskFree,
		BatteryLevel:      batLevel,
		BatteryStatus:     batStatus,
		HostMemoryTotalMB: hostTotalMB,
		HostMemoryUsedMB:  hostUsedMB,
		HostMemoryFreeMB:  hostFreeMB,
	}

	id, err := c.repo.InsertSample(ctx, sample)
	if err != nil {
		c.mu.Lock()
		c.lastError = fmt.Sprintf("insert sample failed: %v", err)
		c.mu.Unlock()
		return nil, err
	}
	sample.ID = id
	c.mu.Lock()
	c.lastCollected = now
	c.lastError = ""
	c.mu.Unlock()

	// Prune old samples
	if c.retentionAge > 0 || c.retentionRows > 0 {
		if _, err := c.repo.PruneSamples(ctx, c.retentionAge, c.retentionRows); err != nil {
			log.Printf("[telemetry] retention pruning error: %v", err)
		}
	}

	// Feed to alert manager if configured
	if c.alertProc != nil {
		alertSample := alerts.Sample{
			Timestamp:     sample.Timestamp,
			UptimeSeconds: sample.UptimeSeconds,
			MemAllocMB:    sample.MemoryAllocMB,
			MemSysMB:      sample.MemorySysMB,
			DiskTotalGB:   sample.DiskTotalGB,
			DiskFreeGB:    sample.DiskFreeGB,
			BatteryLevel:  sample.BatteryLevel,
			BatteryStatus: sample.BatteryStatus,
			ServiceOK:     true, // Collector is healthy
		}
		if err := c.alertProc.ProcessSample(alertSample); err != nil {
			log.Printf("[telemetry] alert processing error: %v", err)
		}
	}

	return &sample, nil
}

// LastCollectionInfo returns telemetry metadata for status queries.
func (c *Collector) LastCollectionInfo() (time.Time, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCollected, c.lastError
}

// --- Default Providers ---

type DefaultSystemProvider struct {
	startTime time.Time
}

func NewDefaultSystemProvider(startTime time.Time) *DefaultSystemProvider {
	return &DefaultSystemProvider{startTime: startTime}
}

func (p *DefaultSystemProvider) GetMetrics() (int64, float64, float64, float64, float64, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := int64(time.Since(p.startTime).Seconds())
	allocMB := float64(mem.Alloc) / 1024 / 1024
	sysMB := float64(mem.Sys) / 1024 / 1024

	targetPath := "/data"
	if _, err := os.Stat(targetPath); err != nil {
		targetPath = "."
	}

	var stat syscall.Statfs_t
	var diskTotalGB, diskFreeGB float64
	if err := syscall.Statfs(targetPath, &stat); err == nil {
		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bavail * uint64(stat.Bsize)
		diskTotalGB = float64(totalBytes) / (1024 * 1024 * 1024)
		diskFreeGB = float64(freeBytes) / (1024 * 1024 * 1024)
	}

	return uptime, allocMB, sysMB, diskTotalGB, diskFreeGB, nil
}

type DefaultBatteryProvider struct{}

func (p *DefaultBatteryProvider) GetBattery(ctx context.Context) (*float64, *string, error) {
	bctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmdPath := "termux-battery-status"
	termuxBin := "/data/data/com.termux/files/usr/bin/termux-battery-status"
	if _, err := os.Stat(termuxBin); err == nil {
		cmdPath = termuxBin
	}

	cmd := exec.CommandContext(bctx, cmdPath)
	cmd.Env = append(os.Environ(), "PATH=/data/data/com.termux/files/usr/bin:/system/bin:"+os.Getenv("PATH"))
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}

	var data struct {
		Percentage float64 `json:"percentage"`
		Status     string  `json:"status"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, nil, err
	}

	return &data.Percentage, &data.Status, nil
}

type DefaultHostMemoryProvider struct{}

func (p *DefaultHostMemoryProvider) GetHostMemory() (*float64, *float64, *float64, error) {
	info, err := hostmem.Read()
	if err != nil {
		return nil, nil, nil, err
	}
	if info.TotalMB <= 0 {
		return nil, nil, nil, nil
	}
	t := info.TotalMB
	u := info.UsedMB
	f := info.AvailableMB
	return &t, &u, &f, nil
}
