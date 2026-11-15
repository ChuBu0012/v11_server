package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DiscordNotifier sends messages to a Discord channel via incoming webhook.
type DiscordNotifier struct {
	webhookURL string
	client     *http.Client
}

// DiscordOption allows customizing the DiscordNotifier.
type DiscordOption func(*DiscordNotifier)

// WithDiscordHTTPClient sets a custom HTTP client (useful for unit testing).
func WithDiscordHTTPClient(c *http.Client) DiscordOption {
	return func(d *DiscordNotifier) { d.client = c }
}

// NewDiscordNotifier creates a Discord notifier. Returns NoopNotifier if webhookURL is empty.
func NewDiscordNotifier(webhookURL string, timeout time.Duration, opts ...DiscordOption) Notifier {
	cleanURL := strings.TrimSpace(webhookURL)
	if cleanURL == "" {
		return &NoopNotifier{}
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dn := &DiscordNotifier{
		webhookURL: cleanURL,
		client:     &http.Client{Timeout: timeout},
	}
	for _, opt := range opts {
		opt(dn)
	}
	return dn
}

var (
	htmlBoldRegex   = regexp.MustCompile(`<b>(.*?)</b>`)
	htmlItalicRegex = regexp.MustCompile(`<i>(.*?)</i>`)
	htmlCodeRegex   = regexp.MustCompile(`<code>(.*?)</code>`)
	htmlTagStrip    = regexp.MustCompile(`<[^>]*>`)
)

// convertHTMLToDiscordMarkdown converts basic HTML formatting from alerts/digest to Discord markdown.
func convertHTMLToDiscordMarkdown(msg string) string {
	res := htmlBoldRegex.ReplaceAllString(msg, "**$1**")
	res = htmlItalicRegex.ReplaceAllString(res, "*$1*")
	res = htmlCodeRegex.ReplaceAllString(res, "`$1`")
	res = htmlTagStrip.ReplaceAllString(res, "")
	return strings.TrimSpace(res)
}

type discordPayload struct {
	Content string `json:"content"`
}

// Send delivers a message to the Discord webhook.
func (d *DiscordNotifier) Send(ctx context.Context, msg string) error {
	content := convertHTMLToDiscordMarkdown(msg)
	// Discord character limit per message is 2000
	if len(content) > 1950 {
		content = content[:1950] + "\n..."
	}

	payload := discordPayload{Content: content}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("discord: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord: request failed: %s", redactDiscordURL(err.Error(), d.webhookURL))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("discord: API returned status %d: %s", resp.StatusCode, redactDiscordURL(string(body), d.webhookURL))
	}

	return nil
}

// redactDiscordURL redacts the webhook token from log and error messages.
func redactDiscordURL(input, webhookURL string) string {
	if webhookURL == "" {
		return input
	}
	parts := strings.Split(webhookURL, "/")
	if len(parts) > 1 {
		token := parts[len(parts)-1]
		if len(token) > 4 {
			input = strings.ReplaceAll(input, token, "DISCORD_TOKEN_REDACTED")
		}
	}
	return strings.ReplaceAll(input, webhookURL, "https://discord.com/api/webhooks/...REDACTED...")
}
