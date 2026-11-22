package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// StatusCardProvider supplies real-time status card HTML and uptime.
type StatusCardProvider interface {
	GetStatusCard(ctx context.Context) (string, error)
	GetUptimeString() string
}

// PanelCardProvider supplies formatted HTML for the interactive /panel tabs.
type PanelCardProvider interface {
	GetPanelCard(ctx context.Context, tab string) (string, error)
}

// KeyboardsModule manages inline keyboards and callback query routing.
type KeyboardsModule struct {
	client     *Client
	sender     TelegramSender
	statusProv StatusCardProvider
	panelProv  PanelCardProvider
	torch      *TorchController
	ring       *RingController
	rebootFunc func()
}

// NewKeyboardsModule creates an inline keyboard manager.
func NewKeyboardsModule(client *Client, statusProv StatusCardProvider, torch *TorchController) *KeyboardsModule {
	km := &KeyboardsModule{
		client:     client,
		statusProv: statusProv,
		torch:      torch,
	}
	if pp, ok := statusProv.(PanelCardProvider); ok {
		km.panelProv = pp
	}
	return km
}

// SetSender sets the TelegramSender instance.
func (k *KeyboardsModule) SetSender(s TelegramSender) {
	k.sender = s
}

func (k *KeyboardsModule) editMessage(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error {
	if k.client != nil {
		return k.client.EditMessageText(ctx, chatID, messageID, text, replyMarkup)
	}
	if editor, ok := k.sender.(interface {
		EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error
	}); ok {
		return editor.EditMessageText(ctx, chatID, messageID, text, replyMarkup)
	}
	return nil
}

func (k *KeyboardsModule) answerCallback(ctx context.Context, queryID string, text string, alert bool) error {
	if k.client != nil {
		return k.client.AnswerCallbackQuery(ctx, queryID, text, alert)
	}
	return nil
}

// SetPanelProvider sets the panel card provider.
func (k *KeyboardsModule) SetPanelProvider(p PanelCardProvider) {
	k.panelProv = p
}

// SetRingController sets the RingController instance.
func (k *KeyboardsModule) SetRingController(r *RingController) {
	k.ring = r
}

// SetRebootFunc assigns the reboot handler.
func (k *KeyboardsModule) SetRebootFunc(f func()) {
	k.rebootFunc = f
}

// MakeDefaultReplyKeyboard creates a persistent, compact 3x2 reply keyboard for chat shortcuts.
func MakeDefaultReplyKeyboard() *ReplyKeyboardMarkup {
	return &ReplyKeyboardMarkup{
		Keyboard: [][]KeyboardButton{
			{
				{Text: "🎛️ แผงควบคุม"},
				{Text: "📊 สถานะระบบ"},
			},
			{
				{Text: "🗺️ พิกัดสด"},
				{Text: "📰 สรุปประจำวัน"},
			},
			{
				{Text: "☀️ มอร์นิ่งอินไซต์"},
				{Text: "🤖 สั่งงาน AI"},
			},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

// MakeRemoveReplyKeyboard removes the current custom keyboard.
func MakeRemoveReplyKeyboard() *ReplyKeyboardRemove {
	return &ReplyKeyboardRemove{
		RemoveKeyboard: true,
	}
}

// MakePanelKeyboard returns the tabbed inline keyboard for /panel.
func MakePanelKeyboard(activeTab string) *InlineKeyboardMarkup {
	if activeTab == "" {
		activeTab = "system"
	}

	tabBtn := func(key, label string) InlineKeyboardButton {
		if key == activeTab {
			return InlineKeyboardButton{Text: "• " + label + " •", CallbackData: "panel:tab:" + key}
		}
		return InlineKeyboardButton{Text: label, CallbackData: "panel:tab:" + key}
	}

	tabsRow := []InlineKeyboardButton{
		tabBtn("system", "📊 ระบบ"),
		tabBtn("services", "🌐 บริการ"),
		tabBtn("digest", "📰 ข่าว/ทอง"),
		tabBtn("location", "🗺️ พิกัด"),
	}

	var actionRows [][]InlineKeyboardButton

	switch activeTab {
	case "services":
		actionRows = [][]InlineKeyboardButton{
			{
				{Text: "🔄 ตรวจสอบด่วน", CallbackData: "panel:refresh:services"},
				{Text: "📋 เช็ค PM2", CopyText: &CopyTextButton{Text: "/task [low] pm2 list"}},
			},
			{
				{Text: "➕ จัดการ Endpoints", URL: "http://100.92.194.87:8080/endpoints"},
			},
		}
	case "digest":
		actionRows = [][]InlineKeyboardButton{
			{
				{Text: "🔄 รีเฟรช", CallbackData: "panel:refresh:digest"},
				{Text: "📰 สรุปเต็ม /digest", CopyText: &CopyTextButton{Text: "/digest"}},
			},
			{
				{Text: "☀️ มอร์นิ่งอินไซต์", CopyText: &CopyTextButton{Text: "/insight"}},
			},
		}
	case "location":
		actionRows = [][]InlineKeyboardButton{
			{
				{Text: "🔄 รีเฟรช", CallbackData: "panel:refresh:location"},
				{Text: "📍 พิกัดสด /map", CopyText: &CopyTextButton{Text: "/map"}},
			},
			{
				{Text: "🗺️ เปิดแผนที่สด", URL: "http://100.92.194.87:8080/?app=map"},
			},
		}
	default: // "system"
		actionRows = [][]InlineKeyboardButton{
			{
				{Text: "🔄 รีเฟรช", CallbackData: "panel:refresh:system"},
				{Text: "🔦 สลับไฟฉาย", CallbackData: "panel:torch_toggle"},
			},
			{
				{Text: "🔔 ส่งเสียงหาเครื่อง", CallbackData: "panel:ring_toggle"},
				{Text: "🏓 Ping", CallbackData: "ping_check"},
			},
			{
				{Text: "🔄 Reboot", CallbackData: "panel:action:reboot"},
			},
		}
	}

	rows := append([][]InlineKeyboardButton{tabsRow}, actionRows...)
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// MakeStatusKeyboard returns the interactive inline keyboard per specification.
func MakeStatusKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh Status", CallbackData: "status_refresh"},
			},
			{
				{Text: "🔦 Toggle Torch", CallbackData: "torch_toggle"},
				{Text: "🩺 Ping", CallbackData: "ping_check"},
			},
		},
	}
}

// MakeHelpKeyboard combines direct safe actions with editable command drafts.
func MakeHelpKeyboard() *InlineKeyboardMarkup {
	copyCommand := func(label, text string) InlineKeyboardButton {
		return InlineKeyboardButton{Text: label, CopyText: &CopyTextButton{Text: text}}
	}
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{copyCommand("🎛️ แผงควบคุม", "/panel"), {Text: "🔄 สถานะ", CallbackData: "status_refresh"}},
			{{Text: "🔦 ไฟฉาย", CallbackData: "torch_toggle"}, copyCommand("🗺️ ดูพิกัด /live", "/live")},
			{
				copyCommand("💬 ถาม AI", "/ask ทำไมท้องฟ้าเป็นสีฟ้า"),
				copyCommand("🔔 เรียกเครื่อง", "/ring"),
			},
			{
				copyCommand("📋 เช็คเครื่อง", "/task [low] ตรวจสอบ RAM, พื้นที่ disk และสถานะ PM2 go-server"),
				copyCommand("📋 เช็ค PM2", "/task [medium] ตรวจสอบสถานะ PM2 ของ go-server และ gatus"),
			},
			{
				copyCommand("⚡ โควตา AI", "/quota"),
				copyCommand("☀️ Insight", "/insight"),
			},
			{
				copyCommand("📰 Digest", "/digest"),
				copyCommand("💾 Disk", "/exec df -h"),
			},
			{
				copyCommand("🚀 Deploy", "/deploy"),
			},
		},
	}
}

