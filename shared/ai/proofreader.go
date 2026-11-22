package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ProofreadResult represents the outcome of proofreading a text.
type ProofreadResult struct {
	Original    string `json:"original"`
	Suggested   string `json:"suggested"`
	HasChanges  bool   `json:"has_changes"`
	Explanation string `json:"explanation,omitempty"`
	Fallback    bool   `json:"fallback,omitempty"`
	Error       string `json:"error,omitempty"`
}

type proofreadAIResponse struct {
	Suggested   string `json:"suggested"`
	HasChanges  bool   `json:"has_changes"`
	Explanation string `json:"explanation"`
}

// FallbackProofread returns the original text untouched with has_changes=false.
func FallbackProofread(text, reason string) *ProofreadResult {
	return &ProofreadResult{
		Original:    text,
		Suggested:   text,
		HasChanges:  false,
		Explanation: "Deterministic fallback (original text preserved)",
		Fallback:    true,
		Error:       reason,
	}
}

// extractURLs extracts all http:// and https:// URLs from text.
func extractURLs(text string) []string {
	re := regexp.MustCompile(`https?://[^\s<>"'\)]+`)
	return re.FindAllString(text, -1)
}

// extractCodeBlocks extracts inline and multi-line code blocks from text.
func extractCodeBlocks(text string) []string {
	re := regexp.MustCompile("(?s)```.*?```|`[^`]+`")
	return re.FindAllString(text, -1)
}

// verifyPreservation checks that critical items (URLs and code blocks) present in original
// are strictly preserved in suggested. If any are missing, it flags an issue.
func verifyPreservation(original, suggested string) bool {
	origURLs := extractURLs(original)
	for _, u := range origURLs {
		if !strings.Contains(suggested, u) {
			return false
		}
	}

	origCode := extractCodeBlocks(original)
	for _, c := range origCode {
		if !strings.Contains(suggested, c) {
			return false
		}
	}

	return true
}

// ProofreadText checks Thai and English text for typos, grammar, and clarity while strictly
// preserving URLs, code blocks, English technical terms, proper nouns, and intended meaning.
// If AI fails or key is missing, it falls back deterministically to the original text.
func ProofreadText(ctx context.Context, client AIEngine, text string) (*ProofreadResult, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return &ProofreadResult{
			Original:   text,
			Suggested:  text,
			HasChanges: false,
		}, nil
	}

	if client == nil || !client.HasAPIKey() {
		return FallbackProofread(text, "AI engine has no configured API key"), nil
	}

	systemPrompt := `You are an expert Thai and English proofreader for Pocket Sentinel's Telegram bridge.

Your task: Proofread and polish user messages for Telegram communication with STRICT PRESERVATION RULES.

CRITICAL PRESERVATION RULES:
1. URLs: NEVER modify, shorten, translate, or remove any URL (http:// or https://). They must remain character-for-character identical.
2. CODE BLOCKS: NEVER modify, translate, or remove any code blocks, backticks, or inline code (e.g. ` + "`code`" + ` or ` + "```code```" + `).
3. TECHNICAL TERMS: NEVER translate English technical terms into Thai (e.g. keep "PM2", "SQLite", "Termux", "Goroutine", "Gin", "Docker", "API", "CPU", "RAM", "Daemon", "Watchtower", "V11").
4. PROPER NOUNS: NEVER alter proper nouns, names, usernames, or bot commands.
5. INTENDED MEANING: Strictly preserve the original tone, intent, and semantics. Do not hallucinate additional commentary or remove valid content.
6. MINIMAL CORRECTIONS: Fix only genuine spelling mistakes, typos, awkward grammar, and punctuation. If the text is already correct, set "has_changes": false and keep "suggested" identical to original.

OUTPUT FORMAT: Return ONLY a valid JSON object matching this schema without markdown code fences:
{
  "suggested": "corrected text here",
  "has_changes": true,
  "explanation": "concise explanation of changes made in Thai or English"
}`

	userPrompt := fmt.Sprintf("Message to proofread:\n%s", text)

	rawResp, err := client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return FallbackProofread(text, err.Error()), nil
	}

	cleaned := strings.TrimSpace(rawResp)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	var aiResp proofreadAIResponse
	if err := json.Unmarshal([]byte(cleaned), &aiResp); err != nil {
		return FallbackProofread(text, fmt.Sprintf("failed to parse AI JSON response: %v", err)), nil
	}

	suggested := strings.TrimSpace(aiResp.Suggested)
	if suggested == "" {
		return FallbackProofread(text, "empty suggested text from AI"), nil
	}

	// Verify preservation of URLs and code blocks
	if !verifyPreservation(text, suggested) {
		// AI mangled URLs or code blocks; fallback safely to original text
		return FallbackProofread(text, "AI failed preservation check (URL or code altered)"), nil
	}

	hasChanges := aiResp.HasChanges && (suggested != text)
	if !hasChanges {
		suggested = text
	}

	return &ProofreadResult{
		Original:    text,
		Suggested:   suggested,
		HasChanges:  hasChanges,
		Explanation: strings.TrimSpace(aiResp.Explanation),
		Fallback:    false,
	}, nil
}
