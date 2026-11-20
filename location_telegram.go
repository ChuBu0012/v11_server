package main

import (
	"fmt"
	"time"

	"v11-server/modules/bot"
	"v11-server/modules/location"
)

func formatTelegramLocationStatus(me, friend *location.LocationPoint, liveURL string) (string, *bot.InlineKeyboardMarkup, error) {
	if me == nil && friend == nil {
		msg := "📍 <b>Live Map: Chubu & Furturia</b>\n" +
			"<i>ยังไม่ได้รับข้อมูลพิกัดล่าสุดจากอุปกรณ์ทั้งสองเครื่อง</i>"
		kb := &bot.InlineKeyboardMarkup{
			InlineKeyboard: [][]bot.InlineKeyboardButton{
				{{Text: "🗺️ เปิดแผนที่สด (Live Map)", URL: liveURL}},
			},
		}
		return msg, kb, nil
	}

	var statusHeader string
	if me != nil && friend != nil {
		dist := location.DistanceMeters(me.Latitude, me.Longitude, friend.Latitude, friend.Longitude)
		if dist <= 80 {
			statusHeader = fmt.Sprintf("🟢 <b>อยู่ด้วยกัน</b> (ห่าง %s)", location.FormatDistance(dist))
		} else {
			statusHeader = fmt.Sprintf("🔵 <b>ห่างกัน %s</b>", location.FormatDistance(dist))
		}
	} else {
		statusHeader = "🟡 <b>ได้รับพิกัดเพียง 1 เครื่อง</b>"
	}

	formatDeviceLine := func(name string, p *location.LocationPoint) string {
		if p == nil {
			return fmt.Sprintf("• <b>%s:</b> <i>ไม่มีข้อมูล</i>", name)
		}
		battery := "ไม่ทราบ"
		if p.Battery >= 0 {
			charging := ""
			if p.BatteryStatus == 2 || p.BatteryStatus == 3 {
				charging = " ⚡"
			}
			battery = fmt.Sprintf("%d%%%s", p.Battery, charging)
		}
		speed := "หยุดนิ่ง"
		if p.Speed >= 1 {
			speed = fmt.Sprintf("%.0f km/h", p.Speed)
		}
		age := relativeTimeAgo(p.RecordedAt)
		return fmt.Sprintf("• <b>%s:</b> 🔋 %s · %s · %s", name, battery, speed, age)
	}

	msg := fmt.Sprintf("📍 <b>Live Map</b> · %s\n\n%s\n%s",
		statusHeader,
		formatDeviceLine("Chubu", me),
		formatDeviceLine("Furturia", friend),
	)

	kb := &bot.InlineKeyboardMarkup{
		InlineKeyboard: [][]bot.InlineKeyboardButton{
			{{Text: "🗺️ เปิดแผนที่สด", URL: liveURL}},
		},
	}
	return msg, kb, nil
}

func relativeTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "ไม่ทราบเวลา"
	}
	diff := time.Since(t)
	if diff < 0 {
		diff = 0
	}
	secs := int(diff.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%d วินาทีที่แล้ว", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%d นาทีที่แล้ว", mins)
	}
	hours := mins / 60
	if hours < 24 {
		return fmt.Sprintf("%d ชั่วโมงที่แล้ว", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%d วันที่แล้ว", days)
}