// MakeQuotaKeyboard returns inline buttons for quota refresh and dashboard viewing.
func MakeQuotaKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 รีเฟรชโควตา", CallbackData: "quota_refresh"},
				{Text: "📊 ดู Dashboard", URL: "http://100.92.194.87:8080"},
			},
		},
	}
}

// HandleCallbackQuery dispatches incoming inline button clicks and updates messages in-place.
func (k *KeyboardsModule) HandleCallbackQuery(ctx context.Context, q *CallbackQuery) error {
	if q == nil {
		return nil
	}

	var chatID int64
	var messageID int64
	if q.Message != nil && q.Message.Chat != nil {
		chatID = q.Message.Chat.ID
		messageID = q.Message.MessageID
	}

	log.Printf("[bot:callback] handling callback '%s' from user %s (chat=%d, msg=%d)", q.Data, q.From.Username, chatID, messageID)

	switch {
	case strings.HasPrefix(q.Data, "panel:tab:"):
		tab := strings.TrimPrefix(q.Data, "panel:tab:")
		_ = k.answerCallback(ctx, q.ID, "", false)
		if chatID != 0 && messageID != 0 && k.panelProv != nil {
			card, err := k.panelProv.GetPanelCard(ctx, tab)
			if err != nil {
				card = fmt.Sprintf("⚠️ ไม่สามารถโหลดแท็บได้: %v", err)
			}
			return k.editMessage(ctx, chatID, messageID, card, MakePanelKeyboard(tab))
		}

	case strings.HasPrefix(q.Data, "panel:refresh:"):
		tab := strings.TrimPrefix(q.Data, "panel:refresh:")
		_ = k.answerCallback(ctx, q.ID, "🔄 อัปเดตข้อมูลสดแล้ว", false)
		if chatID != 0 && messageID != 0 && k.panelProv != nil {
			card, err := k.panelProv.GetPanelCard(ctx, tab)
			if err != nil {
				card = fmt.Sprintf("⚠️ ไม่สามารถรีเฟรชได้: %v", err)
			}
			return k.editMessage(ctx, chatID, messageID, card, MakePanelKeyboard(tab))
		}

	case q.Data == "panel:torch_toggle":
		var toast string
		if k.torch != nil {
			_, err := k.torch.Toggle(ctx)
			if err != nil {
				toast = fmt.Sprintf("❌ Torch error: %v", err)
			} else {
				if k.torch.IsOn() {
					toast = "🔦 ไฟฉายเปิดแล้ว (ตัดอัตโนมัติใน 3 นาที)"
				} else {
					toast = "🔦 ไฟฉายปิดแล้ว"
				}
			}
			_ = k.answerCallback(ctx, q.ID, toast, false)
			if chatID != 0 && messageID != 0 && k.panelProv != nil {
				card, err := k.panelProv.GetPanelCard(ctx, "system")
				if err == nil {
					_ = k.editMessage(ctx, chatID, messageID, card, MakePanelKeyboard("system"))
				}
			}
		} else {
			_ = k.answerCallback(ctx, q.ID, "Hardware torch not configured", true)
		}

	case q.Data == "panel:ring_toggle":
		var toast string
		if k.ring != nil {
			if k.ring.IsRinging() {
				_, _ = k.ring.Stop(ctx)
				toast = "🔕 ปิดเสียงโทรศัพท์แล้ว"
			} else {
				_, _ = k.ring.Ring(ctx)
				toast = "🔔 กำลังส่งเสียงเรียกโทรศัพท์ (ตัดใน 30 วิ)"
			}
			_ = k.answerCallback(ctx, q.ID, toast, false)
			if chatID != 0 && messageID != 0 && k.panelProv != nil {
				card, err := k.panelProv.GetPanelCard(ctx, "system")
				if err == nil {
					_ = k.editMessage(ctx, chatID, messageID, card, MakePanelKeyboard("system"))
				}
			}
		} else {
			_ = k.answerCallback(ctx, q.ID, "Ring controller not configured", true)
		}

	case q.Data == "ring:off":
		if k.ring != nil {
			msg, _ := k.ring.Stop(ctx)
			_ = k.answerCallback(ctx, q.ID, "🔕 ปิดเสียงเรียบร้อยแล้ว", false)
			if chatID != 0 && messageID != 0 {
				_ = k.editMessage(ctx, chatID, messageID, msg, nil)
			}
		} else {
			_ = k.answerCallback(ctx, q.ID, "Ring controller not configured", true)
		}

	case q.Data == "panel:action:reboot":
		_ = k.answerCallback(ctx, q.ID, "🔄 กำลังเตรียม Restart Go Server...", false)
		if k.rebootFunc != nil {
			go func() {
				time.Sleep(500 * time.Millisecond)
				k.rebootFunc()
			}()
		}

	case q.Data == "status_refresh":
		// Acknowledge immediately to dismiss Telegram loading spinner
		_ = k.answerCallback(ctx, q.ID, "🔄 Updating status...", false)

		if chatID != 0 && messageID != 0 && k.statusProv != nil {
			card, err := k.statusProv.GetStatusCard(ctx)
			if err != nil {
				card = fmt.Sprintf("⚠️ Failed to refresh status: %v", err)
			}
			return k.editMessage(ctx, chatID, messageID, card, MakeStatusKeyboard())
		}

	case q.Data == "torch_toggle":
		var toast string
		if k.torch != nil {
			msg, err := k.torch.Toggle(ctx)
			if err != nil {
				toast = fmt.Sprintf("❌ Torch error: %v", err)
			} else {
				if k.torch.IsOn() {
					toast = "🔦 Torch turned ON (3m safety)"
				} else {
					toast = "🔦 Torch turned OFF"
				}
			}
			_ = k.answerCallback(ctx, q.ID, toast, false)

			// If the callback came from a status card, refresh it in-place
			if chatID != 0 && messageID != 0 && k.statusProv != nil {
				card, err := k.statusProv.GetStatusCard(ctx)
				if err == nil {
					_ = k.editMessage(ctx, chatID, messageID, card, MakeStatusKeyboard())
				}
			}
			_ = msg
		} else {
			_ = k.answerCallback(ctx, q.ID, "Hardware torch not configured", true)
		}

	case q.Data == "ping_check":
		uptimeStr := "OK"
		if k.statusProv != nil {
			uptimeStr = k.statusProv.GetUptimeString()
		}
		toast := fmt.Sprintf("🏓 Pong! (Uptime: %s, Server: OK)", uptimeStr)
		_ = k.answerCallback(ctx, q.ID, toast, false)

	default:
		_ = k.answerCallback(ctx, q.ID, "Unknown action", false)
	}

	return nil
}
