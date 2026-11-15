package bot

import (
	"sync"
	"testing"
)

type mockSecurityLogger struct {
	mu      sync.Mutex
	records []struct {
		level    string
		message  string
		metadata string
	}
}

func (m *mockSecurityLogger) RecordLog(level, message, metadata string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, struct {
		level    string
		message  string
		metadata string
	}{level, message, metadata})
	return nil
}

func TestAuth(t *testing.T) {
	logger := &mockSecurityLogger{}
	auth := NewAuthorizer("987654321", logger)

	// Case 1: Authorized Message
	authMsgUpdate := Update{
		UpdateID: 1,
		Message: &Message{
			MessageID: 10,
			Chat:      &Chat{ID: 987654321},
			From:      &User{ID: 987654321, Username: "owner"},
			Text:      "/status",
		},
	}
	if !auth.IsAuthorized(authMsgUpdate) {
		t.Error("Expected authorized update to pass")
	}
	if len(logger.records) != 0 {
		t.Errorf("Expected 0 security logs for authorized update, got %d", len(logger.records))
	}

	// Case 2: Unauthorized Message
	unauthMsgUpdate := Update{
		UpdateID: 2,
		Message: &Message{
			MessageID: 11,
			Chat:      &Chat{ID: 111222333},
			From:      &User{ID: 111222333, Username: "attacker"},
			Text:      "/exec rm -rf /",
		},
	}
	if auth.IsAuthorized(unauthMsgUpdate) {
		t.Error("Expected unauthorized update to be rejected")
	}
	if len(logger.records) != 1 {
		t.Fatalf("Expected 1 security log for unauthorized attempt, got %d", len(logger.records))
	}
	rec := logger.records[0]
	if rec.level != "warn" || rec.message != "Unauthorized Telegram access attempt" {
		t.Errorf("Unexpected log content: level=%s, msg=%s", rec.level, rec.message)
	}

	// Case 3: Authorized CallbackQuery
	authCallbackUpdate := Update{
		UpdateID: 3,
		CallbackQuery: &CallbackQuery{
			ID:   "cb1",
			From: &User{ID: 987654321, Username: "owner"},
			Message: &Message{
				MessageID: 12,
				Chat:      &Chat{ID: 987654321},
			},
			Data: "status_refresh",
		},
	}
	if !auth.IsAuthorized(authCallbackUpdate) {
		t.Error("Expected authorized callback to pass")
	}

	// Case 4: Unauthorized CallbackQuery
	unauthCallbackUpdate := Update{
		UpdateID: 4,
		CallbackQuery: &CallbackQuery{
			ID:   "cb2",
			From: &User{ID: 666666, Username: "malicious"},
			Message: &Message{
				MessageID: 13,
				Chat:      &Chat{ID: 666666},
			},
			Data: "torch_toggle",
		},
	}
	if auth.IsAuthorized(unauthCallbackUpdate) {
		t.Error("Expected unauthorized callback to be rejected")
	}
	if len(logger.records) != 2 {
		t.Errorf("Expected 2 security logs total, got %d", len(logger.records))
	}

	// Case 5: Unconfigured chat ID
	unconfiguredAuth := NewAuthorizer("", logger)
	if unconfiguredAuth.IsAuthorized(authMsgUpdate) {
		t.Error("Expected rejection when TELEGRAM_CHAT_ID is empty")
	}

	// Case 6: Unauthorized clicker on authorized chat's message
	spoofedCallbackUpdate := Update{
		UpdateID: 5,
		CallbackQuery: &CallbackQuery{
			ID:   "cb3",
			From: &User{ID: 666666, Username: "attacker"},
			Message: &Message{
				MessageID: 14,
				Chat:      &Chat{ID: 987654321},
			},
			Data: "torch_toggle",
		},
	}
	if auth.IsAuthorized(spoofedCallbackUpdate) {
		t.Error("Expected unauthorized user clicking on authorized message to be rejected")
	}

	// Case 7: Whitelist configured with whitespace
	whitespaceAuth := NewAuthorizer("  987654321 \n", logger)
	if !whitespaceAuth.IsAuthorized(authMsgUpdate) {
		t.Error("Expected authorizer to trim whitespace and authorize valid user")
	}
}
