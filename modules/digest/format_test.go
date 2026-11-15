package digest

import (
	"strings"
	"testing"
)

func TestSplitSectionsWithMarkers(t *testing.T) {
	raw := `===SECTION:AI===
🤖 **อัปเดต AI & เทคโนโลยีเด่น**
* **Nvidia:** ชิปใหม่แรงขึ้น 2 เท่า
ความยาว 2 บรรทัดสรุป

===SECTION:POLITICS===
🏛️ **สรุปประเด็นการเมือง**
* **นโยบายเศรษฐกิจ:** รัฐบาลแถลงมาตรการใหม่

===SECTION:TRIVIA===
💡 **สาระน่ารู้ประจำวัน**
* **เรื่องน่ารู้:** ท้องฟ้าเป็นสีฟ้าเพราะการกระเจิงของแสง`

	sections := SplitSections(raw)
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}

	if sections[0].ID != "ai" {
		t.Errorf("expected section 0 ID 'ai', got %q", sections[0].ID)
	}
	if sections[1].ID != "politics" {
		t.Errorf("expected section 1 ID 'politics', got %q", sections[1].ID)
	}
	if sections[2].ID != "trivia" {
		t.Errorf("expected section 2 ID 'trivia', got %q", sections[2].ID)
	}

	msg1 := FormatTelegramSection(sections[0], 1, 3)
	if !strings.Contains(msg1, "(1/3)") {
		t.Errorf("expected msg1 to contain (1/3), got %s", msg1)
	}
	if !strings.Contains(msg1, "• <b>Nvidia:</b>") {
		t.Errorf("expected msg1 to format bullet, got %s", msg1)
	}
	if !strings.Contains(msg1, "───────────────────────") {
		t.Errorf("expected divider in msg1, got %s", msg1)
	}
}

func TestSplitSectionsFallback(t *testing.T) {
	raw := `### 🤖 1. อัปเดต AI
* **Nvidia:** ข้อความ

### 🏛️ 2. การเมือง
* **สภา:** ข้อความ

### 💡 3. สาระน่ารู้
* **เกร็ดความรู้:** ข้อความ`

	sections := SplitSections(raw)
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections from fallback, got %d", len(sections))
	}
	if sections[0].ID != "ai" {
		t.Errorf("expected section 0 ID 'ai', got %q", sections[0].ID)
	}
}

func TestFormatTelegramDigestEmpty(t *testing.T) {
	if got := FormatTelegramDigest(""); got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}
