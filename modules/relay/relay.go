package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"v11-server/modules/bot"
)

// TelegramSender defines the interface required by the relay module to forward messages and files.
type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, text string, replyMarkup *bot.InlineKeyboardMarkup) (*bot.Message, error)
	SendPhoto(ctx context.Context, chatID int64, filename string, r io.Reader, caption string) (*bot.Message, error)
	SendDocument(ctx context.Context, chatID int64, filename string, r io.Reader, caption string) (*bot.Message, error)
}

// Module handles relaying text messages, photos, and files to Telegram.
type Module struct {
	sender         TelegramSender
	chatID         int64
	maxUploadBytes int64
}

// New creates a new Relay module instance.
func New(sender TelegramSender, chatIDStr string) *Module {
	var chatID int64
	if chatIDStr != "" {
		chatID, _ = strconv.ParseInt(chatIDStr, 10, 64)
	}
	return &Module{
		sender:         sender,
		chatID:         chatID,
		maxUploadBytes: 20 * 1024 * 1024, // 20 MB cap
	}
}

// RegisterRoutes registers relay routes on the Gin router.
func (m *Module) RegisterRoutes(r *gin.Engine) {
	// Status endpoint (no pin required to check if server has relay enabled)
	r.GET("/api/relay/status", m.handleStatus)

	// Protected Relay Endpoints
	api := r.Group("/api/relay") // public: no PIN by owner's choice
	{
		api.POST("/send", m.handleSend)
		api.POST("/message", m.handleTextMessage)
		api.POST("/upload", m.handleFileUpload)
	}
}

func (m *Module) handleStatus(c *gin.Context) {
	configured := m.sender != nil && m.chatID != 0
	c.JSON(http.StatusOK, gin.H{
		"service":          "pocket-sentinel-relay",
		"configured":       configured,
		"max_upload_bytes": m.maxUploadBytes,
		"max_upload_mb":    m.maxUploadBytes / (1024 * 1024),
	})
}

type textPayload struct {
	Message string `json:"message" form:"message"`
	Text    string `json:"text" form:"text"`
	PIN     string `json:"pin" form:"pin"`
}

// handleTextMessage forwards plain text messages to Telegram.
func (m *Module) handleTextMessage(c *gin.Context) {
	if m.sender == nil || m.chatID == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Telegram bot or chat_id is not configured", "success": false})
		return
	}

	var payload textPayload
	_ = c.ShouldBind(&payload)

	msgText := payload.Message
	if msgText == "" {
		msgText = payload.Text
	}
	msgText = strings.TrimSpace(msgText)
	if msgText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message content cannot be empty", "success": false})
		return
	}

	headerTag := "📬 <b>Relay Message:</b>\n\n"
	finalText := headerTag + htmlEscape(msgText)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	sentMsg, err := m.sender.SendMessage(ctx, m.chatID, finalText, nil)
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

// handleFileUpload handles multipart file/photo uploads.
func (m *Module) handleFileUpload(c *gin.Context) {
	if m.sender == nil || m.chatID == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Telegram bot or chat_id is not configured", "success": false})
		return
	}

	// Limit request body before parsing
	// Multipart boundaries, headers and captions count toward the HTTP body but
	// not toward Telegram's per-file upload limit.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, m.maxUploadBytes+(1<<20))

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// Fallback check for "photo" or "image" field
		file, header, err = c.Request.FormFile("photo")
		if err != nil {
			file, header, err = c.Request.FormFile("image")
		}
	}
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   fmt.Sprintf("File size exceeds maximum allowed limit of %d MB", m.maxUploadBytes/(1024*1024)),
				"success": false,
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file/photo field in multipart form", "success": false})
		return
	}
	defer file.Close()
	if header.Size > m.maxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("File size exceeds maximum allowed limit of %d MB", m.maxUploadBytes/(1024*1024)), "success": false})
		return
	}

	caption := strings.TrimSpace(c.Request.FormValue("caption"))
	if caption == "" {
		caption = strings.TrimSpace(c.Request.FormValue("message"))
	}
	if caption != "" {
		caption = htmlEscape(caption)
	}

	// Allow up to 60s for large file transfers (up to 20MB)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	contentType := header.Header.Get("Content-Type")
	isImg := isImage(header.Filename, contentType)

	var sentMsg *bot.Message
	var sendErr error

	if isImg {
		sentMsg, sendErr = m.sender.SendPhoto(ctx, m.chatID, header.Filename, file, caption)
		if sendErr != nil {
			// Telegram Bot API sendPhoto fails if dimensions exceed 10,000 px or aspect ratio > 20.
			// Automatically fallback to SendDocument so high-res artwork/posters always succeed.
			if seeker, ok := file.(io.ReadSeeker); ok {
				if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr == nil {
					sentMsg, sendErr = m.sender.SendDocument(ctx, m.chatID, header.Filename, file, caption)
					if sendErr == nil {
						isImg = false
					}
				}
			}
		}
	} else {
		sentMsg, sendErr = m.sender.SendDocument(ctx, m.chatID, header.Filename, file, caption)
	}

	if sendErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to forward to Telegram: %v", sendErr), "success": false})
		return
	}

	msgID := int64(0)
	if sentMsg != nil {
		msgID = sentMsg.MessageID
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"type":       map[bool]string{true: "photo", false: "document"}[isImg],
		"filename":   header.Filename,
		"size":       header.Size,
		"message_id": msgID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// handleSend is the unified endpoint accepting either JSON, Form text, or Multipart uploads.
func (m *Module) handleSend(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Limit request body immediately before any multipart parsing
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, m.maxUploadBytes+(1<<20))

		// Parse multipart form (max 32 MB buffer; triggers MaxBytesReader if body > maxUploadBytes)
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error":   fmt.Sprintf("Request entity exceeds limit of %d MB", m.maxUploadBytes/(1024*1024)),
					"success": false,
				})
				return
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart upload", "success": false})
				return
			}
		}

		// Check if file is present in multipart form
		if c.Request.MultipartForm != nil && c.Request.MultipartForm.File != nil {
			if len(c.Request.MultipartForm.File["file"]) > 0 ||
				len(c.Request.MultipartForm.File["photo"]) > 0 ||
				len(c.Request.MultipartForm.File["image"]) > 0 {
				m.handleFileUpload(c)
				return
			}
		}
	}

	// Default to text message handling
	m.handleTextMessage(c)
}

func isImage(filename string, contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.HasPrefix(ct, "image/jpeg") || strings.HasPrefix(ct, "image/png") ||
		strings.HasPrefix(ct, "image/gif") || strings.HasPrefix(ct, "image/webp") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp"
}

func htmlEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "&", "&amp;"), "<", "&lt;"), ">", "&gt;")
}
