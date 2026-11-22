package bot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"v11-server/shared/ai"
	"v11-server/shared/db"
)

type mockAskEngine struct {
	hasKey      bool
	modelCalled string
	response    string
	err         error
}

func (m *mockAskEngine) GenerateWithModel(ctx context.Context, model string, req ai.Request) (string, string, error) {
	m.modelCalled = model
	if m.err != nil {
		return "", "", m.err
	}
	prov := model
	if prov == "" || prov == "auto" {
		prov = "groq"
	}
	return m.response, prov, nil
}

func (m *mockAskEngine) HasAPIKey() bool {
	return m.hasKey
}

func TestIsDynamicQuery(t *testing.T) {
	dynamicPrompts := []string{
		"แบตเตอรี่เหลือเท่าไหร่",
		"เช็ค RAM ปัจจุบันของ v11 หน่อย",
		"พิกัดล่าสุดอยู่ที่ไหน",
		"โควต้า ai ตอนนี้เหลือเท่าไหร่",
		"git status ล่าสุด",
		"ราคาทองวันนี้เป็นอย่างไร",
		"ราคาหุ้น ptt ตอนนี้",
	}
	for _, p := range dynamicPrompts {
		if !IsDynamicQuery(p) {
			t.Errorf("Expected IsDynamicQuery(%q) to be true, got false", p)
		}
	}

	staticPrompts := []string{
		"ทำไมท้องฟ้าถึงเป็นสีฟ้า",
		"อธิบายทฤษฎีสัมพัทธภาพ",
		"แปลง 100 USD เป็น THB",
		"Golang interface ทำงานอย่างไร",
	}
	for _, p := range staticPrompts {
		if IsDynamicQuery(p) {
			t.Errorf("Expected IsDynamicQuery(%q) to be false, got true", p)
		}
	}
}

func TestRequiresAgent(t *testing.T) {
	agentPrompts := []string{
		"ช่วยแก้บั๊กใน runner.go หน่อย",
		"รันคำสั่ง go test ./...",
		"เขียนไฟล์ config.json ขึ้นมาใหม่",
		"deploy ระบบไปยัง vivo v11",
		"ตรวจสอบโค้ดใน git commit ล่าสุด",
	}
	for _, p := range agentPrompts {
		if !RequiresAgent(p) {
			t.Errorf("Expected RequiresAgent(%q) to be true, got false", p)
		}
	}

	knowledgePrompts := []string{
		"ทำไมโลกถึงหมุนรอบตัวเอง",
		"สรุปประวัติศาสตร์สงครามโลก",
		"สูตรทำต้มยำกุ้งมีอะไรบ้าง",
	}
	for _, p := range knowledgePrompts {
		if RequiresAgent(p) {
			t.Errorf("Expected RequiresAgent(%q) to be false, got true", p)
		}
	}
}

func TestAskTriageRouter_Triage(t *testing.T) {
	mockEng := &mockAskEngine{hasKey: true, response: "FAST"}
	router := NewAskTriageRouter(mockEng, nil)
	ctx := context.Background()

	// 1. Explicit model = agy
	v, err := router.Triage(ctx, "คำถามทั่วไป", "agy")
	if err != nil || v != VerdictAgent {
		t.Errorf("Explicit agy want AGY, got %v, err %v", v, err)
	}

	// 2. Explicit model = groq
	v, err = router.Triage(ctx, "แก้โค้ดหน่อย", "groq")
	if err != nil || v != VerdictFastLLM {
		t.Errorf("Explicit groq want FAST, got %v, err %v", v, err)
	}

	// 3. Auto model + Heuristic agent query
	v, err = router.Triage(ctx, "แก้บั๊กใน runner.go", "auto")
	if err != nil || v != VerdictAgent {
		t.Errorf("Auto + Agent query want AGY, got %v, err %v", v, err)
	}

	// 4. Auto model + General knowledge query
	mockEng.response = "FAST"
	v, err = router.Triage(ctx, "ทำไมท้องฟ้าสีฟ้า", "auto")
	if err != nil || v != VerdictFastLLM {
		t.Errorf("Auto + General question want FAST, got %v, err %v", v, err)
	}

	// 5. Auto model + Ambiguous query classified as AGY by AI
	mockEng.response = "AGY"
	v, err = router.Triage(ctx, "ทำไมระบบถึงค้าง", "auto")
	if err != nil || v != VerdictAgent {
		t.Errorf("AI classified as AGY want AGY, got %v, err %v", v, err)
	}
}

func TestAskTriageRouter_ExecuteFast_WithCache(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ask-router-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	cacheStore := NewAskCacheStore(database)
	mockEng := &mockAskEngine{hasKey: true, response: "ท้องฟ้าเป็นสีฟ้าเพราะการกระเจิงของแสง Rayleigh"}
	router := NewAskTriageRouter(mockEng, cacheStore)
	ctx := context.Background()

	opts := AskOptions{
		Model:       "auto",
		Effort:      "medium",
		NoCache:     false,
		CleanPrompt: "ทำไมท้องฟ้าเป็นสีฟ้า",
	}

	// First execution: Cache Miss -> Fast LLM
	res1, err := router.ExecuteFast(ctx, opts)
	if err != nil {
		t.Fatalf("ExecuteFast failed: %v", err)
	}
	if res1.IsCached {
		t.Errorf("Expected first run to not be cached")
	}
	if !strings.Contains(res1.ResponseText, "Rayleigh") {
		t.Errorf("Unexpected response: %s", res1.ResponseText)
	}

	// Second execution: Cache Hit
	res2, err := router.ExecuteFast(ctx, opts)
	if err != nil {
		t.Fatalf("ExecuteFast second run failed: %v", err)
	}
	if !res2.IsCached {
		t.Errorf("Expected second run to be cached")
	}
	if res2.ResponseText != res1.ResponseText {
		t.Errorf("Cached text mismatch: %s != %s", res2.ResponseText, res1.ResponseText)
	}

	// Third execution: With --no-cache
	optsNoCache := opts
	optsNoCache.NoCache = true
	res3, err := router.ExecuteFast(ctx, optsNoCache)
	if err != nil {
		t.Fatalf("ExecuteFast no-cache failed: %v", err)
	}
	if res3.IsCached {
		t.Errorf("Expected no-cache run to not be cached")
	}
}
