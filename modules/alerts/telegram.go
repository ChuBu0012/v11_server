package alerts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Notifier is the interface for sending alert messages.
type Notifier interface {
	Send(ctx context.Context, msg string) error
}

// TelegramNotifier sends messages via the Telegram Bot API.
type TelegramNotifier struct {
	botToken string
	chatID   string
	client   *http.Client
	baseURL  string // overridable for testing; defaults to Telegram API
}

// TelegramOption allows customization of the TelegramNotifier.
type TelegramOption func(*TelegramNotifier)

// WithHTTPClient sets a custom HTTP client (useful for testing).
func WithHTTPClient(c *http.Client) TelegramOption {
	return func(t *TelegramNotifier) { t.client = c }
}

// WithBaseURL overrides the Telegram API base URL (useful for testing).
func WithBaseURL(u string) TelegramOption {
	return func(t *TelegramNotifier) { t.baseURL = u }
}

// NewTelegramNotifier creates a Telegram notifier. Returns NoopNotifier if
// token or chatID is empty.
func NewTelegramNotifier(botToken, chatID string, timeout time.Duration, opts ...TelegramOption) Notifier {
	if botToken == "" || chatID == "" {
		return &NoopNotifier{}
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tn := &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: timeout},
		baseURL:  "https://api.telegram.org",
	}
	for _, opt := range opts {
		opt(tn)
	}
	return tn
}

// Send posts a message to the configured Telegram chat.
func (t *TelegramNotifier) Send(ctx context.Context, msg string) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", t.baseURL, t.botToken)

	data := url.Values{}
	data.Set("chat_id", t.chatID)
	data.Set("text", msg)
	data.Set("parse_mode", "HTML")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("telegram: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		// Redact bot token from error messages
		return fmt.Errorf("telegram: request failed: %w", redactToken(err, t.botToken))
	}
	defer resp.Body.Close()

	// Bounded read to prevent memory issues
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK {
		// Never include bot token in error details
		return fmt.Errorf("telegram: API returned %d: %s", resp.StatusCode, redactBody(string(body), t.botToken))
	}
	return nil
}

// String returns a safe representation that never exposes the bot token.
func (t *TelegramNotifier) String() string {
	masked := "bot***..."
	if len(t.botToken) > 6 {
		masked = "bot" + t.botToken[:3] + "***..."
	}
	return fmt.Sprintf("TelegramNotifier(%s, chat=%s)", masked, t.chatID)
}

// NoopNotifier discards messages (used when Telegram is not configured).
type NoopNotifier struct{}

func (n *NoopNotifier) Send(_ context.Context, _ string) error { return nil }

func (n *NoopNotifier) String() string { return "NoopNotifier" }

// redactToken replaces occurrences of the token in an error message.
func redactToken(err error, token string) error {
	if token == "" {
		return err
	}
	msg := err.Error()
	redacted := strings.ReplaceAll(msg, token, "***REDACTED***")
	if redacted == msg {
		return err
	}
	return fmt.Errorf("%s", redacted)
}

// redactBody replaces occurrences of the token in an API response body.
func redactBody(body, token string) string {
	if token == "" {
		return body
	}
	return strings.ReplaceAll(body, token, "***REDACTED***")
}
