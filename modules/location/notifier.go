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
	timestamp := event.OccurredAt.In(bangkokZone).Format("15:04")
	switch event.Kind {
	case EventMeet:
		me := event.Point
		friend := LocationPoint{}
		if event.Partner != nil {
			friend = *event.Partner
		}
		header := "🤝 **Chubu & Furturia อยู่ด้วยกันแล้ว**"
		if event.HasGeofence {
			header = fmt.Sprintf("🤝 **Chubu & Furturia อยู่ด้วยกันที่ %s**", escapeDiscordText(event.Geofence.Name))
		}
		var details []string
		details = append(details, fmt.Sprintf("ห่าง %s", FormatDistance(event.DistanceMeters)))
		details = append(details, timestamp)
		if me.Battery >= 0 || friend.Battery >= 0 {
			details = append(details, fmt.Sprintf("🔋 Chubu %s · Furturia %s", formatBattery(me), formatBattery(friend)))
		}
		return fmt.Sprintf("%s\n%s", header, strings.Join(details, " · "))
	case EventSeparate:
		return fmt.Sprintf("👋 **Chubu & Furturia แยกย้ายกัน**\nห่าง %s · %s",
			FormatDistance(event.DistanceMeters), timestamp)
	case EventGeofenceEnter:
		name := pureDeviceName(event.DeviceID)
		var details []string
		details = append(details, timestamp)
		if event.Point.Battery >= 0 {
			details = append(details, "🔋 "+formatBattery(event.Point))
		}
		return fmt.Sprintf("📍 **%s ถึง %s**\n%s",
			name, escapeDiscordText(event.Geofence.Name), strings.Join(details, " · "))
	case EventGeofenceExit:
		name := pureDeviceName(event.DeviceID)
		var details []string
		details = append(details, timestamp)
		if event.Point.Speed >= 5 {
			details = append(details, fmt.Sprintf("%.0f km/h", event.Point.Speed))
		}
		if event.Point.Battery >= 0 {
			details = append(details, "🔋 "+formatBattery(event.Point))
		}
		return fmt.Sprintf("🚗 **%s ออกจาก %s**\n%s",
			name, escapeDiscordText(event.Geofence.Name), strings.Join(details, " · "))
	case EventBatteryCritical:
		name := pureDeviceName(event.DeviceID)
		return fmt.Sprintf("🪫 **แบตเตอรี่ %s เหลือ %d%%**\n%s · %s",
			name, event.Point.Battery, timestamp, chargingStatus(event.Point.BatteryStatus))
	default:
		return "📍 **Pocket Sentinel:** มีเหตุการณ์ตำแหน่งใหม่"
	}
}

func pureDeviceName(device string) string {
	if device == DeviceMe {
		return "Chubu"
	}
	return "Furturia"
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
