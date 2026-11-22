package ailimit

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"v11-server/modules/bot"
)

// CommandRunner defines the signature for running the quota checking command.
type CommandRunner func(ctx context.Context) (string, error)

// TelegramSender defines the interface for dispatching quota alert notifications.
type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, text string, replyMarkup *bot.InlineKeyboardMarkup) (*bot.Message, error)
}

// Service manages collection, insight analysis, and alerting for AI quotas.
type Service struct {
	repo          *Repository
	runner        CommandRunner
	sender        TelegramSender
	chatID        int64
	mu            sync.Mutex
	alertDebounce map[string]time.Time // bucket_id -> last alerted reset_time
	debounceMu    sync.Mutex
}

// NewService constructs an AI Limit Service.
func NewService(repo *Repository, sender TelegramSender, chatID int64, runner ...CommandRunner) *Service {
	var r CommandRunner = DefaultAgyRunner
	if len(runner) > 0 && runner[0] != nil {
		r = runner[0]
	}

	return &Service{
		repo:          repo,
		runner:        r,
		sender:        sender,
		chatID:        chatID,
		alertDebounce: make(map[string]time.Time),
	}
}

// DefaultAgyRunner resolves and runs `agy -p '/usage'` with environment variables.
func DefaultAgyRunner(ctx context.Context) (string, error) {
	binPath := resolveAgyBinary()
	if binPath == "" {
		return "", fmt.Errorf("agy binary not found on host")
	}

	cmd := exec.CommandContext(ctx, binPath, "-p", "/usage")
	// Ensure common paths in PATH
	env := os.Environ()
	home, _ := os.UserHomeDir()
	extraPath := fmt.Sprintf("PATH=%s:%s/bin:%s/.local/bin:%s",
		os.Getenv("PATH"),
		"/data/data/com.termux/files/usr",
		home,
		"/usr/local/bin",
	)
	cmd.Env = append(env, extraPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("agy execution error (code %v): %w (output: %s)", cmd.ProcessState.ExitCode(), err, string(out))
	}

	return string(out), nil
}

func resolveAgyBinary() string {
	candidates := []string{
		"agy",
		"/data/data/com.termux/files/home/.local/bin/agy",
		filepath.Join(os.Getenv("HOME"), ".local/bin/agy"),
		"/usr/local/bin/agy",
	}

	for _, c := range candidates {
		if strings.Contains(c, string(filepath.Separator)) {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		} else {
			if lp, err := exec.LookPath(c); err == nil {
				return lp
			}
		}
	}
	return ""
}

// Collect triggers a fresh collection pass, parses, persists, and evaluates alerts.
func (s *Service) Collect(ctx context.Context) (*QuotaResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := s.runner(ctx)
	if err != nil {
		return nil, fmt.Errorf("run quota runner failed: %w", err)
	}

	now := time.Now().UTC()
	buckets, err := ParseUsageOutput(raw, now)
	if err != nil {
		return nil, fmt.Errorf("parse quota output failed: %w", err)
	}

	if err := s.repo.SaveStates(ctx, buckets); err != nil {
		log.Printf("[ailimit] warning: save states failed: %v", err)
	}
	if err := s.repo.SaveSnapshots(ctx, buckets, now); err != nil {
		log.Printf("[ailimit] warning: save snapshots failed: %v", err)
	}

	// Check and send alert for critical quotas
	s.evaluateAlerts(ctx, buckets)

	return s.buildResponse(ctx, buckets, false, 0)
}

// GetStatus returns the latest known quota status and insights.
func (s *Service) GetStatus(ctx context.Context, includeHistory bool, historyHours int) (*QuotaResponse, error) {
	buckets, err := s.repo.GetLatestStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("get latest states failed: %w", err)
	}

	// If no data exists yet, attempt an initial collection
	if len(buckets) == 0 {
		return s.Collect(ctx)
	}

	return s.buildResponse(ctx, buckets, includeHistory, historyHours)
}

