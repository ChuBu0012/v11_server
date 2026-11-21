package reminders

import (
	"context"
	"strings"
	"testing"
	"time"

	"v11-server/modules/bot"
)

func TestFormatThaiDateTime(t *testing.T) {
	loc := location()

	// 1. Wednesday 21 Nov 2026, 14:30
	t1 := time.Date(2026, time.November, 21, 14, 30, 0, 0, loc)
	got1 := formatThaiDateTime(t1)
	expected1 := "วันเสาร์ 21 พ.ย. 14:30" // 2026-11-21 is Saturday!
	if got1 != expected1 {
		t.Errorf("expected %q, got %q", expected1, got1)
	}

	// 2. Monday 2 Jan 2026, 09:05
	// Check weekday of 2026-01-02: 2026-01-01 is Thursday, 2026-01-02 is Friday
	t2 := time.Date(2026, time.January, 5, 9, 5, 0, 0, loc) // 2026-01-05 is Monday
	got2 := formatThaiDateTime(t2)
	expected2 := "วันจันทร์ 5 ม.ค. 09:05"
	if got2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, got2)
	}
}

func TestFormatMessage(t *testing.T) {
	loc := location()
	event := time.Date(2026, time.November, 21, 18, 0, 0, 0, loc)

	d := Delivery{
		ID:         1,
		ReminderID: 10,
		Title:      "กินยา & พักผ่อน",
		Details:    "ยาแก้แพ้ 1 เม็ดหลังอาหาร <ห้ามลืม>",
		EventAt:    event,
		Offset:     0,
		Kind:       "notice",
	}

	msg := formatMessage(d)

	if !strings.Contains(msg, "⏰ <b>กินยา &amp; พักผ่อน</b>") {
		t.Errorf("expected escaped title in bold, got: %s", msg)
	}
	if !strings.Contains(msg, "วันเสาร์ 21 พ.ย. 18:00 · ถึงเวลาแล้ว") {
		t.Errorf("expected correct Thai date and timing, got: %s", msg)
	}
	if !strings.Contains(msg, "&lt;ห้ามลืม&gt;") {
		t.Errorf("expected HTML-escaped details, got: %s", msg)
	}

	// Test offset
	d.Offset = 15
	msgOffset := formatMessage(d)
	if !strings.Contains(msgOffset, "อีก 15 นาที") {
		t.Errorf("expected 'อีก 15 นาที', got: %s", msgOffset)
	}

	// Test snooze
	d.Offset = 0
	d.Kind = "snooze"
	msgSnooze := formatMessage(d)
	if !strings.Contains(msgSnooze, "เตือนซ้ำหลังเลื่อน") {
		t.Errorf("expected 'เตือนซ้ำหลังเลื่อน', got: %s", msgSnooze)
	}
}

type mockTelegram struct {
	sentMessages []string
	callbacks    []string
}

func (m *mockTelegram) SendMessageSilent(ctx context.Context, chatID int64, text string, markup *bot.InlineKeyboardMarkup) (*bot.Message, error) {
	m.sentMessages = append(m.sentMessages, text)
	return &bot.Message{MessageID: int64(len(m.sentMessages))}, nil
}

func (m *mockTelegram) AnswerCallbackQuery(ctx context.Context, callbackID, text string, showAlert bool) error {
	m.callbacks = append(m.callbacks, text)
	return nil
}

func TestService_HandleCallback(t *testing.T) {
	mock := &mockTelegram{}
	svc := NewService(nil, mock, 123456)

	// Invalid callback format
	err := svc.HandleCallback(context.Background(), &bot.CallbackQuery{ID: "cb1", Data: "invalid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.callbacks) == 0 || mock.callbacks[0] != "ไม่พบรายการนี้" {
		t.Errorf("expected 'ไม่พบรายการนี้', got: %v", mock.callbacks)
	}
}
