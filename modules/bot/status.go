package bot

import (
	"context"
	"encoding/json"
	"fmt"
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
	startTime           time.Time
	httpClient          *http.Client
	execRunner          ExecOutputRunner
	digestSummaryFunc   func(ctx context.Context) (string, error)
	locationSummaryFunc func(ctx context.Context) (string, error)
}

// StatusProviderOption configures a DefaultStatusProvider.
type StatusProviderOption func(*DefaultStatusProvider)

// WithStatusDigestSummaryFunc sets custom callback for digest summary on panel.
func WithStatusDigestSummaryFunc(f func(ctx context.Context) (string, error)) StatusProviderOption {
	return func(p *DefaultStatusProvider) {
		p.digestSummaryFunc = f
	}
}

// WithStatusLocationSummaryFunc sets custom callback for location summary on panel.
func WithStatusLocationSummaryFunc(f func(ctx context.Context) (string, error)) StatusProviderOption {
	return func(p *DefaultStatusProvider) {
		p.locationSummaryFunc = f
	}
}

// SetDigestSummaryFunc assigns digest summary provider dynamically.
func (p *DefaultStatusProvider) SetDigestSummaryFunc(f func(ctx context.Context) (string, error)) {
	p.digestSummaryFunc = f
}

// SetLocationSummaryFunc assigns location summary provider dynamically.
func (p *DefaultStatusProvider) SetLocationSummaryFunc(f func(ctx context.Context) (string, error)) {
	p.locationSummaryFunc = f
}

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

	// 5. Active endpoint URL (Tailscale or Tunnel)
	endpointURL := p.getEndpointURL(ctx)

	// Memory line (with Host RAM if available)
	memLine := fmt.Sprintf("🧠 <b>Memory:</b> %.1f MB (Sys: %.1f MB)", allocMB, sysMB)
	if hInfo, err := hostmem.Read(); err == nil && hInfo.TotalMB > 0 {
		memLine = fmt.Sprintf("🧠 <b>Host RAM:</b> %.2f GB / %.2f GB (%.0f%%)\n⚙️ <b>Go Memory:</b> %.1f MB (Sys: %.1f MB)",
			hInfo.UsedMB/1024, hInfo.TotalMB/1024, hInfo.UsedPercent, allocMB, sysMB)
	}

	// Format clean HTML status card
	cardHTML := fmt.Sprintf(
		"📊 <b>Pocket Sentinel — System Status</b>\n\n"+
			"• 🔋 <b>Battery:</b> %s\n"+
			"%s\n"+
			"• 💾 <b>Disk (%s):</b> %.1f GB free / %.1f GB\n"+
			"• ⏱️ <b>Uptime:</b> %s\n"+
			"• 🌐 <b>Endpoint:</b> <code>%s</code>\n\n"+
			"<i>อัปเดต %s UTC</i>",
		batteryStr,
		memLine,
		targetPath, diskFreeGB, diskTotalGB,
		uptimeStr,
		endpointURL,
		time.Now().UTC().Format("15:04"),
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

func (p *DefaultStatusProvider) getEndpointURL(_ context.Context) string {
	baseURL := strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	if baseURL != "" {
		return fmt.Sprintf("%s (Tailscale)", baseURL)
	}
	return "http://100.92.194.87:8080 (Tailscale)"
}

// GetPanelCard returns formatted HTML for a specific tab of the interactive /panel.
func (p *DefaultStatusProvider) GetPanelCard(ctx context.Context, tab string) (string, error) {
	switch tab {
	case "services":
		return p.GetServicesPanel(ctx), nil
	case "digest":
		return p.GetDigestPanel(ctx), nil
	case "location":
		return p.GetLocationPanel(ctx), nil
	default:
		return p.GetSystemPanel(ctx), nil
	}
}

// GetSystemPanel formats the system vitals tab.
func (p *DefaultStatusProvider) GetSystemPanel(ctx context.Context) string {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
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

	batteryStr := p.getBatteryString(ctx)
	uptimeStr := p.GetUptimeString()
	endpointURL := p.getEndpointURL(ctx)

	memLine := fmt.Sprintf("⚙️ <b>Go RAM:</b> %.1f MB (Sys: %.1f MB)", allocMB, sysMB)
	if hInfo, err := hostmem.Read(); err == nil && hInfo.TotalMB > 0 {
		memLine = fmt.Sprintf("🧠 <b>Host RAM:</b> %.2f / %.2f GB (%.0f%%)\n⚙️ <b>Go RAM:</b> %.1f MB",
			hInfo.UsedMB/1024, hInfo.TotalMB/1024, hInfo.UsedPercent, allocMB)
	}

	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04")

	return fmt.Sprintf(
		"🎛️ <b>Pocket Sentinel · ควบคุมระบบ</b>\n\n"+
			"• 🔋 <b>แบตเตอรี่:</b> %s\n"+
			"%s\n"+
			"• 💾 <b>พื้นที่จัดเก็บ:</b> %.1f GB ว่าง / %.1f GB\n"+
			"• ⏱️ <b>เปิดต่อเนื่อง:</b> %s\n"+
			"• 🌐 <b>ที่อยู่เครือข่าย:</b> <code>%s</code>\n\n"+
			"<i>อัปเดต %s · แท็บ 📊 ระบบ</i>",
		batteryStr,
		memLine,
		diskFreeGB, diskTotalGB,
		uptimeStr,
		endpointURL,
		nowStr,
	)
}

// GetServicesPanel formats the services and health check tab.
func (p *DefaultStatusProvider) GetServicesPanel(ctx context.Context) string {
	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04")

	// Check local go-server health
	sentinelStatus := "🟢 ปกติ (พอร์ต 8080)"
	pCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	sReq, err := http.NewRequestWithContext(pCtx, http.MethodGet, "http://localhost:8080/health", nil)
	if err == nil {
		start := time.Now()
		if resp, err := p.httpClient.Do(sReq); err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			sentinelStatus = fmt.Sprintf("🟢 ปกติ · %d ms", time.Since(start).Milliseconds())
		} else {
			sentinelStatus = "🔴 ขัดข้อง"
		}
	}

	// Check local gatus health
	gatusStatus := "🟢 ปกติ (พอร์ต 8081)"
	gReq, err := http.NewRequestWithContext(pCtx, http.MethodGet, "http://localhost:8081/", nil)
	if err == nil {
		if resp, err := p.httpClient.Do(gReq); err == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound) {
			_ = resp.Body.Close()
		} else {
			gatusStatus = "⚪ ไม่ได้เริ่มทำงาน"
		}
	}

	return fmt.Sprintf(
		"🌐 <b>Pocket Sentinel · สถานะบริการ</b>\n\n"+
			"• 🚀 <b>Sentinel API:</b> %s\n"+
			"• 📊 <b>Gatus Monitor:</b> %s\n"+
			"• 🔒 <b>Tailscale:</b> 🟢 ออนไลน์ (<code>100.92.194.87</code>)\n\n"+
			"<i>อัปเดต %s · แตะ '➕ จัดการ Endpoints' เพื่อเพิ่มบริการ</i>",
		sentinelStatus,
		gatusStatus,
		nowStr,
	)
}

