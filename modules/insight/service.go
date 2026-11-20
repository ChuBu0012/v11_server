package insight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Report kinds.
const (
	KindMorning = "morning"
	KindManual  = "manual"
)

const (
	manualCooldown  = 60 * time.Second
	morningHour     = 7
	catchUpUntilHr  = 12
	morningRetryGap = 10 * time.Minute
)

// Sources is everything the report reads; adapters live in sources.go.
type Sources interface {
	Health(ctx context.Context, since time.Time) ([]HealthPoint, error)
	Points(ctx context.Context, device string, from, to time.Time) ([]GeoPoint, error)
	Places(ctx context.Context) ([]Place, error)
	Reminders(ctx context.Context, now time.Time) ([]ReminderItem, error)
}

// Sender delivers the morning message (alerts.TelegramNotifier satisfies it).
type Sender interface {
	Send(ctx context.Context, msg string) error
}

// Report is one stored insight.
type Report struct {
	ID         int64      `json:"id"`
	ReportDate string     `json:"report_date"`
	Kind       string     `json:"kind"`
	Source     string     `json:"source"`
	FactsJSON  string     `json:"-"`
	TextHTML   string     `json:"text_html"`
	CreatedAt  time.Time  `json:"created_at"`
	SentAt     *time.Time `json:"sent_at,omitempty"`
}

// ---------------------------------------------------------------- store

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) (*Store, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS insight_reports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		report_date TEXT NOT NULL,
		kind TEXT NOT NULL,
		source TEXT NOT NULL,
		facts_json TEXT NOT NULL,
		text_html TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		sent_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_insight_reports_date ON insight_reports(report_date, kind);`)
	if err != nil {
		return nil, fmt.Errorf("create insight_reports: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) insert(ctx context.Context, r Report) (Report, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO insight_reports (report_date, kind, source, facts_json, text_html, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, r.ReportDate, r.Kind, r.Source, r.FactsJSON, r.TextHTML, r.CreatedAt.UTC())
	if err != nil {
		return Report{}, fmt.Errorf("insert insight: %w", err)
	}
	r.ID, err = res.LastInsertId()
	return r, err
}

func (s *Store) latest(ctx context.Context) (Report, bool, error) {
	var r Report
	var sent sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id, report_date, kind, source, facts_json, text_html, created_at, sent_at
		FROM insight_reports ORDER BY id DESC LIMIT 1`).
		Scan(&r.ID, &r.ReportDate, &r.Kind, &r.Source, &r.FactsJSON, &r.TextHTML, &r.CreatedAt, &sent)
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, err
	}
	if sent.Valid {
		r.SentAt = &sent.Time
	}
	return r, true, nil
}

func (s *Store) morningSent(ctx context.Context, date string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM insight_reports WHERE report_date=? AND kind=? AND sent_at IS NOT NULL`, date, KindMorning).Scan(&n)
	return n > 0, err
}

func (s *Store) markSent(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE insight_reports SET sent_at=? WHERE id=?`, at.UTC(), id)
	return err
}

// ---------------------------------------------------------------- service

