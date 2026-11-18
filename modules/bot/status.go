package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"v11-server/shared/hostmem"
)

// DefaultStatusProvider gathers real-time metrics and formats the status card HTML.
type DefaultStatusProvider struct {
	startTime  time.Time
	httpClient *http.Client
	execRunner ExecOutputRunner
}

// StatusProviderOption configures a DefaultStatusProvider.
type StatusProviderOption func(*DefaultStatusProvider)

// WithStatusHTTPClient sets custom HTTP client for status queries (e.g. ngrok inspection).
func WithStatusHTTPClient(client *http.Client) StatusProviderOption {
	return func(p *DefaultStatusProvider) {
		p.httpClient = client
	}
}

// WithStatusExecRunner sets custom command runner for hardware status (e.g. battery).
func WithStatusExecRunner(runner ExecOutputRunner) StatusProviderOption {
	return func(p *DefaultStatusProvider) {
		p.execRunner = runner
	}
}

// NewDefaultStatusProvider creates a status card provider.
func NewDefaultStatusProvider(startTime time.Time, opts ...StatusProviderOption) *DefaultStatusProvider {
	p := &DefaultStatusProvider{
		startTime: startTime,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		execRunner: DefaultExecRunner,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// GetUptimeString returns formatted uptime duration.
func (p *DefaultStatusProvider) GetUptimeString() string {
	d := time.Since(p.startTime).Round(time.Second)
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
	}
	return fmt.Sprintf("%dm %ds", mins, secs)
}

// GetStatusCard produces the formatted HTML status card.
func (p *DefaultStatusProvider) GetStatusCard(ctx context.Context) (string, error) {
	// 1. Memory stats
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	allocMB := float64(mem.Alloc) / 1024 / 1024
	sysMB := float64(mem.Sys) / 1024 / 1024

	// 2. Disk stats
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

	// 3. Battery stats
	batteryStr := p.getBatteryString(ctx)

	// 4. Uptime
	uptimeStr := p.GetUptimeString()

	// 5. Ngrok public URL
	ngrokURL := p.getNgrokURL(ctx)

	// Memory line (with Host RAM if available)
	memLine := fmt.Sprintf("🧠 <b>Memory:</b> %.1f MB (Sys: %.1f MB)", allocMB, sysMB)
	if hInfo, err := hostmem.Read(); err == nil && hInfo.TotalMB > 0 {
		memLine = fmt.Sprintf("🧠 <b>Host RAM:</b> %.2f GB / %.2f GB (%.0f%%)\n⚙️ <b>Go Memory:</b> %.1f MB (Sys: %.1f MB)",
			hInfo.UsedMB/1024, hInfo.TotalMB/1024, hInfo.UsedPercent, allocMB, sysMB)
	}

	// Format HTML status card
	cardHTML := fmt.Sprintf(
		"📊 <b>Pocket Sentinel — System Status</b>\n\n"+
			"🔋 <b>Battery:</b> %s\n"+
			"%s\n"+
			"💾 <b>Disk (%s):</b> %.1f GB free / %.1f GB\n"+
			"⏱️ <b>Uptime:</b> %s\n"+
			"🌐 <b>Tunnel:</b> %s\n\n"+
			"<i>Updated: %s UTC</i>",
		batteryStr,
		memLine,
		targetPath, diskFreeGB, diskTotalGB,
		uptimeStr,
		ngrokURL,
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)

	return cardHTML, nil
}

func (p *DefaultStatusProvider) getBatteryString(ctx context.Context) string {
	bctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmdPath := "termux-battery-status"
	termuxBin := "/data/data/com.termux/files/usr/bin/termux-battery-status"
	if _, err := os.Stat(termuxBin); err == nil {
		cmdPath = termuxBin
	}

	// Try reading battery via exec
	execCmd := p.execRunner
	if execCmd == nil {
		execCmd = DefaultExecRunner
	}
	out, err := execCmd(bctx, cmdPath)
	if err != nil {
		return "N/A (Termux:API unavailable)"
	}

	var data struct {
		Percentage  float64 `json:"percentage"`
		Status      string  `json:"status"`
		Temperature float64 `json:"temperature"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return "Available"
	}

	if data.Temperature > 0 {
		return fmt.Sprintf("%.0f%% (%s, %.1f°C)", data.Percentage, data.Status, data.Temperature)
	}
	return fmt.Sprintf("%.0f%% (%s)", data.Percentage, data.Status)
}

func (p *DefaultStatusProvider) getNgrokURL(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:4040/api/tunnels", nil)
	if err != nil {
		return "inactive"
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "inactive (tunnel not running)"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return "inactive"
	}

	var res struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
			Proto     string `json:"proto"`
		} `json:"tunnels"`
	}
	if err := json.Unmarshal(body, &res); err != nil || len(res.Tunnels) == 0 {
		return "inactive"
	}

	for _, t := range res.Tunnels {
		if strings.HasPrefix(t.PublicURL, "https://") {
			return t.PublicURL
		}
	}
	return res.Tunnels[0].PublicURL
}
