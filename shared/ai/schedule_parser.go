package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// DefaultTimezone for Sentinel reminders is Asia/Bangkok (+07:00).
	DefaultTimezone = "Asia/Bangkok"
	bangkokOffset   = 7 * 60 * 60
)

// SchedulePreview matches the reminder input schema for creating reminders.
type SchedulePreview struct {
	Title      string `json:"title"`
	Details    string `json:"details"`
	StartsAt   string `json:"starts_at"`  // RFC3339 in Asia/Bangkok
	Recurrence string `json:"recurrence"` // "once" or "weekly"
	Offsets    []int  `json:"offsets"`    // [0, 5, 10, 30]
}

// ScheduleParseResult is returned by ParseSchedulePrompt.
type ScheduleParseResult struct {
	NeedsClarification bool             `json:"needs_clarification"`
	Question           string           `json:"question,omitempty"`
	Preview            *SchedulePreview `json:"preview,omitempty"`
	Fallback           bool             `json:"fallback,omitempty"`
	Error              string           `json:"error,omitempty"`
}

// ScheduleAIResponse represents the raw structured JSON expected from the AI model.
type ScheduleAIResponse struct {
	NeedsClarification bool             `json:"needs_clarification"`
	Question           string           `json:"question,omitempty"`
	Preview            *SchedulePreview `json:"preview,omitempty"`
}

// getBangkokLocation returns *time.Location for Asia/Bangkok, falling back to FixedZone +07:00.
func getBangkokLocation() *time.Location {
	loc, err := time.LoadLocation(DefaultTimezone)
	if err != nil {
		return time.FixedZone(DefaultTimezone, bangkokOffset)
	}
	return loc
}

// NormalizeOffsets cleans, filters to allowed values (0, 5, 10, 30), deduplicates, and sorts offsets.
func NormalizeOffsets(values []int) []int {
	allowed := map[int]bool{0: true, 5: true, 10: true, 30: true}
	var out []int
	seen := make(map[int]bool)
	for _, v := range values {
		if allowed[v] && !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	if len(out) == 0 {
		return []int{0}
	}
	sort.Ints(out)
	return out
}

// TrimRunes limits a string to a maximum number of runes.
func TrimRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit])
	}
	return s
}

// FallbackSchedule returns a deterministic manual form structure with original text preserved.
func FallbackSchedule(prompt string, reason string) *ScheduleParseResult {
	trimmed := strings.TrimSpace(prompt)
	title := TrimRunes(trimmed, 120)
	if title == "" {
		title = "การแจ้งเตือนใหม่"
	}
	return &ScheduleParseResult{
		NeedsClarification: false,
		Fallback:           true,
		Error:              reason,
		Preview: &SchedulePreview{
			Title:      title,
			Details:    trimmed,
			StartsAt:   "", // Empty so the user fills it in manually
			Recurrence: "once",
			Offsets:    []int{0},
		},
	}
}

// IsObviouslyAmbiguous checks for clear ambiguity indicators in Thai and English
// where no concrete time/hour is specified.
func IsObviouslyAmbiguous(prompt string) (bool, string) {
	lower := strings.ToLower(prompt)

	// Patterns without any specific time (digits followed by : or am/pm/น./โมง/นาฬิกา)
	hasSpecificTime := regexp.MustCompile(`(\d{1,2}[:.]\d{2}|\d{1,2}\s*(am|pm|น\.|โมง|นาฬิกา))`).MatchString(lower)

	// Vague Thai time expressions without specific time
	vagueThai := []string{"ตอนสายๆ", "สายๆ", "ตอนบ่ายๆ", "บ่ายๆ", "ตอนเย็นๆ", "เย็นๆ", "ตอนค่ำๆ", "ค่ำๆ", "ดึกๆ", "สักวัน", "สัปดาห์หน้า", "เดือนหน้า", "ว่างๆ", "เร็วๆ นี้"}
	for _, v := range vagueThai {
		if strings.Contains(prompt, v) && !hasSpecificTime {
			return true, fmt.Sprintf("ระบุเวลาที่แน่นอน เช่น \"%s 09:00 น.\" หรือไม่ครับ?", v)
		}
	}

	// If prompt contains generic reminder words with NO time and NO date at all
	hasDateIndicator := regexp.MustCompile(`(วันนี้|พรุ่งนี้|มะรืน|วัน[จันทร์|อังคาร|พุธ|พฤหัส|ศุกร์|เสาร์|อาทิตย์]|today|tomorrow|monday|tuesday|wednesday|thursday|friday|saturday|sunday|\d{1,2}/\d{1,2})`).MatchString(lower)
	if !hasSpecificTime && !hasDateIndicator {
		return true, "ต้องการให้แจ้งเตือนวันที่และเวลาใดครับ? (เช่น พรุ่งนี้ 09:00 น.)"
	}

	// Vague English time expressions
	vagueEnglish := []string{"sometime", "later", "next week", "in the morning", "in the afternoon", "in the evening"}
	for _, v := range vagueEnglish {
		if strings.Contains(lower, v) && !hasSpecificTime {
			return true, fmt.Sprintf("Could you specify the exact time for \"%s\"? (e.g. 09:00 AM)", v)
		}
	}

	return false, ""
}

