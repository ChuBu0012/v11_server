package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"v11-server/shared/hostmem"
)

// Router handles incoming Telegram messages and routes commands.
type Router struct {
	client     *Client
	authorizer *Authorizer
	torch      *TorchController
	executor   *SafeExecutor
	keyboards  *KeyboardsModule
	statusProv StatusCardProvider
	rebootFunc func()
	digestFunc func(ctx context.Context) (string, error)
}

// RouterOption configures a Router.
type RouterOption func(*Router)

// WithDigestFunc sets the callback for on-demand AI news digest.
func WithDigestFunc(f func(ctx context.Context) (string, error)) RouterOption {
	return func(r *Router) {
		r.digestFunc = f
	}
}

// WithRebootFunc overrides the reboot callback (useful for testing).
func WithRebootFunc(f func()) RouterOption {
	return func(r *Router) {
		r.rebootFunc = f
	}
}

// WithStatusProvider overrides the status provider.
func WithStatusProvider(p StatusCardProvider) RouterOption {
	return func(r *Router) {
		r.statusProv = p
	}
}

// NewRouter creates a new command router.
func NewRouter(
	client *Client,
	authorizer *Authorizer,
	torch *TorchController,
	executor *SafeExecutor,
	statusProv StatusCardProvider,
	opts ...RouterOption,
) *Router {
	keyboards := NewKeyboardsModule(client, statusProv, torch)
	r := &Router{
		client:     client,
		authorizer: authorizer,
		torch:      torch,
		executor:   executor,
		keyboards:  keyboards,
		statusProv: statusProv,
		rebootFunc: func() {
			log.Printf("[bot] /reboot triggered: sending SIGTERM for graceful PM2 resurrection...")
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = p.Signal(syscall.SIGTERM)
			}
		},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// GetKeyboardsModule returns the keyboards module.
func (r *Router) GetKeyboardsModule() *KeyboardsModule {
	return r.keyboards
}

// HandleUpdate satisfies the Handler interface for Poller.
func (r *Router) HandleUpdate(ctx context.Context, u Update) error {
	// 1. Enforce strict single-user whitelist authentication
	if r.authorizer != nil && !r.authorizer.IsAuthorized(u) {
		return nil // Drop unauthorized updates silently
	}

	// 2. Handle CallbackQuery (inline button clicks)
	if u.CallbackQuery != nil {
		return r.keyboards.HandleCallbackQuery(ctx, u.CallbackQuery)
	}

	// 3. Handle Message commands
	if u.Message != nil && strings.TrimSpace(u.Message.Text) != "" {
		return r.handleMessage(ctx, u.Message)
	}

	return nil
}

func (r *Router) handleMessage(ctx context.Context, msg *Message) error {
	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/") {
		return nil // Ignore non-command chat messages
	}

	// Parse command and arguments
	parts := strings.Fields(text)
	cmdPart := parts[0]
	args := parts[1:]

	// Strip @bot_username suffix if present (e.g. /status@my_sentinel_bot)
	if atIdx := strings.Index(cmdPart, "@"); atIdx != -1 {
		cmdPart = cmdPart[:atIdx]
	}
	cmd := strings.ToLower(cmdPart)

	chatID := msg.Chat.ID
	keyboard := MakeStatusKeyboard()

	switch cmd {
	case "/start", "/help":
		helpHTML := "🛡️ <b>Pocket Sentinel 2.0</b> — Terminal &amp; Hardware Sentinel\n\n" +
			"<b>Available Commands:</b>\n" +
			"📊 /status — Live system vitals &amp; tunnel URL\n" +
			"📰 /digest — Generate AI News &amp; Insights Digest\n" +
			"🏓 /ping — Check server latency &amp; heartbeat\n" +
			"🔦 /torch &lt;on|off|status&gt; — Toggle phone LED flash (3m safety)\n" +
			"⚡ /exec &lt;cmd&gt; — Run diagnostic command (ps, df, free, uptime, pm2)\n" +
			"♻️ /reboot — Restart Go server process via PM2\n" +
			"ℹ️ /help — Show this message"
		_, err := r.client.SendMessage(ctx, chatID, helpHTML, keyboard)
		return err

	case "/digest":
		if r.digestFunc == nil {
			_, err := r.client.SendMessage(ctx, chatID, "ℹ️ AI Digest feature is not configured.", keyboard)
			return err
		}
		_, _ = r.client.SendMessage(ctx, chatID, "⏳ <i>กำลังดึงข่าวสดและให้ Gemini สรุปข่าว AI &amp; รอบวันสักครู่...</i>", nil)
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			res, err := r.digestFunc(bgCtx)
			if err != nil {
				_, _ = r.client.SendMessage(bgCtx, chatID, fmt.Sprintf("❌ <b>สรุปข่าวไม่สำเร็จ:</b> %s", html.EscapeString(err.Error())), nil)
				return
			}
			msg := fmt.Sprintf("📰 <b>Pocket Sentinel · สรุปข่าว AI &amp; รอบวัน</b>\n\n%s", res)
			_, _ = r.client.SendMessage(bgCtx, chatID, msg, nil)
		}()
		return nil

	case "/ping":
		uptimeStr := "OK"
		if r.statusProv != nil {
			uptimeStr = r.statusProv.GetUptimeString()
		}
		pingHTML := fmt.Sprintf("🏓 <b>Pong!</b> (Uptime: %s, Server: OK)", html.EscapeString(uptimeStr))
		_, err := r.client.SendMessage(ctx, chatID, pingHTML, keyboard)
		return err

	case "/status":
		statusHTML := "⚠️ Status provider unavailable"
		if r.statusProv != nil {
			var err error
			statusHTML, err = r.statusProv.GetStatusCard(ctx)
			if err != nil {
				statusHTML = fmt.Sprintf("⚠️ Failed to generate status: %v", err)
			}
		}
		_, err := r.client.SendMessage(ctx, chatID, statusHTML, keyboard)
		return err

	case "/torch":
		if r.torch == nil {
			_, err := r.client.SendMessage(ctx, chatID, "⚠️ Hardware torch not configured", keyboard)
			return err
		}

		subCmd := ""
		if len(args) > 0 {
			subCmd = strings.ToLower(args[0])
		}

		var res string
		var err error
		switch subCmd {
		case "on":
			res, err = r.torch.On(ctx)
		case "off":
			res, err = r.torch.Off(ctx)
		case "status":
			res = r.torch.Status()
		default:
			// Toggle if no subcommand or unknown
			res, err = r.torch.Toggle(ctx)
		}

		if err != nil {
			res = fmt.Sprintf("❌ Torch error: %v", err)
		}
		_, sendErr := r.client.SendMessage(ctx, chatID, res, keyboard)
		return sendErr

	case "/exec":
		if r.executor == nil {
			_, err := r.client.SendMessage(ctx, chatID, "⚠️ Command executor not configured", keyboard)
			return err
		}
		cmdInput := strings.TrimSpace(strings.Join(args, " "))
		res := r.executor.Execute(ctx, cmdInput)
		_, err := r.client.SendMessage(ctx, chatID, res, keyboard)
		return err

	case "/reboot":
		rebootMsg := "♻️ <b>Restarting Pocket Sentinel via PM2...</b>\n<i>Process will cleanly exit and restart within 3 seconds.</i>"
		_, err := r.client.SendMessage(ctx, chatID, rebootMsg, nil)
		if r.rebootFunc != nil {
			go func() {
				time.Sleep(500 * time.Millisecond)
				r.rebootFunc()
			}()
		}
		return err

	default:
		unknownHTML := "❓ <b>Unknown command.</b>\nType /help to see available commands."
		_, err := r.client.SendMessage(ctx, chatID, unknownHTML, keyboard)
		return err
	}
}

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
