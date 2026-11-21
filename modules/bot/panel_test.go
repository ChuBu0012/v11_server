package bot

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMakeReplyKeyboard(t *testing.T) {
	kb := MakeDefaultReplyKeyboard()
	if kb == nil {
		t.Fatal("Expected MakeDefaultReplyKeyboard to return non-nil")
	}
	if !kb.ResizeKeyboard || !kb.IsPersistent {
		t.Errorf("Expected ResizeKeyboard and IsPersistent to be true")
	}
	if len(kb.Keyboard) != 3 {
		t.Fatalf("Expected 3 rows in reply keyboard, got %d", len(kb.Keyboard))
	}
	if len(kb.Keyboard[0]) != 2 || kb.Keyboard[0][0].Text != "🎛️ แผงควบคุม" {
		t.Errorf("Expected first button to be '🎛️ แผงควบคุม', got %q", kb.Keyboard[0][0].Text)
	}

	rm := MakeRemoveReplyKeyboard()
	if rm == nil || !rm.RemoveKeyboard {
		t.Fatal("Expected MakeRemoveReplyKeyboard to have RemoveKeyboard=true")
	}
}

func TestMakePanelKeyboard(t *testing.T) {
	kb := MakePanelKeyboard("system")
	if kb == nil || len(kb.InlineKeyboard) < 2 {
		t.Fatal("Expected panel keyboard to have at least 2 rows (tabs + actions)")
	}

	// Verify Tab row has 4 items and 'system' is highlighted
	tabRow := kb.InlineKeyboard[0]
	if len(tabRow) != 4 {
		t.Fatalf("Expected 4 tabs, got %d", len(tabRow))
	}
	if !strings.Contains(tabRow[0].Text, "•") {
		t.Errorf("Expected active system tab to have bullet indicator, got %q", tabRow[0].Text)
	}
	if strings.Contains(tabRow[1].Text, "•") {
		t.Errorf("Expected inactive services tab to not have bullet, got %q", tabRow[1].Text)
	}

	// Switch to services
	kbServices := MakePanelKeyboard("services")
	if !strings.Contains(kbServices.InlineKeyboard[0][1].Text, "•") {
		t.Errorf("Expected services tab to be highlighted, got %q", kbServices.InlineKeyboard[0][1].Text)
	}
}

func TestDefaultStatusProvider_GetPanelCard(t *testing.T) {
	startTime := time.Now().Add(-2 * time.Hour)
	p := NewDefaultStatusProvider(startTime)

	p.SetDigestSummaryFunc(func(ctx context.Context) (string, error) {
		return "🥇 ทองคำ: 57,600 บาท", nil
	})
	p.SetLocationSummaryFunc(func(ctx context.Context) (string, error) {
		return "📍 สถานะ: ออนไลน์ (บ้าน)", nil
	})

	ctx := context.Background()

	// 1. System tab
	sysCard, err := p.GetPanelCard(ctx, "system")
	if err != nil {
		t.Fatalf("GetPanelCard(system) failed: %v", err)
	}
	if !strings.Contains(sysCard, "Pocket Sentinel · ควบคุมระบบ") || !strings.Contains(sysCard, "แบตเตอรี่") {
		t.Errorf("Unexpected system card content: %s", sysCard)
	}

	// 2. Services tab
	srvCard, err := p.GetPanelCard(ctx, "services")
	if err != nil {
		t.Fatalf("GetPanelCard(services) failed: %v", err)
	}
	if !strings.Contains(srvCard, "Pocket Sentinel · สถานะบริการ") || !strings.Contains(srvCard, "Gatus") {
		t.Errorf("Unexpected services card content: %s", srvCard)
	}

	// 3. Digest tab
	digCard, err := p.GetPanelCard(ctx, "digest")
	if err != nil {
		t.Fatalf("GetPanelCard(digest) failed: %v", err)
	}
	if !strings.Contains(digCard, "Pocket Sentinel · ข้อมูลสรุปและตลาด") || !strings.Contains(digCard, "57,600") {
		t.Errorf("Unexpected digest card content: %s", digCard)
	}

	// 4. Location tab
	locCard, err := p.GetPanelCard(ctx, "location")
	if err != nil {
		t.Fatalf("GetPanelCard(location) failed: %v", err)
	}
	if !strings.Contains(locCard, "Pocket Sentinel · ตำแหน่งและความปลอดภัย") || !strings.Contains(locCard, "ออนไลน์ (บ้าน)") {
		t.Errorf("Unexpected location card content: %s", locCard)
	}
}

