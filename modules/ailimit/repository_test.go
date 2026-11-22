package ailimit

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	schema := `
	CREATE TABLE ai_quota_states (
		provider TEXT NOT NULL,
		group_name TEXT NOT NULL,
		bucket_id TEXT NOT NULL,
		display_name TEXT NOT NULL,
		window TEXT NOT NULL,
		used_percent REAL NOT NULL,
		remaining_fraction REAL NOT NULL,
		reset_time DATETIME NOT NULL,
		last_checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, bucket_id)
	);

	CREATE TABLE ai_quota_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT NOT NULL,
		group_name TEXT NOT NULL,
		bucket_id TEXT NOT NULL,
		remaining_fraction REAL NOT NULL,
		reset_time DATETIME NOT NULL,
		recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	return db
}

func TestRepository_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	buckets := []QuotaBucket{
		{
			Provider:          "antigravity",
			GroupName:         "Gemini Models",
			BucketID:          "gemini-5h",
			DisplayName:       "Five Hour Limit Remaining",
			Window:            "5h",
			UsedPercent:       30.0,
			RemainingPercent:  70.0,
			RemainingFraction: 0.70,
			ResetTime:         now.Add(2 * time.Hour),
			LastCheckedAt:     now,
			IsLow:             false,
		},
		{
			Provider:          "antigravity",
			GroupName:         "Claude and GPT models",
			BucketID:          "3p-weekly",
			DisplayName:       "Weekly Limit Remaining",
			Window:            "weekly",
			UsedPercent:       97.0,
			RemainingPercent:  3.0,
			RemainingFraction: 0.03,
			ResetTime:         now.Add(48 * time.Hour),
			LastCheckedAt:     now,
			IsLow:             true,
		},
	}

	if err := repo.SaveStates(ctx, buckets); err != nil {
		t.Fatalf("SaveStates failed: %v", err)
	}
	if err := repo.SaveSnapshots(ctx, buckets, now); err != nil {
		t.Fatalf("SaveSnapshots failed: %v", err)
	}

	// Verify states
	latest, err := repo.GetLatestStates(ctx)
	if err != nil {
		t.Fatalf("GetLatestStates failed: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected 2 states, got %d", len(latest))
	}

	// Verify history
	history, err := repo.GetHistory(ctx, 24)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(history))
	}
	if history[0].BucketID != "gemini-5h" || history[1].BucketID != "3p-weekly" {
		t.Errorf("history bucket mismatch: %+v", history)
	}
}
