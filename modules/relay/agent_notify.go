package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"v11-server/modules/bot"
)

// AgentNotificationRequest defines the incoming payload for agent task completion notifications.
type AgentNotificationRequest struct {
	Project  string `json:"project" form:"project"`
	Status   string `json:"status" form:"status"` // success, failed, warning, info
	Title    string `json:"title" form:"title"`
	Summary  string `json:"summary" form:"summary"`
	Files    any    `json:"files" form:"files"` // []string or comma/newline-separated string
	Branch   string `json:"branch" form:"branch"`
	Commit   string `json:"commit" form:"commit"`
	Device   string `json:"device" form:"device"`
	Duration string `json:"duration" form:"duration"`
	Silent   bool   `json:"silent" form:"silent"`
}

// FormatAgentTelegramCard formats an AgentNotificationRequest into a clean Telegram HTML card.
func FormatAgentTelegramCard(req *AgentNotificationRequest) string {
	if req == nil {
		req = &AgentNotificationRequest{}
	}

	project := strings.TrimSpace(req.Project)
	if project == "" {
		project = "General"
	}

	badge := "🔵"
	statusTH := "แจ้งเตือน (Info)"
	st := strings.ToLower(strings.TrimSpace(req.Status))
	switch st {
	case "success", "ok", "done", "pass":
		badge = "🟢"
		statusTH = "สำเร็จ (Success)"
	case "failed", "fail", "error", "err":
		badge = "🔴"
		statusTH = "ล้มเหลว (Failed)"
	case "warning", "warn", "review":
		badge = "🟡"
		statusTH = "แจ้งเตือน / ต้องตรวจทาน (Warning)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🤖 <b>Agent Notification</b> [<code>%s</code>]\n", htmlEscape(project)))
	sb.WriteString(fmt.Sprintf("สถานะ: %s <b>%s</b>\n", badge, statusTH))

	title := strings.TrimSpace(req.Title)
	if title != "" {
		sb.WriteString(fmt.Sprintf("📌 <b>หัวข้อ:</b> %s\n", htmlEscape(title)))
	}

	summary := strings.TrimSpace(req.Summary)
	if summary != "" {
		sb.WriteString("\n📝 <b>สรุปผล:</b>\n")
		lines := strings.Split(summary, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "• ") {
				content := strings.TrimSpace(line[2:])
				sb.WriteString(fmt.Sprintf("• %s\n", htmlEscape(content)))
			} else {
				sb.WriteString(fmt.Sprintf("• %s\n", htmlEscape(line)))
			}
		}
	}

	files := parseFiles(req.Files)
	if len(files) > 0 {
		var escapedFiles []string
		for _, f := range files {
			if f != "" {
				escapedFiles = append(escapedFiles, fmt.Sprintf("<code>%s</code>", htmlEscape(f)))
			}
		}
		if len(escapedFiles) > 0 {
			sb.WriteString(fmt.Sprintf("\n📂 <b>ไฟล์:</b> %s\n", strings.Join(escapedFiles, ", ")))
		}
	}

	var metaParts []string
	if b := strings.TrimSpace(req.Branch); b != "" {
		metaParts = append(metaParts, fmt.Sprintf("🌿 <code>%s</code>", htmlEscape(b)))
	}
	if c := strings.TrimSpace(req.Commit); c != "" {
		if len(c) > 7 {
			c = c[:7]
		}
		metaParts = append(metaParts, fmt.Sprintf("🏷️ <code>%s</code>", htmlEscape(c)))
	}
	if d := strings.TrimSpace(req.Device); d != "" {
		metaParts = append(metaParts, fmt.Sprintf("💻 %s", htmlEscape(d)))
	}
	if dur := strings.TrimSpace(req.Duration); dur != "" {
		metaParts = append(metaParts, fmt.Sprintf("⏱️ %s", htmlEscape(dur)))
	}

	if len(metaParts) > 0 {
		sb.WriteString("\n" + strings.Join(metaParts, " | ") + "\n")
	}

	nowTH := time.Now().UTC().Add(7 * time.Hour).Format("15:04 น.")
	sb.WriteString(fmt.Sprintf("⏰ <b>เวลา:</b> %s", nowTH))

	return sb.String()
}

