package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MonitorEvidence contains bounded, non-secret health measurements for analysis.
type MonitorEvidence struct {
	Status           string                 `json:"status"`
	HostRAMUsedMB    float64                `json:"host_ram_used_mb"`
	HostRAMTotalMB   float64                `json:"host_ram_total_mb"`
	GoRAMMB          float64                `json:"go_ram_mb"`
	DiskFreeGB       float64                `json:"disk_free_gb"`
	DiskTotalGB      float64                `json:"disk_total_gb"`
	BatteryPct       float64                `json:"battery_pct"`
	BatteryAvailable bool                   `json:"battery_available"`
	UptimeSeconds    int64                  `json:"uptime_seconds"`
	Conditions       []string               `json:"conditions"`
	Trend            []MonitorTrendPoint    `json:"trend"`
	Watchtower       MonitorWatchtowerState `json:"watchtower"`
}

type MonitorTrendPoint struct {
	GoRAMMB          float64 `json:"go_ram_mb"`
	DiskFreeGB       float64 `json:"disk_free_gb"`
	BatteryPct       float64 `json:"battery_pct"`
	BatteryAvailable bool    `json:"battery_available"`
}

type MonitorWatchtowerState struct {
	Total  int `json:"total"`
	Up     int `json:"up"`
	Down   int `json:"down"`
	Paused int `json:"paused"`
}

type MonitorInsight struct {
	Summary         string   `json:"summary"`
	Observations    []string `json:"observations"`
	NextChecks      []string `json:"next_checks"`
	Fallback        bool     `json:"fallback,omitempty"`
	FallbackMessage string   `json:"fallback_message,omitempty"`
}

type monitorAIResponse struct {
	Summary      string   `json:"summary"`
	Observations []string `json:"observations"`
	NextChecks   []string `json:"next_checks"`
}

// SummarizeMonitorInsight explains provided measurements without making system state decisions.
func SummarizeMonitorInsight(ctx context.Context, client *Client, evidence MonitorEvidence) *MonitorInsight {
	fallback := fallbackMonitorInsight(evidence)
	if client == nil || !client.HasAPIKey() {
		fallback.FallbackMessage = "ยังไม่ได้ตั้งค่า OpenRouter API key จึงแสดง insight จากค่าระบบโดยตรง"
		return fallback
	}

	systemPrompt := `คุณเป็นผู้ช่วย SRE ของ Pocket Sentinel ให้สรุปข้อมูลสุขภาพระบบเป็นภาษาไทยอย่างกระชับ
แยกข้อสังเกตที่ยืนยันได้จากข้อมูลออกจากรายการตรวจสอบถัดไป ห้ามตัดสินสถานะระบบใหม่หรือเปลี่ยนผลจากระบบ deterministic ห้ามแต่งข้อมูลที่ไม่มีในหลักฐาน ถ้าข้อมูลไม่พอให้บอกตามตรง
ตอบเป็น JSON เท่านั้น ตาม schema: {"summary":"...","observations":["..."],"next_checks":["..."]}`
	data, err := json.Marshal(evidence)
	if err != nil {
		fallback.FallbackMessage = "ไม่สามารถเตรียมข้อมูลสำหรับ AI ได้"
		return fallback
	}
	raw, err := client.Generate(ctx, systemPrompt, "หลักฐาน telemetry แบบจำกัด:\n"+string(data))
	if err != nil {
		fallback.FallbackMessage = "AI ใช้งานไม่ได้ชั่วคราว จึงแสดง insight จากค่าระบบโดยตรง"
		return fallback
	}
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimSuffix(cleaned, "```")
	var response monitorAIResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &response); err != nil || strings.TrimSpace(response.Summary) == "" {
		fallback.FallbackMessage = "AI ส่งผลลัพธ์ที่อ่านไม่ได้ จึงแสดง insight จากค่าระบบโดยตรง"
		return fallback
	}
	response.Summary = strings.TrimSpace(response.Summary)
	if runes := []rune(response.Summary); len(runes) > 300 {
		response.Summary = string(runes[:300])
	}
	return &MonitorInsight{Summary: response.Summary, Observations: limitInsightLines(response.Observations), NextChecks: limitInsightLines(response.NextChecks)}
}

func fallbackMonitorInsight(e MonitorEvidence) *MonitorInsight {
	observations := make([]string, 0, 4)
	if e.HostRAMTotalMB > 0 {
		pct := e.HostRAMUsedMB / e.HostRAMTotalMB * 100
		observations = append(observations, fmt.Sprintf("RAM ของอุปกรณ์ใช้ %.0f%%", pct))
	}
	if e.DiskTotalGB > 0 {
		observations = append(observations, fmt.Sprintf("พื้นที่ว่าง %.1f GB จากทั้งหมด %.1f GB", e.DiskFreeGB, e.DiskTotalGB))
	}
	if e.BatteryAvailable {
		observations = append(observations, fmt.Sprintf("แบตเตอรี่ %.0f%%", e.BatteryPct))
	}
	if len(e.Conditions) > 0 {
		observations = append(observations, fmt.Sprintf("มีเงื่อนไขแจ้งเตือนที่ระบบตรวจพบ %d รายการ", len(e.Conditions)))
	}
	checks := []string{}
	if e.Watchtower.Down > 0 {
		checks = append(checks, fmt.Sprintf("ตรวจสอบ Watchtower ที่ Down จำนวน %d รายการ", e.Watchtower.Down))
	}
	if len(e.Conditions) > 0 {
		checks = append(checks, "ตรวจดูรายละเอียดเงื่อนไขแจ้งเตือนและแนวโน้ม telemetry ล่าสุด")
	}
	if len(checks) == 0 {
		checks = append(checks, "ติดตามค่า telemetry ต่อเนื่องเพื่อสังเกตการเปลี่ยนแปลง")
	}
	if len(observations) == 0 {
		observations = append(observations, "ยังมีข้อมูล telemetry ไม่เพียงพอสำหรับสรุปแนวโน้ม")
	}
	return &MonitorInsight{Summary: "สถานะระบบ: " + e.Status, Observations: observations, NextChecks: checks, Fallback: true}
}

func limitInsightLines(lines []string) []string {
	if len(lines) > 5 {
		lines = lines[:5]
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && len([]rune(line)) <= 240 {
			result = append(result, line)
		}
	}
	return result
}
