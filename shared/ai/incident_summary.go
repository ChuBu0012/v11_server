package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// IncidentMetadata holds only filtered monitoring evidence.
// Sensitive data (query parameters, credentials, internal headers) must never be passed to AI.
type IncidentMetadata struct {
	Timestamp      time.Time `json:"timestamp"`
	WatcherName    string    `json:"watcher_name"`
	URL            string    `json:"url"` // Public URL
	ExpectedStatus int       `json:"expected_status"`
	ActualStatus   int       `json:"actual_status"`
	LatencyMs      int64     `json:"latency_ms"`
	ErrorSummary   string    `json:"error_summary"`
}

// IncidentSummary holds the structured AI analysis strictly separating Facts from Hypotheses.
type IncidentSummary struct {
	Facts      []string `json:"facts"`      // ข้อเท็จจริงจากการตรวจวัด
	Hypotheses []string `json:"hypotheses"` // ข้อสันนิษฐานและสิ่งที่ควรตรวจสอบต่อ
	RawSummary string   `json:"raw_summary,omitempty"`
	Fallback   bool     `json:"fallback,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type incidentAIResponse struct {
	Facts      []string `json:"facts"`
	Hypotheses []string `json:"hypotheses"`
}

// SanitizeURL removes user credentials, query strings, and fragments from a URL
// to ensure no tokens, keys, or private parameters are ever transmitted to an LLM.
func SanitizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		// If it doesn't parse, do regex-based stripping of query strings and userinfo
		reQuery := regexp.MustCompile(`\?.*$`)
		cleaned := reQuery.ReplaceAllString(rawURL, "")
		reCreds := regexp.MustCompile(`//[^@]+@`)
		return reCreds.ReplaceAllString(cleaned, "//")
	}

	// Strip userinfo, query, and fragment
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}

// SanitizeErrorSummary redacts known sensitive token patterns, bearer tokens, or query strings from error strings.
func SanitizeErrorSummary(errStr string) string {
	if errStr == "" {
		return ""
	}
	s := errStr
	// Redact bearer tokens
	reBearer := regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_\-\.]+`)
	s = reBearer.ReplaceAllString(s, "Bearer [REDACTED]")

	// Redact API keys / secret parameters
	reKey := regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)=[^&\s]+`)
	s = reKey.ReplaceAllString(s, "$1=[REDACTED]")

	return strings.TrimSpace(s)
}

