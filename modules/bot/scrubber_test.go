package bot

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestScrubber_TelegramToken(t *testing.T) {
	s := NewOutputScrubber()

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "9-digit bot token",
			input:    "Bot token is 123456789:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678 in env",
			expected: "Bot token is [REDACTED_TELEGRAM_TOKEN] in env",
		},
		{
			name:     "8-digit bot token",
			input:    "token=12345678:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678",
			expected: "token=[REDACTED_TELEGRAM_TOKEN]",
		},
		{
			name:     "10-digit bot token",
			input:    "token=1234567890:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678",
			expected: "token=[REDACTED_TELEGRAM_TOKEN]",
		},
		{
			name:     "multiple tokens",
			input:    "old: 111111111:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678 new: 222222222:ZYXwvuTSRqpoNMLkjihGFEDCBA_87654321",
			expected: "old: [REDACTED_TELEGRAM_TOKEN] new: [REDACTED_TELEGRAM_TOKEN]",
		},
		{
			name:     "token ending in hyphen/dash",
			input:    "token is 123456789:ABCdefGHIjklMNOpqrSTUvwxYZ12345678- and next",
			expected: "token is [REDACTED_TELEGRAM_TOKEN] and next",
		},
		{
			name:     "token ending in hyphen at end of string",
			input:    "token=123456789:ABCdefGHIjklMNOpqrSTUvwxYZ12345678-",
			expected: "token=[REDACTED_TELEGRAM_TOKEN]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := s.Scrub(tc.input)
			if actual != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestScrubber_GeminiKey(t *testing.T) {
	s := NewOutputScrubber()

	geminiKey := "AIzaSyD-1234567890abcdef1234567890abc12"
	input := "API_KEY=" + geminiKey + " active"
	actual := s.Scrub(input)
	expected := "API_KEY=[REDACTED_API_KEY] active"

	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}

	// Test custom tag option [REDACTED_GEMINI_KEY]
	sCustom := NewOutputScrubber(WithGeminiKeyTag(RedactedGeminiKey))
	actualCustom := sCustom.Scrub(input)
	expectedCustom := "API_KEY=[REDACTED_GEMINI_KEY] active"

	if actualCustom != expectedCustom {
		t.Errorf("expected %q, got %q", expectedCustom, actualCustom)
	}

	// Test key ending in hyphen / dash
	geminiKeyEndingHyphen := "AIza1234567890123456789012345678901234-"
	inputHyphen := "key=" + geminiKeyEndingHyphen + " next"
	actualHyphen := s.Scrub(inputHyphen)
	expectedHyphen := "key=[REDACTED_API_KEY] next"
	if actualHyphen != expectedHyphen {
		t.Errorf("expected %q, got %q", expectedHyphen, actualHyphen)
	}

	inputHyphenEnd := "key=" + geminiKeyEndingHyphen
	actualHyphenEnd := s.Scrub(inputHyphenEnd)
	expectedHyphenEnd := "key=[REDACTED_API_KEY]"
	if actualHyphenEnd != expectedHyphenEnd {
		t.Errorf("expected %q, got %q", expectedHyphenEnd, actualHyphenEnd)
	}
}

