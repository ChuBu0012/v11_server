package insight

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

// TextGenerator is satisfied by shared/ai.Client (OpenRouter) and digest.GeminiClient.
type TextGenerator interface {
	Generate(ctx context.Context, system, user string) (string, error)
}

// Provider is one step of the narration fallback chain.
type Provider struct {
	Name    string
	Gen     TextGenerator
	Timeout time.Duration
}

const (
	sourceTemplate = "template"
	maxNarration   = 3500
)

const narrationSystem = `คุณคือผู้ช่วยของ Pocket Sentinel เขียนสรุปตอนเช้าให้เจ้าของเครื่องอ่านบน Telegram
งาน: เรียบเรียงรายงานด้านล่างใหม่เป็นภาษาไทยที่เป็นกันเองและกระชับ
กฎ:
- คงตัวเลขทุกตัวไว้ตรงตามต้นฉบับทุกตัวอักษร ห้ามคำนวณใหม่ ห้ามปัดเศษ
- คงลำดับหัวข้อ 🛡️ 🚶 🤝 📅 และอีโมจิหัวข้อไว้
- เพิ่มบรรทัดที่ขึ้นต้นด้วย 💡 ได้ไม่เกิน 2 บรรทัดท้ายรายงาน โดยดึงจากข้อมูลในรายงานเท่านั้น
- ใช้ HTML ของ Telegram ได้แค่ <b> <i> <code> ห้ามใช้แท็กอื่น ห้ามใช้ Markdown
- ตอบเฉพาะตัวรายงาน ไม่ต้องมีคำนำหรือคำลงท้าย`

// Narrate tries each provider in order and returns the first narration that
// keeps every number of the template; otherwise the template itself.
func Narrate(ctx context.Context, f Facts, providers []Provider) (string, string) {
	tpl := Template(f)
	for _, p := range providers {
		if p.Gen == nil {
			continue
		}
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = 15 * time.Second
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		out, err := p.Gen.Generate(callCtx, narrationSystem, tpl)
		cancel()
		if err != nil {
			continue
		}
		out = strings.TrimSpace(stripCodeFence(out))
		if validNarration(out, tpl) {
			return out, p.Name
		}
	}
	return tpl, sourceTemplate
}

var (
	numberToken = regexp.MustCompile(`\d+(?:[.:]\d+)?`)
	anyTag      = regexp.MustCompile(`<[^>]*>`)
	allowedTag  = regexp.MustCompile(`^</?(b|i|code)>$`)
)

