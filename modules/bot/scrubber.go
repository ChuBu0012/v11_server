package bot

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxTelegramOutputLength is the maximum character limit before truncation.
	MaxTelegramOutputLength = 3500
	// MaxErrorMessageLength is the maximum character limit for error messages.
	MaxErrorMessageLength = 500
	// TruncationNotice is appended when output exceeds MaxTelegramOutputLength.
	TruncationNotice = "\n... [Output truncated for Telegram]"

	// RedactedTelegramToken is the placeholder for redacted Telegram Bot tokens.
	RedactedTelegramToken = "[REDACTED_TELEGRAM_TOKEN]"
	// RedactedAPIKey is the default placeholder for redacted Gemini / Google API keys.
	RedactedAPIKey = "[REDACTED_API_KEY]"
	// RedactedGeminiKey is the alternate placeholder for Gemini keys if requested.
	RedactedGeminiKey = "[REDACTED_GEMINI_KEY]"
	// RedactedKey is the placeholder for generic SSH, Bearer, or GitHub keys.
	RedactedKey = "[REDACTED_KEY]"
)

// TaskResult holds the outcome of an agy background run.
type TaskResult struct {
	Success      bool
	Output       string
	Duration     time.Duration
	ErrorMessage string
	ExitCode     int
}

// OutputScrubberOption allows configuring OutputScrubber behavior.
type OutputScrubberOption func(*OutputScrubber)

// WithGeminiKeyTag configures the replacement tag for Gemini API keys.
func WithGeminiKeyTag(tag string) OutputScrubberOption {
	return func(s *OutputScrubber) {
		s.geminiTag = tag
	}
}

// OutputScrubber handles redacting secrets, escaping HTML, and truncating command outputs.
type OutputScrubber struct {
	telegramRegex   *regexp.Regexp
	geminiRegex     *regexp.Regexp
	githubRegex     *regexp.Regexp
	sshKeyRegex     *regexp.Regexp
	privateKeyRegex *regexp.Regexp
	bearerRegex     *regexp.Regexp

	geminiTag string
}

// NewOutputScrubber creates and initializes an OutputScrubber with pre-compiled regexes.
func NewOutputScrubber(opts ...OutputScrubberOption) *OutputScrubber {
	s := &OutputScrubber{
		telegramRegex:   regexp.MustCompile(`\b([0-9]{8,10}:[a-zA-Z0-9_-]{35})([^a-zA-Z0-9_-]|$)`),
		geminiRegex:     regexp.MustCompile(`\b(AIza[0-9A-Za-z-_]{35})([^a-zA-Z0-9_-]|$)`),
		githubRegex:     regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{36,255}\b`),
		sshKeyRegex:     regexp.MustCompile(`\bssh-(?:rsa|dss|ed25519)\s+[A-Za-z0-9+/]+={0,2}`),
		privateKeyRegex: regexp.MustCompile(`-----BEGIN [A-Z\s]+PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z\s]+PRIVATE KEY-----|$)`),
		bearerRegex:     regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9_\-\.\+/=]{15,}`),
		geminiTag:       RedactedAPIKey,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Scrub scans the input string and replaces sensitive tokens and secrets with redaction placeholders.
func (s *OutputScrubber) Scrub(raw string) string {
	out := raw

	// 1. Redact Private Keys first (multi-line block)
	out = s.privateKeyRegex.ReplaceAllString(out, RedactedKey)

	// 2. Redact SSH Public Keys
	out = s.sshKeyRegex.ReplaceAllString(out, RedactedKey)

	// 3. Redact GitHub Tokens
	out = s.githubRegex.ReplaceAllString(out, RedactedKey)

	// 4. Redact Bearer Tokens
	out = s.bearerRegex.ReplaceAllString(out, "Bearer "+RedactedKey)

	// 5. Redact Telegram Tokens
	out = s.telegramRegex.ReplaceAllString(out, RedactedTelegramToken+"$2")

	// 6. Redact Gemini / Google API Keys
	out = s.geminiRegex.ReplaceAllString(out, s.geminiTag+"$2")

	return out
}

// Truncate truncates text to maxLen characters and appends a truncation notice if exceeded.
func (s *OutputScrubber) Truncate(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = MaxTelegramOutputLength
	}

	if utf8.RuneCountInString(text) <= maxLen {
		return text
	}

	runes := []rune(text)
	return string(runes[:maxLen]) + TruncationNotice
}

// EscapeHTML escapes special characters (&, <, >) to ensure compatibility with Telegram HTML parse mode.
func (s *OutputScrubber) EscapeHTML(text string) string {
	r := strings.ReplaceAll(text, "&", "&amp;")
	r = strings.ReplaceAll(r, "<", "&lt;")
	r = strings.ReplaceAll(r, ">", "&gt;")
	return r
}

