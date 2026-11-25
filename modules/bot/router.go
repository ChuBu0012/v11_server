package bot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// TelegramSender abstracts sending Telegram messages for Router.
type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, text string, replyMarkup *InlineKeyboardMarkup) (*Message, error)
}

// Router handles incoming Telegram messages and routes commands.
type Router struct {
	client          *Client
	sender          TelegramSender
	authorizer      *Authorizer
	torch           *TorchController
	executor        *SafeExecutor
	keyboards       *KeyboardsModule
	statusProv      StatusCardProvider
	rebootFunc      func()
	digestFunc      func(ctx context.Context) (string, error)
	callbackHandler func(ctx context.Context, callback *CallbackQuery) error
	callbackRoutes  map[string]func(ctx context.Context, callback *CallbackQuery) error

	runner            *AgentRunner
	guard             *PromptGuard
	scrubber          *OutputScrubber
	ramProvider       func() float64
	locationFunc      func(ctx context.Context) (string, *InlineKeyboardMarkup, error)
	deployer          DeployStarter
	quotaFunc         func(ctx context.Context, refresh bool) (string, error)
	quotaPostTaskFunc func(ctx context.Context) (string, error)
	ringCtrl          *RingController
	askRouter         *AskTriageRouter
	liveMapEnabled    bool
}

// WithLiveMapEnabled controls whether Live Map entries are shown in /help, /panel, and reply keyboard.
func WithLiveMapEnabled(enabled bool) RouterOption {
	return func(r *Router) {
		r.liveMapEnabled = enabled
		if r.keyboards != nil {
			r.keyboards.SetLiveMapEnabled(enabled)
		}
	}
}

// WithRingController configures the device ring/alarm controller.
func WithRingController(ctrl *RingController) RouterOption {
	return func(r *Router) {
		r.ringCtrl = ctrl
		if r.keyboards != nil {
			r.keyboards.SetRingController(ctrl)
		}
	}
}

// WithAskRouter configures the hybrid Fast LLM & cache router for /ask.
func WithAskRouter(ar *AskTriageRouter) RouterOption {
	return func(r *Router) {
		r.askRouter = ar
	}
}

// WithQuotaFunc sets the callback for on-demand AI quota tracking.
func WithQuotaFunc(f func(ctx context.Context, refresh bool) (string, error)) RouterOption {
	return func(r *Router) {
		r.quotaFunc = f
	}
}

// WithQuotaPostTaskFunc sets the callback for post-task AI quota reporting.
func WithQuotaPostTaskFunc(f func(ctx context.Context) (string, error)) RouterOption {
	return func(r *Router) {
		r.quotaPostTaskFunc = f
	}
}

// WithDeployer enables the /deploy command.
func WithDeployer(d DeployStarter) RouterOption {
	return func(r *Router) {
		r.deployer = d
	}
}

// WithLocationFunc sets the callback for on-demand location status and Live Map URL.
func WithLocationFunc(f func(ctx context.Context) (string, *InlineKeyboardMarkup, error)) RouterOption {
	return func(r *Router) {
		r.locationFunc = f
	}
}

// SetLocationFunc dynamically assigns the location handler after router construction.
func (r *Router) SetLocationFunc(f func(ctx context.Context) (string, *InlineKeyboardMarkup, error)) {
	r.locationFunc = f
}

// WithCallbackHandler registers an application callback handler after owner authorization.
func WithCallbackHandler(handler func(context.Context, *CallbackQuery) error) RouterOption {
	return func(r *Router) { r.callbackHandler = handler }
}

