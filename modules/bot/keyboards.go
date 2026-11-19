package bot

import (
	"context"
	"fmt"
	"log"
)

// StatusCardProvider supplies real-time status card HTML and uptime.
type StatusCardProvider interface {
	GetStatusCard(ctx context.Context) (string, error)
	GetUptimeString() string
}

// KeyboardsModule manages inline keyboards and callback query routing.
type KeyboardsModule struct {
	client     *Client
	statusProv StatusCardProvider
	torch      *TorchController
}

// NewKeyboardsModule creates an inline keyboard manager.
func NewKeyboardsModule(client *Client, statusProv StatusCardProvider, torch *TorchController) *KeyboardsModule {
	return &KeyboardsModule{
		client:     client,
		statusProv: statusProv,
		torch:      torch,
	}
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
			{{Text: "🔄 สถานะ", CallbackData: "status_refresh"}, {Text: "🏓 Ping", CallbackData: "ping_check"}},
			{{Text: "🔦 ไฟฉาย", CallbackData: "torch_toggle"}, copyCommand("🗺️ ดูพิกัด /live", "/live")},
			{
				copyCommand("💬 ถาม agy", "/ask [median] อธิบายภาพรวมสถานะระบบ Pocket Sentinel ในปัจจุบัน"),
				copyCommand("📋 เช็คเครื่อง", "/task [low] ตรวจสอบ RAM, พื้นที่ disk และสถานะ PM2 go-server"),
			},
			{
				copyCommand("📋 เช็ค PM2", "/task [medium] ตรวจสอบสถานะ PM2 ของ go-server และ ngrok-tunnel"),
				copyCommand("📋 Task status", "/task_status"),
			},
			{
				copyCommand("📰 Digest", "/digest"),
				copyCommand("💾 Disk", "/exec df -h"),
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

	switch q.Data {
	case "status_refresh":
		// Acknowledge immediately to dismiss Telegram loading spinner
		_ = k.client.AnswerCallbackQuery(ctx, q.ID, "🔄 Updating status...", false)

		if chatID != 0 && messageID != 0 && k.statusProv != nil {
			card, err := k.statusProv.GetStatusCard(ctx)
			if err != nil {
				card = fmt.Sprintf("⚠️ Failed to refresh status: %v", err)
			}
			return k.client.EditMessageText(ctx, chatID, messageID, card, MakeStatusKeyboard())
		}

	case "torch_toggle":
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
			_ = k.client.AnswerCallbackQuery(ctx, q.ID, toast, false)

			// If the callback came from a status card, refresh it in-place
			if chatID != 0 && messageID != 0 && k.statusProv != nil {
				card, err := k.statusProv.GetStatusCard(ctx)
				if err == nil {
					_ = k.client.EditMessageText(ctx, chatID, messageID, card, MakeStatusKeyboard())
				}
			}
			_ = msg
		} else {
			_ = k.client.AnswerCallbackQuery(ctx, q.ID, "Hardware torch not configured", true)
		}

	case "ping_check":
		uptimeStr := "OK"
		if k.statusProv != nil {
			uptimeStr = k.statusProv.GetUptimeString()
		}
		toast := fmt.Sprintf("🏓 Pong! (Uptime: %s, Server: OK)", uptimeStr)
		_ = k.client.AnswerCallbackQuery(ctx, q.ID, toast, false)

	default:
		_ = k.client.AnswerCallbackQuery(ctx, q.ID, "Unknown action", false)
	}

	return nil
}
