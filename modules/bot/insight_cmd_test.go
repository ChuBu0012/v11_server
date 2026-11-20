package bot

import (
	"context"
	"strings"
	"testing"
)

func TestRouter_Insight(t *testing.T) {
	sender := &mockDirectSender{}
	r := NewRouter(nil, NewAuthorizer("123456", nil), nil, nil, nil, WithTelegramSender(sender),
		WithInsightFunc(func(ctx context.Context) (string, error) { return "☀️ <b>สรุปเช้า</b>", nil }))
	sendCommand(t, r, 123456, "/insight")
	if len(sender.sent) != 2 || !strings.Contains(sender.sent[0], "กำลังสรุป") || !strings.Contains(sender.sent[1], "สรุปเช้า") {
		t.Fatalf("replies = %#v", sender.sent)
	}
}

func TestRouter_HelpListsInsight(t *testing.T) {
	sender := &mockDirectSender{}
	r := NewRouter(nil, NewAuthorizer("123456", nil), nil, nil, nil, WithTelegramSender(sender))
	sendCommand(t, r, 123456, "/help")
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0], "/insight") || !strings.Contains(sender.sent[0], "/deploy") {
		t.Fatalf("help = %#v", sender.sent)
	}
	found := false
	for _, row := range MakeHelpKeyboard().InlineKeyboard {
		for _, b := range row {
			if b.CopyText != nil && b.CopyText.Text == "/insight" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("help keyboard has no /insight button")
	}
}