type Service struct {
	store     *Store
	src       Sources
	providers []Provider
	sender    Sender
	clock     func() time.Time
	loc       *time.Location
	mu        sync.Mutex
}

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func NewService(store *Store, src Sources, providers []Provider, sender Sender, opts ...Option) *Service {
	s := &Service{store: store, src: src, providers: providers, sender: sender, clock: time.Now, loc: bangkok()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Latest returns the most recent report.
func (s *Service) Latest(ctx context.Context) (Report, bool, error) { return s.store.latest(ctx) }

// Generate builds and stores a report. Manual runs within 60 s of the last
// report return that report instead (cached=true) without calling any AI.
func (s *Service) Generate(ctx context.Context, kind string) (Report, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generateLocked(ctx, kind)
}

func (s *Service) generateLocked(ctx context.Context, kind string) (Report, bool, error) {
	now := s.clock()
	if kind == KindManual {
		if last, ok, err := s.store.latest(ctx); err == nil && ok && now.Sub(last.CreatedAt) < manualCooldown {
			return last, true, nil
		}
	}
	facts := s.buildFacts(ctx, now)
	text, source := Narrate(ctx, facts, s.providers)
	factsJSON, _ := json.Marshal(facts)
	r, err := s.store.insert(ctx, Report{ReportDate: facts.Date, Kind: kind, Source: source,
		FactsJSON: string(factsJSON), TextHTML: text, CreatedAt: now})
	return r, false, err
}

// buildFacts gathers inputs; a failing source leaves its section empty
// rather than cancelling the whole report.
func (s *Service) buildFacts(ctx context.Context, now time.Time) Facts {
	today := startOfDay(now, s.loc)
	yesterday := today.AddDate(0, 0, -1)
	weekStart := yesterday.AddDate(0, 0, -6)
	health, err := s.src.Health(ctx, now.Add(-72*time.Hour))
	logSourceErr("health", err)
	places, err := s.src.Places(ctx)
	logSourceErr("places", err)
	me, err := s.src.Points(ctx, "me", weekStart, today)
	logSourceErr("points me", err)
	friend, err := s.src.Points(ctx, "friend", weekStart, today)
	logSourceErr("points friend", err)
	reminders, err := s.src.Reminders(ctx, now)
	logSourceErr("reminders", err)
	return Facts{
		Date:     now.In(s.loc).Format("2006-01-02"),
		Server:   ComputeServer(health, now),
		Me:       ComputePerson("Chubu", me, places, yesterday, s.loc),
		Friend:   ComputePerson("Furturia", friend, places, yesterday, s.loc),
		Together: ComputeTogether(me, friend, yesterday, s.loc),
		Today:    ComputeToday(reminders, now, s.loc),
	}
}

func logSourceErr(name string, err error) {
	if err != nil {
		log.Printf("[insight] source %s failed: %v", name, err)
	}
}

// SendMorning sends today's morning report once; repeated calls are no-ops.
func (s *Service) SendMorning(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	date := s.clock().In(s.loc).Format("2006-01-02")
	if sent, err := s.store.morningSent(ctx, date); err != nil || sent {
		return err
	}
	r, _, err := s.generateLocked(ctx, KindMorning)
	if err != nil {
		return err
	}
	if s.sender == nil {
		return errors.New("insight: no sender configured")
	}
	if err := s.sender.Send(ctx, r.TextHTML); err != nil {
		return fmt.Errorf("send morning insight: %w", err)
	}
	return s.store.markSent(ctx, r.ID, s.clock())
}

// NextMorning returns the next 07:00 Bangkok strictly after now.
func NextMorning(now time.Time, loc *time.Location) time.Time {
	at := startOfDay(now, loc).Add(morningHour * time.Hour)
	if !at.After(now) {
		at = at.AddDate(0, 0, 1)
	}
	return at
}

// catchUp sends the morning report when the server starts between 07:00 and
// 12:00 and today's report has not gone out (e.g. a deploy at 07:00).
func (s *Service) catchUp(ctx context.Context) {
	now := s.clock().In(s.loc)
	if now.Hour() < morningHour || now.Hour() >= catchUpUntilHr {
		return
	}
	if err := s.SendMorning(ctx); err != nil {
		log.Printf("[insight] morning catch-up failed: %v", err)
	}
}

// RunScheduler catches up, then sends at every 07:00 until ctx is done.
// A failed send is retried every 10 minutes until 12:00.
func (s *Service) RunScheduler(ctx context.Context) {
	s.catchUp(ctx)
	for {
		wait := NextMorning(s.clock(), s.loc).Sub(s.clock())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		for {
			err := s.SendMorning(ctx)
			if err == nil {
				break
			}
			log.Printf("[insight] morning send failed: %v", err)
			if s.clock().In(s.loc).Hour() >= catchUpUntilHr {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(morningRetryGap):
			}
		}
	}
}

// ---------------------------------------------------------------- HTTP

// RegisterRoutes mounts the public insight API.
func (s *Service) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/insight/latest", func(c *gin.Context) {
		rep, ok, err := s.Latest(c.Request.Context())
		switch {
		case err != nil:
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "โหลดสรุปไม่สำเร็จ"})
		case !ok:
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "ยังไม่มีสรุป"})
		default:
			c.JSON(http.StatusOK, rep)
		}
	})
	r.POST("/api/insight/run", func(c *gin.Context) {
		rep, cached, err := s.Generate(c.Request.Context(), KindManual)
		switch {
		case err != nil:
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "สร้างสรุปไม่สำเร็จ"})
		case cached:
			c.JSON(http.StatusTooManyRequests, rep)
		default:
			c.JSON(http.StatusOK, rep)
		}
	})
}