// GetDigestPanel formats the digest and markets tab.
func (p *DefaultStatusProvider) GetDigestPanel(ctx context.Context) string {
	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04")

	content := "ยังไม่มีรอบสรุปประจำวันล่าสุด\nแตะปุ่มด้านล่างเพื่อรันสรุปข่าวทันที"
	if p.digestSummaryFunc != nil {
		if summary, err := p.digestSummaryFunc(ctx); err == nil && summary != "" {
			content = summary
		}
	}

	return fmt.Sprintf(
		"📰 <b>Pocket Sentinel · ข้อมูลสรุปและตลาด</b>\n\n"+
			"<blockquote>%s</blockquote>\n\n"+
			"<i>อัปเดต %s · แท็บ 📰 ข่าว/ทอง</i>",
		content,
		nowStr,
	)
}

// GetLocationPanel formats the location tab.
func (p *DefaultStatusProvider) GetLocationPanel(ctx context.Context) string {
	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04")

	content := "ยังไม่มีข้อมูลพิกัดล่าสุด\nระบบจะบันทึกเมื่ออุปกรณ์ส่งสัญญาณ"
	if p.locationSummaryFunc != nil {
		if locHTML, err := p.locationSummaryFunc(ctx); err == nil && locHTML != "" {
			content = locHTML
		}
	}

	return fmt.Sprintf(
		"🗺️ <b>Pocket Sentinel · ตำแหน่งและความปลอดภัย</b>\n\n"+
			"%s\n\n"+
			"<i>อัปเดต %s · แท็บ 🗺️ พิกัด</i>",
		content,
		nowStr,
	)
}
