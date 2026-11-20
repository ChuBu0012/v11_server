package insight

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"v11-server/shared/db"
)

type emptySources struct{}

func (emptySources) Health(context.Context, time.Time) ([]HealthPoint, error) { return nil, nil }
func (emptySources) Points(context.Context, string, time.Time, time.Time) ([]GeoPoint, error) {
	return nil, nil
}
func (emptySources) Places(context.Context) ([]Place, error)                      { return nil, nil }
func (emptySources) Reminders(context.Context, time.Time) ([]ReminderItem, error) { return nil, nil }

type countingSender struct {
	mu   sync.Mutex
	msgs []string
}

func (c *countingSender) Send(ctx context.Context, msg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, msg)
	return nil
}

func newTestService(t *testing.T, now *time.Time, gen TextGenerator) (*Service, *countingSender) {
	t.Helper()
	database, err := db.InitDB(filepath.Join(t.TempDir(), "insight.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	sender := &countingSender{}
	var providers []Provider
	if gen != nil {
		providers = []Provider{{Name: "openrouter", Gen: gen, Timeout: time.Second}}
	}
	svc := NewService(store, emptySources{}, providers, sender, WithClock(func() time.Time { return *now }))
	return svc, sender
}

func TestNextMorning(t *testing.T) {
	if got := NextMorning(time.Date(2026, 11, 21, 6, 59, 0, 0, bkk), bkk); !got.Equal(time.Date(2026, 11, 21, 7, 0, 0, 0, bkk)) {
		t.Fatalf("06:59 -> %v", got)
	}
	if got := NextMorning(time.Date(2026, 11, 21, 7, 0, 1, 0, bkk), bkk); !got.Equal(time.Date(2026, 11, 22, 7, 0, 0, 0, bkk)) {
		t.Fatalf("07:00:01 -> %v", got)
	}
}

func TestMorningSendIsIdempotent(t *testing.T) {
	now := time.Date(2026, 11, 21, 7, 0, 0, 0, bkk)
	svc, sender := newTestService(t, &now, nil)
	ctx := context.Background()
	if err := svc.SendMorning(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := svc.SendMorning(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sender.msgs) != 1 || !strings.Contains(sender.msgs[0], "สรุปเช้า") {
		t.Fatalf("sent %d messages: %v", len(sender.msgs), sender.msgs)
	}
}

func TestMorningCatchUpWindow(t *testing.T) {
	late := time.Date(2026, 11, 21, 13, 0, 0, 0, bkk)
	svcLate, senderLate := newTestService(t, &late, nil)
	svcLate.catchUp(context.Background())
	if len(senderLate.msgs) != 0 {
		t.Fatal("catch-up must not send after 12:00")
	}
	morning := time.Date(2026, 11, 21, 8, 0, 0, 0, bkk)
	svc, sender := newTestService(t, &morning, nil)
	svc.catchUp(context.Background())
	if len(sender.msgs) != 1 {
		t.Fatalf("catch-up at 08:00 sent %d, want 1", len(sender.msgs))
	}
}

func TestServiceCooldown(t *testing.T) {
	now := time.Date(2026, 11, 21, 9, 0, 0, 0, bkk)
	gen := &stubGen{err: context.DeadlineExceeded}
	svc, _ := newTestService(t, &now, gen)
	ctx := context.Background()
	first, cached, err := svc.Generate(ctx, KindManual)
	if err != nil || cached || first.Source != "template" {
		t.Fatalf("first = %+v cached=%v err=%v", first, cached, err)
	}
	now = now.Add(30 * time.Second)
	second, cached, err := svc.Generate(ctx, KindManual)
	if err != nil || !cached || second.ID != first.ID {
		t.Fatalf("second = %+v cached=%v err=%v", second, cached, err)
	}
	if gen.calls != 1 {
		t.Fatalf("provider called %d times, want 1", gen.calls)
	}
	now = now.Add(time.Minute)
	if _, cached, _ := svc.Generate(ctx, KindManual); cached {
		t.Fatal("cooldown must expire after 60 s")
	}
}