var (
	mdCodeBlockRegex      = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9_-]+)?\n?(.*?)```")
	mdInlineCodeRegex     = regexp.MustCompile("`([^`\n]+)`")
	mdBoldAsteriskRegex   = regexp.MustCompile(`\*\*([^*\n]+?)\*\*`)
	mdBoldUnderscoreRegex = regexp.MustCompile(`__([^_\n]+?)__`)
	mdItalicAsteriskRegex = regexp.MustCompile(`(^|[^\*])\*([^*\n]+?)\*`)
	mdStrikethroughRegex  = regexp.MustCompile(`~~([^~\n]+?)~~`)
	mdHeaderRegex         = regexp.MustCompile(`(?m)^#{1,6}\s*(.*)$`)
	mdBulletRegex         = regexp.MustCompile(`(?m)^[\t ]*[-*][\t ]+(.*)$`)
	mdLinkRegex           = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s\)]+)\)`)
)

// MarkdownToTelegramHTML converts basic Markdown formatting (bold, italic, inline code, code blocks, bullets, headers)
// into valid Telegram HTML entities and ensures special characters are properly escaped.
func (s *OutputScrubber) MarkdownToTelegramHTML(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	// 1. First escape HTML entities (&, <, >) so raw text is safe.
	escaped := s.EscapeHTML(text)

	// 2. Extract code blocks and inline code to protect them from markup replacements.
	var codeBlocks []string
	placeholderBlock := func(match string) string {
		idx := len(codeBlocks)
		sub := mdCodeBlockRegex.FindStringSubmatch(match)
		content := match
		if len(sub) > 1 {
			content = sub[1]
		}
		codeBlocks = append(codeBlocks, "<pre>"+strings.TrimSpace(content)+"</pre>")
		return fmt.Sprintf("\x00CB_%d\x00", idx)
	}
	escaped = mdCodeBlockRegex.ReplaceAllStringFunc(escaped, placeholderBlock)

	var inlineCodes []string
	placeholderInline := func(match string) string {
		idx := len(inlineCodes)
		sub := mdInlineCodeRegex.FindStringSubmatch(match)
		content := match
		if len(sub) > 1 {
			content = sub[1]
		}
		inlineCodes = append(inlineCodes, "<code>"+content+"</code>")
		return fmt.Sprintf("\x00IC_%d\x00", idx)
	}
	escaped = mdInlineCodeRegex.ReplaceAllStringFunc(escaped, placeholderInline)

	// 3. Convert Markdown headers (### Header) -> <b>Header</b>
	escaped = mdHeaderRegex.ReplaceAllString(escaped, "<b>$1</b>")

	// 4. Convert Bullet points (- Item or * Item) -> • Item
	escaped = mdBulletRegex.ReplaceAllString(escaped, "• $1")

	// 5. Convert Bold (**bold** or __bold__) -> <b>bold</b>
	escaped = mdBoldAsteriskRegex.ReplaceAllString(escaped, "<b>$1</b>")
	escaped = mdBoldUnderscoreRegex.ReplaceAllString(escaped, "<b>$1</b>")

	// 6. Convert Italic (*italic*) -> <i>italic</i>
	escaped = mdItalicAsteriskRegex.ReplaceAllString(escaped, "${1}<i>$2</i>")

	// 7. Convert Strikethrough (~~strike~~) -> <s>strike</s>
	escaped = mdStrikethroughRegex.ReplaceAllString(escaped, "<s>$1</s>")

	// 8. Convert Markdown Links [text](url) -> <a href="url">text</a>
	escaped = mdLinkRegex.ReplaceAllString(escaped, `<a href="$2">$1</a>`)

	// 9. Restore protected inline code and code blocks
	for i, code := range inlineCodes {
		escaped = strings.ReplaceAll(escaped, fmt.Sprintf("\x00IC_%d\x00", i), code)
	}
	for i, block := range codeBlocks {
		escaped = strings.ReplaceAll(escaped, fmt.Sprintf("\x00CB_%d\x00", i), block)
	}

	return escaped
}

// ExtractSummary extracts an optional structured summary section from agent execution output.
// It looks for markdown headers like '### สรุปการดำเนินงาน', '### สรุปคำตอบ', '### สรุปการทำงาน', or 'สรุปการทำงาน:'.
func (s *OutputScrubber) ExtractSummary(text string) (string, string) {
	re := regexp.MustCompile(`(?i)(?:^|\n)(?:###?\s*(?:สรุปการดำเนินงาน|สรุปคำตอบ|สรุปการทำงาน|สรุปผลการทำงาน|สรุปผล|summary)[\s:]*|สรุปการทำงาน:?\s*\n)([\s\S]*)$`)
	loc := re.FindStringSubmatchIndex(text)
	if loc != nil && len(loc) >= 4 {
		summaryPart := strings.TrimSpace(text[loc[2]:loc[3]])
		mainPart := strings.TrimSpace(text[:loc[0]])
		if summaryPart != "" {
			return summaryPart, mainPart
		}
	}
	return "", text
}

