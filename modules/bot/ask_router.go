package bot

import (
	"context"
	"errors"
	"strings"
	"time"

	"v11-server/shared/ai"
)

// TriageVerdict indicates whether an /ask query should be handled by Fast LLM or delegated to agy.
type TriageVerdict string

const (
	VerdictFastLLM TriageVerdict = "FAST"
	VerdictAgent   TriageVerdict = "AGY"
)

var (
	// ErrNoFastEngine is returned when fast execution is attempted without a valid AI engine.
	ErrNoFastEngine = errors.New("ai engine unconfigured or has no api keys")
)

var dynamicKeywords = []string{
	"แบต", "battery", "ram", "disk", "uptime",
	"พิกัด", "gps", "location",
	"โควต้า", "quota",
	"git", "commit",
	"log", "logs",
	"ตอนนี้", "เวลา", "กี่โมง", "ปัจจุบัน", "ล่าสุด",
	"ทอง", "หุ้น",
}

var agentKeywords = []string{
	"แก้", "fix", "แก้ไข", "เขียนไฟล์", "create file", "edit",
	"รัน", "run", "exec", "shell", "terminal", "bash",
	"deploy", "restart", "reboot", "recompile", "build",
	"โค้ด", "code", "บั๊ก", "bug", "refactor",
	"อ่านไฟล์", "read file", "inspect", "debug",
}

// AskExecutionResult represents the outcome of processing an /ask query via Fast LLM.
type AskExecutionResult struct {
	ResponseText string
	Provider     string
	IsCached     bool
	CachedAt     time.Time
	Duration     time.Duration
}

// AskEngine defines the AI interface needed by AskTriageRouter.
type AskEngine interface {
	GenerateWithModel(ctx context.Context, model string, req ai.Request) (string, string, error)
	HasAPIKey() bool
}

// AskTriageRouter coordinates dynamic query detection, agent triage, Fast LLM execution, and SQLite caching.
type AskTriageRouter struct {
	aiEngine   AskEngine
	cacheStore *AskCacheStore
}

// NewAskTriageRouter constructs a new AskTriageRouter.
func NewAskTriageRouter(engine AskEngine, cacheStore *AskCacheStore) *AskTriageRouter {
	return &AskTriageRouter{
		aiEngine:   engine,
		cacheStore: cacheStore,
	}
}

// IsDynamicQuery reports true if the prompt references real-time status where cached results would be stale.
func IsDynamicQuery(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, kw := range dynamicKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// RequiresAgent reports true if the prompt contains actions or tasks requiring agent tools (git, shell, code).
func RequiresAgent(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, kw := range agentKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// Triage decides whether the query should be handled by Fast LLM or delegated to agy.
func (r *AskTriageRouter) Triage(ctx context.Context, prompt string, requestedModel string) (TriageVerdict, error) {
	normModel := strings.ToLower(strings.TrimSpace(requestedModel))

	// 1. Explicit model overrides
	if normModel == "agy" || normModel == "agent" {
		return VerdictAgent, nil
	}
	if normModel == "groq" || normModel == "gemini" || normModel == "openrouter" {
		return VerdictFastLLM, nil
	}

	// 2. Heuristic check: does it require tools or filesystem access?
	if RequiresAgent(prompt) {
		return VerdictAgent, nil
	}

	// 3. If AI Engine is not configured, fall back to agy
	if r.aiEngine == nil || !r.aiEngine.HasAPIKey() {
		return VerdictAgent, nil
	}

	// 4. Two-stage fast classification for ambiguous queries under auto
	triageReq := ai.Request{
		SystemPrompt: "You are an AI triage router. Answer with ONLY one word: 'AGY' if the user request requires coding, editing files, running shell commands, git operations, or local server debugging; or 'FAST' if it is a general question, explanation, translation, calculation, or fact lookup.",
		UserPrompt:   prompt,
		MaxTokens:    10,
		Timeout:      3 * time.Second,
	}

	// Use Groq or first available for ultra-low latency triage
	ans, _, err := r.aiEngine.GenerateWithModel(ctx, "groq", triageReq)
	if err == nil {
		clean := strings.ToUpper(strings.TrimSpace(ans))
		if strings.Contains(clean, "AGY") {
			return VerdictAgent, nil
		}
	}

	return VerdictFastLLM, nil
}

// ExecuteFast processes the prompt using Fast LLM with SQLite caching.
func (r *AskTriageRouter) ExecuteFast(ctx context.Context, opts AskOptions) (*AskExecutionResult, error) {
	if r.aiEngine == nil || !r.aiEngine.HasAPIKey() {
		return nil, ErrNoFastEngine
	}

	isDynamic := IsDynamicQuery(opts.CleanPrompt)
	shouldCheckCache := !opts.NoCache && !isDynamic && r.cacheStore != nil

	// 1. Check cache
	if shouldCheckCache {
		entry, err := r.cacheStore.Get(ctx, opts.CleanPrompt, opts.Model)
		if err == nil && entry != nil {
			return &AskExecutionResult{
				ResponseText: entry.ResponseText,
				Provider:     entry.ModelName,
				IsCached:     true,
				CachedAt:     entry.CreatedAt,
				Duration:     0,
			}, nil
		}
	}

	// 2. Execute via Fast LLM
	start := time.Now()
	systemPrompt := "You are Pocket Sentinel Assistant, a direct, concise, and helpful AI running on vivo V11. " +
		"Answer clearly, accurately, and concisely in Thai (or English if prompted). " +
		"Avoid unnecessary pleasantries or filler to conserve bandwidth and quota."

	req := ai.Request{
		SystemPrompt: systemPrompt,
		UserPrompt:   opts.CleanPrompt,
		Temperature:  0.4,
		Timeout:      15 * time.Second,
	}

	content, providerName, err := r.aiEngine.GenerateWithModel(ctx, opts.Model, req)
	if err != nil {
		return nil, err
	}
	elapsed := time.Since(start)

	// 3. Store in cache if eligible
	if !opts.NoCache && !isDynamic && r.cacheStore != nil {
		_ = r.cacheStore.Set(ctx, opts.CleanPrompt, opts.Model, content, DefaultAskCacheTTL)
	}

	return &AskExecutionResult{
		ResponseText: content,
		Provider:     providerName,
		IsCached:     false,
		Duration:     elapsed,
	}, nil
}