func TestScrubber_GenericKeysAndBearer(t *testing.T) {
	s := NewOutputScrubber()

	// 1. GitHub Token
	ghInput := "github_pat: ghp_1234567890abcdefghijklmnopqrstuvwxyz"
	ghActual := s.Scrub(ghInput)
	if !strings.Contains(ghActual, "[REDACTED_KEY]") || strings.Contains(ghActual, "ghp_") {
		t.Errorf("expected GitHub token to be redacted, got: %s", ghActual)
	}

	// 2. SSH Public Key
	sshInput := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC3t9 user@pocket-sentinel"
	sshActual := s.Scrub(sshInput)
	if !strings.Contains(sshActual, "[REDACTED_KEY]") || strings.Contains(sshActual, "AAAAB3NzaC1yc2E") {
		t.Errorf("expected SSH public key to be redacted, got: %s", sshActual)
	}

	// 3. Private Key Block
	privInput := "-----BEGIN RSA PRIVATE KEY-----\nMIIEogIBAAKCAQEA0Y...\n-----END RSA PRIVATE KEY-----"
	privActual := s.Scrub(privInput)
	if !strings.Contains(privActual, "[REDACTED_KEY]") || strings.Contains(privActual, "MIIEogIBAAKCAQEA0Y") {
		t.Errorf("expected private key to be redacted, got: %s", privActual)
	}

	// 4. Bearer Token
	bearerInput := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0"
	bearerActual := s.Scrub(bearerInput)
	if !strings.Contains(bearerActual, "Bearer [REDACTED_KEY]") || strings.Contains(bearerActual, "eyJhbGci") {
		t.Errorf("expected Bearer token to be redacted, got: %s", bearerActual)
	}

	// 5. Bearer Token with + and /
	bearerPlusSlash := "Authorization: Bearer abcdefghijklmn+/0123456789"
	bearerPlusActual := s.Scrub(bearerPlusSlash)
	if !strings.Contains(bearerPlusActual, "Bearer [REDACTED_KEY]") || strings.Contains(bearerPlusActual, "abcdefghijklmn") {
		t.Errorf("expected Bearer token with + and / to be redacted, got: %s", bearerPlusActual)
	}

	// 6. Bearer Token with padding =
	bearerPadded := "Authorization: Bearer dXNlcjpwYXNzd29yZA=="
	bearerPaddedActual := s.Scrub(bearerPadded)
	if !strings.Contains(bearerPaddedActual, "Bearer [REDACTED_KEY]") || strings.Contains(bearerPaddedActual, "dXNlcjpw") {
		t.Errorf("expected padded Bearer token to be redacted, got: %s", bearerPaddedActual)
	}

	// 7. Long Padded Bearer Token with ==
	bearerLongPadded := "Authorization: Bearer MTIzNDU2Nzg5MDEyMzQ1Njc4OTA=="
	bearerLongActual := s.Scrub(bearerLongPadded)
	if bearerLongActual != "Authorization: Bearer [REDACTED_KEY]" {
		t.Errorf("expected long padded Bearer token to be fully redacted, got: %s", bearerLongActual)
	}
}

func TestScrubber_MixedSecrets(t *testing.T) {
	s := NewOutputScrubber()

	input := `Config Dump:
TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678
GEMINI_API_KEY=AIzaSyD-1234567890abcdef1234567890abc12
GITHUB_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz
AUTH_HEADER=Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0`

	actual := s.Scrub(input)

	if !strings.Contains(actual, "TELEGRAM_BOT_TOKEN=[REDACTED_TELEGRAM_TOKEN]") {
		t.Errorf("expected telegram token redacted, got: %s", actual)
	}
	if !strings.Contains(actual, "GEMINI_API_KEY=[REDACTED_API_KEY]") {
		t.Errorf("expected gemini key redacted, got: %s", actual)
	}
	if !strings.Contains(actual, "GITHUB_TOKEN=[REDACTED_KEY]") {
		t.Errorf("expected github token redacted, got: %s", actual)
	}
	if !strings.Contains(actual, "AUTH_HEADER=Authorization: Bearer [REDACTED_KEY]") {
		t.Errorf("expected bearer token redacted, got: %s", actual)
	}
}

