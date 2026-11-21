package reminders

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStore_CreateAndQuery(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	futureTime := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)

	input := Input{
		Title:      "ประชุมทีม",
		Details:    "สรุป roadmap ประจำไตรมาส",
		StartsAt:   futureTime,
		Recurrence: "once",
		Offsets:    []int{0, 15},
	}

	created, err := store.Create(ctx, input)
	if err != nil {
		t.Fatalf("create reminder: %v", err)
	}
	if created.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if created.Title != "ประชุมทีม" {
		t.Errorf("expected title 'ประชุมทีม', got %s", created.Title)
	}

	// List
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list reminders: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(list))
	}

	// Query pending deliveries due by futureTime + 1 minute (Due calls ensureOccurrences internally)
	dueAt := time.Now().Add(2*time.Hour + time.Minute)
	due, err := store.Due(ctx, dueAt)
	if err != nil {
		t.Fatalf("query due deliveries: %v", err)
	}
	if len(due) == 0 {
		t.Fatalf("expected at least 1 due delivery")
	}

	// Test MarkSent
	deliveryID := due[0].ID
	if err := store.MarkSent(ctx, deliveryID, 999); err != nil {
		t.Fatalf("mark sent: %v", err)
	}

	// Test Snooze
	if len(due) > 1 {
		secondID := due[1].ID
		if err := store.MarkSent(ctx, secondID, 1000); err != nil {
			t.Fatalf("mark sent: %v", err)
		}
		if err := store.Snooze(ctx, secondID, 10); err != nil {
			t.Fatalf("snooze: %v", err)
		}
	}

	// Test Complete
	startsTime, _ := time.Parse(time.RFC3339, futureTime)
	if err := store.Complete(ctx, created.ID, startsTime); err != nil {
		t.Fatalf("complete reminder: %v", err)
	}

	// History
	hist, err := store.History(ctx)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) == 0 {
		t.Fatalf("expected at least 1 history item")
	}
}