// WithCallbackRoute registers a callback query handler for a specific data prefix.
func WithCallbackRoute(prefix string, h func(ctx context.Context, q *CallbackQuery) error) RouterOption {
	return func(r *Router) {
		if r.callbackRoutes == nil {
			r.callbackRoutes = make(map[string]func(ctx context.Context, callback *CallbackQuery) error)
		}
		r.callbackRoutes[prefix] = h
	}
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
		client:         client,
		sender:         client,
		authorizer:     authorizer,
		torch:          torch,
		executor:       executor,
		keyboards:      keyboards,
		statusProv:     statusProv,
		liveMapEnabled: true,
		callbackRoutes: make(map[string]func(ctx context.Context, callback *CallbackQuery) error),
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
	r.keyboards.SetSender(r.sender)
	if r.ringCtrl != nil {
		r.keyboards.SetRingController(r.ringCtrl)
	}
	if r.rebootFunc != nil {
		r.keyboards.SetRebootFunc(r.rebootFunc)
	}
	if pp, ok := statusProv.(PanelCardProvider); ok {
		r.keyboards.SetPanelProvider(pp)
	}
	if r.guard == nil {
		r.guard = NewPromptGuard()
	}
	if r.scrubber == nil {
		r.scrubber = NewOutputScrubber()
	}
	if r.quotaFunc != nil {
		if _, exists := r.callbackRoutes["quota_refresh"]; !exists {
			r.callbackRoutes["quota_refresh"] = func(ctx context.Context, q *CallbackQuery) error {
				if q.Message == nil || q.Message.Chat == nil {
					return nil
				}
				if r.client != nil {
					_ = r.client.AnswerCallbackQuery(ctx, q.ID, "🔄 กำลังอัปเดตโควตา AI...", false)
				}
				res, err := r.quotaFunc(ctx, true)
				if err != nil {
					res = fmt.Sprintf("⚠️ รีเฟรชโควตาไม่สำเร็จ: %s", html.EscapeString(err.Error()))
				}
				return r.editMessage(ctx, q.Message.Chat.ID, q.Message.MessageID, res, MakeQuotaKeyboard())
			}
		}
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

func (r *Router) sendReplyMarkup(ctx context.Context, chatID int64, text string, replyMarkup any) (*Message, error) {
	if r.client != nil {
		return r.client.SendMessageWithReplyMarkup(ctx, chatID, text, replyMarkup)
	}
	if sender, ok := r.sender.(interface {
		SendMessageWithReplyMarkup(ctx context.Context, chatID int64, text string, replyMarkup any) (*Message, error)
	}); ok {
		return sender.SendMessageWithReplyMarkup(ctx, chatID, text, replyMarkup)
	}
	if ikm, ok := replyMarkup.(*InlineKeyboardMarkup); ok {
		return r.sendMessage(ctx, chatID, text, ikm)
	}
	return r.sendMessage(ctx, chatID, text, nil)
}

func (r *Router) getPanelCard(ctx context.Context, tab string) (string, error) {
	if pp, ok := r.statusProv.(PanelCardProvider); ok {
		return pp.GetPanelCard(ctx, tab)
	}
	if r.statusProv != nil {
		return r.statusProv.GetStatusCard(ctx)
	}
	return "🎛️ <b>Pocket Sentinel · ควบคุมระบบ</b>\n(Status provider not configured)", nil
}

func (r *Router) editMessage(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error {
	if r.client != nil {
		return r.client.EditMessageText(ctx, chatID, messageID, text, replyMarkup)
	}
	if editor, ok := r.sender.(interface {
		EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error
	}); ok {
		return editor.EditMessageText(ctx, chatID, messageID, text, replyMarkup)
	}
	return nil
}

// ParseTaskEffort extracts an optional [low|medium|median|high|max] or --effort=... prefix
// and returns the normalized effort (low, medium, high, max) and the clean prompt.
func ParseTaskEffort(input string) (string, string) {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "[") {
		closeIdx := strings.Index(trimmed, "]")
		if closeIdx > 1 {
			eff := strings.ToLower(strings.TrimSpace(trimmed[1:closeIdx]))
			if eff == "median" {
				eff = "medium"
			}
			switch eff {
			case "low", "medium", "high", "max":
				rest := strings.TrimSpace(trimmed[closeIdx+1:])
				return eff, rest
			}
		}
	}
	if strings.HasPrefix(trimmed, "--effort=") {
		parts := strings.SplitN(trimmed, " ", 2)
		eff := strings.ToLower(strings.TrimPrefix(parts[0], "--effort="))
		if eff == "median" {
			eff = "medium"
		}
		switch eff {
		case "low", "medium", "high", "max":
			rest := ""
			if len(parts) > 1 {
				rest = strings.TrimSpace(parts[1])
			}
			return eff, rest
		}
	}
	return "medium", trimmed
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
		for prefix, handler := range r.callbackRoutes {
			if strings.HasPrefix(u.CallbackQuery.Data, prefix) {
				return handler(ctx, u.CallbackQuery)
			}
		}
		if r.callbackHandler != nil && strings.HasPrefix(u.CallbackQuery.Data, "qs:") {
			return r.callbackHandler(ctx, u.CallbackQuery)
		}
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

	// Map persistent reply keyboard buttons directly to canonical slash commands
	switch text {
	case "🎛️ แผงควบคุม":
		text = "/panel"
	case "📊 สถานะระบบ":
		text = "/status"
	case "⚡ โควตา AI":
		text = "/quota"
	case "🗺️ พิกัดสด":
		text = "/map"
	case "📰 สรุปประจำวัน":
		text = "/digest"
	case "🤖 สั่งงาน AI":
		text = "/help"
	case "⌨️ ซ่อนแป้นพิมพ์":
		text = "/keyboard_hide"
	}

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
	helpKeyboard := MakeHelpKeyboardWithOptions(r.liveMapEnabled)

	switch cmd {
	case "/start", "/help":
		locationHelp := ""
		panelTabs := "ระบบ / บริการ / ข่าวทอง"
		if r.liveMapEnabled {
			panelTabs = "ระบบ / บริการ / ข่าวทอง / พิกัด"
			locationHelp = "\n<b>📍 แผนที่และพิกัดสด</b>\n" +
				"<code>/live</code> หรือ <code>/map</code> ดูตำแหน่ง Chubu & Furturia พร้อมลิงก์แผนที่สด\n"
		}
		helpHTML := "🛡️ <b>Pocket Sentinel</b> · ศูนย์ควบคุม v11\n" +
			"<i>เลือกปุ่มลัดด้านล่าง หรือคัดลอกคำสั่งเพื่อแก้ไขก่อนส่ง</i>\n\n" +
			"<b>🎛️ แผงควบคุม &amp; สถานะ</b>\n" +
			"• <code>/panel</code> — เปิดแผงควบคุมแบบแท็บ (" + panelTabs + ")\n" +
			"• <code>/status</code> · <code>/ping</code> — สถานะเครื่องและการตอบสนอง\n" +
			"• <code>/keyboard</code> · <code>/keyboard_hide</code> — เปิด/ซ่อนแป้นพิมพ์ลัด\n" +
			"• <code>/torch [on|off|status]</code> · <code>/ring [on|off]</code> — ไฟฉาย / เสียงเรียกเครื่อง\n" +
			locationHelp +
			"\n<b>🤖 AI, ข่าวสาร &amp; ระบบ</b>\n" +
			"• <code>/ask [model] [effort] &lt;คำถาม&gt;</code> — ถาม AI ด่วน (auto, groq, gemini, agy)\n" +
			"• <code>/task [low|medium|high] &lt;คำสั่ง&gt;</code> — สั่ง Agent ทำงาน (<code>/task_status</code>)\n" +
			"• <code>/digest</code> — สรุปข่าวและราคาทอง\n" +
			"• <code>/quota</code> · <code>/limits</code> — เช็คโควตา AI พร้อมเวลารีเซ็ต\n" +
			"• <code>/exec ps|df|free|uptime|pm2</code> — ตรวจระบบปลอดภัย\n" +
			"• <code>/deploy [ข้อความ]</code> · <code>/reboot</code> — อัปเดต/รีสตาร์ตเซิร์ฟเวอร์"
		if cmd == "/start" {
			_, _ = r.sendReplyMarkup(ctx, chatID, "👋 <b>ยินดีต้อนรับสู่ Pocket Sentinel</b>\nเปิดใช้งานแป้นพิมพ์ลัดด้านล่างจอแล้ว", MakeDefaultReplyKeyboardWithOptions(r.liveMapEnabled))
		}
		_, err := r.sendMessage(ctx, chatID, helpHTML, helpKeyboard)
		return err

	case "/panel":
		panelHTML, err := r.getPanelCard(ctx, "system")
		if err != nil {
			panelHTML = "⚠️ ไม่สามารถโหลดแผงควบคุมได้: " + html.EscapeString(err.Error())
		}
		_, err = r.sendMessage(ctx, chatID, panelHTML, MakePanelKeyboardWithOptions("system", r.liveMapEnabled))
		return err

	case "/keyboard", "/show_keyboard":
		msg := "⌨️ เปิดใช้งานแป้นพิมพ์ลัดด้านล่างจอแล้ว"
		_, err := r.sendReplyMarkup(ctx, chatID, msg, MakeDefaultReplyKeyboardWithOptions(r.liveMapEnabled))
		return err

	case "/keyboard_hide", "/hide_keyboard":
		msg := "⌨️ ซ่อนแป้นพิมพ์ลัดแล้ว — พิมพ์ <code>/keyboard</code> หรือ <code>/start</code> หากต้องการเปิดอีกครั้ง"
		_, err := r.sendReplyMarkup(ctx, chatID, msg, MakeRemoveReplyKeyboard())
		return err

	case "/live", "/map":
		if r.locationFunc == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ <b>Live Map ไม่ได้ถูกเปิดใช้งาน</b>\nระบบ Location Tracker ยังไม่ได้เริ่มต้นทำงาน", keyboard)
			return err
		}
		msgHTML, mapKeyboard, err := r.locationFunc(ctx)
		if err != nil {
			_, err := r.sendMessage(ctx, chatID, "⚠️ <b>โหลดข้อมูลตำแหน่งไม่สำเร็จ:</b> "+html.EscapeString(err.Error()), keyboard)
			return err
		}
		_, err = r.sendMessage(ctx, chatID, msgHTML, mapKeyboard)
		return err

	case "/task":
		if r.runner == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ <b>Agent Runner ไม่ได้ถูกตั้งค่า:</b> Agent Runner feature is not configured.", keyboard)
			return err
		}

		rawInput := ""
		if len(text) > len(parts[0]) {
			rawInput = strings.TrimSpace(text[len(parts[0]):])
		}

		if rawInput == "" {
			usageHTML := "🤖 <b>สร้างงานให้ Agent</b>\n" +
				"ส่งคำสั่งในรูปแบบนี้:\n<code>/task [low|medium|high] &lt;สิ่งที่ต้องการ&gt;</code>\n\n" +
				"ใช้ปุ่มคัดลอกตัวอย่าง แล้วแก้รายละเอียดก่อนส่งได้เลย"
			_, err := r.sendMessage(ctx, chatID, usageHTML, helpKeyboard)
			return err
		}

		effort, cleanPrompt := ParseTaskEffort(rawInput)
		if cleanPrompt == "" {
			cleanPrompt = rawInput
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

		framedPrompt, err := r.guard.ValidateAndFrame(cleanPrompt)
		if err != nil {
			if errors.Is(err, ErrPromptTooLong) {
				lenHTML := fmt.Sprintf("⚠️ <b>ความยาวข้อความเกินกำหนด:</b> คำสั่งต้องไม่เกิน %d ตัวอักษร (ปัจจุบัน %d ตัวอักษร)",
					MaxPromptLength, utf8.RuneCountInString(cleanPrompt))
				_, sendErr := r.sendMessage(ctx, chatID, lenHTML, keyboard)
				return sendErr
			}
			if errors.Is(err, ErrPromptEmpty) {
				usageHTML := "🤖 <b>สร้างงานให้ Agent</b>\n<code>/task [low|medium|high] &lt;สิ่งที่ต้องการ&gt;</code>\n\n" +
					"เลือกปุ่มตัวอย่างเพื่อคัดลอกข้อความ แล้วแก้ก่อนส่ง"
				_, sendErr := r.sendMessage(ctx, chatID, usageHTML, helpKeyboard)
				return sendErr
			}
			secHTML := "🛡️ <b>Security Warning:</b> คำสั่งถูกปฏิเสธเนื่องจากละเมิดนโยบายความปลอดภัย (Prompt Injection / Destructive Command)"
			_, sendErr := r.sendMessage(ctx, chatID, secHTML, keyboard)
			return sendErr
		}

		taskReq := TaskRequest{
			ChatID:    chatID,
			RawPrompt: framedPrompt,
			Effort:    effort,
			CreatedAt: time.Now(),
		}

		ackSent := make(chan *Message, 1)
		stopHeartbeat := make(chan struct{})

		dispatchErr := r.runner.DispatchAsync(context.Background(), taskReq, func(result TaskResult) {
			<-ackSent // Guarantee ACK has finished sending before transmitting result
			select {
			case <-stopHeartbeat:
			default:
				close(stopHeartbeat) // Stop heartbeat ticker if still active
			}

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
			if r.quotaPostTaskFunc != nil || r.quotaFunc != nil {
				go r.sendQuotaUsageFollowUp(chatID, 30*time.Second)
			}
		})

		if errors.Is(dispatchErr, ErrAgentBusy) {
			ackSent <- nil
			busyHTML := "⏳ <b>Agent กำลังทำงานอื่นอยู่:</b> ไม่สามารถรับงานซ้อนได้ในขณะนี้ กรุณารอสักครู่แล้วลองใหม่อีกครั้ง\n(พิมพ์ /task_status เพื่อดูสถานะ)"
			_, sendErr := r.sendMessage(ctx, chatID, busyHTML, keyboard)
			return sendErr
		}
		if dispatchErr != nil {
			ackSent <- nil
			_, sendErr := r.sendMessage(ctx, chatID, fmt.Sprintf("❌ ไม่สามารถเริ่มงานได้: %s", r.scrubber.EscapeHTML(dispatchErr.Error())), keyboard)
			return sendErr
		}

		// Send immediate ACK (< 500ms)
		timeoutDesc := "1 ชั่วโมง"
		if r.runner != nil && r.runner.Timeout() > 0 {
			timeoutDesc = formatTimeoutTH(r.runner.Timeout())
		}
		promptSnippet := cleanPrompt
		if utf8.RuneCountInString(promptSnippet) > 60 {
			runes := []rune(promptSnippet)
			promptSnippet = string(runes[:60]) + "..."
		}
		ackHTML := fmt.Sprintf("🤖 <b>รับงานแล้ว:</b> <i>\"%s\"</i>\n⚡ ระดับ Effort: <code>%s</code> | Timeout: %s\n⏱ กำลังประมวลผลในเบื้องหลัง...\n💡 คุณสามารถไปทำอย่างอื่นได้เลย ระบบจะแจ้งเตือนเมื่อเสร็จสิ้น",
			r.scrubber.EscapeHTML(promptSnippet), strings.ToUpper(effort), timeoutDesc)
		ackMsg, ackErr := r.sendMessage(ctx, chatID, ackHTML, keyboard)
		ackSent <- ackMsg

		// Start in-place heartbeat ticker if message was sent successfully
		if ackErr == nil && ackMsg != nil && ackMsg.MessageID > 0 {
			go func(msgID int64, snippet, eff, tDesc string) {
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				start := time.Now()
				for {
					select {
					case <-stopHeartbeat:
						return
					case <-ticker.C:
						elapsed := time.Since(start).Round(time.Second)
						ramMB := 0.0
						if r.ramProvider != nil {
							ramMB = r.ramProvider()
						}
						progressHTML := fmt.Sprintf("🤖 <b>รับงานแล้ว:</b> <i>\"%s\"</i>\n⚡ ระดับ Effort: <code>%s</code> | Timeout: %s\n⏳ <b>กำลังประมวลผล...</b> (ผ่านไปแล้ว %s | RAM: %.1f MB)\n💡 คุณสามารถไปทำอย่างอื่นได้เลย ระบบจะแจ้งเตือนเมื่อเสร็จสิ้น",
							r.scrubber.EscapeHTML(snippet), strings.ToUpper(eff), tDesc, elapsed.String(), ramMB)
						editCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						_ = r.editMessage(editCtx, chatID, msgID, progressHTML, nil)
						cancel()
					}
				}
			}(ackMsg.MessageID, promptSnippet, effort, timeoutDesc)
		}

		return ackErr

	case "/ring":
		if r.ringCtrl == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ <b>Ring Controller ไม่ได้ถูกตั้งค่า:</b> Hardware sound controller is not configured.", keyboard)
			return err
		}
		subCmd := ""
		if len(parts) > 1 {
			subCmd = strings.ToLower(strings.TrimSpace(parts[1]))
		}
		if subCmd == "off" || subCmd == "stop" {
			msg, err := r.ringCtrl.Stop(ctx)
			if err != nil {
				_, sendErr := r.sendMessage(ctx, chatID, fmt.Sprintf("❌ ไม่สามารถปิดเสียงได้: %v", err), keyboard)
				return sendErr
			}
			_, sendErr := r.sendMessage(ctx, chatID, msg, keyboard)
			return sendErr
		}

		msg, err := r.ringCtrl.Ring(ctx)
		if err != nil {
			_, sendErr := r.sendMessage(ctx, chatID, fmt.Sprintf("❌ ไม่สามารถส่งเสียงได้: %v", err), keyboard)
			return sendErr
		}

		ringKeyboard := &InlineKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{
				{
					{Text: "🔇 ปิดเสียงทันที", CallbackData: "ring:off"},
				},
			},
		}
		_, sendErr := r.sendMessage(ctx, chatID, msg, ringKeyboard)
		return sendErr

	case "/ask":
		rawInput := ""
		if len(text) > len(parts[0]) {
			rawInput = strings.TrimSpace(text[len(parts[0]):])
		}

		if rawInput == "" {
			usageHTML := "💬 <b>ถามคำถามกับ AI (Fast &amp; Smart)</b>\n" +
				"ส่งคำถามในรูปแบบนี้:\n<code>/ask [model] [effort] &lt;คำถามหรือข้อสงสัย&gt;</code>\n\n" +
				"⚡ <b>เลือกโมเดล (Model - พิมพ์ต่อท้าย /ask ได้เลย ไม่ต้องใส่วงเล็บ):</b>\n" +
				"• <code>auto</code> AI วิเคราะห์และเลือกโมเดลให้อัตโนมัติ (ค่าเริ่มต้น)\n" +
				"• <code>groq</code> หรือ <code>gemini</code> ตอบเร็วพิเศษ (&lt; 2 วินาที)\n" +
				"• <code>agy</code> สั่งงานเชิงลึกด้วย Agent พร้อมเครื่องมือไฟล์/ระบบ\n\n" +
				"🎯 <b>ระดับความลึก (Effort):</b>\n" +
				"• <code>low</code> | <code>medium</code> | <code>high</code> (ค่าเริ่มต้น: <code>medium</code>)\n\n" +
				"💡 ตัวอย่าง:\n" +
				"• <code>/ask ทำไมทองคำราคาขึ้น</code>\n" +
				"• <code>/ask groq high อธิบายทฤษฎีควอนตัม</code>\n" +
				"• <code>/ask agy ตรวจสอบ git status ล่าสุด</code>"
			_, err := r.sendMessage(ctx, chatID, usageHTML, helpKeyboard)
			return err
		}

		askOpts := ParseAskInput(rawInput)
		cleanPrompt := askOpts.CleanPrompt
		if cleanPrompt == "" {
			cleanPrompt = rawInput
		}

		framedPrompt, err := r.guard.ValidateAndFrameQuestion(cleanPrompt)
		if err != nil {
			if errors.Is(err, ErrPromptTooLong) {
				lenHTML := fmt.Sprintf("⚠️ <b>ความยาวข้อความเกินกำหนด:</b> คำถามต้องไม่เกิน %d ตัวอักษร (ปัจจุบัน %d ตัวอักษร)",
					MaxPromptLength, utf8.RuneCountInString(cleanPrompt))
				_, sendErr := r.sendMessage(ctx, chatID, lenHTML, keyboard)
				return sendErr
			}
			if errors.Is(err, ErrPromptEmpty) {
				usageHTML := "💬 <b>ถามคำถามกับ AI</b>\n<code>/ask [model] [effort] &lt;คำถาม&gt;</code>\n\n" +
					"กรุณาระบุคำถามที่ต้องการถาม"
				_, sendErr := r.sendMessage(ctx, chatID, usageHTML, helpKeyboard)
				return sendErr
			}
			secHTML := "🛡️ <b>Security Warning:</b> คำถามถูกปฏิเสธเนื่องจากละเมิดนโยบายความปลอดภัย (Prompt Injection / Forbidden Content)"
			_, sendErr := r.sendMessage(ctx, chatID, secHTML, keyboard)
			return sendErr
		}

		// Fast Router Path (Groq / Gemini / Cache)
		if r.askRouter != nil {
			triageVerdict, _ := r.askRouter.Triage(ctx, cleanPrompt, askOpts.Model)
			if triageVerdict == VerdictFastLLM {
				fastRes, err := r.askRouter.ExecuteFast(ctx, askOpts)
				if err == nil && fastRes != nil {
					sourceBadge := fmt.Sprintf("%s · %.2fs", strings.ToUpper(fastRes.Provider), fastRes.Duration.Seconds())
					if fastRes.IsCached {
						sourceBadge = fmt.Sprintf("Cache · %s", formatCachedAgo(fastRes.CachedAt))
					}
					formatted := fmt.Sprintf("💬 <b>คำตอบจาก AI</b> · <i>%s</i>\n\n%s", sourceBadge, r.scrubber.EscapeHTML(fastRes.ResponseText))
					_, sendErr := r.sendMessage(ctx, chatID, formatted, keyboard)
					return sendErr
				}
				log.Printf("[bot:router] Fast LLM execution failed (%v), delegating to agy runner", err)
			}
		}

		if r.runner == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ <b>Agent Runner ไม่ได้ถูกตั้งค่า:</b> Agent Runner feature is not configured.", keyboard)
			return err
		}

		if !r.runner.CanAcquireLock() {
			st := r.runner.Status()
			elapsedStr := "สักครู่"
			if st.Elapsed > 0 {
				elapsedStr = st.Elapsed.Round(time.Second).String()
			}
			busyHTML := fmt.Sprintf("⏳ <b>Agent กำลังทำงานอื่นอยู่:</b> เริ่มเมื่อ %s ที่แล้ว\nไม่สามารถรับคำถามซ้อนได้ในขณะนี้ กรุณารอสักครู่แล้วลองใหม่อีกครั้ง\n(พิมพ์ /task_status เพื่อดูสถานะ)", elapsedStr)
			_, err := r.sendMessage(ctx, chatID, busyHTML, keyboard)
			return err
		}

		taskReq := TaskRequest{
			ChatID:    chatID,
			RawPrompt: framedPrompt,
			Effort:    askOpts.Effort,
			CreatedAt: time.Now(),
		}

		ackSent := make(chan *Message, 1)
		stopHeartbeat := make(chan struct{})

		dispatchErr := r.runner.DispatchAsync(context.Background(), taskReq, func(result TaskResult) {
			<-ackSent // Guarantee ACK has finished sending before transmitting result
			select {
			case <-stopHeartbeat:
			default:
				close(stopHeartbeat) // Stop heartbeat ticker if still active
			}

			ramMB := 0.0
			if r.ramProvider != nil {
				ramMB = r.ramProvider()
			}
			formatted := r.scrubber.FormatQuestionResult(result, ramMB)
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, sendResErr := r.sendMessage(bgCtx, chatID, formatted, nil)
			if sendResErr != nil {
				log.Printf("[bot:router] failed to send question result to chat %d: %v", chatID, sendResErr)
			}
			if r.quotaPostTaskFunc != nil || r.quotaFunc != nil {
				go r.sendQuotaUsageFollowUp(chatID, 30*time.Second)
			}
		})

		if errors.Is(dispatchErr, ErrAgentBusy) {
			ackSent <- nil
			busyHTML := "⏳ <b>Agent กำลังทำงานอื่นอยู่:</b> ไม่สามารถรับคำถามซ้อนได้ในขณะนี้ กรุณารอสักครู่แล้วลองใหม่อีกครั้ง\n(พิมพ์ /task_status เพื่อดูสถานะ)"
			_, sendErr := r.sendMessage(ctx, chatID, busyHTML, keyboard)
			return sendErr
		}
		if dispatchErr != nil {
			ackSent <- nil
			_, sendErr := r.sendMessage(ctx, chatID, fmt.Sprintf("❌ ไม่สามารถประมวลผลคำถามได้: %s", r.scrubber.EscapeHTML(dispatchErr.Error())), keyboard)
			return sendErr
		}

		// Send immediate ACK (< 500ms)
		timeoutDesc := "1 ชั่วโมง"
		if r.runner != nil && r.runner.Timeout() > 0 {
			timeoutDesc = formatTimeoutTH(r.runner.Timeout())
		}
		promptSnippet := cleanPrompt
		if utf8.RuneCountInString(promptSnippet) > 60 {
			runes := []rune(promptSnippet)
			promptSnippet = string(runes[:60]) + "..."
		}
		delegationNotice := ""
		if askOpts.Model == "auto" {
			delegationNotice = "\n🔄 <b>ส่งต่อให้ agy:</b> งานนี้ต้องใช้ Agent เครื่องมือเฉพาะทาง"
		}
		ackHTML := fmt.Sprintf("💬 <b>รับคำถามแล้ว:</b> <i>\"%s\"</i>%s\n⚡ ระดับ Effort: <code>%s</code> | Timeout: %s\n⏱ กำลังค้นหาคำตอบในเบื้องหลัง...\n💡 ระบบจะแจ้งเตือนเมื่อได้รับคำตอบจาก agy",
			r.scrubber.EscapeHTML(promptSnippet), delegationNotice, strings.ToUpper(askOpts.Effort), timeoutDesc)
		ackMsg, ackErr := r.sendMessage(ctx, chatID, ackHTML, keyboard)
		ackSent <- ackMsg

		// Start in-place heartbeat ticker if message was sent successfully
		if ackErr == nil && ackMsg != nil && ackMsg.MessageID > 0 {
			go func(msgID int64, snippet, eff, tDesc string) {
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				start := time.Now()
				for {
					select {
					case <-stopHeartbeat:
						return
					case <-ticker.C:
						elapsed := time.Since(start).Round(time.Second)
						ramMB := 0.0
						if r.ramProvider != nil {
							ramMB = r.ramProvider()
						}
						progressHTML := fmt.Sprintf("💬 <b>รับคำถามแล้ว:</b> <i>\"%s\"</i>\n⚡ ระดับ Effort: <code>%s</code> | Timeout: %s\n⏳ <b>กำลังวิเคราะห์คำตอบ...</b> (ผ่านไปแล้ว %s | RAM: %.1f MB)\n🔍 agy กำลังคิดและประมวลผลคำตอบ...",
							r.scrubber.EscapeHTML(snippet), strings.ToUpper(eff), tDesc, elapsed.String(), ramMB)
						editCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						_ = r.editMessage(editCtx, chatID, msgID, progressHTML, nil)
						cancel()
					}
				}
			}(ackMsg.MessageID, promptSnippet, askOpts.Effort, timeoutDesc)
		}

		return ackErr

	case "/task_status":
		if r.runner == nil {
			_, err := r.sendMessage(ctx, chatID, "ℹ️ <b>Agent Runner ไม่ได้ถูกตั้งค่า:</b> Agent Runner feature is not configured.", keyboard)
			return err
		}

		st := r.runner.Status()
		if !st.IsRunning {
			idleHTML := "💤 <b>สถานะ Agent:</b> ว่าง (Idle)\n" +
				"พร้อมรับคำสั่งใหม่ สามารถสั่งงานได้ด้วย <code>/task &lt;คำสั่ง&gt;</code> หรือถามด้วย <code>/ask &lt;คำถาม&gt;</code>"
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
			if strings.TrimSpace(res) != "" {
				msg := fmt.Sprintf("📰 <b>Pocket Sentinel · สรุปข่าว AI &amp; รอบวัน</b>\n\n%s", res)
				_, _ = r.sendMessage(bgCtx, chatID, msg, nil)
			}
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

	case "/quota", "/limits":
		if r.quotaFunc == nil {
			_, err := r.sendMessage(ctx, chatID, "⚠️ ฟีเจอร์ตรวจสอบโควตา AI ยังไม่ได้ตั้งค่า", keyboard)
			return err
		}
		res, err := r.quotaFunc(ctx, false)
		if err != nil {
			_, err2 := r.sendMessage(ctx, chatID, fmt.Sprintf("⚠️ ดึงโควตาไม่สำเร็จ: %s", html.EscapeString(err.Error())), keyboard)
			return err2
		}
		_, err = r.sendMessage(ctx, chatID, res, MakeQuotaKeyboard())
		return err

	case "/deploy":
		if r.deployer == nil {
			_, err := r.sendMessage(ctx, chatID, "⚠️ Deploy not configured", keyboard)
			return err
		}
		message := strings.TrimSpace(strings.Join(args, " "))
		var reply string
		switch err := r.deployer.Start(message); {
		case err == nil:
			reply = "🚀 <b>รับคำสั่ง deploy แล้ว</b>\nกำลังดึงโค้ดล่าสุดจาก GitHub แล้ว build + restart\n<i>ระบบจะแจ้งผลเมื่อเสร็จ (log: logs/deploy.log)</i>"
		case errors.Is(err, ErrDeployRunning):
			reply = "⏳ <b>มี deploy กำลังทำงานอยู่</b>\nรอให้รอบนี้เสร็จก่อนแล้วค่อยสั่งใหม่"
		default:
			reply = "❌ <b>เริ่ม deploy ไม่ได้:</b> " + escapeDeployError(err)
		}
		_, err := r.sendMessage(ctx, chatID, reply, keyboard)
		return err

	case "/reboot":
		rebootMsg := "♻️ <b>กำลังรีสตาร์ต Pocket Sentinel</b>\n\n<i>ระบบจะปิดโปรเซสอย่างปลอดภัย และเริ่มทำงานใหม่ภายใน 3 วินาที</i>"
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

// formatTimeoutTH formats a duration into a human-readable Thai string.
func formatTimeoutTH(d time.Duration) string {
	if d <= 0 {
		return "ไม่จำกัด"
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%d ชั่วโมง", d/time.Hour)
	}
	if d > time.Hour {
		hrs := d / time.Hour
		mins := (d % time.Hour) / time.Minute
		if mins > 0 {
			return fmt.Sprintf("%d ชั่วโมง %d นาที", hrs, mins)
		}
		return fmt.Sprintf("%d ชั่วโมง", hrs)
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%d นาที", d/time.Minute)
	}
	if d >= time.Minute {
		mins := d / time.Minute
		secs := (d % time.Minute) / time.Second
		if secs > 0 {
			return fmt.Sprintf("%d นาที %d วินาที", mins, secs)
		}
		return fmt.Sprintf("%d นาที", mins)
	}
	return fmt.Sprintf("%d วินาที", int(d.Seconds()))
}

func escapeDeployError(err error) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(err.Error())
}

func (r *Router) sendQuotaUsageFollowUp(chatID int64, delay time.Duration) {
	time.Sleep(delay)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var msg string
	var err error
	if r.quotaPostTaskFunc != nil {
		msg, err = r.quotaPostTaskFunc(ctx)
	} else if r.quotaFunc != nil {
		msg, err = r.quotaFunc(ctx, true)
	}
	if err != nil {
		log.Printf("[bot:router] failed to fetch post-task quota: %v", err)
		return
	}
	if strings.TrimSpace(msg) == "" {
		return
	}

	if _, sendErr := r.sendMessage(ctx, chatID, msg, nil); sendErr != nil {
		log.Printf("[bot:router] failed to send post-task quota message: %v", sendErr)
	}
}

// formatCachedAgo returns a concise Thai relative time string for cached responses.
func formatCachedAgo(t time.Time) string {
	diff := time.Since(t).Round(time.Minute)
	if diff < time.Minute {
		return "เมื่อสักครู่"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%d นาทีที่แล้ว", int(diff.Minutes()))
	}
	hours := int(diff.Hours())
	mins := int(diff.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%d ชั่วโมงที่แล้ว", hours)
	}
	return fmt.Sprintf("%d ชม. %d นาทีที่แล้ว", hours, mins)
}