func TestScrubber_EscapeHTML(t *testing.T) {
	s := NewOutputScrubber()

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tags",
			input:    "<script>alert('xss')</script>",
			expected: "&lt;script&gt;alert('xss')&lt;/script&gt;",
		},
		{
			name:     "ampersand and angle brackets",
			input:    "cat <file> & grep > output",
			expected: "cat &lt;file&gt; &amp; grep &gt; output",
		},
		{
			name:     "already safe characters",
			input:    "normal text 12345 / - = _",
			expected: "normal text 12345 / - = _",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := s.EscapeHTML(tc.input)
			if actual != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestScrubber_Truncate(t *testing.T) {
	s := NewOutputScrubber()

	// 3,499 chars -> not truncated
	under := strings.Repeat("a", 3499)
	if s.Truncate(under, MaxTelegramOutputLength) != under {
		t.Error("expected 3499 chars to remain untruncated")
	}

	// 3,500 chars -> not truncated
	exact := strings.Repeat("a", 3500)
	if s.Truncate(exact, MaxTelegramOutputLength) != exact {
		t.Error("expected 3500 chars to remain untruncated")
	}

	// 3,501 chars -> truncated to 3500 + TruncationNotice
	over := strings.Repeat("a", 3501)
	truncated := s.Truncate(over, MaxTelegramOutputLength)
	expectedPrefix := strings.Repeat("a", 3500)
	if !strings.HasPrefix(truncated, expectedPrefix) {
		t.Error("expected truncated output to have 3500 prefix characters")
	}
	if !strings.HasSuffix(truncated, TruncationNotice) {
		t.Errorf("expected truncation notice suffix, got: %s", truncated[len(truncated)-40:])
	}
	// Verify exact rune count of the trimmed text part
	rawPart := strings.TrimSuffix(truncated, TruncationNotice)
	if utf8.RuneCountInString(rawPart) != 3500 {
		t.Errorf("expected 3500 runes in body, got %d", utf8.RuneCountInString(rawPart))
	}

	// Unicode / Thai characters boundary safety
	thaiOver := strings.Repeat("ก", 3501)
	thaiTruncated := s.Truncate(thaiOver, 3500)
	thaiPart := strings.TrimSuffix(thaiTruncated, TruncationNotice)
	if utf8.RuneCountInString(thaiPart) != 3500 {
		t.Errorf("expected 3500 Thai runes in body, got %d", utf8.RuneCountInString(thaiPart))
	}
	if !utf8.ValidString(thaiTruncated) {
		t.Error("expected truncated Thai string to remain valid UTF-8")
	}

	// Custom maxLen
	custom := s.Truncate("1234567890abcdefghij", 10)
	if custom != "1234567890"+TruncationNotice {
		t.Errorf("expected custom truncation at 10 runes, got: %s", custom)
	}
}

func TestScrubber_FormatResult(t *testing.T) {
	s := NewOutputScrubber()

	// 1. Success result
	successRes := TaskResult{
		Success:  true,
		Duration: 45 * time.Second,
		Output:   "Checking disk: <OK> & token=123456789:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678",
	}
	formattedSuccess := s.FormatResult(successRes, 18.2)

	if !strings.Contains(formattedSuccess, "✅ งานเสร็จสมบูรณ์!") {
		t.Error("expected success title in formatted result")
	}
	if !strings.Contains(formattedSuccess, "⏱ ใช้เวลา: 45s | 💾 RAM ปัจจุบัน: 18.2MB") {
		t.Errorf("expected duration and RAM, got: %s", formattedSuccess)
	}
	if !strings.Contains(formattedSuccess, "<pre>") || !strings.Contains(formattedSuccess, "</pre>") {
		t.Error("expected monospace <pre> tags in formatted result")
	}
	if !strings.Contains(formattedSuccess, "[REDACTED_TELEGRAM_TOKEN]") {
		t.Error("expected Telegram token to be scrubbed inside FormatResult")
	}
	if !strings.Contains(formattedSuccess, "&lt;OK&gt; &amp;") {
		t.Error("expected HTML characters to be escaped inside FormatResult")
	}

	// 2. Failure result
	failRes := TaskResult{
		Success:      false,
		Duration:     180 * time.Second,
		ErrorMessage: "execution timeout > 180s & terminated",
		Output:       "Partial logs before crash: <SIGKILL>",
	}
	formattedFail := s.FormatResult(failRes, 22.0)

	if !strings.Contains(formattedFail, "❌ งานล้มเหลว (Timeout / Error)!") {
		t.Error("expected failure title in formatted result")
	}
	if !strings.Contains(formattedFail, "⚠️ รายละเอียด: execution timeout &gt; 180s &amp; terminated") {
		t.Errorf("expected escaped error message, got: %s", formattedFail)
	}
	if !strings.Contains(formattedFail, "&lt;SIGKILL&gt;") {
		t.Error("expected escaped partial log block")
	}
}

func TestScrubber_AdversarialStressTest(t *testing.T) {
	s := NewOutputScrubber()

	// 1. Telegram token ending in hyphen
	tg := "token is 123456789:ABCdefGHIjklMNOpqrSTUvwxYZ12345678- and next"
	scrubbedTG := s.Scrub(tg)
	if strings.Contains(scrubbedTG, "123456789:ABCdef") {
		t.Fatalf("Telegram token ending in dash leaked: %s", scrubbedTG)
	}
	if !strings.Contains(scrubbedTG, "[REDACTED_TELEGRAM_TOKEN] and next") {
		t.Errorf("expected clean redaction with trailing space, got: %s", scrubbedTG)
	}

	// 2. Gemini key ending in hyphen
	gem := "key=AIza1234567890123456789012345678901234- next"
	scrubbedGem := s.Scrub(gem)
	if strings.Contains(scrubbedGem, "AIza1234567890") {
		t.Fatalf("Gemini key ending in dash leaked: %s", scrubbedGem)
	}
	if !strings.Contains(scrubbedGem, "key=[REDACTED_API_KEY] next") {
		t.Errorf("expected clean redaction with trailing space, got: %s", scrubbedGem)
	}

	// 3. Bearer token with + and /
	bearer := "Authorization: Bearer abcdefghijklmn+/0123456789"
	scrubbedBearer := s.Scrub(bearer)
	if strings.Contains(scrubbedBearer, "abcdefghijklmn") {
		t.Fatalf("Bearer token with +/ leaked: %s", scrubbedBearer)
	}
	if !strings.Contains(scrubbedBearer, "Bearer [REDACTED_KEY]") {
		t.Errorf("expected redacted bearer tag, got: %s", scrubbedBearer)
	}

	// 4. Secret in ErrorMessage formatted via FormatResult
	secretErrRes := TaskResult{
		Success:      false,
		Duration:     5 * time.Second,
		ErrorMessage: "curl: (22) 401 for token 123456789:ABCdefGHIjklMNOpqrSTUvwxYZ_12345678 and key AIzaSyD-1234567890abcdef1234567890abc12",
		Output:       "some output",
	}
	formattedErr := s.FormatResult(secretErrRes, 20.0)
	if strings.Contains(formattedErr, "123456789:ABCdef") {
		t.Fatalf("Telegram token leaked in FormatResult ErrorMessage: %s", formattedErr)
	}
	if strings.Contains(formattedErr, "AIzaSyD-1234567890") {
		t.Fatalf("Gemini key leaked in FormatResult ErrorMessage: %s", formattedErr)
	}
	if !strings.Contains(formattedErr, "[REDACTED_TELEGRAM_TOKEN]") || !strings.Contains(formattedErr, "[REDACTED_API_KEY]") {
		t.Errorf("expected redacted placeholders in ErrorMessage, got: %s", formattedErr)
	}

	// 5. Overflow protection: long ErrorMessage truncated to 500 chars
	longErrRes := TaskResult{
		Success:      false,
		Duration:     10 * time.Second,
		ErrorMessage: strings.Repeat("error details overflow ", 250), // 5,750 runes
		Output:       "output block",
	}
	formattedLongErr := s.FormatResult(longErrRes, 25.0)
	totalRunes := utf8.RuneCountInString(formattedLongErr)
	if totalRunes >= 4096 {
		t.Fatalf("FormatResult output length %d exceeded Telegram 4096 rune limit", totalRunes)
	}
	if !strings.Contains(formattedLongErr, TruncationNotice) {
		t.Errorf("expected TruncationNotice in formatted error, got: %s", formattedLongErr)
	}
}

func TestScrubber_FormatResult_WithSummary(t *testing.T) {
	s := NewOutputScrubber()

	res := TaskResult{
		Success:  true,
		Duration: 30 * time.Second,
		Output: `Executing health check...
Disk: 45% used
Memory: 18MB

### สรุปการดำเนินงาน
- ตรวจสอบพื้นที่ดิสก์เรียบร้อย อยู่ที่ 45%
- อัปเดตการทำงานของระบบและเคลียร์แคชสำเร็จ`,
	}

	formatted := s.FormatResult(res, 18.5)

	if !strings.Contains(formatted, "📝 <b>สรุปการทำงาน:</b>") {
		t.Fatalf("expected summary header, got: %s", formatted)
	}
	if !strings.Contains(formatted, "ตรวจสอบพื้นที่ดิสก์เรียบร้อย อยู่ที่ 45%") {
		t.Fatalf("expected bullet point 1 in summary, got: %s", formatted)
	}
	if !strings.Contains(formatted, "อัปเดตการทำงานของระบบและเคลียร์แคชสำเร็จ") {
		t.Fatalf("expected bullet point 2 in summary, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Executing health check...") {
		t.Fatalf("expected remaining logs in pre block, got: %s", formatted)
	}
}

func TestScrubber_FormatQuestionResult(t *testing.T) {
	s := NewOutputScrubber()

	res := TaskResult{
		Success:  true,
		Duration: 12 * time.Second,
		Output: `กำลังวิเคราะห์คำถามเกี่ยวกับระบบ...
โมดูลหลักประกอบด้วย bot, telemetry, dashboard, location, alerts

### สรุปคำตอบ
- Pocket Sentinel มี 5 โมดูลหลักที่ทำงานร่วมกัน
- ระบบจัดการผ่าน PM2 และสื่อสารผ่าน Telegram Long Polling`,
	}

	formatted := s.FormatQuestionResult(res, 21.0)

	if !strings.Contains(formatted, "💡 <b>คำตอบจาก Agent (agy):</b>") {
		t.Fatalf("expected question header, got: %s", formatted)
	}
	if !strings.Contains(formatted, "📝 <b>สรุปคำตอบ:</b>") {
		t.Fatalf("expected question summary header, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Pocket Sentinel มี 5 โมดูลหลักที่ทำงานร่วมกัน") {
		t.Fatalf("expected summary bullet 1, got: %s", formatted)
	}
	if !strings.Contains(formatted, "กำลังวิเคราะห์คำถามเกี่ยวกับระบบ...") {
		t.Fatalf("expected remaining logs in pre block, got: %s", formatted)
	}
}

func TestScrubber_MarkdownToTelegramHTML(t *testing.T) {
	s := NewOutputScrubber()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "User screenshot case: 1 + 1 is 2",
			input:    "- ผลลัพธ์ของ 1 + 1 คือ **2**",
			expected: "• ผลลัพธ์ของ 1 + 1 คือ <b>2</b>",
		},
		{
			name:     "Multiple bullets with bold and inline code",
			input:    "- ข้อ 1: ทำการแก้ไขไฟล์ `main.go` สำเร็จ\n- ข้อ 2: ผลทดสอบ **ผ่าน 100%**",
			expected: "• ข้อ 1: ทำการแก้ไขไฟล์ <code>main.go</code> สำเร็จ\n• ข้อ 2: ผลทดสอบ <b>ผ่าน 100%</b>",
		},
		{
			name:     "HTML escaping with markdown",
			input:    "- ใช้แท็ก <div> & <script> ปลอดภัยด้วย **HTML Escape**",
			expected: "• ใช้แท็ก &lt;div&gt; &amp; &lt;script&gt; ปลอดภัยด้วย <b>HTML Escape</b>",
		},
		{
			name:     "Code block preserved",
			input:    "ตัวอย่างโค้ด:\n```go\nfmt.Println(1 + 1)\n```",
			expected: "ตัวอย่างโค้ด:\n<pre>fmt.Println(1 + 1)</pre>",
		},
		{
			name:     "Markdown header to bold",
			input:    "### สรุปผล\n- ผ่าน",
			expected: "<b>สรุปผล</b>\n• ผ่าน",
		},
		{
			name:     "Markdown link",
			input:    "ดูรายละเอียดที่ [Pocket Sentinel](https://spindle-said-hemstitch.ngrok-free.dev)",
			expected: `ดูรายละเอียดที่ <a href="https://spindle-said-hemstitch.ngrok-free.dev">Pocket Sentinel</a>`,
		},
		{
			name:     "Strikethrough",
			input:    "ข้อความนี้ ~~ยกเลิกแล้ว~~ ใช้งานได้",
			expected: "ข้อความนี้ <s>ยกเลิกแล้ว</s> ใช้งานได้",
		},
		{
			name:     "Consecutive italics",
			input:    "*ข้อ 1* *ข้อ 2*",
			expected: "<i>ข้อ 1</i> <i>ข้อ 2</i>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := s.MarkdownToTelegramHTML(tc.input)
			if actual != tc.expected {
				t.Errorf("\nexpected: %q\ngot:      %q", tc.expected, actual)
			}
		})
	}
}
