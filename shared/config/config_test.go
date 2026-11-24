package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

// withEnv sets environment variables for the duration of the test and restores them after.
func withEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	originals := make(map[string]string)
	for k := range vars {
		originals[k], _ = os.LookupEnv(k)
	}
	t.Cleanup(func() {
		for k := range vars {
			if orig, ok := originals[k]; ok {
				os.Setenv(k, orig)
			} else {
				os.Unsetenv(k)
			}
		}
	})
	for k, v := range vars {
		os.Setenv(k, v)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"SAMPLE_INTERVAL", "RETENTION_MAX_AGE", "RETENTION_MAX_ROWS",
		"BATTERY_THRESHOLD", "DISK_THRESHOLD_GB",
		"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "TELEGRAM_TIMEOUT",
		"GROQ_API_KEY", "GROQ_MODEL", "GEMINI_API_KEY", "GEMINI_MODEL", "REMINDER_PIN", "WATCHTOWER_PIN", "OPENROUTER_API_KEY",
		"ENABLE_LIVE_MAP", "LOCATION_SECRET_TOKEN", "LOCATION_VIEWER_TOKEN",
	}
	saved := make(map[string]string)
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = v
		}
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			os.Setenv(k, v)
		}
	})
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SampleInterval != 60*time.Second {
		t.Errorf("SampleInterval = %v, want 60s", cfg.SampleInterval)
	}
	if cfg.RetentionMaxAge != 72*time.Hour {
		t.Errorf("RetentionMaxAge = %v, want 72h", cfg.RetentionMaxAge)
	}
	if cfg.RetentionMaxRows != 4320 {
		t.Errorf("RetentionMaxRows = %d, want 4320", cfg.RetentionMaxRows)
	}
	if cfg.BatteryThreshold != 20 {
		t.Errorf("BatteryThreshold = %d, want 20", cfg.BatteryThreshold)
	}
	if cfg.DiskThresholdGB != 5.0 {
		t.Errorf("DiskThresholdGB = %v, want 5.0", cfg.DiskThresholdGB)
	}
	if cfg.TelegramTimeout != 10*time.Second {
		t.Errorf("TelegramTimeout = %v, want 10s", cfg.TelegramTimeout)
	}
	if len(cfg.LocationSecretToken) != 32 {
		t.Errorf("LocationSecretToken length = %d, want 32", len(cfg.LocationSecretToken))
	}
	if len(cfg.LocationViewerToken) != 32 {
		t.Errorf("LocationViewerToken length = %d, want 32", len(cfg.LocationViewerToken))
	}
	if cfg.LocationSecretToken == cfg.LocationViewerToken {
		t.Error("location ingress and viewer tokens must be distinct")
	}
	if cfg.LocationSecretTokenExplicit || cfg.LocationViewerTokenExplicit {
		t.Error("defaulted location tokens must not be marked as explicit environment configuration")
	}
	if cfg.EnableLiveMap {
		t.Error("EnableLiveMap should default to false (disabled)")
	}
	if cfg.TelegramEnabled() {
		t.Error("TelegramEnabled should be false with no token/chatID")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearConfigEnv(t)
	withEnv(t, map[string]string{
		"SAMPLE_INTERVAL":       "30s",
		"RETENTION_MAX_AGE":     "24h",
		"RETENTION_MAX_ROWS":    "1000",
		"BATTERY_THRESHOLD":     "15",
		"DISK_THRESHOLD_GB":     "2.5",
		"TELEGRAM_BOT_TOKEN":    "123456:ABC-DEF-token",
		"TELEGRAM_CHAT_ID":      "-100999",
		"TELEGRAM_TIMEOUT":      "5s",
		"GROQ_API_KEY":          "gsk-test-groq-key",
		"GROQ_MODEL":            "llama-3.1-8b-instant",
		"GEMINI_API_KEY":        "AIza-test-gemini-key",
		"GEMINI_MODEL":          "gemini-3.5-flash-lite",
		"WATCHTOWER_PIN":        "wt-pin-1234",
		"OPENROUTER_API_KEY":    "sk-or-v1-secret-key",
		"LOCATION_SECRET_TOKEN": "owner-device-token-32-characters",
		"LOCATION_VIEWER_TOKEN": "private-viewer-token-32-characters",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SampleInterval != 30*time.Second {
		t.Errorf("SampleInterval = %v, want 30s", cfg.SampleInterval)
	}
	if cfg.RetentionMaxAge != 24*time.Hour {
		t.Errorf("RetentionMaxAge = %v, want 24h", cfg.RetentionMaxAge)
	}
	if cfg.RetentionMaxRows != 1000 {
		t.Errorf("RetentionMaxRows = %d, want 1000", cfg.RetentionMaxRows)
	}
	if cfg.BatteryThreshold != 15 {
		t.Errorf("BatteryThreshold = %d, want 15", cfg.BatteryThreshold)
	}
	if cfg.DiskThresholdGB != 2.5 {
		t.Errorf("DiskThresholdGB = %v, want 2.5", cfg.DiskThresholdGB)
	}
	if !cfg.TelegramEnabled() {
		t.Error("TelegramEnabled should be true")
	}
	if cfg.TelegramTimeout != 5*time.Second {
		t.Errorf("TelegramTimeout = %v, want 5s", cfg.TelegramTimeout)
	}
	if cfg.WatchtowerPIN != "wt-pin-1234" {
		t.Errorf("WatchtowerPIN = %v, want wt-pin-1234", cfg.WatchtowerPIN)
	}
	if cfg.GroqAPIKey != "gsk-test-groq-key" {
		t.Errorf("GroqAPIKey = %v, want gsk-test-groq-key", cfg.GroqAPIKey)
	}
	if cfg.GroqModel != "llama-3.1-8b-instant" {
		t.Errorf("GroqModel = %v, want llama-3.1-8b-instant", cfg.GroqModel)
	}
	if cfg.GeminiAPIKey != "AIza-test-gemini-key" {
		t.Errorf("GeminiAPIKey = %v, want AIza-test-gemini-key", cfg.GeminiAPIKey)
	}
	if cfg.GeminiModel != "gemini-3.5-flash-lite" {
		t.Errorf("GeminiModel = %v, want gemini-3.5-flash-lite", cfg.GeminiModel)
	}
	if cfg.OpenRouterAPIKey != "sk-or-v1-secret-key" {
		t.Errorf("OpenRouterAPIKey = %v, want sk-or-v1-secret-key", cfg.OpenRouterAPIKey)
	}
	if cfg.LocationSecretToken != "owner-device-token-32-characters" {
		t.Errorf("LocationSecretToken = %q, want environment value", cfg.LocationSecretToken)
	}
	if cfg.LocationViewerToken != "private-viewer-token-32-characters" {
		t.Errorf("LocationViewerToken = %q, want environment value", cfg.LocationViewerToken)
	}
	if !cfg.LocationSecretTokenExplicit || !cfg.LocationViewerTokenExplicit {
		t.Error("environment-provided location tokens should be marked explicit")
	}
}

func TestLoadInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string // substring of error
	}{
		{
			name: "interval too short",
			env:  map[string]string{"SAMPLE_INTERVAL": "2s"},
			want: "below minimum 5s",
		},
		{
			name: "interval not a duration",
			env:  map[string]string{"SAMPLE_INTERVAL": "abc"},
			want: "invalid duration",
		},
		{
			name: "negative retention age",
			env:  map[string]string{"RETENTION_MAX_AGE": "-1h"},
			want: "must be positive",
		},
		{
			name: "zero max rows",
			env:  map[string]string{"RETENTION_MAX_ROWS": "0"},
			want: "must be positive",
		},
		{
			name: "battery out of range",
			env:  map[string]string{"BATTERY_THRESHOLD": "150"},
			want: "must be 0-100",
		},
		{
			name: "negative disk threshold",
			env:  map[string]string{"DISK_THRESHOLD_GB": "-1"},
			want: "must be non-negative",
		},
		{
			name: "invalid telegram timeout",
			env:  map[string]string{"TELEGRAM_TIMEOUT": "notaduration"},
			want: "invalid duration",
		},
		{
			name: "short location token",
			env:  map[string]string{"LOCATION_SECRET_TOKEN": "too-short"},
			want: "must contain at least 32 characters",
		},
		{
			name: "identical location tokens",
			env: map[string]string{
				"LOCATION_SECRET_TOKEN": "abcdefghijklmnopqrstuvwxyz123456",
				"LOCATION_VIEWER_TOKEN": "abcdefghijklmnopqrstuvwxyz123456",
			},
			want: "must be different",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearConfigEnv(t)
			withEnv(t, tc.env)
			_, err := Load()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestStringRedaction(t *testing.T) {
	cfg := &Config{
		SampleInterval:      60 * time.Second,
		RetentionMaxAge:     72 * time.Hour,
		RetentionMaxRows:    4320,
		BatteryThreshold:    20,
		DiskThresholdGB:     5.0,
		TelegramBotToken:    "123456789:ABCDEF-long-token",
		TelegramChatID:      "-100999888",
		TelegramTimeout:     10 * time.Second,
		WatchtowerPIN:       "secret-watchtower-pin",
		OpenRouterAPIKey:    "sk-or-v1-secret-openrouter-key",
		LocationSecretToken: "location-secret-token",
		LocationViewerToken: "location-viewer-token",
	}

	s := cfg.String()

	// Secrets should not be present in cleartext
	if strings.Contains(s, "secret-watchtower-pin") {
		t.Error("String() should not contain WatchtowerPIN")
	}
	if strings.Contains(s, "sk-or-v1-secret-openrouter-key") {
		t.Error("String() should not contain OpenRouterAPIKey")
	}
	if strings.Contains(s, "location-secret-token") || strings.Contains(s, "location-viewer-token") {
		t.Error("String() should not contain location tokens")
	}

	// Token should be partially masked
	if strings.Contains(s, "123456789:ABCDEF-long-token") {
		t.Error("String() should not contain the full bot token")
	}
	if !strings.Contains(s, "1234") {
		t.Error("String() should show first 4 chars of token")
	}
	if !strings.Contains(s, "oken") {
		t.Error("String() should show last 4 chars of token")
	}

	// Chat ID should be fully masked
	if strings.Contains(s, "-100999888") {
		t.Error("String() should not contain the chat ID")
	}
	if !strings.Contains(s, "****") {
		t.Error("String() should mask secrets with ****")
	}
}

func TestStringNoSecrets(t *testing.T) {
	cfg := &Config{
		SampleInterval:   60 * time.Second,
		RetentionMaxAge:  72 * time.Hour,
		RetentionMaxRows: 4320,
		BatteryThreshold: 20,
		DiskThresholdGB:  5.0,
		TelegramTimeout:  10 * time.Second,
	}

	s := cfg.String()
	if !strings.Contains(s, "(not set)") {
		t.Error("String() should show '(not set)' for unconfigured secrets")
	}
}

func TestShortTokenRedaction(t *testing.T) {
	cfg := &Config{
		TelegramBotToken: "short",
		TelegramTimeout:  10 * time.Second,
	}
	s := cfg.String()
	if strings.Contains(s, "short") {
		t.Error("String() should not reveal short tokens")
	}
}
