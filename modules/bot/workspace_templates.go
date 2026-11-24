package bot

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"
)

// BotNoteItem represents note data for bot templates.
type BotNoteItem struct {
	ID        string
	Title     string
	Content   string
	Tags      []string
	IsPinned  bool
	UpdatedAt time.Time
}

// WorkspaceBotService defines operations needed by the bot router.
type WorkspaceBotService interface {
	CreateNote(ctx context.Context, text string) (*BotNoteItem, error)
	ListNotes(ctx context.Context, tag string, limit int) ([]BotNoteItem, error)
	GetNote(ctx context.Context, id string) (*BotNoteItem, error)
	DeleteNote(ctx context.Context, id string) error
	SetNotePinned(ctx context.Context, id string, pinned bool) error
}

func formatTagsHTML(tags []string) string {
	if len(tags) == 0 {
		return "<i>ไม่มีแท็ก</i>"
	}
	var tagPills []string
	for _, t := range tags {
		clean := html.EscapeString(strings.TrimPrefix(t, "#"))
		tagPills = append(tagPills, fmt.Sprintf("<code>#%s</code>", clean))
	}
	return strings.Join(tagPills, " ")
}

func previewContent(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	var previewLines []string
	count := 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			previewLines = append(previewLines, html.EscapeString(trimmed))
			count++
			if count >= maxLines {
				break
			}
		}
	}
	if len(previewLines) == 0 {
		return "<i>ไม่มีเนื้อหา</i>"
	}
	res := strings.Join(previewLines, "\n")
	if len(lines) > maxLines {
		res += "\n..."
	}
	return res
}

// FormatNoteCreated builds the confirmation message and keyboard for /note.
func FormatNoteCreated(note *BotNoteItem) (string, *InlineKeyboardMarkup) {
	title := html.EscapeString(note.Title)
	tagsHTML := formatTagsHTML(note.Tags)
	preview := previewContent(note.Content, 4)

	text := fmt.Sprintf("📝 <b>บันทึกโน้ตสำเร็จ</b>\n"+
		"<b>หัวข้อ:</b> %s\n"+
		"<b>หมวดหมู่:</b> %s\n"+
		"<blockquote>%s</blockquote>\n"+
		"<i>บันทึกใน Pocket Sentinel เรียบร้อย</i>", title, tagsHTML, preview)

	pinBtn := InlineKeyboardButton{Text: "📌 ปักหมุด", CallbackData: "wn:pin:" + note.ID}
	if note.IsPinned {
		pinBtn = InlineKeyboardButton{Text: "📍 ปลดหมุด", CallbackData: "wn:unpin:" + note.ID}
	}

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🗑️ ลบ", CallbackData: "wn:del:" + note.ID},
				pinBtn,
			},
		},
	}
	return text, keyboard
}

// FormatNoteList builds the message and keyboard for /notes.
func FormatNoteList(notes []BotNoteItem, tag string) (string, *InlineKeyboardMarkup) {
	tagLabel := ""
	if tag != "" {
		tagLabel = fmt.Sprintf(" (แท็ก: <code>#%s</code>)", html.EscapeString(tag))
	}

	if len(notes) == 0 {
		msg := fmt.Sprintf("📭 <b>ไม่พบบันทึกโน้ต</b>%s\n<i>คุณสามารถสร้างโน้ตใหม่ด้วยคำสั่ง:</i>\n<code>/note ข้อความที่ต้องการบันทึก</code>", tagLabel)
		return msg, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📚 <b>บันทึกโน้ตล่าสุด%s</b> (%d รายการ)\n\n", tagLabel, len(notes)))

	var keyboardRows [][]InlineKeyboardButton

	for idx, n := range notes {
		pinMark := ""
		if n.IsPinned {
			pinMark = "📌 "
		}
		title := html.EscapeString(n.Title)
		tagText := ""
		if len(n.Tags) > 0 {
			tagText = " [" + html.EscapeString(strings.Join(n.Tags, ", ")) + "]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s<b>%s</b>%s\n", idx+1, pinMark, title, tagText))

		btnTitle := n.Title
		if len([]rune(btnTitle)) > 14 {
			btnTitle = string([]rune(btnTitle)[:12]) + ".."
		}
		keyboardRows = append(keyboardRows, []InlineKeyboardButton{
			{Text: fmt.Sprintf("📄 %s", btnTitle), CallbackData: "wn:view:" + n.ID},
			{Text: "🗑️", CallbackData: "wn:del:" + n.ID},
		})
	}

	sb.WriteString("\n<i>แตะปุ่มเพื่อเปิดอ่านหรือลบโน้ต</i>")

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: keyboardRows,
	}
	return sb.String(), keyboard
}

// FormatNoteDetail builds the full note view for wn:view:<id>.
func FormatNoteDetail(note *BotNoteItem) (string, *InlineKeyboardMarkup) {
	pinMark := ""
	if note.IsPinned {
		pinMark = "📌 "
	}
	title := html.EscapeString(note.Title)
	tagsHTML := formatTagsHTML(note.Tags)
	content := html.EscapeString(note.Content)
	if content == "" {
		content = "<i>ไม่มีเนื้อหา</i>"
	}

	text := fmt.Sprintf("📝 %s<b>%s</b>\n"+
		"<b>หมวดหมู่:</b> %s\n"+
		"<b>แก้ไขล่าสุด:</b> <code>%s</code>\n\n"+
		"<blockquote>%s</blockquote>",
		pinMark, title, tagsHTML, note.UpdatedAt.Format("02/01 15:04"), content)

	pinBtn := InlineKeyboardButton{Text: "📌 ปักหมุด", CallbackData: "wn:pin:" + note.ID}
	if note.IsPinned {
		pinBtn = InlineKeyboardButton{Text: "📍 ปลดหมุด", CallbackData: "wn:unpin:" + note.ID}
	}

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🗑️ ลบ", CallbackData: "wn:del:" + note.ID},
				pinBtn,
				{Text: "📚 รายการโน้ต", CallbackData: "wn:list"},
			},
		},
	}
	return text, keyboard
}
