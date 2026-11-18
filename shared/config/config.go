package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the Pocket Sentinel service.
type Config struct {
	// Collection
	SampleInterval time.Duration // ENV: SAMPLE_INTERVAL (default: 60s, min: 5s)

	// Retention
	RetentionMaxAge  time.Duration // ENV: RETENTION_MAX_AGE (default: 72h)
	RetentionMaxRows int           // ENV: RETENTION_MAX_ROWS (default: 4320)

	// Alert thresholds
	BatteryThreshold int     // ENV: BATTERY_THRESHOLD (default: 20, percent)
	DiskThresholdGB  float64 // ENV: DISK_THRESHOLD_GB (default: 5.0)

	// Telegram
	TelegramBotToken string        // ENV: TELEGRAM_BOT_TOKEN
	TelegramChatID   string        // ENV: TELEGRAM_CHAT_ID
	TelegramTimeout  time.Duration // ENV: TELEGRAM_TIMEOUT (default: 10s)

	// Gemini AI
	GeminiAPIKey string // ENV: GEMINI_API_KEY

	// Relay
	RelayPIN string // ENV: RELAY_PIN (default: "1804")

	// Quiet Schedule
	ReminderPIN string // ENV: REMINDER_PIN

	// Personal Watchtower
	WatchtowerPIN string // ENV: WATCHTOWER_PIN

	// OpenRouter AI
	OpenRouterAPIKey string // ENV: OPENROUTER_API_KEY
}

// Load reads configuration from environment variables, applying defaults
// where values are not set. Returns an error for invalid values.
func Load() (*Config, error) {
	cfg := &Config{
		SampleInterval:   60 * time.Second,
		RetentionMaxAge:  72 * time.Hour,
		RetentionMaxRows: 4320,
		BatteryThreshold: 20,
		DiskThresholdGB:  5.0,
		TelegramTimeout:  10 * time.Second,
	}

	var errs []string

	if v := os.Getenv("SAMPLE_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("SAMPLE_INTERVAL: invalid duration %q: %v", v, err))
		} else if d < 5*time.Second {
			errs = append(errs, fmt.Sprintf("SAMPLE_INTERVAL: %v is below minimum 5s", d))
		} else {
			cfg.SampleInterval = d
		}
	}

	if v := os.Getenv("RETENTION_MAX_AGE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("RETENTION_MAX_AGE: invalid duration %q: %v", v, err))
		} else if d <= 0 {
			errs = append(errs, fmt.Sprintf("RETENTION_MAX_AGE: must be positive, got %v", d))
		} else {
			cfg.RetentionMaxAge = d
		}
	}

	if v := os.Getenv("RETENTION_MAX_ROWS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("RETENTION_MAX_ROWS: invalid integer %q: %v", v, err))
		} else if n <= 0 {
			errs = append(errs, fmt.Sprintf("RETENTION_MAX_ROWS: must be positive, got %d", n))
		} else {
			cfg.RetentionMaxRows = n
		}
	}

	if v := os.Getenv("BATTERY_THRESHOLD"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("BATTERY_THRESHOLD: invalid integer %q: %v", v, err))
		} else if n < 0 || n > 100 {
			errs = append(errs, fmt.Sprintf("BATTERY_THRESHOLD: must be 0-100, got %d", n))
		} else {
			cfg.BatteryThreshold = n
		}
	}

	if v := os.Getenv("DISK_THRESHOLD_GB"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf("DISK_THRESHOLD_GB: invalid float %q: %v", v, err))
		} else if f < 0 {
			errs = append(errs, fmt.Sprintf("DISK_THRESHOLD_GB: must be non-negative, got %v", f))
		} else {
			cfg.DiskThresholdGB = f
		}
	}

	if v := os.Getenv("TELEGRAM_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("TELEGRAM_TIMEOUT: invalid duration %q: %v", v, err))
		} else if d <= 0 {
			errs = append(errs, fmt.Sprintf("TELEGRAM_TIMEOUT: must be positive, got %v", d))
		} else {
			cfg.TelegramTimeout = d
		}
	}

	cfg.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	cfg.TelegramChatID = os.Getenv("TELEGRAM_CHAT_ID")
	cfg.GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	cfg.ReminderPIN = os.Getenv("REMINDER_PIN")
	cfg.WatchtowerPIN = os.Getenv("WATCHTOWER_PIN")
	cfg.OpenRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")

	if pin := os.Getenv("RELAY_PIN"); pin != "" {
		cfg.RelayPIN = pin
	} else {
		cfg.RelayPIN = "1804"
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
	}

	return cfg, nil
}

// TelegramEnabled returns true when both bot token and chat ID are configured.
func (c *Config) TelegramEnabled() bool {
	return c.TelegramBotToken != "" && c.TelegramChatID != ""
}

// String returns a human-readable representation with secrets redacted.
func (c *Config) String() string {
	token := "(not set)"
	if c.TelegramBotToken != "" {
		if len(c.TelegramBotToken) > 8 {
			token = c.TelegramBotToken[:4] + "****" + c.TelegramBotToken[len(c.TelegramBotToken)-4:]
		} else {
			token = "****"
		}
	}

	chatID := "(not set)"
	if c.TelegramChatID != "" {
		chatID = "****"
	}

	geminiKey := "(not set)"
	if c.GeminiAPIKey != "" {
		if len(c.GeminiAPIKey) > 8 {
			geminiKey = c.GeminiAPIKey[:4] + "****" + c.GeminiAPIKey[len(c.GeminiAPIKey)-4:]
		} else {
			geminiKey = "****"
		}
	}

	return fmt.Sprintf(
		"Config{SampleInterval:%v, RetentionMaxAge:%v, RetentionMaxRows:%d, "+
			"BatteryThreshold:%d%%, DiskThresholdGB:%.1f, "+
			"TelegramBotToken:%s, TelegramChatID:%s, TelegramTimeout:%v, GeminiAPIKey:%s, RelayPIN:****, ReminderPIN:%s, WatchtowerPIN:%s, OpenRouterAPIKey:%s}",
		c.SampleInterval, c.RetentionMaxAge, c.RetentionMaxRows,
		c.BatteryThreshold, c.DiskThresholdGB,
		token, chatID, c.TelegramTimeout, geminiKey, secretState(c.ReminderPIN), secretState(c.WatchtowerPIN), secretState(c.OpenRouterAPIKey),
	)
}

func secretState(value string) string {
	if value == "" {
		return "(not set)"
	}
	return "****"
}