func (s *Service) buildResponse(ctx context.Context, buckets []QuotaBucket, includeHistory bool, historyHours int) (*QuotaResponse, error) {
	// Group buckets
	groupMap := make(map[string][]QuotaBucket)
	for _, b := range buckets {
		groupMap[b.GroupName] = append(groupMap[b.GroupName], b)
	}

	var groups []QuotaGroup
	for groupName, bList := range groupMap {
		groups = append(groups, QuotaGroup{
			GroupName:   groupName,
			DisplayName: groupName,
			Buckets:     bList,
		})
	}

	var history []QuotaSnapshot
	if includeHistory {
		if historyHours <= 0 {
			historyHours = 24
		}
		var err error
		history, err = s.repo.GetHistory(ctx, historyHours)
		if err != nil {
			log.Printf("[ailimit] warning: fetch history failed: %v", err)
		}
	}

	insight := s.GenerateInsight(buckets, history)

	resp := &QuotaResponse{
		UpdatedAt: time.Now().UTC(),
		Groups:    groups,
		Buckets:   buckets,
		Insight:   insight,
		History:   history,
	}

	return resp, nil
}

// GenerateInsight computes actionable usage velocity and recommendations.
func (s *Service) GenerateInsight(buckets []QuotaBucket, history []QuotaSnapshot) QuotaInsight {
	var criticalBucket *QuotaBucket
	var warningBucket *QuotaBucket

	for i := range buckets {
		b := &buckets[i]
		// Ignore Claude / 3P models as requested by user (limits exhaust too quickly)
		if strings.Contains(strings.ToLower(b.GroupName), "claude") || strings.HasPrefix(b.BucketID, "3p") {
			continue
		}

		if b.RemainingPercent <= 15.0 {
			if criticalBucket == nil || b.RemainingPercent < criticalBucket.RemainingPercent {
				criticalBucket = b
			}
		} else if b.RemainingPercent <= 35.0 {
			if warningBucket == nil || b.RemainingPercent < warningBucket.RemainingPercent {
				warningBucket = b
			}
		}
	}

	if criticalBucket != nil {
		targetName := fmt.Sprintf("%s (%s)", criticalBucket.GroupName, criticalBucket.DisplayName)
		countdown := formatDuration(time.Until(criticalBucket.ResetTime))
		rec := "โควตา Gemini เหลือน้อยมาก แนะนำลดระดับ Effort หรือใช้คำสั่งขนาดกะทัดรัด"

		return QuotaInsight{
			Level:          "critical",
			Title:          fmt.Sprintf("⚠️ โควตา %s วิกฤต!", targetName),
			Summary:        fmt.Sprintf("โควตาคงเหลือเพียง %.0f%% (ใช้ไป %.0f%%) จะรีเซ็ตเต็มในอีก %s", criticalBucket.RemainingPercent, criticalBucket.UsedPercent, countdown),
			Recommendation: rec,
			CriticalBucket: &criticalBucket.BucketID,
			AlertTriggered: true,
		}
	}

	if warningBucket != nil {
		targetName := fmt.Sprintf("%s (%s)", warningBucket.GroupName, warningBucket.DisplayName)
		countdown := formatDuration(time.Until(warningBucket.ResetTime))
		return QuotaInsight{
			Level:          "warning",
			Title:          fmt.Sprintf("⚡ โควตา %s เริ่มเหลือน้อย", targetName),
			Summary:        fmt.Sprintf("โควตาคงเหลือ %.0f%% จะรีเซ็ตในอีก %s", warningBucket.RemainingPercent, countdown),
			Recommendation: "ควรบริหารจัดการรอบการส่งคำสั่งอย่างประหยัด",
			CriticalBucket: &warningBucket.BucketID,
			AlertTriggered: false,
		}
	}

	return QuotaInsight{
		Level:          "normal",
		Title:          "✅ โควตา Gemini พร้อมใช้งานสมบูรณ์",
		Summary:        "โควตาระยะสั้นและรายสัปดาห์ของ Gemini อยู่ในเกณฑ์ปกติ พร้อมสำหรับงานพัฒนาเต็มรูปแบบ",
		Recommendation: "สามารถเรียกใช้งานโมเดล Gemini ได้ตามปกติอย่างเต็มประสิทธิภาพ",
		AlertTriggered: false,
	}
}

