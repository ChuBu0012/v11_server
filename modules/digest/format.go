package digest

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	reHeading   = regexp.MustCompile(`(?m)^#{1,4}\s+(.+)$`)
	reBold      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic    = regexp.MustCompile(`\*([^*\n]+?)\*`)
	reDivider   = regexp.MustCompile(`(?m)^\s*[-*_]{3,}\s*$`)
	reBullet    = regexp.MustCompile(`(?m)^[ \t]*[\*\-][ \t]+`)
	reSubBullet = regexp.MustCompile(`(?m)^[ \t]{2,}[\*\-][ \t]+`)
)

// DigestSection represents a single thematic section of the daily digest.
type DigestSection struct {
	ID      string `json:"id"`      // "ai", "politics", "trivia"
	Title   string `json:"title"`   // e.g. "🤖 AI & เทคโนโลยีเด่น"
	Content string `json:"content"` // Markdown content of this section
}

// SplitSections parses raw AI summary markdown into structured sections.
// It detects ===SECTION:NAME=== tags or gracefully falls back to heading delimiters.
func SplitSections(raw string) []DigestSection {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Check if explicit section markers are present
	hasMarkers := strings.Contains(raw, "===SECTION:")
	if hasMarkers {
		var sections []DigestSection
		parts := strings.Split(raw, "===SECTION:")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			idx := strings.Index(part, "===")
			if idx == -1 {
				continue
			}
			secKey := strings.ToLower(strings.TrimSpace(part[:idx]))
			content := strings.TrimSpace(part[idx+3:])
			if content == "" {
				continue
			}

			title := extractSectionTitle(secKey, content)
			sections = append(sections, DigestSection{
				ID:      secKey,
				Title:   title,
				Content: content,
			})
		}
		if len(sections) > 0 {
			return sections
		}
	}

	// Fallback: Split by Markdown H3/H2 headings
	headingRe := regexp.MustCompile(`(?m)^###?\s+(.+)$`)
	locs := headingRe.FindAllStringIndex(raw, -1)
	if len(locs) >= 2 {
		var sections []DigestSection
		for i := 0; i < len(locs); i++ {
			start := locs[i][0]
			end := len(raw)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			chunk := strings.TrimSpace(raw[start:end])
			if chunk == "" {
				continue
			}
			secID := fmt.Sprintf("part-%d", i+1)
			if i == 0 {
				secID = "ai"
			} else if i == 1 {
				secID = "politics"
			} else if i == 2 {
				secID = "trivia"
			}
			title := extractSectionTitle(secID, chunk)
			sections = append(sections, DigestSection{
				ID:      secID,
				Title:   title,
				Content: chunk,
			})
		}
		if len(sections) > 0 {
			return sections
		}
	}

	// If no delimiter matched, return the whole text as a single section
	return []DigestSection{
		{
			ID:      "all",
			Title:   "📰 สรุปข่าวรอบวัน",
			Content: raw,
		},
	}
}

func extractSectionTitle(secKey, content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		clean := strings.Trim(trimmed, "#* -:")
		if clean != "" {
			return clean
		}
	}
	switch secKey {
	case "ai":
		return "🤖 AI & เทคโนโลยีเด่น"
	case "politics":
		return "🏛️ การเมือง & ประเด็นรอบวัน"
	case "trivia":
		return "💡 สาระน่ารู้ประจำวัน"
	default:
		return "📰 ข่าวและสาระน่ารู้"
	}
}

// FormatTelegramSection formats a single section into a crisp, readable Telegram HTML message.
func FormatTelegramSection(sec DigestSection, partNum, totalParts int) string {
	if strings.TrimSpace(sec.Content) == "" {
		return ""
	}

	// 1. Escape HTML special characters (&, <, >)
	text := strings.ReplaceAll(sec.Content, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	// 2. Parse lines
	lines := strings.Split(text, "\n")
	var bodyLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip duplicate section header line if it matches title
		if reHeading.MatchString(trimmed) {
			continue
		}
		if strings.HasPrefix(trimmed, "🤖 **") || strings.HasPrefix(trimmed, "🏛️ **") || strings.HasPrefix(trimmed, "💡 **") {
			continue
		}
		if reDivider.MatchString(trimmed) {
			continue
		}

		// Sub-bullet (2+ spaces indentation)
		if reSubBullet.MatchString(line) {
			bText := reSubBullet.ReplaceAllString(line, "   ▫️ ")
			bText = reBold.ReplaceAllString(bText, "<b>$1</b>")
			bText = reItalic.ReplaceAllString(bText, "<i>$1</i>")
			bodyLines = append(bodyLines, bText)
			continue
		}

		// Top-level bullet (* or -)
		if reBullet.MatchString(line) {
			bText := reBullet.ReplaceAllString(line, "• ")
			bText = reBold.ReplaceAllString(bText, "<b>$1</b>")
			bText = reItalic.ReplaceAllString(bText, "<i>$1</i>")
			bodyLines = append(bodyLines, bText)
			continue
		}

		// Regular line
		lText := reBold.ReplaceAllString(line, "<b>$1</b>")
		lText = reItalic.ReplaceAllString(lText, "<i>$1</i>")
		bodyLines = append(bodyLines, lText)
	}

	body := strings.Join(bodyLines, "\n\n")

	// Header & footer assembly
	nowStr := time.Now().Format("15:04 น.")
	tagTitle := sec.Title
	if !strings.Contains(tagTitle, sec.ID) && len(tagTitle) > 40 {
		tagTitle = tagTitle[:40] + "..."
	}

	header := fmt.Sprintf("📰 <b>Pocket Sentinel · %s (%d/%d)</b>\n<i>ประจำวันที่ %s</i>\n\n───────────────────────\n\n",
		tagTitle, partNum, totalParts, nowStr)

	footer := "\n\n───────────────────────\n✨ <i>Pocket Sentinel AI · vivo V11</i>"

	full := header + body + footer

	// Length safety check (Telegram max 4096 runes)
	runes := []rune(full)
	if len(runes) > 4000 {
		full = string(runes[:3900]) + "\n\n<i>...(ตัดทอนเนื้อหาเนื่องจากความยาวเกินกำหนด)</i>" + footer
	}

	return full
}

// FormatTelegramDigest converts full raw markdown into Telegram messages (or single message if fallback).
func FormatTelegramDigest(raw string) string {
	sections := SplitSections(raw)
	if len(sections) == 0 {
		return ""
	}
	if len(sections) == 1 {
		return FormatTelegramSection(sections[0], 1, 1)
	}
	var sb strings.Builder
	for i, sec := range sections {
		if i > 0 {
			sb.WriteString("\n\n=======================\n\n")
		}
		sb.WriteString(FormatTelegramSection(sec, i+1, len(sections)))
	}
	return sb.String()
}