// validNarration accepts AI text only when it is short enough, uses only
// b/i/code tags, and still contains every number token of the template.
func validNarration(ai, template string) bool {
	if ai == "" || len([]rune(ai)) > maxNarration {
		return false
	}
	for _, tag := range anyTag.FindAllString(ai, -1) {
		if !allowedTag.MatchString(tag) {
			return false
		}
	}
	for _, num := range numberToken.FindAllString(template, -1) {
		if !strings.Contains(ai, num) {
			return false
		}
	}
	return true
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```html")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return s
}

var thaiMonths = []string{"", "ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
var thaiWeekdays = []string{"อา.", "จ.", "อ.", "พ.", "พฤ.", "ศ.", "ส."}

// Template renders the complete, deterministic report as Telegram HTML.
func Template(f Facts) string {
	var b strings.Builder
	title := "สรุปเช้า"
	if d, err := time.ParseInLocation("2006-01-02", f.Date, bangkok()); err == nil {
		title = fmt.Sprintf("สรุปเช้า %s %d %s", thaiWeekdays[d.Weekday()], d.Day(), thaiMonths[d.Month()])
	}
	fmt.Fprintf(&b, "☀️ <b>%s</b>\n\n", title)

	// Server.
	s := f.Server
	status := "ปกติ"
	if len(s.Attention) > 0 {
		status = "ต้องดู"
	}
	fmt.Fprintf(&b, "🛡️ <b>เครื่อง v11 · %s</b>\n", status)
	switch {
	case s.AlwaysCharging:
		b.WriteString("แบต: ชาร์จอยู่ตลอด\n")
	case s.BatteryVerdict == VerdictAbnormal:
		fmt.Fprintf(&b, "แบต: ลด %.1f%%/ชม. (ปกติ ~%.1f%%/ชม.) ⚠️ เร็วกว่าปกติ\n", s.BatteryRate, s.BatteryBaseline)
	case s.BatteryVerdict == VerdictNormal:
		fmt.Fprintf(&b, "แบต: ลด %.1f%%/ชม. ปกติ\n", s.BatteryRate)
	default:
		b.WriteString("แบต: ข้อมูลยังไม่พอเทียบ\n")
	}
	if s.RAMVerdict != VerdictUnknown {
		warn := ""
		if s.RAMVerdict == VerdictAbnormal {
			warn = " ⚠️"
		}
		fmt.Fprintf(&b, "RAM: ใช้ %.1f%% (เฉลี่ย 24 ชม. %.1f%%)%s\n", s.RAMNow, s.RAMAvg, warn)
	}
	if s.DiskVerdict != VerdictUnknown {
		warn := ""
		if s.DiskVerdict == VerdictAbnormal {
			warn = " ⚠️"
		}
		fmt.Fprintf(&b, "ดิสก์: เหลือ %.1f GB · ลด %.1f GB/วัน%s\n", s.DiskFree, s.DiskPerDay, warn)
	}

	// Movement.
	b.WriteString("\n🚶 <b>เมื่อวาน</b>\n")
	for _, p := range []PersonFacts{f.Me, f.Friend} {
		writePerson(&b, p)
	}

	// Together.
	b.WriteString("\n🤝 <b>ด้วยกัน</b>\n")
	t := f.Together
	switch {
	case !t.HasData:
		b.WriteString("ยังไม่มีพิกัดเมื่อวานของทั้งสองคน\n")
	case t.Minutes == 0:
		b.WriteString("เมื่อวานไม่ได้อยู่ด้วยกัน\n")
	default:
		fmt.Fprintf(&b, "อยู่ด้วยกัน %s", durationThai(t.Minutes))
		if t.MedianMinutes7 > 0 {
			fmt.Fprintf(&b, " (ปกติ ~%s)", durationThai(t.MedianMinutes7))
		}
		if !t.LongestStart.IsZero() {
			fmt.Fprintf(&b, " · นานสุด %s–%s", t.LongestStart.In(bangkok()).Format("15:04"), t.LongestEnd.In(bangkok()).Format("15:04"))
		}
		b.WriteString("\n")
	}

	// Today.
	b.WriteString("\n📅 <b>วันนี้</b>\n")
	if f.Today.Count == 0 {
		b.WriteString("ไม่มีรายการเตือนที่เหลือวันนี้")
	} else {
		fmt.Fprintf(&b, "%d รายการ · ถัดไป %s %s", f.Today.Count, f.Today.NextAt.In(bangkok()).Format("15:04"), html.EscapeString(f.Today.NextTitle))
	}
	return b.String()
}

func writePerson(b *strings.Builder, p PersonFacts) {
	name := html.EscapeString(p.Name)
	if !p.HasData {
		fmt.Fprintf(b, "<b>%s</b>: ยังไม่มีพิกัดเมื่อวาน\n", name)
		return
	}
	fmt.Fprintf(b, "<b>%s</b>: เดิน %.1f กม. · รวม %.1f กม. · เคลื่อนที่ %s\n", name, p.WalkKm, p.TotalKm, durationThai(p.MovingMin))
	if len(p.Places) > 0 {
		escaped := make([]string, len(p.Places))
		for i, place := range p.Places {
			escaped[i] = html.EscapeString(place)
		}
		fmt.Fprintf(b, "ไป: %s\n", strings.Join(escaped, ", "))
	}
	if p.WalkKmMedian7 > 0 {
		fmt.Fprintf(b, "เดินปกติ ~%.1f กม./วัน\n", p.WalkKmMedian7)
	}
	if p.LowBattery {
		fmt.Fprintf(b, "แบตมือถือต่ำสุด %d%% ⚠️\n", p.MinBattery)
	}
}

func durationThai(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d นาที", minutes)
	}
	h, m := minutes/60, minutes%60
	if m == 0 {
		return fmt.Sprintf("%d ชม.", h)
	}
	return fmt.Sprintf("%d ชม. %d นาที", h, m)
}

func bangkok() *time.Location {
	if loc, err := time.LoadLocation("Asia/Bangkok"); err == nil {
		return loc
	}
	return time.FixedZone("Asia/Bangkok", 7*3600)
}
