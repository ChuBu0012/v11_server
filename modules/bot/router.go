package bot

import (
	"context"
	"encoding/json"
	"errors"
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
	"unicode/utf8"

	"v11-server/shared/hostmem"
)

// TelegramSender abstracts sending Telegram messages for Router.
type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, text string, replyMarkup *InlineKeyboardMarkup) (*Message, error)
}

// Router handles incoming Telegram messages and routes commands.
type Router struct {
	client     *Client
	sender     TelegramSender
	authorizer *Authorizer
	torch      *TorchController
	executor   *SafeExecutor
	keyboards  *KeyboardsModule
	statusProv StatusCardProvider
	rebootFunc func()
	digestFunc func(ctx context.Context) (string, error)

	runner      *AgentRunner
	guard       *PromptGuard
	scrubber    *OutputScrubber
	ramProvider func() float64
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

// WithAgentRunner sets the agent runner for executing /task commands.
func WithAgentRunner(runner *AgentRunner) RouterOption {
	return func(r *Router) {
		r.runner = runner
	}
}

// WithPromptGuard overrides the prompt guard used for /task validation.
func WithPromptGuard(guard *PromptGuard) RouterOption {
	return func(r *Router) {
		r.guard = guard
	}
}

// WithOutputScrubber overrides the output scrubber used for formatting task results.
func WithOutputScrubber(scrubber *OutputScrubber) RouterOption {
	return func(r *Router) {
		r.scrubber = scrubber
	}
}

// WithTelegramSender overrides the telegram sender (useful for testing).
func WithTelegramSender(sender TelegramSender) RouterOption {
	return func(r *Router) {
		r.sender = sender
	}
}

// WithRAMFunc overrides the RAM calculation function for testing.
func WithRAMFunc(f func() float64) RouterOption {
	return func(r *Router) {
		r.ramProvider = f
	}
}

// WithRAMProvider is an alias for WithRAMFunc.
func WithRAMProvider(f func() float64) RouterOption {
	return WithRAMFunc(f)
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
		sender:     client,
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
	if r.sender == nil {
		r.sender = client
	}
	if r.guard == nil {
		r.guard = NewPromptGuard()
	}
	if r.scrubber == nil {
		r.scrubber = NewOutputScrubber()
	}
	if r.ramProvider == nil {
		r.ramProvider = func() float64 {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			return float64(mem.Alloc) / 1024 / 1024
		}
	}
	return r
}

func (r *Router) sendMessage(ctx context.Context, chatID int64, text string, replyMarkup *InlineKeyboardMarkup) (*Message, error) {
	if r.sender != nil {
		return r.sender.SendMessage(ctx, chatID, text, replyMarkup)
	}
	if r.client != nil {
		return r.client.SendMessage(ctx, chatID, text, replyMarkup)
	}
	return nil, nil
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
			"🤖 /task &lt;prompt&gt; — Dispatch autonomous task to Antigravity\n" +
			"⚡ /task_status — Check active agent task status\n" +
			"📰 /digest — Generate AI News &amp; Insights Digest\n" +
			"🏓 /ping — Check server latency &amp; heartbeat\n" +
			"🔦 /torch &lt;on|off|status&gt; — Toggle phone LED flash (3m safety)\n" +
			"⚡ /exec &lt;cmd&gt; — Run diagnostic command (ps, df, free, uptime, pm2)\n" +
			"♻️ /reboot — Restart Go server process via PM2\n" +
			"ℹ️ /help — Show this message"
		_, err := r.sendMessage(ctx, chatID, helpHTML, keyboard)
		return err

	case "/task":
		if r.runner == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ <b>Agent Runner ไม่ได้ถูกตั้งค่า:</b> Agent Runner feature is not configured.", keyboard)
			return err
		}

		prompt := ""
		if len(text) > len(parts[0]) {
			prompt = strings.TrimSpace(text[len(parts[0]):])
		}

		if prompt == "" {
			usageHTML := "💡 <b>วิธีใช้งาน /task:</b>\n<code>/task &lt;คำสั่งหรือคำถาม&gt;</code>\n\n" +
				"ตัวอย่าง:\n" +
				"• <code>/task ตรวจสอบ disk และ memory</code>\n" +
				"• <code>/task เช็คสถานะ pm2 go-server</code>\n" +
				"• <code>/task ตรวจสอบ log ล่าสุดของระบบ</code>"
			_, err := r.sendMessage(ctx, chatID, usageHTML, keyboard)
			return err
		}

		if !r.runner.CanAcquireLock() {
			st := r.runner.Status()
			elapsedStr := "สักครู่"
			if st.Elapsed > 0 {
				elapsedStr = st.Elapsed.Round(time.Second).String()
			}
			busyHTML := fmt.Sprintf("⏳ <b>Agent กำลังทำงานอื่นอยู่:</b> เริ่มเมื่อ %s ที่แล้ว\nไม่สามารถรับงานซ้อนได้ในขณะนี้ กรุณารอสักครู่แล้วลองใหม่อีกครั้ง\n(พิมพ์ /task_status เพื่อดูสถานะ)", elapsedStr)
			_, err := r.sendMessage(ctx, chatID, busyHTML, keyboard)
			return err
		}

		framedPrompt, err := r.guard.ValidateAndFrame(prompt)
		if err != nil {
			if errors.Is(err, ErrPromptTooLong) {
				lenHTML := fmt.Sprintf("⚠️ <b>ความยาวข้อความเกินกำหนด:</b> คำสั่งต้องไม่เกิน %d ตัวอักษร (ปัจจุบัน %d ตัวอักษร)",
					MaxPromptLength, utf8.RuneCountInString(prompt))
				_, sendErr := r.sendMessage(ctx, chatID, lenHTML, keyboard)
				return sendErr
			}
			if errors.Is(err, ErrPromptEmpty) {
				usageHTML := "💡 <b>วิธีใช้งาน /task:</b>\n<code>/task &lt;คำสั่งหรือคำถาม&gt;</code>\n\n" +
					"ตัวอย่าง:\n" +
					"• <code>/task ตรวจสอบ disk และ memory</code>"
				_, sendErr := r.sendMessage(ctx, chatID, usageHTML, keyboard)
				return sendErr
			}
			secHTML := "🛡️ <b>Security Warning:</b> คำสั่งถูกปฏิเสธเนื่องจากละเมิดนโยบายความปลอดภัย (Prompt Injection / Destructive Command)"
			_, sendErr := r.sendMessage(ctx, chatID, secHTML, keyboard)
			return sendErr
		}

		taskReq := TaskRequest{
			ChatID:    chatID,
			RawPrompt: framedPrompt,
			CreatedAt: time.Now(),
		}

		ackSent := make(chan struct{})
		dispatchErr := r.runner.DispatchAsync(context.Background(), taskReq, func(result TaskResult) {
			<-ackSent // Guarantee ACK has finished sending before transmitting result
			ramMB := 0.0
			if r.ramProvider != nil {
				ramMB = r.ramProvider()
			}
			formatted := r.scrubber.FormatResult(result, ramMB)
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, sendResErr := r.sendMessage(bgCtx, chatID, formatted, nil)
			if sendResErr != nil {
				log.Printf("[bot:router] failed to send task result to chat %d: %v", chatID, sendResErr)
			}
		})

		if errors.Is(dispatchErr, ErrAgentBusy) {
			close(ackSent) // Unblock on error
			busyHTML := "⏳ <b>Agent กำลังทำงานอื่นอยู่:</b> ไม่สามารถรับงานซ้อนได้ในขณะนี้ กรุณารอสักครู่แล้วลองใหม่อีกครั้ง\n(พิมพ์ /task_status เพื่อดูสถานะ)"
			_, sendErr := r.sendMessage(ctx, chatID, busyHTML, keyboard)
			return sendErr
		}
		if dispatchErr != nil {
			close(ackSent) // Unblock on error
			_, sendErr := r.sendMessage(ctx, chatID, fmt.Sprintf("❌ ไม่สามารถเริ่มงานได้: %s", r.scrubber.EscapeHTML(dispatchErr.Error())), keyboard)
			return sendErr
		}

		// Send immediate ACK (< 500ms)
		promptSnippet := prompt
		if utf8.RuneCountInString(promptSnippet) > 60 {
			runes := []rune(promptSnippet)
			promptSnippet = string(runes[:60]) + "..."
		}
		ackHTML := fmt.Sprintf("🤖 <b>รับงานแล้ว:</b> <i>\"%s\"</i>\n⏱ กำลังประมวลผลในเบื้องหลัง (Timeout: 3 นาที)\n💡 คุณสามารถไปทำอย่างอื่นได้เลย ระบบจะแจ้งเตือนเมื่อเสร็จสิ้น",
			r.scrubber.EscapeHTML(promptSnippet))
		_, ackErr := r.sendMessage(ctx, chatID, ackHTML, keyboard)
		close(ackSent) // Unblock background completion handler
		return ackErr

	case "/task_status":
		if r.runner == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ <b>Agent Runner ไม่ได้ถูกตั้งค่า:</b> Agent Runner feature is not configured.", keyboard)
			return err
		}

		st := r.runner.Status()
		if !st.IsRunning {
			idleHTML := "💤 <b>สถานะ Agent:</b> ว่าง (Idle)\n" +
				"พร้อมรับคำสั่งใหม่ สามารถสั่งงานได้ด้วย <code>/task &lt;คำสั่ง&gt;</code>"
			_, err := r.sendMessage(ctx, chatID, idleHTML, keyboard)
			return err
		}

		displayPrompt := st.Prompt
		if startIdx := strings.Index(displayPrompt, "<<<TASK>>>\n"); startIdx != -1 {
			displayPrompt = displayPrompt[startIdx+len("<<<TASK>>>\n"):]
			if endIdx := strings.Index(displayPrompt, "\n<<<END_TASK>>>"); endIdx != -1 {
				displayPrompt = displayPrompt[:endIdx]
			}
		}
		displayPrompt = strings.TrimSpace(displayPrompt)
		if utf8.RuneCountInString(displayPrompt) > 80 {
			runes := []rune(displayPrompt)
			displayPrompt = string(runes[:80]) + "..."
		}

		elapsedStr := st.Elapsed.Round(time.Second).String()
		startedAtStr := st.StartedAt.UTC().Format("15:04:05")
		runningHTML := fmt.Sprintf(
			"⚡ <b>สถานะ Agent: กำลังประมวลผล</b> (Running)\n\n"+
				"⏱ <b>ทำงานมาแล้ว:</b> %s (เริ่มเมื่อ %s UTC)\n"+
				"📋 <b>งานที่กำลังทำ:</b> <code>%s</code>\n\n"+
				"<i>ระบบจะแจ้งเตือนอัตโนมัติเมื่อทำงานเสร็จสิ้น</i>",
			elapsedStr, startedAtStr, html.EscapeString(displayPrompt),
		)
		_, err := r.sendMessage(ctx, chatID, runningHTML, keyboard)
		return err

	case "/digest":
		if r.digestFunc == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ AI Digest feature is not configured.", keyboard)
			return err
		}
		_, _ = r.sendMessage(ctx, chatID, "⏳ <i>กำลังดึงข่าวสดและให้ Gemini สรุปข่าว AI &amp; รอบวันสักครู่...</i>", nil)
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			res, err := r.digestFunc(bgCtx)
			if err != nil {
				_, _ = r.sendMessage(bgCtx, chatID, fmt.Sprintf("❌ <b>สรุปข่าวไม่สำเร็จ:</b> %s", html.EscapeString(err.Error())), nil)
				return
			}
			msg := fmt.Sprintf("📰 <b>Pocket Sentinel · สรุปข่าว AI &amp; รอบวัน</b>\n\n%s", res)
			_, _ = r.sendMessage(bgCtx, chatID, msg, nil)
		}()
		return nil

	case "/ping":
		uptimeStr := "OK"
		if r.statusProv != nil {
			uptimeStr = r.statusProv.GetUptimeString()
		}
		pingHTML := fmt.Sprintf("🏓 <b>Pong!</b> (Uptime: %s, Server: OK)", html.EscapeString(uptimeStr))
		_, err := r.sendMessage(ctx, chatID, pingHTML, keyboard)
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
		_, err := r.sendMessage(ctx, chatID, statusHTML, keyboard)
		return err

	case "/torch":
		if r.torch == nil {
			_, err := r.sendMessage(ctx, chatID, "⚠️ Hardware torch not configured", keyboard)
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
		_, sendErr := r.sendMessage(ctx, chatID, res, keyboard)
		return sendErr

	case "/exec":
		if r.executor == nil {
			_, err := r.sendMessage(ctx, chatID, "⚠️ Command executor not configured", keyboard)
			return err
		}
		cmdInput := strings.TrimSpace(strings.Join(args, " "))
		res := r.executor.Execute(ctx, cmdInput)
		_, err := r.sendMessage(ctx, chatID, res, keyboard)
		return err

	case "/reboot":
		rebootMsg := "♻️ <b>Restarting Pocket Sentinel via PM2...</b>\n<i>Process will cleanly exit and restart within 3 seconds.</i>"
		_, err := r.sendMessage(ctx, chatID, rebootMsg, nil)
		if r.rebootFunc != nil {
			go func() {
				time.Sleep(500 * time.Millisecond)
				r.rebootFunc()
			}()
		}
		return err

	default:
		unknownHTML := "❓ <b>Unknown command.</b>\nType /help to see available commands."
		_, err := r.sendMessage(ctx, chatID, unknownHTML, keyboard)
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
