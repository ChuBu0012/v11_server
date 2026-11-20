package digest

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"time"
)

// Button is one inline keyboard button (kept bot-agnostic; main.go adapts it).
type Button struct {
	Text string
	Data string
}

// Messenger publishes the single digest message with its category buttons.
type Messenger interface {
	SendMenu(ctx context.Context, text string, buttons [][]Button) error
}

// GoldSource is satisfied by *GoldFetcher; tests inject a fake.
type GoldSource interface {
	Fetch(ctx context.Context) GoldQuote
}

const disclaimer = "<i>ไม่ใช่คำแนะนำการลงทุน</i>"

type menuItem struct{ key, label string }

// Order of the buttons (two rows of three).
var menuItems = []menuItem{
	{"gold", "🥇 ทอง"}, {"stocks", "📈 หุ้น"}, {"ai", "🤖 AI"},
	{"politics", "🏛️ การเมือง"}, {"trivia", "💡 สาระ"}, {"overview", "🏠 ภาพรวม"},
}

var sectionTitles = map[string]string{
	"gold": "🥇 ทอง", "stocks": "📈 หุ้น", "ai": "🤖 AI & เทคโนโลยี", "politics": "🏛️ การเมือง", "trivia": "💡 สาระน่ารู้",
}

// MenuButtons builds the keyboard; the active view is marked with "• ".
func MenuButtons(runID int64, active string) [][]Button {
	rows := make([][]Button, 0, 2)
	for i := 0; i < len(menuItems); i += 3 {
		row := make([]Button, 0, 3)
		for _, it := range menuItems[i : i+3] {
			text := it.label
			if it.key == active {
				text = "• " + text
			}
			row = append(row, Button{Text: text, Data: fmt.Sprintf("dg:%d:%s", runID, it.key)})
		}
		rows = append(rows, row)
	}
	return rows
}

// ParseCallback validates "dg:<runID>:<key>".
func ParseCallback(data string) (int64, string, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != "dg" {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	for _, it := range menuItems {
		if it.key == parts[2] {
			return id, parts[2], true
		}
	}
	return 0, "", false
}

// HandleCallback renders the view a button asked for; ok=false means ignore.
func (s *Service) HandleCallback(ctx context.Context, data string) (string, [][]Button, bool) {
	id, key, ok := ParseCallback(data)
	if !ok {
		return "", nil, false
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil || run == nil {
		return "", nil, false
	}
	prev, _ := s.store.PreviousGold(ctx, id)
	return RenderView(run, key, prev), MenuButtons(id, key), true
}

// RenderView returns the Telegram HTML for one view of a stored run.
func RenderView(run *RunRecord, key string, prev *GoldQuote) string {
	sections := map[string]string{}
	for _, sec := range SplitSections(run.SummaryMD) {
		sections[sec.ID] = sec.Content
	}
	var quote *GoldQuote
	if run.GoldJSON != "" {
		var q GoldQuote
		if json.Unmarshal([]byte(run.GoldJSON), &q) == nil {
			quote = &q
		}
	}
	date := run.StartedAt
	if date.IsZero() {
		date = time.Now()
	}
	header := fmt.Sprintf("📰 <b>สรุปข่าว %s</b>", thaiDate(date))

	var out string
	if key == "overview" || key == "" {
		out = header + "\n\n" + goldSummaryLine(quote, prev) + "\n" + overviewLines(sections) + "\n\n" + disclaimer
	} else {
		title := sectionTitles[key]
		body := ""
		if content := strings.TrimSpace(sections[key]); content != "" {
			body = sectionBodyHTML(content)
		}
		if key == "gold" {
			// The stored GOLD section already starts with the price block when
			// prices were fetched; older runs fall back to the quote alone.
			if !strings.Contains(sections[key], "ทองแท่ง") {
				body = goldTable(quote, prev) + "\n\n" + body
			}
		}
		if strings.TrimSpace(body) == "" {
			body = "<i>ไม่มีข่าวในหมวดนี้</i>"
		}
		out = header + "\n<b>" + title + "</b>\n\n" + strings.TrimSpace(body)
		if key == "gold" || key == "stocks" {
			out += "\n\n" + disclaimer
		}
	}
	if r := []rune(out); len(r) > 4000 {
		out = string(r[:3900]) + "\n<i>…(ตัดทอน)</i>"
	}
	return out
}

func overviewLines(sections map[string]string) string {
	if ov := strings.TrimSpace(sections["overview"]); ov != "" {
		var lines []string
		for _, l := range strings.Split(ov, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				lines = append(lines, inlineHTML(l))
			}
		}
		return strings.Join(lines, "\n")
	}
	// Older runs have no OVERVIEW: use each section's first bullet.
	var lines []string
	for _, key := range []string{"stocks", "ai", "politics", "trivia"} {
		if first := firstBullet(sections[key]); first != "" {
			lines = append(lines, sectionTitles[key]+": "+first)
		}
	}
	return strings.Join(lines, "\n")
}

func firstBullet(content string) string {
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "* ") || strings.HasPrefix(l, "- ") {
			return inlineHTML(strings.TrimSpace(l[2:]))
		}
	}
	return ""
}

// inlineHTML escapes a markdown line and keeps **bold** / *italic*.
func inlineHTML(line string) string {
	line = html.EscapeString(line)
	line = reBold.ReplaceAllString(line, "<b>$1</b>")
	return reItalic.ReplaceAllString(line, "<i>$1</i>")
}

