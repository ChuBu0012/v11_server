package config

import (
	"crypto/rand"
	"encoding/base64"
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

	// Groq AI
	GroqAPIKey string // ENV: GROQ_API_KEY
	GroqModel  string // ENV: GROQ_MODEL (default: llama-3.3-70b-versatile)

	// Gemini AI
	GeminiAPIKey string // ENV: GEMINI_API_KEY
	GeminiModel  string // ENV: GEMINI_MODEL (default: gemini-2.0-flash)

	// Personal Watchtower
	WatchtowerPIN string // ENV: WATCHTOWER_PIN

	// OpenRouter AI
	OpenRouterAPIKey string // ENV: OPENROUTER_API_KEY

	// Location & Rendezvous Tracker
	LocationSecretToken         string // ENV: LOCATION_SECRET_TOKEN
	LocationViewerToken         string // ENV: LOCATION_VIEWER_TOKEN
	LocationSecretTokenExplicit bool   // true when LOCATION_SECRET_TOKEN is explicitly set
	LocationViewerTokenExplicit bool   // true when LOCATION_VIEWER_TOKEN is explicitly set
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
	cfg.GroqAPIKey = strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	cfg.GroqModel = strings.TrimSpace(os.Getenv("GROQ_MODEL"))
	if cfg.GroqModel == "" {
		cfg.GroqModel = "openai/gpt-oss-120b"
	}
	cfg.GeminiAPIKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	cfg.GeminiModel = strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	if cfg.GeminiModel == "" {
		cfg.GeminiModel = "gemini-2.5-flash"
	}
	cfg.WatchtowerPIN = os.Getenv("WATCHTOWER_PIN")
	cfg.OpenRouterAPIKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))

	var err error
	if cfg.LocationSecretToken, cfg.LocationSecretTokenExplicit, err = configuredLocationToken("LOCATION_SECRET_TOKEN"); err != nil {
		return nil, err
	}
	if cfg.LocationViewerToken, cfg.LocationViewerTokenExplicit, err = configuredLocationToken("LOCATION_VIEWER_TOKEN"); err != nil {
		return nil, err
	}
	if cfg.LocationSecretToken == cfg.LocationViewerToken {
		return nil, fmt.Errorf("LOCATION_SECRET_TOKEN and LOCATION_VIEWER_TOKEN must be different")
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
	}

	return cfg, nil
}

func configuredLocationToken(envName string) (string, bool, error) {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		if len(value) < 32 {
			return "", true, fmt.Errorf("%s must contain at least 32 characters", envName)
		}
		return value, true, nil
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", false, fmt.Errorf("generate default %s: %w", envName, err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), false, nil
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
			"TelegramBotToken:%s, TelegramChatID:%s, TelegramTimeout:%v, GroqAPIKey:%s, GeminiAPIKey:%s, WatchtowerPIN:%s, OpenRouterAPIKey:%s, LocationSecretToken:%s, LocationViewerToken:%s}",
		c.SampleInterval, c.RetentionMaxAge, c.RetentionMaxRows,
		c.BatteryThreshold, c.DiskThresholdGB,
		token, chatID, c.TelegramTimeout, secretState(c.GroqAPIKey), geminiKey, secretState(c.WatchtowerPIN), secretState(c.OpenRouterAPIKey),
		secretState(c.LocationSecretToken), secretState(c.LocationViewerToken),
	)
}

func secretState(value string) string {
	if value == "" {
		return "(not set)"
	}
	return "****"
}
