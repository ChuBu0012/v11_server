package bot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DefaultAskCacheTTL is the standard time-to-live for cached /ask responses (6 hours).
const DefaultAskCacheTTL = 6 * time.Hour

var multiSpaceRe = regexp.MustCompile(`\s+`)

// AskCacheEntry represents a cached Q&A pair stored in SQLite.
type AskCacheEntry struct {
	ID           int64
	PromptHash   string
	PromptText   string
	ModelName    string
	ResponseText string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// AskCacheStore manages persistent storage and retrieval of /ask query answers.
type AskCacheStore struct {
	db *sql.DB
}

// NewAskCacheStore creates a new AskCacheStore instance.
func NewAskCacheStore(db *sql.DB) *AskCacheStore {
	return &AskCacheStore{db: db}
}

// ComputeHash generates a deterministic SHA-256 hash for normalized prompt and model.
func (s *AskCacheStore) ComputeHash(prompt, model string) string {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	normalized = multiSpaceRe.ReplaceAllString(normalized, " ")
	normModel := strings.ToLower(strings.TrimSpace(model))
	payload := normModel + "|" + normalized
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// Get retrieves a valid (unexpired) cached response for the given prompt and model.
// Returns nil, nil if no valid cache entry exists.
func (s *AskCacheStore) Get(ctx context.Context, prompt, model string) (*AskCacheEntry, error) {
	if s.db == nil {
		return nil, nil
	}
	hash := s.ComputeHash(prompt, model)
	query := `
		SELECT id, prompt_hash, prompt_text, model_name, response_text, created_at, expires_at
		FROM ask_cache
		WHERE prompt_hash = ? AND datetime(expires_at) > datetime('now')
		LIMIT 1;
	`
	var entry AskCacheEntry
	err := s.db.QueryRowContext(ctx, query, hash).Scan(
		&entry.ID,
		&entry.PromptHash,
		&entry.PromptText,
		&entry.ModelName,
		&entry.ResponseText,
		&entry.CreatedAt,
		&entry.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ask cache query failed: %w", err)
	}
	return &entry, nil
}

// Set stores or updates a cached response with the specified TTL.
func (s *AskCacheStore) Set(ctx context.Context, prompt, model, response string, ttl time.Duration) error {
	if s.db == nil {
		return nil
	}
	hash := s.ComputeHash(prompt, model)
	expiresAt := time.Now().UTC().Add(ttl).Format("2006-01-02 15:04:05")

	query := `
		INSERT INTO ask_cache (prompt_hash, prompt_text, model_name, response_text, created_at, expires_at)
		VALUES (?, ?, ?, ?, datetime('now'), ?)
		ON CONFLICT(prompt_hash) DO UPDATE SET
			response_text = excluded.response_text,
			expires_at = excluded.expires_at,
			created_at = datetime('now');
	`
	_, err := s.db.ExecContext(ctx, query, hash, strings.TrimSpace(prompt), strings.TrimSpace(model), response, expiresAt)
	if err != nil {
		return fmt.Errorf("ask cache insert failed: %w", err)
	}

	// Opportunistic asynchronous cleanup of stale records
	go func() {
		pruneCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = s.PruneExpired(pruneCtx)
	}()

	return nil
}

// PruneExpired deletes all expired cache entries from SQLite.
func (s *AskCacheStore) PruneExpired(ctx context.Context) (int64, error) {
	if s.db == nil {
		return 0, nil
	}
	query := `DELETE FROM ask_cache WHERE datetime(expires_at) <= datetime('now');`
	res, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("ask cache prune failed: %w", err)
	}
	return res.RowsAffected()
}
