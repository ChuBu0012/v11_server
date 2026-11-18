package watchtower

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	// Regex patterns for sensitive data in error strings or logs.
	bearerRegex   = regexp.MustCompile(`(?i)(bearer\s+|token=)[a-zA-Z0-9_\-\.]{8,}`)
	apiKeyRegex   = regexp.MustCompile(`(?i)(api[_\-]?key|secret|password|passwd|auth)=([^\s&]+)`)
	urlInErrRegex = regexp.MustCompile(`https?://[^\s]+`)
)

// SanitizeURL removes query parameters, fragments, and credentials from a URL.
// Returns a clean, safe URL string suitable for logging, alerts, and AI prompts.
func SanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		// If unparseable, return up to first '?' or '#'
		idx := strings.IndexAny(rawURL, "?#")
		if idx != -1 {
			return rawURL[:idx]
		}
		return rawURL
	}

	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil

	return u.String()
}

// SanitizeError strips sensitive tokens, authorization headers, passwords, and query parameters
// from error strings to prevent secret leakage in logs, UI, and notifications.
func SanitizeError(errStr string) string {
	if errStr == "" {
		return ""
	}

	// 1. Sanitize any URLs embedded in the error message
	clean := urlInErrRegex.ReplaceAllStringFunc(errStr, func(matchedURL string) string {
		return SanitizeURL(matchedURL)
	})

	// 2. Redact Bearer tokens and generic token parameters
	clean = bearerRegex.ReplaceAllString(clean, "${1}[REDACTED]")

	// 3. Redact API keys and passwords
	clean = apiKeyRegex.ReplaceAllString(clean, "${1}=[REDACTED]")

	return strings.TrimSpace(clean)
}

// FormatDowntime returns a human-friendly string describing downtime duration.
func FormatDowntime(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	totalSeconds := int64(d.Seconds())
	if totalSeconds < 60 {
		return fmt.Sprintf("%ds", totalSeconds)
	}

	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes < 60 {
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}

	hours := minutes / 60
	remainingMinutes := minutes % 60
	if hours < 24 {
		if remainingMinutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, remainingMinutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	days := hours / 24
	remainingHours := hours % 24
	if remainingHours > 0 {
		return fmt.Sprintf("%dd %dh", days, remainingHours)
	}
	return fmt.Sprintf("%dd", days)
}