func parseFiles(val any) []string {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case []string:
		var res []string
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				res = append(res, s)
			}
		}
		return res
	case []any:
		var res []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					res = append(res, s)
				}
			}
		}
		return res
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		// If it's a JSON array string e.g. ["a.go", "b.go"]
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			var parsed []string
			if err := json.Unmarshal([]byte(v), &parsed); err == nil {
				return parseFiles(parsed)
			}
		}
		// Split by comma or newline
		var res []string
		rawParts := strings.FieldsFunc(v, func(r rune) bool {
			return r == ',' || r == '\n' || r == ';'
		})
		for _, p := range rawParts {
			if p = strings.TrimSpace(p); p != "" {
				res = append(res, p)
			}
		}
		return res
	default:
		return nil
	}
}

// handleAgentNotify handles requests to POST /api/agent/notify.
func (m *Module) handleAgentNotify(c *gin.Context) {
	if m.sender == nil || m.chatID == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Telegram bot or chat_id is not configured", "success": false})
		return
	}

	var req AgentNotificationRequest
	contentType := c.GetHeader("Content-Type")

	// Check if request is multipart form (with potential file attachment)
	if strings.HasPrefix(contentType, "multipart/form-data") {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, m.maxUploadBytes+(1<<20))
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error":   fmt.Sprintf("Request entity exceeds limit of %d MB", m.maxUploadBytes/(1024*1024)),
					"success": false,
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form", "success": false})
			return
		}

		_ = c.ShouldBind(&req)

		cardText := FormatAgentTelegramCard(&req)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()

		// Check if a file was uploaded
		file, fileHeader, err := c.Request.FormFile("file")
		if err != nil {
			file, fileHeader, err = c.Request.FormFile("photo")
		}
		if err != nil {
			file, fileHeader, err = c.Request.FormFile("image")
		}
		if err != nil {
			file, fileHeader, err = c.Request.FormFile("attachment")
		}

		if err == nil && file != nil && fileHeader != nil {
			defer file.Close()

			isImg := isImage(fileHeader.Filename, fileHeader.Header.Get("Content-Type"))
			var sentMsg *bot.Message
			var sendErr error

			if isImg {
				sentMsg, sendErr = m.sender.SendPhoto(ctx, m.chatID, fileHeader.Filename, file, cardText)
				if sendErr != nil {
					// Fallback to document
					if seeker, ok := file.(io.ReadSeeker); ok {
						_, _ = seeker.Seek(0, io.SeekStart)
						sentMsg, sendErr = m.sender.SendDocument(ctx, m.chatID, fileHeader.Filename, file, cardText)
					}
				}
			} else {
				sentMsg, sendErr = m.sender.SendDocument(ctx, m.chatID, fileHeader.Filename, file, cardText)
			}

			if sendErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to send attachment to Telegram: %v", sendErr), "success": false})
				return
			}

			msgID := int64(0)
			if sentMsg != nil {
				msgID = sentMsg.MessageID
			}
			c.JSON(http.StatusOK, gin.H{
				"success":    true,
				"type":       map[bool]string{true: "photo", false: "document"}[isImg],
				"message_id": msgID,
				"timestamp":  time.Now().UTC().Format(time.RFC3339),
			})
			return
		}

		// Multipart form but no file: send formatted card text
		sentMsg, err := m.sender.SendMessage(ctx, m.chatID, cardText, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to send to Telegram: %v", err), "success": false})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"type":       "text",
			"message_id": sentMsg.MessageID,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	// Default: JSON or standard URL-encoded form
	if err := c.ShouldBind(&req); err != nil {
		// If binding fails, try raw body fallback
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		req.Summary = string(bodyBytes)
	}

	cardText := FormatAgentTelegramCard(&req)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	sentMsg, err := m.sender.SendMessage(ctx, m.chatID, cardText, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to send to Telegram: %v", err), "success": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"type":       "text",
		"message_id": sentMsg.MessageID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}
