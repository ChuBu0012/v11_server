package location

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFormatDiscordEventMeeting(t *testing.T) {
	got := FormatDiscordEvent(NotificationEvent{
		Kind: EventMeet, Point: LocationPoint{DeviceID: DeviceMe, Latitude: 13.7462, Longitude: 100.5348, Battery: 85, BatteryStatus: 2},
		Partner:        &LocationPoint{DeviceID: DeviceFriend, Latitude: 13.7464, Longitude: 100.5349, Battery: 62, BatteryStatus: 1},
		DistanceMeters: 35, OccurredAt: time.Date(2026, 11, 19, 7, 15, 20, 0, time.UTC),
	})
	for _, want := range []string{"🤝 **Chubu และ Furturia อยู่ที่นี่ด้วยกันแล้ว**", "`35 ม.`", "13.7463, 100.5349", "14:15:20 ICT", "Chubu 85% ⚡ · Furturia 62%"} {
		if !strings.Contains(got, want) {
			t.Errorf("meeting message missing %q:\n%s", want, got)
		}
	}
}

type notifierRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip notifierRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestDiscordNotifierPreservesMarkdownAndRedactsWebhookOnError(t *testing.T) {
	const webhook = "https://discord.com/api/webhooks/123/very-secret-token"
	var posted string
	client := &http.Client{Transport: notifierRoundTripper(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		var payload discordMessagePayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		posted = payload.Content
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	notifier := NewDiscordNotifier(webhook, time.Second, WithDiscordHTTPClient(client))
	message := FormatDiscordEvent(NotificationEvent{
		Kind: EventGeofenceEnter, DeviceID: DeviceFriend,
		Point: LocationPoint{Battery: 38}, Geofence: Geofence{Name: "<home> @everyone **alert**", Radius: 100},
		HasGeofence: true, OccurredAt: time.Now(),
	})
	if err := notifier.Send(context.Background(), message); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(posted, `\<home\>`) || strings.Contains(posted, "@everyone") || !strings.Contains(posted, `\*\*alert\*\*`) {
		t.Fatalf("Discord Markdown was not safely preserved: %s", posted)
	}

	client.Transport = notifierRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("failed very-secret-token")), Header: make(http.Header)}, nil
	})
	err := notifier.Send(context.Background(), "message")
	if err == nil || strings.Contains(err.Error(), "very-secret-token") {
		t.Fatalf("webhook failure error must redact secret token: %v", err)
	}
}

func TestDiscordNotifierTruncatesOnRuneBoundary(t *testing.T) {
	message := strings.Repeat("ก", 1901)
	truncated := truncateDiscordMessage(message, 1900)
	if utf8.RuneCountInString(truncated) != 1900 || !utf8.ValidString(truncated) || !strings.HasSuffix(truncated, "…") {
		t.Fatalf("truncation is not valid UTF-8 at 1900 runes")
	}
}

func TestFormatDiscordEventEscapesPlaceMentionsAndMarkdown(t *testing.T) {
	got := FormatDiscordEvent(NotificationEvent{
		Kind: EventGeofenceEnter, DeviceID: DeviceFriend, Point: LocationPoint{Battery: 40},
		Geofence: Geofence{Name: "@everyone **secret**", Radius: 150}, HasGeofence: true,
		OccurredAt: time.Date(2026, 11, 19, 7, 15, 20, 0, time.UTC),
	})
	if strings.Contains(got, "@everyone") {
		t.Fatalf("message contains live Discord mention: %s", got)
	}
	if !strings.Contains(got, "\\*\\*secret\\*\\*") || !strings.Contains(got, "Furturia มาถึง") {
		t.Fatalf("message did not escape user text or name Furturia: %s", got)
	}
	if len(got) >= 1950 {
		t.Fatalf("message is too long: %d chars", len(got))
	}
}

func TestFormatDiscordEventSeparationAndBattery(t *testing.T) {
	separation := FormatDiscordEvent(NotificationEvent{Kind: EventSeparate, DistanceMeters: 310, OccurredAt: time.Now()})
	if !strings.Contains(separation, "👋 **Chubu และ Furturia แยกย้ายกันแล้ว**") || !strings.Contains(separation, "310 ม.") {
		t.Errorf("unexpected separation message: %s", separation)
	}
	battery := FormatDiscordEvent(NotificationEvent{
		Kind: EventBatteryCritical, DeviceID: DeviceFriend,
		Point: LocationPoint{Battery: 12, BatteryStatus: 1}, OccurredAt: time.Now(),
	})
	if !strings.Contains(battery, "🪫 **แบตเตอรี่ของ Furturia ต่ำ") || !strings.Contains(battery, "12%") || !strings.Contains(battery, "ยังไม่ได้เสียบชาร์จ") {
		t.Errorf("unexpected battery message: %s", battery)
	}
}

func TestFormatDiscordEventGeofenceExit(t *testing.T) {
	got := FormatDiscordEvent(NotificationEvent{
		Kind: EventGeofenceExit, DeviceID: DeviceMe,
		Point: LocationPoint{Speed: 42}, Geofence: Geofence{Name: "ที่ทำงาน"},
		OccurredAt: time.Date(2026, 11, 19, 10, 15, 0, 0, time.UTC),
	})
	for _, want := range []string{"🚗 **Chubu ออกจาก ที่ทำงาน แล้ว**", "42 km/h", "17:15:00 ICT"} {
		if !strings.Contains(got, want) {
			t.Errorf("exit message missing %q: %s", want, got)
		}
	}
}
