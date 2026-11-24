package cloudsync

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// DigestRunnerFunc defines the callback signature for triggering digest generation.
type DigestRunnerFunc func(ctx context.Context) (string, error)

// TelegramNotifierFunc defines the callback signature for sending Telegram status alerts.
type TelegramNotifierFunc func(ctx context.Context, htmlMsg string) error

// ActionDispatcher handles execution of safe whitelisted actions.
type ActionDispatcher struct {
	digestRunner DigestRunnerFunc
	notifier     TelegramNotifierFunc
}

// NewActionDispatcher creates a dispatcher with safe handlers.
func NewActionDispatcher(dr DigestRunnerFunc, notif TelegramNotifierFunc) *ActionDispatcher {
	return &ActionDispatcher{
		digestRunner: dr,
		notifier:     notif,
	}
}

// Dispatch executes the requested action if permitted by the whitelist.
func (d *ActionDispatcher) Dispatch(ctx context.Context, req ActionRequest) (string, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))

	switch action {
	case "ping":
		result := fmt.Sprintf("pong: v11 pocket server active at %s", time.Now().Format("2006-01-02 15:04:05 MST"))
		if d.notifier != nil {
			_ = d.notifier(ctx, fmt.Sprintf("🌐 <b>Public Web Action:</b> <code>ping</code> ได้รับการตอบสนองแล้ว"))
		}
		return result, nil

	case "refresh_digest":
		if d.digestRunner == nil {
			return "", fmt.Errorf("digest runner not configured")
		}
		log.Printf("[cloudsync] Triggering safe action: refresh_digest")
		summary, err := d.digestRunner(ctx)
		if err != nil {
			return "", fmt.Errorf("digest failed: %w", err)
		}
		if d.notifier != nil {
			_ = d.notifier(ctx, fmt.Sprintf("🌐 <b>Public Web Action:</b> <code>refresh_digest</code> ทำงานสำเร็จ"))
		}
		snippet := summary
		if len(snippet) > 100 {
			snippet = snippet[:100] + "..."
		}
		return fmt.Sprintf("digest updated: %s", snippet), nil

	default:
		log.Printf("[cloudsync] Rejected disallowed or unknown action: %q", req.Action)
		return "", fmt.Errorf("action %q is not permitted by whitelist policy", req.Action)
	}
}