type mockPanelSender struct {
	mu     sync.Mutex
	sent   []string
	edited []string
}

func (m *mockPanelSender) SendMessage(ctx context.Context, chatID int64, text string, replyMarkup *InlineKeyboardMarkup) (*Message, error) {
	m.mu.Lock()
	m.sent = append(m.sent, text)
	m.mu.Unlock()
	return &Message{MessageID: 10, Text: text}, nil
}

func (m *mockPanelSender) SendMessageSilent(ctx context.Context, chatID int64, text string, replyMarkup *InlineKeyboardMarkup) (*Message, error) {
	return m.SendMessage(ctx, chatID, text, replyMarkup)
}

func (m *mockPanelSender) EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error {
	m.mu.Lock()
	m.edited = append(m.edited, text)
	m.mu.Unlock()
	return nil
}

func (m *mockPanelSender) SendMessageWithReplyMarkup(ctx context.Context, chatID int64, text string, replyMarkup any) (*Message, error) {
	m.mu.Lock()
	m.sent = append(m.sent, text)
	m.mu.Unlock()
	return &Message{MessageID: 10, Text: text}, nil
}

func TestRouter_PanelRouting(t *testing.T) {
	sender := &mockPanelSender{}
	auth := NewAuthorizer("123456", nil)
	p := NewDefaultStatusProvider(time.Now())
	router := NewRouter(nil, auth, nil, nil, p, WithTelegramSender(sender))

	ctx := context.Background()

	// Test /panel command
	update := Update{
		UpdateID: 1,
		Message: &Message{
			MessageID: 10,
			Chat:      &Chat{ID: 123456},
			Text:      "/panel",
		},
	}
	if err := router.HandleUpdate(ctx, update); err != nil {
		t.Fatalf("HandleUpdate /panel failed: %v", err)
	}
	sender.mu.Lock()
	lastSent := ""
	if len(sender.sent) > 0 {
		lastSent = sender.sent[len(sender.sent)-1]
	}
	sender.mu.Unlock()

	if !strings.Contains(lastSent, "Pocket Sentinel · ควบคุมระบบ") {
		t.Errorf("Expected panel card, got: %s", lastSent)
	}

	// Test reply keyboard text mapping: '🎛️ แผงควบคุม'
	updateBtn := Update{
		UpdateID: 2,
		Message: &Message{
			MessageID: 11,
			Chat:      &Chat{ID: 123456},
			Text:      "🎛️ แผงควบคุม",
		},
	}
	if err := router.HandleUpdate(ctx, updateBtn); err != nil {
		t.Fatalf("HandleUpdate '🎛️ แผงควบคุม' failed: %v", err)
	}
	sender.mu.Lock()
	lastSent = sender.sent[len(sender.sent)-1]
	sender.mu.Unlock()

	if !strings.Contains(lastSent, "Pocket Sentinel · ควบคุมระบบ") {
		t.Errorf("Expected button '🎛️ แผงควบคุม' to trigger panel, got: %s", lastSent)
	}

	// Test callback tab switch to 'services'
	cbUpdate := Update{
		UpdateID: 3,
		CallbackQuery: &CallbackQuery{
			ID:   "cb1",
			From: &User{ID: 123456, Username: "owner"},
			Message: &Message{
				MessageID: 10,
				Chat:      &Chat{ID: 123456},
			},
			Data: "panel:tab:services",
		},
	}
	if err := router.HandleUpdate(ctx, cbUpdate); err != nil {
		t.Fatalf("HandleUpdate callback failed: %v", err)
	}
	sender.mu.Lock()
	lastEdited := ""
	if len(sender.edited) > 0 {
		lastEdited = sender.edited[len(sender.edited)-1]
	}
	sender.mu.Unlock()

	if !strings.Contains(lastEdited, "Pocket Sentinel · สถานะบริการ") {
		t.Errorf("Expected services tab content in edited messages, got: %s", lastEdited)
	}
}