// ParseSchedulePrompt parses a natural language Thai/English prompt into a Reminder SchedulePreview.
// It detects ambiguous dates/times and prompts for clarification instead of hallucinating timestamps.
// If the AI call fails or key is missing, it falls back deterministically to manual form entry.
func ParseSchedulePrompt(ctx context.Context, client *Client, prompt string, userTZ string) (*ScheduleParseResult, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return FallbackSchedule("", "empty prompt"), nil
	}

	// Pre-check for explicit ambiguity before making expensive/slow AI call
	if ambiguous, question := IsObviouslyAmbiguous(prompt); ambiguous {
		return &ScheduleParseResult{
			NeedsClarification: true,
			Question:           question,
			Preview:            nil,
			Fallback:           false,
		}, nil
	}

	if client == nil || !client.HasAPIKey() {
		return FallbackSchedule(prompt, "openrouter API key is not configured"), nil
	}

	tz := strings.TrimSpace(userTZ)
	if tz == "" {
		tz = DefaultTimezone
	}
	loc := getBangkokLocation()
	now := time.Now().In(loc)

	// Normalize common Thai dot-time notation (e.g. "11.59" -> "11:59")
	reDotTime := regexp.MustCompile(`\b([01]?\d|2[0-3])\.([0-5]\d)\b`)
	normalizedPrompt := reDotTime.ReplaceAllString(prompt, "$1:$2")

	systemPrompt := fmt.Sprintf(`You are Pocket Sentinel's schedule parsing AI.
Current reference time: %s (Timezone: %s).

Your task: Parse user schedule/reminder requests in Thai or English into strict JSON.

CRITICAL RULES:
1. AMBIGUITY DETECTION: If the user provides an ambiguous, vague, or incomplete date/time (e.g. "พรุ่งนี้ตอนสายๆ", "meeting sometime tomorrow", "เตือนกินยา"), you MUST NOT guess or invent a random time. Set "needs_clarification": true and formulate a polite "question" in the user's language asking for the exact time.
2. SPECIFIC DATE & TIME: If the user specifies an exact or clearly deducible time (e.g. "วันนี้ 11:59", "พรุ่งนี้ 9 โมงเช้า", "11.59", "every Monday at 14:00", "tomorrow 15:30"), calculate the exact RFC3339 timestamp with %s (+07:00 offset). In Thai, "11.59" or "11:59" means 11:59.
3. RECURRENCE: Must be strictly "once" or "weekly". Default is "once".
4. OFFSETS: Array of integer minutes before event for alerts. Allowed values: 0, 5, 10, 30. Example: "ก่อน 5, 10 และเมื่อถึงเวลา" means [0, 5, 10]. Default: [0].
5. TITLE: Concise summary (max 120 characters). Strip conversational request suffixes such as "ช่วยแก้ไขด้วย", "ช่วยเตือนด้วย", "นะครับ", "หน่อยครับ" from the title.
6. DETAILS: Additional context or notes from the prompt (max 1500 characters).

OUTPUT FORMAT: Return ONLY a valid JSON object matching one of these two structures without markdown code blocks:

Case A (Ambiguous time):
{
  "needs_clarification": true,
  "question": "ต้องการให้เตือนเวลาใดครับ? (เช่น 09:00 น.)"
}

Case B (Clear time):
{
  "needs_clarification": false,
  "preview": {
    "title": "ประชุมทีม",
    "details": "ประชุมทีมประจำสัปดาห์",
    "starts_at": "2026-11-20T09:00:00+07:00",
    "recurrence": "once",
    "offsets": [0]
  }
}`, now.Format(time.RFC3339), tz, tz)

	userPrompt := fmt.Sprintf("User schedule prompt:\n\"%s\"", normalizedPrompt)

	rawResp, err := client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		// Deterministic fallback if OpenRouter fails, times out, or errors
		return FallbackSchedule(prompt, err.Error()), nil
	}

	// Clean any potential markdown wrapping (e.g. ```json ... ```)
	cleaned := strings.TrimSpace(rawResp)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	var aiResp ScheduleAIResponse
	if err := json.Unmarshal([]byte(cleaned), &aiResp); err != nil {
		// If AI returned malformed JSON, return deterministic fallback
		return FallbackSchedule(prompt, fmt.Sprintf("failed to parse AI response JSON: %v", err)), nil
	}

	// If AI determined that clarification is needed
	if aiResp.NeedsClarification {
		q := strings.TrimSpace(aiResp.Question)
		if q == "" {
			q = "ต้องการให้แจ้งเตือนวันที่และเวลาใดครับ?"
		}
		return &ScheduleParseResult{
			NeedsClarification: true,
			Question:           q,
			Preview:            nil,
			Fallback:           false,
		}, nil
	}

	// If AI returned a preview, validate and normalize all fields
	if aiResp.Preview == nil {
		return FallbackSchedule(prompt, "ai response missing preview object"), nil
	}

	prev := aiResp.Preview
	prev.Title = strings.TrimSpace(TrimRunes(prev.Title, 120))
	if prev.Title == "" {
		prev.Title = TrimRunes(prompt, 120)
	}
	prev.Details = TrimRunes(prev.Details, 1500)

	// Validate starts_at RFC3339
	parsedTime, err := time.Parse(time.RFC3339, prev.StartsAt)
	if err != nil {
		// If time couldn't be parsed, ask for clarification rather than saving invalid time
		return &ScheduleParseResult{
			NeedsClarification: true,
			Question:           "ระบบไม่สามารถระบุเวลาที่ถูกต้องได้ กรุณาระบุเวลาอีกครั้ง (เช่น 09:00 น.)",
			Preview:            nil,
			Fallback:           false,
		}, nil
	}

	// Ensure Bangkok timezone
	parsedTime = parsedTime.In(loc)
	prev.StartsAt = parsedTime.Format(time.RFC3339)

	// Recurrence validation
	if prev.Recurrence != "weekly" {
		prev.Recurrence = "once"
	}

	// Offsets normalization
	prev.Offsets = NormalizeOffsets(prev.Offsets)

	return &ScheduleParseResult{
		NeedsClarification: false,
		Question:           "",
		Preview:            prev,
		Fallback:           false,
	}, nil
}
