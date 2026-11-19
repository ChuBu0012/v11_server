package main

import (
	"strings"
	"testing"
	"time"

	"v11-server/modules/location"
)

func TestFormatTelegramLocationStatus(t *testing.T) {
	now := time.Now().UTC()
	me := &location.LocationPoint{
		DeviceID:      location.DeviceMe,
		Latitude:      13.6515,
		Longitude:     100.4939,
		Battery:       80,
		BatteryStatus: 2,
		Speed:         5,
		Accuracy:      10,
		RecordedAt:    now,
	}
	friend := &location.LocationPoint{
		DeviceID:      location.DeviceFriend,
		Latitude:      13.6516,
		Longitude:     100.4940,
		Battery:       65,
		BatteryStatus: 1,
		Speed:         0,
		Accuracy:      8,
		RecordedAt:    now,
	}

	msg, kb, err := formatTelegramLocationStatus(me, friend, "https://example.com/live/token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"📍 <b>Live Map: Chubu & Furturia</b>", "Chubu & Furturia อยู่ที่นี่ด้วยกันแล้ว", "Chubu:</b> 🔋 80% ⚡", "Furturia:</b> 🔋 65%"} {
		if !strings.Contains(msg, want) {
			t.Errorf("msg missing %q:\n%s", want, msg)
		}
	}

	if kb == nil || len(kb.InlineKeyboard) != 1 || kb.InlineKeyboard[0][0].URL != "https://example.com/live/token" {
		t.Errorf("invalid inline keyboard: %+v", kb)
	}
}

func TestFormatTelegramLocationStatusNil(t *testing.T) {
	msg, kb, err := formatTelegramLocationStatus(nil, nil, "https://example.com/live/token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "ยังไม่ได้รับข้อมูลพิกัดล่าสุด") {
		t.Errorf("expected missing message, got: %s", msg)
	}
	if kb == nil || len(kb.InlineKeyboard) != 1 {
		t.Errorf("invalid inline keyboard: %+v", kb)
	}
}
