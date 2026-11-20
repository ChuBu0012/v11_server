package digest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const taggedSummary = `===SECTION:OVERVIEW===
📈 หุ้น: SET ปิดลบเล็กน้อยจากแรงขายกลุ่มพลังงาน
🤖 AI: OpenAI เปิดตัวโมเดลใหม่
🏛️ การเมือง: สภาถกงบประมาณ
💡 สาระ: ทำไมท้องฟ้าเป็นสีฟ้า
===SECTION:AI===
🤖 **อัปเดต AI**
* **โมเดลใหม่:** รายละเอียด
===SECTION:POLITICS===
🏛️ **การเมือง**
* **งบประมาณ:** รายละเอียด
===SECTION:STOCKS===
📈 **ข่าวหุ้น**
* **SET:** ปิดลบ
===SECTION:GOLD===
🥇 **ข่าวทอง**
* **ทองโลก:** ทรงตัว
===SECTION:TRIVIA===
💡 **สาระน่ารู้**
* **ท้องฟ้า:** แสงกระเจิง`

type fakeGold struct{ q GoldQuote }

func (f fakeGold) Fetch(context.Context) GoldQuote { return f.q }

type fakeMessenger struct {
	texts   []string
	buttons [][][]Button
}

func (m *fakeMessenger) SendMenu(ctx context.Context, text string, buttons [][]Button) error {
	m.texts = append(m.texts, text)
	m.buttons = append(m.buttons, buttons)
	return nil
}

func geminiReturning(text string) *GeminiClient {
	resp, _ := json.Marshal(geminiResponse{Candidates: []geminiCandidate{{Content: geminiContent{Parts: []geminiPart{{Text: text}}}}}})
	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(resp)), Header: make(http.Header)}, nil
	})
	return NewGeminiClient("KEY", WithBaseURL("http://mock.gemini"), WithHTTPClient(client))
}

var sampleQuote = GoldQuote{SpotUSD: 3541.15, BarSell: 57600, BarBuy: 57400, OrnSell: 58400, OrnBuy: 56258.76, AnnouncedAt: "20/11/2569 เวลา 17:29 น."}

func TestRecategorizeGold(t *testing.T) {
	items := recategorizeGold([]NewsItem{
		{Category: "Stocks", Title: "ราคาทองวันนี้พุ่ง"},
		{Category: "AI", Title: "Gold standard for AI evals"},
		{Category: "Stocks", Title: "SET ปิดลบ"},
	})
	if items[0].Category != "Gold" || items[1].Category != "Gold" || items[2].Category != "Stocks" {
		t.Fatalf("categories = %s %s %s", items[0].Category, items[1].Category, items[2].Category)
	}
}

func TestMenuButtonsDataUnder64(t *testing.T) {
	rows := MenuButtons(9223372036854775807, "politics")
	keys := 0
	for _, row := range rows {
		for _, b := range row {
			keys++
			if len(b.Data) > 64 {
				t.Fatalf("callback data too long: %q", b.Data)
			}
			if strings.HasSuffix(b.Data, ":politics") && !strings.HasPrefix(b.Text, "• ") {
				t.Fatalf("active button not marked: %q", b.Text)
			}
		}
	}
	if keys != 6 {
		t.Fatalf("buttons = %d, want 6", keys)
	}
}

