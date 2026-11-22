package bot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"v11-server/shared/db"
)

func setupTestAskCacheDB(t *testing.T) (*AskCacheStore, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "ask-cache-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test_ask.db")
	database, err := db.InitDB(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("InitDB failed: %v", err)
	}

	store := NewAskCacheStore(database)
	cleanup := func() {
		database.Close()
		os.RemoveAll(tempDir)
	}
	return store, cleanup
}

func TestAskCacheStore(t *testing.T) {
	store, cleanup := setupTestAskCacheDB(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Test ComputeHash normalization
	h1 := store.ComputeHash("  ทำไม  ท้องฟ้า สีฟ้า ", "auto")
	h2 := store.ComputeHash("ทำไม ท้องฟ้า สีฟ้า", "AUTO")
	if h1 != h2 {
		t.Errorf("ComputeHash mismatch for normalized input: %s != %s", h1, h2)
	}

	// 2. Test Get on empty cache (miss)
	entry, err := store.Get(ctx, "คำถามทดสอบ", "auto")
	if err != nil {
		t.Fatalf("Get on empty cache returned error: %v", err)
	}
	if entry != nil {
		t.Fatalf("Expected nil entry on cache miss, got: %+v", entry)
	}

	// 3. Test Set and Get (hit)
	testPrompt := "ทำไมทองคำราคาขึ้น"
	testResponse := "ทองคำราคาขึ้นเนื่องจากเงินเฟ้อและความไม่แน่นอนทางเศรษฐกิจ"
	err = store.Set(ctx, testPrompt, "groq", testResponse, 1*time.Hour)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	hit, err := store.Get(ctx, testPrompt, "groq")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if hit == nil {
		t.Fatalf("Expected cache hit, got nil")
	}
	if hit.ResponseText != testResponse {
		t.Errorf("ResponseText = %q, want %q", hit.ResponseText, testResponse)
	}
	if hit.ModelName != "groq" {
		t.Errorf("ModelName = %q, want %q", hit.ModelName, "groq")
	}

	// 4. Test Upsert (overwrite existing response)
	updatedResponse := "คำตอบที่อัปเดตใหม่"
	err = store.Set(ctx, testPrompt, "groq", updatedResponse, 1*time.Hour)
	if err != nil {
		t.Fatalf("Set overwrite failed: %v", err)
	}
	hitUpdated, err := store.Get(ctx, testPrompt, "groq")
	if err != nil || hitUpdated == nil {
		t.Fatalf("Get updated failed: %v", err)
	}
	if hitUpdated.ResponseText != updatedResponse {
		t.Errorf("ResponseText = %q, want %q", hitUpdated.ResponseText, updatedResponse)
	}

	// 5. Test Expiration (negative TTL)
	expiredPrompt := "คำถามหมดอายุ"
	err = store.Set(ctx, expiredPrompt, "auto", "คำตอบเก่า", -1*time.Hour)
	if err != nil {
		t.Fatalf("Set expired failed: %v", err)
	}
	expiredHit, err := store.Get(ctx, expiredPrompt, "auto")
	if err != nil {
		t.Fatalf("Get expired error: %v", err)
	}
	if expiredHit != nil {
		t.Errorf("Expected nil for expired entry, got: %+v", expiredHit)
	}

	// 6. Test PruneExpired
	pruned, err := store.PruneExpired(ctx)
	if err != nil {
		t.Fatalf("PruneExpired failed: %v", err)
	}
	if pruned < 1 {
		t.Errorf("Expected at least 1 pruned entry, got %d", pruned)
	}

	// 7. Test Nil DB safety
	nilStore := NewAskCacheStore(nil)
	nilEntry, nilErr := nilStore.Get(ctx, "foo", "bar")
	if nilErr != nil || nilEntry != nil {
		t.Errorf("Nil DB Get expected nil, nil; got %v, %v", nilEntry, nilErr)
	}
	if err := nilStore.Set(ctx, "foo", "bar", "baz", time.Hour); err != nil {
		t.Errorf("Nil DB Set expected nil error, got %v", err)
	}
}
