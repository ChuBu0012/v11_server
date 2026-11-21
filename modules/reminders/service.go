package reminders

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"v11-server/modules/bot"
)

type Telegram interface {
	SendMessageSilent(context.Context, int64, string, *bot.InlineKeyboardMarkup) (*bot.Message, error)
	AnswerCallbackQuery(context.Context, string, string, bool) error
}

type Service struct {
	store    *Store
	telegram Telegram
	chatID   int64
}

func NewService(store *Store, telegram Telegram, chatID int64) *Service {
	return &Service{store: store, telegram: telegram, chatID: chatID}
}

func (s *Service) RunOnce(ctx context.Context, now time.Time) error {
	if s.telegram == nil || s.chatID == 0 {
		return nil
	}
	deliveries, err := s.store.Due(ctx, now)
	if err != nil {
		return err
	}
	for _, d := range deliveries {
		markup := scheduleKeyboard(d.ID)
		text := formatMessage(d)
		msg, sendErr := s.telegram.SendMessageSilent(ctx, s.chatID, text, markup)
		if sendErr != nil {
			if err := s.store.MarkFailed(ctx, d, sendErr, now); err != nil {
				return err
			}
			continue
		}
		if msg == nil {
			msg = &bot.Message{}
		}
		if err := s.store.MarkSent(ctx, d.ID, msg.MessageID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) HandleCallback(ctx context.Context, callback *bot.CallbackQuery) error {
	if callback == nil {
		return nil
	}
	parts := strings.Split(callback.Data, ":")
	if len(parts) != 3 || parts[0] != "qs" {
		return s.telegram.AnswerCallbackQuery(ctx, callback.ID, "ไม่พบรายการนี้", true)
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return s.telegram.AnswerCallbackQuery(ctx, callback.ID, "รายการไม่ถูกต้อง", true)
	}
	switch parts[1] {
	case "done":
		d, err := s.store.GetDelivery(ctx, id)
		if err == nil {
			err = s.store.Complete(ctx, d.ReminderID, d.EventAt)
		}
		if err != nil {
			return s.telegram.AnswerCallbackQuery(ctx, callback.ID, "ทำรายการไม่สำเร็จ", true)
		}
		return s.telegram.AnswerCallbackQuery(ctx, callback.ID, "ทำเครื่องหมายเสร็จแล้ว", false)
	case "s5", "s10", "s30":
		minutes, _ := strconv.Atoi(strings.TrimPrefix(parts[1], "s"))
		if err = s.store.Snooze(ctx, id, minutes); err != nil {
			return s.telegram.AnswerCallbackQuery(ctx, callback.ID, "เลื่อนเตือนไม่สำเร็จ", true)
		}
		return s.telegram.AnswerCallbackQuery(ctx, callback.ID, fmt.Sprintf("เลื่อนเตือน %d นาทีแล้ว", minutes), false)
	default:
		return s.telegram.AnswerCallbackQuery(ctx, callback.ID, "ไม่รู้จักคำสั่งนี้", true)
	}
}

func scheduleKeyboard(id int64) *bot.InlineKeyboardMarkup {
	return &bot.InlineKeyboardMarkup{InlineKeyboard: [][]bot.InlineKeyboardButton{{{Text: "เลื่อน 5 นาที", CallbackData: fmt.Sprintf("qs:s5:%d", id)}, {Text: "เลื่อน 10 นาที", CallbackData: fmt.Sprintf("qs:s10:%d", id)}, {Text: "เลื่อน 30 นาที", CallbackData: fmt.Sprintf("qs:s30:%d", id)}}, {{Text: "เสร็จแล้ว", CallbackData: fmt.Sprintf("qs:done:%d", id)}}}}
}

var (
	thaiWeekdays = [...]string{
		"วันอาทิตย์", "วันจันทร์", "วันอังคาร", "วันพุธ", "วันพฤหัสบดี", "วันศุกร์", "วันเสาร์",
	}
	thaiMonths = [...]string{
		"", "ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค.",
	}
)

func formatThaiDateTime(t time.Time) string {
	w := thaiWeekdays[t.Weekday()]
	m := thaiMonths[t.Month()]
	return fmt.Sprintf("%s %d %s %s", w, t.Day(), m, t.Format("15:04"))
}

func formatMessage(d Delivery) string {
	loc := location()
	event := d.EventAt.In(loc)
	when := formatThaiDateTime(event)
	timing := "ถึงเวลาแล้ว"
	if d.Kind == "snooze" {
		timing = "เตือนซ้ำหลังเลื่อน"
	}
	if d.Offset > 0 {
		timing = fmt.Sprintf("อีก %d นาที", d.Offset)
	}
	text := fmt.Sprintf("⏰ <b>%s</b>\n%s · %s", html.EscapeString(d.Title), when, timing)
	if d.Details != "" {
		text += "\n\n" + html.EscapeString(d.Details)
	}
	return text
}