// FormatQuestionResult formats a TaskResult for Q&A (/ask) with question-specific headers and formatting.
func (s *OutputScrubber) FormatQuestionResult(result TaskResult, ramMB float64) string {
	scrubbed := s.Scrub(result.Output)
	summary, remaining := s.ExtractSummary(scrubbed)

	displayLog := remaining
	if strings.TrimSpace(displayLog) == "" {
		displayLog = scrubbed
	}

	truncated := s.Truncate(displayLog, MaxTelegramOutputLength)
	sanitized := s.EscapeHTML(truncated)

	durationStr := formatDuration(result.Duration)
	ramStr := fmt.Sprintf("%.1fMB", ramMB)

	var summaryBlock string
	if summary != "" {
		sanitizedSummary := s.MarkdownToTelegramHTML(s.Truncate(summary, 1000))
		summaryBlock = fmt.Sprintf("\n\n📝 <b>สรุปคำตอบ:</b>\n%s", sanitizedSummary)
	}

	if result.Success {
		return fmt.Sprintf("💡 <b>คำตอบจาก Agent (agy):</b>\n⏱ ใช้เวลา: %s | 💾 RAM ปัจจุบัน: %s%s\n\n📋 รายละเอียดคำตอบ:\n---------------------------------------------\n<pre>%s</pre>\n---------------------------------------------",
			durationStr, ramStr, summaryBlock, sanitized)
	}

	scrubbedErr := s.Scrub(result.ErrorMessage)
	truncatedErr := s.Truncate(scrubbedErr, MaxErrorMessageLength)
	errMsg := s.EscapeHTML(truncatedErr)
	if errMsg == "" {
		errMsg = "Unknown error"
	}
	return fmt.Sprintf("❌ ไม่สามารถตอบคำถามได้ (Timeout / Error)!\n⏱ ใช้เวลา: %s\n\n⚠️ รายละเอียด: %s%s\n---------------------------------------------\n<pre>%s</pre>\n---------------------------------------------",
		durationStr, errMsg, summaryBlock, sanitized)
}

// FormatResult formats a TaskResult with status emoji, duration, RAM usage, and a sanitized log block.
func (s *OutputScrubber) FormatResult(result TaskResult, ramMB float64) string {
	scrubbed := s.Scrub(result.Output)
	summary, remaining := s.ExtractSummary(scrubbed)

	displayLog := remaining
	if strings.TrimSpace(displayLog) == "" {
		displayLog = scrubbed
	}

	truncated := s.Truncate(displayLog, MaxTelegramOutputLength)
	sanitized := s.EscapeHTML(truncated)

	durationStr := formatDuration(result.Duration)
	ramStr := fmt.Sprintf("%.1fMB", ramMB)

	var summaryBlock string
	if summary != "" {
		sanitizedSummary := s.MarkdownToTelegramHTML(s.Truncate(summary, 1000))
		summaryBlock = fmt.Sprintf("\n\n📝 <b>สรุปการทำงาน:</b>\n%s", sanitizedSummary)
	}

	if result.Success {
		return fmt.Sprintf("✅ งานเสร็จสมบูรณ์!\n⏱ ใช้เวลา: %s | 💾 RAM ปัจจุบัน: %s%s\n\n📋 ผลลัพธ์:\n---------------------------------------------\n<pre>%s</pre>\n---------------------------------------------",
			durationStr, ramStr, summaryBlock, sanitized)
	}

	scrubbedErr := s.Scrub(result.ErrorMessage)
	truncatedErr := s.Truncate(scrubbedErr, MaxErrorMessageLength)
	errMsg := s.EscapeHTML(truncatedErr)
	if errMsg == "" {
		errMsg = "Unknown error"
	}
	return fmt.Sprintf("❌ งานล้มเหลว (Timeout / Error)!\n⏱ ใช้เวลา: %s\n\n⚠️ รายละเอียด: %s%s\n---------------------------------------------\n<pre>%s</pre>\n---------------------------------------------",
		durationStr, errMsg, summaryBlock, sanitized)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	sec := d.Seconds()
	if sec == float64(int(sec)) {
		return fmt.Sprintf("%ds", int(sec))
	}
	return fmt.Sprintf("%.1fs", sec)
}
