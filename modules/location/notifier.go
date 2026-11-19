package location

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

var bangkokZone = time.FixedZone("ICT", 7*60*60)

type DiscordNotifier struct {
	webhookURL string
	client     *http.Client
}

type DiscordOption func(*DiscordNotifier)

func WithDiscordHTTPClient(client *http.Client) DiscordOption {
	return func(notifier *DiscordNotifier) {
		if client != nil {
			notifier.client = client
		}
	}
}

func NewDiscordNotifier(webhookURL string, timeout time.Duration, options ...DiscordOption) MessageNotifier {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return noopMessageNotifier{}
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	notifier := &DiscordNotifier{webhookURL: webhookURL, client: &http.Client{Timeout: timeout}}
	for _, option := range options {
		option(notifier)
	}
	return notifier
}

type discordMessagePayload struct {
	Content string `json:"content"`
}

type noopMessageNotifier struct{}

func (noopMessageNotifier) Send(context.Context, string) error { return nil }

func (d *DiscordNotifier) Send(ctx context.Context, message string) error {
	message = truncateDiscordMessage(message, 1900)
	body, err := json.Marshal(discordMessagePayload{Content: message})
	if err != nil {
		return fmt.Errorf("marshal location Discord message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create location Discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send location Discord message: %s", redactWebhook(err.Error(), d.webhookURL))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("location Discord returned %d: %s", response.StatusCode, redactWebhook(string(data), d.webhookURL))
	}
	return nil
}

func truncateDiscordMessage(message string, maxRunes int) string {
	if utf8.RuneCountInString(message) <= maxRunes {
		return message
	}
	runes := []rune(message)
	return string(runes[:maxRunes-1]) + "…"
}

func redactWebhook(value, webhookURL string) string {
	parts := strings.Split(webhookURL, "/")
	if len(parts) > 1 {
		token := parts[len(parts)-1]
		if len(token) > 4 {
			value = strings.ReplaceAll(value, token, "DISCORD_TOKEN_REDACTED")
		}
	}
	return strings.ReplaceAll(value, webhookURL, "https://discord.com/api/webhooks/...REDACTED...")
}

// FormatDiscordEvent renders a location event as compact, mention-safe Discord Markdown.
func FormatDiscordEvent(event NotificationEvent) string {
	timestamp := event.OccurredAt.In(bangkokZone).Format("15:04:05 ICT")
	switch event.Kind {
	case EventMeet:
		me := event.Point
		friend := LocationPoint{}
		if event.Partner != nil {
			friend = *event.Partner
		}
		header := "🤝 **Chubu และ Furturia อยู่ที่นี่ด้วยกันแล้ว**"
		if event.HasGeofence {
			header = fmt.Sprintf("🤝 **Chubu และ Furturia อยู่ที่ %s ด้วยกันแล้ว**", escapeDiscordText(event.Geofence.Name))
		}
		return fmt.Sprintf("%s\n"+
			"• **ระยะห่าง:** `%s`\n"+
			"• **พิกัด:** `%s`\n"+
			"• **เวลา:** %s\n"+
			"• **แบตเตอรี่:** Chubu %s · Furturia %s",
			header, FormatDistance(event.DistanceMeters), midpointCoordinates(me, friend), timestamp,
			formatBattery(me), formatBattery(friend))
	case EventSeparate:
		return fmt.Sprintf("👋 **Chubu และ Furturia แยกย้ายกันแล้ว**\n"+
			"• **ระยะห่างล่าสุด:** `%s`\n"+
			"• **เวลา:** %s", FormatDistance(event.DistanceMeters), timestamp)
	case EventGeofenceEnter:
		name := pureDeviceName(event.DeviceID)
		return fmt.Sprintf("📍 **%s มาถึง %s แล้ว**\n"+
			"• **เวลา:** %s\n"+
			"• **แบตเตอรี่:** %s",
			name, escapeDiscordText(event.Geofence.Name), timestamp, formatBattery(event.Point))
	case EventGeofenceExit:
		name := pureDeviceName(event.DeviceID)
		return fmt.Sprintf("🚗 **%s ออกจาก %s แล้ว**\n"+
			"• **ความเร็ว:** %.0f km/h\n"+
			"• **เวลา:** %s\n"+
			"• **แบตเตอรี่:** %s",
			name, escapeDiscordText(event.Geofence.Name), event.Point.Speed, timestamp, formatBattery(event.Point))
	case EventBatteryCritical:
		name := pureDeviceName(event.DeviceID)
		return fmt.Sprintf("🪫 **แบตเตอรี่ของ %s ต่ำ (`%d%%`)**\n"+
			"• **สถานะ:** %s\n"+
			"• **เวลา:** %s",
			name, event.Point.Battery, chargingStatus(event.Point.BatteryStatus), timestamp)
	default:
		return "📍 **Pocket Sentinel:** มีเหตุการณ์ตำแหน่งใหม่"
	}
}

func meetingPlace(event NotificationEvent) string {
	if event.HasGeofence {
		return fmt.Sprintf("%s (%s)", escapeDiscordText(event.Geofence.Name), FormatDistance(event.Geofence.Radius))
	}
	if event.Partner != nil {
		return "พิกัดที่พบกัน"
	}
	return "พิกัด"
}

func midpointCoordinates(me, friend LocationPoint) string {
	return fmt.Sprintf("%.4f, %.4f", (me.Latitude+friend.Latitude)/2, (me.Longitude+friend.Longitude)/2)
}

func pureDeviceName(device string) string {
	if device == DeviceMe {
		return "Chubu"
	}
	return "Furturia"
}

func deviceName(device string) string {
	if device == DeviceMe {
		return "👤 Chubu"
	}
	return "👥 Furturia"
}

func formatBattery(point LocationPoint) string {
	if point.Battery < 0 {
		return "ไม่ทราบ"
	}
	value := fmt.Sprintf("%d%%", point.Battery)
	if point.BatteryStatus == 2 || point.BatteryStatus == 3 {
		return value + " ⚡"
	}
	return value
}

func chargingStatus(status int) string {
	switch status {
	case 2:
		return "กำลังชาร์จ"
	case 3:
		return "ชาร์จเต็ม"
	case 1:
		return "ยังไม่ได้เสียบชาร์จ"
	default:
		return "ไม่ทราบสถานะการชาร์จ"
	}
}

func escapeDiscordText(value string) string {
	value = strings.ReplaceAll(value, "@", "@\u200b")
	var out strings.Builder
	out.Grow(len(value) + 8)
	for _, r := range value {
		switch r {
		case '\\', '*', '_', '~', '`', '|', '>', '<', '[', ']', '(', ')':
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}
