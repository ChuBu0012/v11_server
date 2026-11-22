package bot

import (
	"regexp"
	"strings"
)

// AskOptions holds the parsed configuration, model selection, effort, and clean prompt for /ask.
type AskOptions struct {
	Model       string // "auto", "groq", "gemini", "openrouter", "agy"
	Effort      string // "low", "medium", "high", "max"
	NoCache     bool   // true if --no-cache or --fresh specified
	CleanPrompt string // prompt text with prefixes and flags stripped
}

var (
	flagNoCacheRe = regexp.MustCompile(`(?i)(^|\s)(--no-cache|--fresh)($|\s)`)
	flagEffortRe  = regexp.MustCompile(`(?i)^--effort=([a-z]+)`)
)

// ParseAskInput extracts optional model, effort, and cache flags from raw user input.
// Syntax: /ask [model] [effort] <prompt>
// Defaults: Model = "auto", Effort = "medium", NoCache = false.
func ParseAskInput(input string) AskOptions {
	opts := AskOptions{
		Model:  "auto",
		Effort: "medium",
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return opts
	}

	// 1. Detect and strip --no-cache or --fresh flags
	if flagNoCacheRe.MatchString(trimmed) {
		opts.NoCache = true
		trimmed = strings.TrimSpace(flagNoCacheRe.ReplaceAllString(trimmed, " "))
	}

	// Helper to normalize effort
	normalizeEffort := func(e string) (string, bool) {
		switch strings.ToLower(strings.TrimSpace(e)) {
		case "low":
			return "low", true
		case "med", "medium", "median":
			return "medium", true
		case "high":
			return "high", true
		case "max":
			return "max", true
		default:
			return "", false
		}
	}

	// Helper to normalize model
	normalizeModel := func(m string) (string, bool) {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "auto":
			return "auto", true
		case "groq":
			return "groq", true
		case "gemini":
			return "gemini", true
		case "openrouter":
			return "openrouter", true
		case "agy", "agent":
			return "agy", true
		default:
			return "", false
		}
	}

	// 2. Process leading bracket or token prefixes in a loop
	for {
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			break
		}

		// Handle bracket format: [tag] or [model:effort]
		if strings.HasPrefix(trimmed, "[") {
			closeIdx := strings.Index(trimmed, "]")
			if closeIdx > 1 {
				content := strings.TrimSpace(trimmed[1:closeIdx])
				rest := strings.TrimSpace(trimmed[closeIdx+1:])

				// Check if content has colon, e.g. [agy:high]
				if strings.Contains(content, ":") {
					parts := strings.SplitN(content, ":", 2)
					if m, ok := normalizeModel(parts[0]); ok {
						opts.Model = m
					}
					if eff, ok := normalizeEffort(parts[1]); ok {
						opts.Effort = eff
					}
					trimmed = rest
					continue
				}

				// Check if content is model
				if m, ok := normalizeModel(content); ok {
					opts.Model = m
					trimmed = rest
					continue
				}

				// Check if content is effort
				if eff, ok := normalizeEffort(content); ok {
					opts.Effort = eff
					trimmed = rest
					continue
				}
			}
		}

		// Handle --effort=... prefix
		if match := flagEffortRe.FindStringSubmatch(trimmed); len(match) > 1 {
			if eff, ok := normalizeEffort(match[1]); ok {
				opts.Effort = eff
				trimmed = strings.TrimSpace(trimmed[len(match[0]):])
				continue
			}
		}

		// Handle bracket-free words
		parts := strings.Fields(trimmed)
		if len(parts) == 0 {
			break
		}

		firstWord := parts[0]
		// Check if first word is a model tag (only if not already explicitly set to non-auto)
		if opts.Model == "auto" {
			if m, ok := normalizeModel(firstWord); ok {
				opts.Model = m
				trimmed = strings.TrimSpace(trimmed[len(firstWord):])
				continue
			}
		}

		// Check if first word is an effort tag
		if eff, ok := normalizeEffort(firstWord); ok {
			opts.Effort = eff
			trimmed = strings.TrimSpace(trimmed[len(firstWord):])
			continue
		}

		// Neither model nor effort tag matched; rest is the prompt body
		break
	}

	opts.CleanPrompt = strings.TrimSpace(trimmed)
	return opts
}
