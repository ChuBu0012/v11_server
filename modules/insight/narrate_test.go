package insight

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleFacts() Facts {
	return Facts{
		Date: "2026-11-21",
		Server: ServerFacts{BatteryRate: 4.2, BatteryBaseline: 1.1, BatteryVerdict: VerdictAbnormal, RAMNow: 62.5, RAMAvg: 60.1, RAMBaseline: 58, RAMVerdict: VerdictNormal,
			DiskFree: 73.4, DiskPerDay: 0.2, DiskVerdict: VerdictNormal, Attention: []string{"แบตเตอรี่ v11 ลดเร็วผิดปกติ"}},
		Me:       PersonFacts{Name: "Chubu", HasData: true, WalkKm: 2.1, TotalKm: 18.4, MovingMin: 95, Places: []string{"บ้าน", "ที่ทำงาน"}, MinBattery: 40, WalkKmMedian7: 1.5},
		Friend:   PersonFacts{Name: "Furturia", HasData: false},
		Together: TogetherFacts{HasData: false},
		Today:    TodayFacts{Count: 2, NextAt: time.Date(2026, 11, 21, 10, 0, 0, 0, bkk), NextTitle: "ประชุมทีม"},
	}
}

type stubGen struct {
	out   string
	err   error
	calls int
}

func (s *stubGen) Generate(ctx context.Context, system, user string) (string, error) {
	s.calls++
	return s.out, s.err
}

func TestTemplateContainsSections(t *testing.T) {
	tpl := Template(sampleFacts())
	for _, want := range []string{"🛡️", "🚶", "🤝", "📅", "4.2", "18.4", "2.1", "73.4", "บ้าน", "ประชุมทีม", "10:00"} {
		if !strings.Contains(tpl, want) {
			t.Errorf("template missing %q:\n%s", want, tpl)
		}
	}
}

func TestTemplateEmptySections(t *testing.T) {
	tpl := Template(sampleFacts())
	if !strings.Contains(tpl, "ยังไม่มีพิกัดเมื่อวาน") {
		t.Fatalf("empty friend section not explained:\n%s", tpl)
	}
	if strings.Contains(tpl, "NaN") {
		t.Fatal("template contains NaN")
	}
}

func TestNarratorUsesFirstValid(t *testing.T) {
	f := sampleFacts()
	good := Template(f) + "\n💡 เดินมากกว่าปกติ"
	or := &stubGen{out: good}
	text, source := Narrate(context.Background(), f, []Provider{{Name: "openrouter", Gen: or, Timeout: time.Second}, {Name: "gemini", Gen: &stubGen{err: errors.New("x")}, Timeout: time.Second}})
	if source != "openrouter" || text != good {
		t.Fatalf("source=%s text=%q", source, text)
	}
}

func TestNarratorRejectsChangedNumbers(t *testing.T) {
	f := sampleFacts()
	bad := strings.Replace(Template(f), "18.4", "19.0", 1)
	gem := &stubGen{out: Template(f)}
	_, source := Narrate(context.Background(), f, []Provider{{Name: "openrouter", Gen: &stubGen{out: bad}, Timeout: time.Second}, {Name: "gemini", Gen: gem, Timeout: time.Second}})
	if source != "gemini" {
		t.Fatalf("source = %s, want gemini after openrouter changed a number", source)
	}
}

func TestNarratorFallsBackToTemplate(t *testing.T) {
	f := sampleFacts()
	text, source := Narrate(context.Background(), f, []Provider{{Name: "openrouter", Gen: &stubGen{err: errors.New("down")}, Timeout: time.Second}, {Name: "gemini", Gen: &stubGen{err: errors.New("down")}, Timeout: time.Second}})
	if source != "template" || text != Template(f) {
		t.Fatalf("source=%s", source)
	}
	if _, source := Narrate(context.Background(), f, nil); source != "template" {
		t.Fatalf("no providers source = %s", source)
	}
}

func TestNarratorRejectsDisallowedTags(t *testing.T) {
	f := sampleFacts()
	withLink := Template(f) + `<a href="https://x">x</a>`
	_, source := Narrate(context.Background(), f, []Provider{{Name: "openrouter", Gen: &stubGen{out: withLink}, Timeout: time.Second}})
	if source != "template" {
		t.Fatalf("source = %s, want template (disallowed tag)", source)
	}
}
