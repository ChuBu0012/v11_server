package bot

import (
	"testing"
)

func TestParseAskInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantModel   string
		wantEffort  string
		wantNoCache bool
		wantPrompt  string
	}{
		{
			name:        "empty input",
			input:       "",
			wantModel:   "auto",
			wantEffort:  "medium",
			wantNoCache: false,
			wantPrompt:  "",
		},
		{
			name:        "whitespace only",
			input:       "   \t  \n ",
			wantModel:   "auto",
			wantEffort:  "medium",
			wantNoCache: false,
			wantPrompt:  "",
		},
		{
			name:        "plain prompt without tags",
			input:       "ทำไมท้องฟ้าถึงเป็นสีฟ้า",
			wantModel:   "auto",
			wantEffort:  "medium",
			wantNoCache: false,
			wantPrompt:  "ทำไมท้องฟ้าถึงเป็นสีฟ้า",
		},
		{
			name:        "bracket-free model only",
			input:       "groq ทำไมทองคำราคาขึ้น",
			wantModel:   "groq",
			wantEffort:  "medium",
			wantNoCache: false,
			wantPrompt:  "ทำไมทองคำราคาขึ้น",
		},
		{
			name:        "bracket-free model and effort",
			input:       "groq high อธิบายทฤษฎีควอนตัม",
			wantModel:   "groq",
			wantEffort:  "high",
			wantNoCache: false,
			wantPrompt:  "อธิบายทฤษฎีควอนตัม",
		},
		{
			name:        "bracket-free effort only",
			input:       "low สรุปข่าวสั้นๆ",
			wantModel:   "auto",
			wantEffort:  "low",
			wantNoCache: false,
			wantPrompt:  "สรุปข่าวสั้นๆ",
		},
		{
			name:        "bracket-free agy model",
			input:       "agy high ตรวจสอบ memory leak ใน runner.go",
			wantModel:   "agy",
			wantEffort:  "high",
			wantNoCache: false,
			wantPrompt:  "ตรวจสอบ memory leak ใน runner.go",
		},
		{
			name:        "case insensitivity",
			input:       "GEMINI Low แปลงเวลา 15:00 UTC",
			wantModel:   "gemini",
			wantEffort:  "low",
			wantNoCache: false,
			wantPrompt:  "แปลงเวลา 15:00 UTC",
		},
		{
			name:        "with --no-cache flag at end",
			input:       "groq อธิบาย Golang interfaces --no-cache",
			wantModel:   "groq",
			wantEffort:  "medium",
			wantNoCache: true,
			wantPrompt:  "อธิบาย Golang interfaces",
		},
		{
			name:        "with --fresh flag in middle",
			input:       "gemini high --fresh วิเคราะห์ข้อมูล",
			wantModel:   "gemini",
			wantEffort:  "high",
			wantNoCache: true,
			wantPrompt:  "วิเคราะห์ข้อมูล",
		},
		{
			name:        "legacy bracket effort [median]",
			input:       "[median] อธิบายภาพรวม Sentinel",
			wantModel:   "auto",
			wantEffort:  "medium",
			wantNoCache: false,
			wantPrompt:  "อธิบายภาพรวม Sentinel",
		},
		{
			name:        "legacy bracket effort [high]",
			input:       "[high] คิดเชิงลึก",
			wantModel:   "auto",
			wantEffort:  "high",
			wantNoCache: false,
			wantPrompt:  "คิดเชิงลึก",
		},
		{
			name:        "legacy bracket combined [agy:high]",
			input:       "[agy:high] รันเทสต์ทั้งหมด",
			wantModel:   "agy",
			wantEffort:  "high",
			wantNoCache: false,
			wantPrompt:  "รันเทสต์ทั้งหมด",
		},
		{
			name:        "legacy bracket model and effort separate",
			input:       "[groq] [low] ตอบสั้นๆ",
			wantModel:   "groq",
			wantEffort:  "low",
			wantNoCache: false,
			wantPrompt:  "ตอบสั้นๆ",
		},
		{
			name:        "flag --effort=max format",
			input:       "openrouter --effort=max คำนวณเลข",
			wantModel:   "openrouter",
			wantEffort:  "max",
			wantNoCache: false,
			wantPrompt:  "คำนวณเลข",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ParseAskInput(tt.input)
			if opts.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", opts.Model, tt.wantModel)
			}
			if opts.Effort != tt.wantEffort {
				t.Errorf("Effort = %q, want %q", opts.Effort, tt.wantEffort)
			}
			if opts.NoCache != tt.wantNoCache {
				t.Errorf("NoCache = %v, want %v", opts.NoCache, tt.wantNoCache)
			}
			if opts.CleanPrompt != tt.wantPrompt {
				t.Errorf("CleanPrompt = %q, want %q", opts.CleanPrompt, tt.wantPrompt)
			}
		})
	}
}