// FallbackIncidentSummary creates a deterministic incident summary based strictly on the provided probe data.
func FallbackIncidentSummary(incident IncidentMetadata, reason string) *IncidentSummary {
	sanitizedURL := SanitizeURL(incident.URL)
	sanitizedErr := SanitizeErrorSummary(incident.ErrorSummary)

	facts := []string{
		fmt.Sprintf("บริการ: %s (%s)", incident.WatcherName, sanitizedURL),
		fmt.Sprintf("เวลาที่ตรวจพบ: %s", incident.Timestamp.In(getBangkokLocation()).Format("2006-01-02 15:04:05 MST")),
	}

	if incident.ActualStatus > 0 {
		facts = append(facts, fmt.Sprintf("HTTP Status: %d (ค่าที่คาดหวังคือ %d)", incident.ActualStatus, incident.ExpectedStatus))
	} else {
		facts = append(facts, "HTTP Status: ไม่ได้รับ HTTP response (Connection failure)")
	}

	if incident.LatencyMs > 0 {
		facts = append(facts, fmt.Sprintf("ความล่าช้า (Latency): %d ms", incident.LatencyMs))
	}

	if sanitizedErr != "" {
		facts = append(facts, fmt.Sprintf("รายละเอียดข้อผิดพลาด: %s", sanitizedErr))
	}

	hypotheses := []string{}
	switch {
	case incident.ActualStatus >= 500:
		hypotheses = append(hypotheses,
			"Origin server ส่งรหัสข้อผิดพลาดฝั่งเซิร์ฟเวอร์ (5xx) — เซิร์ฟเวอร์ปลายทางอาจกำลัง restart, มี uncaught exception หรือ backend service ล่ม",
			"ควรตรวจสอบ application logs, reverse proxy error log (Nginx/Cloudflare) และสถานะ container/process ปลายทาง")
	case incident.ActualStatus >= 400:
		hypotheses = append(hypotheses,
			fmt.Sprintf("Origin server ตอบกลับด้วยรหัส %d — อาจมีการเปลี่ยนเส้นทาง (route) ของหน้าเว็บ หรือต้องการ authentication header", incident.ActualStatus),
			"ควรตรวจสอบ configuration ของ endpoint และ URL path ที่ระบุใน Watchtower")
	default:
		hypotheses = append(hypotheses,
			"ไม่สามารถเชื่อมต่อเครือข่ายไปยังเซิร์ฟเวอร์ปลายทางได้ (Network/DNS timeout หรือ Connection Refused)",
			"ควรตรวจสอบการเชื่อมต่ออินเทอร์เน็ตของ V11, DNS resolution, หรือตรวจสอบว่าเครื่องปลายทางเปิดรับ port ที่ถูกต้อง")
	}

	var sb strings.Builder
	sb.WriteString("📌 **ข้อเท็จจริง (Facts):**\n")
	for _, f := range facts {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	sb.WriteString("\n🔍 **ข้อสันนิษฐาน & สิ่งที่ควรตรวจต่อ (Hypotheses):**\n")
	for _, h := range hypotheses {
		sb.WriteString(fmt.Sprintf("- %s\n", h))
	}

	return &IncidentSummary{
		Facts:      facts,
		Hypotheses: hypotheses,
		RawSummary: sb.String(),
		Fallback:   true,
		Error:      reason,
	}
}

// SummarizeIncident analyzes filtered probe metadata and produces a structured summary
// strictly separating Facts from Hypotheses. AI NEVER decides up/down state or alert triggers.
// If AI fails or key is missing, it returns a deterministic fallback summary.
func SummarizeIncident(ctx context.Context, client *Client, incident IncidentMetadata) (*IncidentSummary, error) {
	// Sanitize URL and error summary before anything touches AI
	sanitizedURL := SanitizeURL(incident.URL)
	sanitizedErr := SanitizeErrorSummary(incident.ErrorSummary)

	if client == nil || !client.HasAPIKey() {
		return FallbackIncidentSummary(incident, "openrouter API key is not configured"), nil
	}

	systemPrompt := `You are Pocket Sentinel's SRE Incident Analyzer.

Your task: Analyze filtered synthetic monitoring probe evidence and produce an incident summary strictly in Thai.

CRITICAL GUARDRAILS:
1. NO STATE DECISIONS: You do NOT determine or announce whether a service is UP or DOWN, nor do you trigger alerts. The system Go state machine has ALREADY determined this outage.
2. STRICT SEPARATION:
   - "facts": Observable, objective measurements from the probe data ONLY (e.g. timestamp, HTTP status received vs expected, latency, specific socket/TLS error).
   - "hypotheses": Plausible root causes and actionable diagnostic steps for the engineer to verify next.
3. CONCISENESS: Output clean, professional Thai bullet points suitable for Telegram or dashboard incident cards.

OUTPUT FORMAT: Return ONLY a valid JSON object matching this schema without markdown code fences:
{
  "facts": [
    "บริการ Todo FM (https://todo-fm-d.vercel.app/) ไม่ตอบสนอง",
    "ได้รับ HTTP Status 503 Service Unavailable (คาดหวัง 200)",
    "ความล่าช้าในการตอบสนอง 120 ms"
  ],
  "hypotheses": [
    "Origin server หรือ Vercel function อาจเกิดข้อผิดพลาดในการประมวลผล",
    "ควรตรวจสอบ Vercel runtime logs และสถานะ database connection"
  ]
}`

	userPrompt := fmt.Sprintf(`Monitoring Probe Evidence:
- Watcher Name: %s
- Target URL: %s
- Timestamp: %s
- Expected HTTP Status: %d
- Actual HTTP Status: %d
- Latency: %d ms
- Error Detail: %s`,
		incident.WatcherName,
		sanitizedURL,
		incident.Timestamp.In(getBangkokLocation()).Format("2006-01-02 15:04:05 MST"),
		incident.ExpectedStatus,
		incident.ActualStatus,
		incident.LatencyMs,
		sanitizedErr,
	)

	rawResp, err := client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return FallbackIncidentSummary(incident, err.Error()), nil
	}

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

	var aiResp incidentAIResponse
	if err := json.Unmarshal([]byte(cleaned), &aiResp); err != nil {
		return FallbackIncidentSummary(incident, fmt.Sprintf("failed to parse AI JSON response: %v", err)), nil
	}

	if len(aiResp.Facts) == 0 || len(aiResp.Hypotheses) == 0 {
		return FallbackIncidentSummary(incident, "ai response missing facts or hypotheses"), nil
	}

	var sb strings.Builder
	sb.WriteString("📌 **ข้อเท็จจริง (Facts):**\n")
	for _, f := range aiResp.Facts {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	sb.WriteString("\n🔍 **ข้อสันนิษฐาน & สิ่งที่ควรตรวจต่อ (Hypotheses):**\n")
	for _, h := range aiResp.Hypotheses {
		sb.WriteString(fmt.Sprintf("- %s\n", h))
	}

	return &IncidentSummary{
		Facts:      aiResp.Facts,
		Hypotheses: aiResp.Hypotheses,
		RawSummary: sb.String(),
		Fallback:   false,
	}, nil
}