// goldSummaryLine is the one-glance gold line of the overview.
func goldSummaryLine(q, prev *GoldQuote) string {
	if q == nil || (q.SpotUSD == 0 && q.BarSell == 0) {
		return "🥇 ทอง: <i>ดึงราคาไม่ได้</i>"
	}
	parts := []string{}
	if q.SpotUSD > 0 {
		s := "โลก $" + money(q.SpotUSD, 0) + "/oz"
		if prev != nil && prev.SpotUSD > 0 {
			s += " (" + pctChange(q.SpotUSD, prev.SpotUSD) + ")"
		}
		parts = append(parts, s)
	}
	if q.BarSell > 0 {
		s := "แท่ง ขาย " + money(q.BarSell, 0)
		if prev != nil && prev.BarSell > 0 && !q.Estimated {
			s += " (" + diff(q.BarSell, prev.BarSell) + ")"
		}
		if q.Estimated {
			s += " <i>(ประเมิน)</i>"
		}
		parts = append(parts, s)
	}
	if q.OrnSell > 0 {
		parts = append(parts, "รูปพรรณ ขาย "+money(q.OrnSell, 0))
	}
	return "🥇 ทอง: " + strings.Join(parts, " · ")
}

// goldTable is the full price block shown in the gold view.
func goldTable(q, prev *GoldQuote) string {
	if q == nil || (q.SpotUSD == 0 && q.BarSell == 0) {
		return "<i>ดึงราคาไม่ได้</i>"
	}
	return strings.TrimSpace(goldMarkdown(q, prev, true))
}

// goldMarkdown renders the price block. asHTML=false gives the markdown
// injected into the stored GOLD section (read by the dashboard too).
func goldMarkdown(q, prev *GoldQuote, asHTML bool) string {
	bold := func(s string) string {
		if asHTML {
			return "<b>" + s + "</b>"
		}
		return "**" + s + "**"
	}
	var lines []string
	if q.BarSell > 0 && !q.Estimated {
		l := fmt.Sprintf("* %s ขาย %s · รับซื้อ %s", bold("ทองแท่ง 96.5%:"), money(q.BarSell, 0), money(q.BarBuy, 0))
		if prev != nil && prev.BarSell > 0 {
			l += " (" + diff(q.BarSell, prev.BarSell) + ")"
		}
		lines = append(lines, l)
		if q.OrnSell > 0 {
			lines = append(lines, fmt.Sprintf("* %s ขาย %s · รับซื้อ %s", bold("ทองรูปพรรณ:"), money(q.OrnSell, 0), money(q.OrnBuy, 2)))
		}
		if q.AnnouncedAt != "" {
			lines = append(lines, "* ประกาศสมาคมค้าทองคำ "+q.AnnouncedAt)
		}
	} else if q.BarSell > 0 {
		lines = append(lines, fmt.Sprintf("* %s ~%s บาท (ราคาประเมินจากราคาโลก ไม่ใช่ราคาประกาศสมาคม)", bold("ทองแท่ง 96.5%:"), money(q.BarSell, 0)))
	} else {
		lines = append(lines, "* "+bold("ราคาทองไทย:")+" ดึงราคาไม่ได้")
	}
	if q.SpotUSD > 0 {
		l := fmt.Sprintf("* %s $%s/oz", bold("ทองโลก (spot):"), money(q.SpotUSD, 2))
		if prev != nil && prev.SpotUSD > 0 {
			l += " (" + pctChange(q.SpotUSD, prev.SpotUSD) + ")"
		}
		lines = append(lines, l)
	}
	out := strings.Join(lines, "\n")
	if asHTML {
		out = strings.ReplaceAll(out, "* ", "• ")
	}
	return out
}

// injectGold puts the markdown price block at the top of the GOLD section.
func injectGold(summary string, q GoldQuote, prev *GoldQuote) string {
	block := goldMarkdown(&q, prev, false)
	const marker = "===SECTION:GOLD==="
	if i := strings.Index(summary, marker); i >= 0 {
		j := i + len(marker)
		return summary[:j] + "\n" + block + "\n" + summary[j:]
	}
	return strings.TrimRight(summary, "\n") + "\n" + marker + "\n" + block + "\n"
}

func money(v float64, decimals int) string {
	s := strconv.FormatFloat(math.Abs(v), 'f', decimals, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if v < 0 {
		return "-" + b.String() + frac
	}
	return b.String() + frac
}

func diff(now, before float64) string {
	d := now - before
	switch {
	case math.Abs(d) < 0.5:
		return "เท่าเดิม"
	case d > 0:
		return "+" + money(d, 0)
	default:
		return "−" + money(-d, 0)
	}
}

func pctChange(now, before float64) string {
	p := (now - before) / before * 100
	if math.Abs(p) < 0.05 {
		return "เท่าเดิม"
	}
	if p > 0 {
		return fmt.Sprintf("+%.1f%%", p)
	}
	return fmt.Sprintf("−%.1f%%", -p)
}

var thaiMonthsShort = []string{"", "ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
var thaiWeekdaysShort = []string{"อา.", "จ.", "อ.", "พ.", "พฤ.", "ศ.", "ส."}

func thaiDate(t time.Time) string {
	if loc, err := time.LoadLocation("Asia/Bangkok"); err == nil {
		t = t.In(loc)
	}
	return fmt.Sprintf("%s %d %s", thaiWeekdaysShort[t.Weekday()], t.Day(), thaiMonthsShort[t.Month()])
}

// recategorizeGold moves gold-related items into their own "Gold" category.
func recategorizeGold(items []NewsItem) []NewsItem {
	for i := range items {
		text := strings.ToLower(items[i].Title + " " + items[i].Description)
		if strings.Contains(text, "ทอง") || strings.Contains(text, "gold") {
			items[i].Category = "Gold"
		}
	}
	return items
}