func TestParseCallback(t *testing.T) {
	if id, key, ok := ParseCallback("dg:42:gold"); !ok || id != 42 || key != "gold" {
		t.Fatalf("parse = %d %s %v", id, key, ok)
	}
	for _, bad := range []string{"dg:abc:xyz", "dg:1:hack", "qs:1:gold", "dg:1", "dg:-3:ai"} {
		if _, _, ok := ParseCallback(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}

func TestRenderOverviewIncludesGoldAndDisclaimer(t *testing.T) {
	gj, _ := json.Marshal(sampleQuote)
	run := &RunRecord{ID: 7, SummaryMD: taggedSummary, GoldJSON: string(gj)}
	prev := GoldQuote{SpotUSD: 3500, BarSell: 57450, OrnSell: 58250}
	out := RenderView(run, "overview", &prev)
	for _, want := range []string{"57,600", "58,400", "$3,541", "+150", "SET ปิดลบเล็กน้อย", "OpenAI", "ไม่ใช่คำแนะนำการลงทุน"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview missing %q:\n%s", want, out)
		}
	}
}

func TestRenderOverviewNoGold(t *testing.T) {
	out := RenderView(&RunRecord{ID: 7, SummaryMD: taggedSummary}, "overview", nil)
	if !strings.Contains(out, "ดึงราคาไม่ได้") || !strings.Contains(out, "ไม่ใช่คำแนะนำการลงทุน") {
		t.Fatalf("overview without gold:\n%s", out)
	}
}

func TestRenderSectionOldRun(t *testing.T) {
	old := &RunRecord{ID: 3, SummaryMD: "===SECTION:AI===\n🤖 **AI**\n* **x:** y"}
	if out := RenderView(old, "ai", nil); !strings.Contains(out, "y") {
		t.Fatalf("ai view = %q", out)
	}
	if out := RenderView(old, "gold", nil); !strings.Contains(out, "ดึงราคาไม่ได้") {
		t.Fatalf("gold view on old run = %q", out)
	}
	if out := RenderView(old, "stocks", nil); !strings.Contains(out, "ไม่มีข่าว") {
		t.Fatalf("missing section view = %q", out)
	}
}

func TestEstimatedQuoteIsLabelled(t *testing.T) {
	q := GoldQuote{SpotUSD: 3541, BarSell: 56850, Estimated: true}
	gj, _ := json.Marshal(q)
	out := RenderView(&RunRecord{ID: 1, SummaryMD: taggedSummary, GoldJSON: string(gj)}, "gold", nil)
	if !strings.Contains(out, "ราคาประเมินจากราคาโลก") {
		t.Fatalf("estimate not labelled:\n%s", out)
	}
}

func TestServicePublishesOneMessage(t *testing.T) {
	store, _ := NewStore(newTestDB(t))
	m := &fakeMessenger{}
	notifier := &mockNotifier{}
	svc := NewService(store, geminiReturning(taggedSummary), notifier, WithFeeds(nil), WithGoldSource(fakeGold{sampleQuote}), WithMessenger(m))
	run, err := svc.GenerateDigest(context.Background(), true)
	if err != nil || run == nil {
		t.Fatalf("generate: %v %+v", err, run)
	}
	if len(m.texts) != 1 || len(notifier.sent) != 0 {
		t.Fatalf("messenger sent %d, notifier sent %d; want exactly one menu message", len(m.texts), len(notifier.sent))
	}
	if !strings.Contains(m.texts[0], "57,600") || len(m.buttons[0]) == 0 {
		t.Fatalf("menu message = %q buttons=%v", m.texts[0], m.buttons[0])
	}
	stored, _ := store.GetRun(context.Background(), run.ID)
	if stored == nil || !strings.Contains(stored.SummaryMD, "57,600") || stored.GoldJSON == "" {
		t.Fatalf("stored run lacks injected gold block: %+v", stored)
	}
	text, buttons, ok := svc.HandleCallback(context.Background(), MenuButtons(run.ID, "")[0][0].Data)
	if !ok || !strings.Contains(text, "57,400") || len(buttons) == 0 {
		t.Fatalf("gold callback = %v %q", ok, text)
	}
}

func TestServiceFallsBackToNotifier(t *testing.T) {
	store, _ := NewStore(newTestDB(t))
	notifier := &mockNotifier{}
	svc := NewService(store, geminiReturning(taggedSummary), notifier, WithFeeds(nil), WithGoldSource(fakeGold{sampleQuote}))
	if _, err := svc.GenerateDigest(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || !strings.Contains(notifier.sent[0], "57,600") {
		t.Fatalf("notifier sent %d: %v", len(notifier.sent), notifier.sent)
	}
}

func TestHandleCallbackRejectsBadData(t *testing.T) {
	store, _ := NewStore(newTestDB(t))
	svc := NewService(store, geminiReturning(taggedSummary), nil, WithFeeds(nil), WithGoldSource(fakeGold{}))
	if _, _, ok := svc.HandleCallback(context.Background(), "dg:abc:xyz"); ok {
		t.Fatal("bad data accepted")
	}
	if _, _, ok := svc.HandleCallback(context.Background(), "dg:999:ai"); ok {
		t.Fatal("unknown run accepted")
	}
}
