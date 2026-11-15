package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

// SecurityLogger records security warning events.
type SecurityLogger interface {
	RecordLog(level, message, metadata string) error
}

// Authorizer verifies incoming Telegram updates against a whitelist.
type Authorizer struct {
	whitelistedChatID string
	logger            SecurityLogger
}

// NewAuthorizer creates a new security gate authorizer.
func NewAuthorizer(whitelistedChatID string, logger SecurityLogger) *Authorizer {
	return &Authorizer{
		whitelistedChatID: strings.TrimSpace(whitelistedChatID),
		logger:            logger,
	}
}

// ExtractChatInfo retrieves the chat ID, username, and validity from an update.
func (a *Authorizer) ExtractChatInfo(u Update) (chatID int64, senderInfo string, ok bool) {
	if u.Message != nil && u.Message.Chat != nil {
		sender := ""
		if u.Message.From != nil {
			sender = u.Message.From.Username
			if sender == "" {
				sender = u.Message.From.FirstName
			}
		}
		return u.Message.Chat.ID, sender, true
	}

	if u.CallbackQuery != nil {
		sender := ""
		if u.CallbackQuery.From != nil {
			sender = u.CallbackQuery.From.Username
			if sender == "" {
				sender = u.CallbackQuery.From.FirstName
			}
			return u.CallbackQuery.From.ID, sender, true
		}
		if u.CallbackQuery.Message != nil && u.CallbackQuery.Message.Chat != nil {
			return u.CallbackQuery.Message.Chat.ID, sender, true
		}
	}

	return 0, "", false
}

// IsAuthorized checks if the incoming update originates from the configured chat ID.
func (a *Authorizer) IsAuthorized(u Update) bool {
	if a.whitelistedChatID == "" {
		log.Printf("[bot:auth] rejecting update: TELEGRAM_CHAT_ID is not configured")
		return false
	}

	// 1. Check Message updates
	if u.Message != nil {
		if u.Message.Chat == nil {
			log.Printf("[bot:auth] rejecting update: message chat is nil")
			return false
		}

		sender := ""
		if u.Message.From != nil {
			sender = u.Message.From.Username
			if sender == "" {
				sender = u.Message.From.FirstName
			}
		}

		chatIDStr := strconv.FormatInt(u.Message.Chat.ID, 10)
		if chatIDStr != a.whitelistedChatID {
			a.logViolation(u.Message.Chat.ID, sender)
			return false
		}

		// If sender is present and whitelist is a user ID, verify sender matches
		if u.Message.From != nil && !strings.HasPrefix(a.whitelistedChatID, "-") {
			fromIDStr := strconv.FormatInt(u.Message.From.ID, 10)
			if fromIDStr != a.whitelistedChatID {
				a.logViolation(u.Message.From.ID, sender)
				return false
			}
		}

		return true
	}

	// 2. Check CallbackQuery updates
	if u.CallbackQuery != nil {
		sender := ""
		var fromID int64
		if u.CallbackQuery.From != nil {
			fromID = u.CallbackQuery.From.ID
			sender = u.CallbackQuery.From.Username
			if sender == "" {
				sender = u.CallbackQuery.From.FirstName
			}
		}

		// The user who pressed the inline button must match the whitelist
		if fromID != 0 && !strings.HasPrefix(a.whitelistedChatID, "-") {
			fromIDStr := strconv.FormatInt(fromID, 10)
			if fromIDStr != a.whitelistedChatID {
				a.logViolation(fromID, sender)
				return false
			}
		}

		// The message's original chat must also match the whitelist if present
		if u.CallbackQuery.Message != nil && u.CallbackQuery.Message.Chat != nil {
			msgChatIDStr := strconv.FormatInt(u.CallbackQuery.Message.Chat.ID, 10)
			if msgChatIDStr != a.whitelistedChatID {
				a.logViolation(u.CallbackQuery.Message.Chat.ID, sender)
				return false
			}
		}

		return true
	}

	log.Printf("[bot:auth] rejecting update: no chat info found in update")
	return false
}

func (a *Authorizer) logViolation(chatID int64, sender string) {
	warningMeta := fmt.Sprintf("chat_id=%d, sender=%s, expected=%s", chatID, sender, a.whitelistedChatID)
	log.Printf("[bot:auth] SECURITY WARNING: Unauthorized Telegram access attempt: %s", warningMeta)

	if a.logger != nil {
		_ = a.logger.RecordLog("warn", "Unauthorized Telegram access attempt", warningMeta)
	}
}