func (s *Service) evaluateAlerts(ctx context.Context, buckets []QuotaBucket) {
	if s.sender == nil || s.chatID == 0 {
		return
	}

	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()

	for _, b := range buckets {
		// Ignore Claude / 3P models as requested by user (limits exhaust too quickly)
		if strings.Contains(strings.ToLower(b.GroupName), "claude") || strings.HasPrefix(b.BucketID, "3p") {
			continue
		}

		if !b.IsLow {
			continue
		}

		lastReset, exists := s.alertDebounce[b.BucketID]
		// Alert only once per reset window or if reset time changed
		if exists && lastReset.Equal(b.ResetTime) {
			continue
		}

		countdown := formatDuration(time.Until(b.ResetTime))
		alertMsg := fmt.Sprintf(
			"🚨 <b>แจ้งเตือนโควตา Gemini ใกล้หมด!</b>\n"+
				"<i>Pocket Sentinel · AI Limit Monitor</i>\n\n"+
				"⚠️ <b>%s — %s</b>\n"+
				"• คงเหลือ: 🔴 <b>%.0f%%</b> (ใช้ไป %.0f%%)\n"+
				"• รีเซ็ตเต็มในอีก: ⏱ <b>%s</b>\n"+
				"• เวลารีเซ็ต: <code>%s</code>\n\n"+
				"💡 <i>คำแนะนำ: แนะนำลดระดับ Effort หรือพักการเรียกคำสั่งหนักเพื่อประหยัดโควตา</i>",
			b.GroupName, b.DisplayName,
			b.RemainingPercent, b.UsedPercent,
			countdown,
			b.ResetTime.Local().Format("02 ม.ค. 15:04 น."),
		)

		if _, err := s.sender.SendMessage(ctx, s.chatID, alertMsg, nil); err != nil {
			log.Printf("[ailimit] failed to dispatch quota alert: %v", err)
		} else {
			log.Printf("[ailimit] sent low quota alert for %s (remaining: %.0f%%)", b.BucketID, b.RemainingPercent)
			s.alertDebounce[b.BucketID] = b.ResetTime
		}
	}
}

// FormatTelegramMessage formats the current status into an informative single-gauge HTML card.
func (s *Service) FormatTelegramMessage(resp *QuotaResponse) string {
	if resp == nil || len(resp.Buckets) == 0 {
		return "⚠️ <b>ไม่พบข้อมูลโควตา AI</b>\n<i>กดปุ่มเพื่อลองตรวจสอบใหม่อีกครั้ง</i>"
	}

	var gemini5h, geminiWeekly *QuotaBucket
	for i := range resp.Buckets {
		b := &resp.Buckets[i]
		if b.BucketID == "gemini-5h" {
			gemini5h = b
		} else if b.BucketID == "gemini-weekly" {
			geminiWeekly = b
		}
	}

	effectivePercent := 100.0
	if gemini5h != nil && geminiWeekly != nil {
		if gemini5h.RemainingPercent < geminiWeekly.RemainingPercent {
			effectivePercent = gemini5h.RemainingPercent
		} else {
			effectivePercent = geminiWeekly.RemainingPercent
		}
	} else if gemini5h != nil {
		effectivePercent = gemini5h.RemainingPercent
	} else if geminiWeekly != nil {
		effectivePercent = geminiWeekly.RemainingPercent
	}

	dot := "🟩"
	statusText := "ปกติ พร้อมใช้งานต่อเนื่อง"
	if effectivePercent <= 15 {
		dot = "🟥"
		statusText = "วิกฤต! แนะนำประหยัดการส่งคำสั่ง"
	} else if effectivePercent <= 40 {
		dot = "🟨"
		statusText = "เริ่มเหลือน้อย ควรระวัง"
	}

	var sb strings.Builder
	sb.WriteString("⚡ <b>โควตาโมเดล Gemini (Antigravity)</b>\n")
	sb.WriteString(fmt.Sprintf("<i>อัปเดต: %s</i>\n\n", resp.UpdatedAt.Local().Format("15:04 น.")))
	sb.WriteString(fmt.Sprintf("📊 <b>โควตาพร้อมใช้งาน: %s %.0f%%</b>\n", dot, effectivePercent))
	if gemini5h != nil {
		sb.WriteString(fmt.Sprintf("• รอบ 5 ชม.: <b>%.0f%%</b> (รีเซ็ตใน %s)\n", gemini5h.RemainingPercent, formatDuration(time.Until(gemini5h.ResetTime))))
	}
	if geminiWeekly != nil {
		sb.WriteString(fmt.Sprintf("• รายสัปดาห์: <b>%.0f%%</b> (รีเซ็ตใน %s)\n", geminiWeekly.RemainingPercent, formatDuration(time.Until(geminiWeekly.ResetTime))))
	}
	sb.WriteString(fmt.Sprintf("\n💡 <b>สถานะ:</b> <i>%s</i>", statusText))

	return sb.String()
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "กำลังรีเซ็ต"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d วัน %d ชม.", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d ชม. %d นาที", hours, mins)
	}
	return fmt.Sprintf("%d นาที", mins)
}
