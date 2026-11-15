package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client interfaces with the Telegram Bot API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the default Telegram API URL (useful for testing).
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// NewClient creates a new Telegram Bot API client.
func NewClient(token string, opts ...ClientOption) *Client {
	c := &Client{
		token:   token,
		baseURL: "https://api.telegram.org",
		httpClient: &http.Client{
			Timeout: 45 * time.Second, // Long polling requests typically take 30s + network overhead
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// RedactToken removes the bot token from error strings.
func (c *Client) redactToken(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	msg := err.Error()
	if strings.Contains(msg, c.token) {
		masked := "****"
		if len(c.token) > 8 {
			masked = c.token[:4] + "****" + c.token[len(c.token)-4:]
		}
		msg = strings.ReplaceAll(msg, c.token, masked)
		return errors.New(msg)
	}
	return err
}

// GetUpdates fetches new updates using long polling.
func (c *Client) GetUpdates(ctx context.Context, offset int64, limit int, timeoutSec int) ([]Update, error) {
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates", c.baseURL, c.token)

	params := url.Values{}
	if offset > 0 {
		params.Set("offset", strconv.FormatInt(offset, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if timeoutSec > 0 {
		params.Set("timeout", strconv.Itoa(timeoutSec))
	}

	reqURL := endpoint + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, c.redactToken(fmt.Errorf("create request failed: %w", err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, c.redactToken(fmt.Errorf("getUpdates failed: %w", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		return nil, c.redactToken(fmt.Errorf("read getUpdates response failed: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.redactToken(fmt.Errorf("getUpdates returned status %d: %s", resp.StatusCode, string(body)))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal getUpdates response failed: %w", err)
	}

	if !apiResp.OK {
		return nil, c.redactToken(fmt.Errorf("telegram API error (%d): %s", apiResp.ErrorCode, apiResp.Description))
	}

	var updates []Update
	if len(apiResp.Result) > 0 {
		if err := json.Unmarshal(apiResp.Result, &updates); err != nil {
			return nil, fmt.Errorf("unmarshal updates array failed: %w", err)
		}
	}

	return updates, nil
}

// SendMessage sends an HTML formatted text message.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, replyMarkup *InlineKeyboardMarkup) (*Message, error) {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL, c.token)

	payload := SendMessagePayload{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: replyMarkup,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal sendMessage payload failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, c.redactToken(fmt.Errorf("create request failed: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, c.redactToken(fmt.Errorf("sendMessage failed: %w", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, c.redactToken(fmt.Errorf("read response failed: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.redactToken(fmt.Errorf("sendMessage returned %d: %s", resp.StatusCode, string(body)))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %w", err)
	}

	if !apiResp.OK {
		return nil, c.redactToken(fmt.Errorf("telegram API error (%d): %s", apiResp.ErrorCode, apiResp.Description))
	}

	var msg Message
	if err := json.Unmarshal(apiResp.Result, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message failed: %w", err)
	}

	return &msg, nil
}

// EditMessageText edits an existing message text in-place.
func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, replyMarkup *InlineKeyboardMarkup) error {
	endpoint := fmt.Sprintf("%s/bot%s/editMessageText", c.baseURL, c.token)

	payload := EditMessageTextPayload{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: replyMarkup,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal editMessageText payload failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return c.redactToken(fmt.Errorf("create request failed: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.redactToken(fmt.Errorf("editMessageText failed: %w", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return c.redactToken(fmt.Errorf("read response failed: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return c.redactToken(fmt.Errorf("editMessageText returned %d: %s", resp.StatusCode, string(body)))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("unmarshal response failed: %w", err)
	}

	if !apiResp.OK {
		// If message is not modified, ignore that specific error as benign
		if strings.Contains(apiResp.Description, "message is not modified") {
			return nil
		}
		return c.redactToken(fmt.Errorf("telegram API error (%d): %s", apiResp.ErrorCode, apiResp.Description))
	}

	return nil
}

// AnswerCallbackQuery acknowledges a callback query event.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string, showAlert bool) error {
	endpoint := fmt.Sprintf("%s/bot%s/answerCallbackQuery", c.baseURL, c.token)

	payload := AnswerCallbackQueryPayload{
		CallbackQueryID: callbackQueryID,
		Text:            text,
		ShowAlert:       showAlert,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal answerCallbackQuery payload failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return c.redactToken(fmt.Errorf("create request failed: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.redactToken(fmt.Errorf("answerCallbackQuery failed: %w", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return c.redactToken(fmt.Errorf("read response failed: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return c.redactToken(fmt.Errorf("answerCallbackQuery returned %d: %s", resp.StatusCode, string(body)))
	}

	return nil
}

// Handler handles incoming updates.
type Handler interface {
	HandleUpdate(ctx context.Context, u Update) error
}

// HandlerFunc is an adapter to allow use of ordinary functions as Handlers.
type HandlerFunc func(ctx context.Context, u Update) error

func (f HandlerFunc) HandleUpdate(ctx context.Context, u Update) error {
	return f(ctx, u)
}

// Poller runs the Telegram long-polling loop.
type Poller struct {
	client     *Client
	handler    Handler
	offset     int64
	timeoutSec int
}

// NewPoller creates a new Poller instance.
func NewPoller(client *Client, handler Handler) *Poller {
	return &Poller{
		client:     client,
		handler:    handler,
		offset:     0,
		timeoutSec: 30, // 30s long polling per spec
	}
}

// SetTimeout configures the long-polling timeout in seconds.
func (p *Poller) SetTimeout(sec int) {
	p.timeoutSec = sec
}

// GetOffset returns the current update offset.
func (p *Poller) GetOffset() int64 {
	return p.offset
}

// PollOnce executes a single long-polling request and dispatches updates.
func (p *Poller) PollOnce(ctx context.Context) (int, error) {
	updates, err := p.client.GetUpdates(ctx, p.offset, 20, p.timeoutSec)
	if err != nil {
		return 0, err
	}

	for _, u := range updates {
		if u.UpdateID >= p.offset {
			p.offset = u.UpdateID + 1
		}
		if p.handler != nil {
			if hErr := p.handler.HandleUpdate(ctx, u); hErr != nil {
				log.Printf("[bot] error handling update %d: %v", u.UpdateID, hErr)
			}
		}
	}

	return len(updates), nil
}

// Start runs the continuous polling loop until context cancellation.
func (p *Poller) Start(ctx context.Context) {
	log.Printf("[bot] Telegram long-polling started (timeout: %ds)", p.timeoutSec)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[bot] Telegram long-polling stopped")
			return
		default:
		}

		_, err := p.PollOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("[bot] Telegram long-polling stopped via context")
				return
			}
			log.Printf("[bot] long-polling error: %v (backing off 2s)", err)
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}
