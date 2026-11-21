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

	// Format HTML status card
	cardHTML := fmt.Sprintf(
		"📊 <b>Pocket Sentinel — System Status</b>\n\n"+
			"🔋 <b>Battery:</b> %s\n"+
			"%s\n"+
			"💾 <b>Disk (%s):</b> %.1f GB free / %.1f GB\n"+
			"⏱️ <b>Uptime:</b> %s\n"+
			"🌐 <b>Endpoint:</b> %s\n\n"+
			"<i>Updated: %s UTC</i>",
		batteryStr,
		memLine,
		targetPath, diskFreeGB, diskTotalGB,
		uptimeStr,
		endpointURL,
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

func (p *DefaultStatusProvider) getEndpointURL(ctx context.Context) string {
	ngrokURL := p.getNgrokURL(ctx)
	if ngrokURL != "" && !strings.HasPrefix(ngrokURL, "inactive") {
		return ngrokURL
	}
	baseURL := strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	if baseURL != "" {
		return fmt.Sprintf("%s (Tailscale)", baseURL)
	}
	return "http://100.92.194.87:8080 (Tailscale)"
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

	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04:05")

	return fmt.Sprintf(
		"🎛️ <b>Pocket Sentinel · ควบคุมระบบ</b>\n"+
			"─────────────────────\n"+
			"🔋 <b>แบตเตอรี่:</b> %s\n"+
			"%s\n"+
			"💾 <b>พื้นที่จัดเก็บ:</b> %.1f GB ว่าง / %.1f GB\n"+
			"⏱️ <b>เปิดต่อเนื่อง:</b> %s\n"+
			"🌐 <b>IP เครือข่าย:</b> <code>%s</code>\n"+
			"─────────────────────\n"+
			"<i>อัปเดต: %s ICT · [📊 ระบบ]</i>",
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
	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04:05")

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
		"🎛️ <b>Pocket Sentinel · สถานะบริการ</b>\n"+
			"─────────────────────\n"+
			"🚀 <b>Pocket Sentinel API:</b> %s\n"+
			"📊 <b>Gatus Health Monitor:</b> %s\n"+
			"🔒 <b>Tailscale Ingress:</b> 🟢 พร้อมใช้งาน (100.92.194.87)\n"+
			"─────────────────────\n"+
			"💡 <i>แตะ '➕ จัดการ Endpoints' เพื่อเพิ่มการตรวจสอบ</i>\n"+
			"─────────────────────\n"+
			"<i>อัปเดต: %s ICT · [🌐 บริการ]</i>",
		sentinelStatus,
		gatusStatus,
		nowStr,
	)
}

// GetDigestPanel formats the digest and markets tab.
func (p *DefaultStatusProvider) GetDigestPanel(ctx context.Context) string {
	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04:05")

	content := "ยังไม่มีรอบสรุปประจำวันล่าสุดในฐานข้อมูล\nแตะปุ่มด้านล่างเพื่อรันสรุปหรือดูข้อมูลเต็ม"
	if p.digestSummaryFunc != nil {
		if summary, err := p.digestSummaryFunc(ctx); err == nil && summary != "" {
			content = summary
		}
	}

	return fmt.Sprintf(
		"🎛️ <b>Pocket Sentinel · ข้อมูลสรุปและตลาด</b>\n"+
			"─────────────────────\n"+
			"%s\n"+
			"─────────────────────\n"+
			"<i>อัปเดต: %s ICT · [📰 ข่าว/ทอง]</i>",
		content,
		nowStr,
	)
}

// GetLocationPanel formats the location tab.
func (p *DefaultStatusProvider) GetLocationPanel(ctx context.Context) string {
	nowStr := time.Now().In(time.FixedZone("ICT", 7*3600)).Format("15:04:05")

	content := "ยังไม่มีข้อมูลพิกัดล่าสุด\nระบบ Location Tracker จะบันทึกเมื่ออุปกรณ์ส่งสัญญาณ"
	if p.locationSummaryFunc != nil {
		if locHTML, err := p.locationSummaryFunc(ctx); err == nil && locHTML != "" {
			content = locHTML
		}
	}

	return fmt.Sprintf(
		"🎛️ <b>Pocket Sentinel · ตำแหน่งและความปลอดภัย</b>\n"+
			"─────────────────────\n"+
			"%s\n"+
			"─────────────────────\n"+
			"<i>อัปเดต: %s ICT · [🗺️ พิกัด]</i>",
		content,
		nowStr,
	)
}
