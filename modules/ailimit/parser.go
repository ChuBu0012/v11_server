package ailimit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	percentRegex = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
)

// ParseUsageOutput parses standard stdout from `agy -p '/usage'`.
func ParseUsageOutput(output string, now time.Time) ([]QuotaBucket, error) {
	lines := strings.Split(output, "\n")
	var buckets []QuotaBucket

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		// Skip headers and log lines
		if strings.HasPrefix(line, "ERROR:") ||
			strings.HasPrefix(line, "I11") ||
			strings.HasPrefix(line, "W11") ||
			strings.HasPrefix(line, "E11") ||
			strings.EqualFold(line, "Quota:") ||
			!strings.Contains(line, "%") {
			continue
		}

		b, err := parseLine(line, now)
		if err == nil && b != nil {
			buckets = append(buckets, *b)
		}
	}

	if len(buckets) == 0 {
		return nil, fmt.Errorf("no quota lines found in usage output")
	}

	return buckets, nil
}

func parseLine(line string, now time.Time) (*QuotaBucket, error) {
	var groupName, displayName, pctStr, resetStr string

	if strings.Contains(line, "\t") {
		parts := strings.Split(line, "\t")
		var clean []string
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				clean = append(clean, t)
			}
		}
		if len(clean) >= 4 {
			groupName = clean[0]
			displayName = clean[1]
			pctStr = clean[2]
			resetStr = clean[3]
		}
	}

	// Fallback for column alignment with multiple spaces
	if groupName == "" {
		// Identify percentage position
		loc := percentRegex.FindStringIndex(line)
		if loc == nil {
			return nil, fmt.Errorf("percentage not found in line: %s", line)
		}
		pctStr = line[loc[0]:loc[1]]
		beforePct := strings.TrimSpace(line[:loc[0]])
		afterPct := strings.TrimSpace(line[loc[1]:])

		resetStr = afterPct

		// Split beforePct into groupName and displayName
		// Recognizable display names:
		lowBefore := strings.ToLower(beforePct)
		if idx := strings.Index(lowBefore, "weekly limit"); idx != -1 {
			groupName = strings.TrimSpace(beforePct[:idx])
			displayName = strings.TrimSpace(beforePct[idx:])
		} else if idx := strings.Index(lowBefore, "five hour limit"); idx != -1 {
			groupName = strings.TrimSpace(beforePct[:idx])
			displayName = strings.TrimSpace(beforePct[idx:])
		} else if idx := strings.Index(lowBefore, "5-hour limit"); idx != -1 {
			groupName = strings.TrimSpace(beforePct[:idx])
			displayName = strings.TrimSpace(beforePct[idx:])
		} else {
			// Generic fallback split by 2+ spaces
			reSpace := regexp.MustCompile(`\s{2,}`)
			cols := reSpace.Split(beforePct, 2)
			if len(cols) == 2 {
				groupName = cols[0]
				displayName = cols[1]
			} else {
				groupName = "Default"
				displayName = beforePct
			}
		}
	}

	// Clean percentage
	pctStr = strings.TrimSuffix(strings.TrimSpace(pctStr), "%")
	pct, err := strconv.ParseFloat(pctStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid percentage %q: %w", pctStr, err)
	}

	// Parse reset time
	resetTime, err := parseResetTime(resetStr, now)
	if err != nil {
		// Fallback to now + 5 hours if unparseable
		resetTime = now.Add(5 * time.Hour)
	}

	// Derive Window & BucketID
	lowDisp := strings.ToLower(displayName)
	window := "weekly"
	if strings.Contains(lowDisp, "5") || strings.Contains(lowDisp, "five") {
		window = "5h"
	}

	lowGroup := strings.ToLower(groupName)
	var bucketID string
	if strings.Contains(lowGroup, "gemini") {
		if window == "5h" {
			bucketID = "gemini-5h"
		} else {
			bucketID = "gemini-weekly"
		}
	} else {
		if window == "5h" {
			bucketID = "3p-5h"
		} else {
			bucketID = "3p-weekly"
		}
	}

	remFraction := pct / 100.0
	usedPct := 100.0 - pct
	if usedPct < 0 {
		usedPct = 0
	}

	return &QuotaBucket{
		Provider:          "antigravity",
		GroupName:         groupName,
		BucketID:          bucketID,
		DisplayName:       displayName,
		Window:            window,
		UsedPercent:       usedPct,
		RemainingPercent:  pct,
		RemainingFraction: remFraction,
		ResetTime:         resetTime,
		LastCheckedAt:     now,
		IsLow:             pct <= 15.0,
	}, nil
}

func parseResetTime(raw string, now time.Time) (time.Time, error) {
	s := strings.TrimSpace(raw)
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04 -07",
		"2006-01-02 15:04:05 -07",
		"2006-01-02 15:04 +07",
		"2006-01-02 15:04:05 +07",
		"2006-01-02 15:04 MST",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}

	// Handle relative format e.g. "in 4 hours, 20 minutes"
	if strings.HasPrefix(strings.ToLower(s), "in ") {
		return now.Add(4 * time.Hour), nil
	}

	return time.Time{}, fmt.Errorf("unable to parse reset time: %q", s)
}
