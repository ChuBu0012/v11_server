package watchtower

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// Notifier defines the interface to deliver alert messages.
type Notifier interface {
	Send(ctx context.Context, msg string) error
}

// EventLogger logs critical transition events.
type EventLogger interface {
	Log(level, message, metadata string) error
}

// IncidentSummarizer produces an AI/diagnostic summary of an outage incident.
type IncidentSummarizer func(ctx context.Context, w *Watcher, res CheckResult) string

// Service coordinates Watchtower check execution, state machine transitions,
// alert decoupling, and background scheduler.
type Service struct {
	store              *Store
	checker            *Checker
	notifier           Notifier
	pin                string
	logger             *log.Logger
	eventLogger        EventLogger
	incidentSummarizer IncidentSummarizer

	sem    chan struct{} // limits concurrency to MaxConcurrency (3)
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewService creates a new Service instance.
func NewService(store *Store, notifier Notifier, pin string, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}

	return &Service{
		store:    store,
		checker:  NewChecker(nil),
		notifier: notifier,
		pin:      pin,
		logger:   logger,
		sem:      make(chan struct{}, MaxConcurrency),
		stopCh:   make(chan struct{}),
	}
}

// SetChecker sets a custom checker (e.g. for testing with mock transport).
func (s *Service) SetChecker(checker *Checker) {
	if checker != nil {
		s.checker = checker
	}
}

// SetIncidentSummarizer sets an optional AI/diagnostic incident summarizer callback.
func (s *Service) SetIncidentSummarizer(fn IncidentSummarizer) {
	s.incidentSummarizer = fn
}

// SetEventLogger sets a custom event logger.
func (s *Service) SetEventLogger(el EventLogger) {
	s.eventLogger = el
}

// PIN returns configured watchtower PIN.
func (s *Service) PIN() string {
	return s.pin
}

// Store returns underlying Store.
func (s *Service) Store() *Store {
	return s.store
}

// logEvent records events to the EventLogger or database event_logs table.
func (s *Service) logEvent(ctx context.Context, level, message, metadata string) {
	if s.eventLogger != nil {
		_ = s.eventLogger.Log(level, message, metadata)
		return
	}
	if s.store != nil && s.store.db != nil {
		query := "INSERT INTO event_logs (level, message, metadata, created_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)"
		_, _ = s.store.db.ExecContext(ctx, query, level, message, metadata)
	}
	if s.logger != nil {
		s.logger.Printf("[watchtower:%s] %s", level, message)
	}
}

// CheckWatcher executes an availability probe on a watcher by ID.
// If isManual is true, it runs as a diagnostic check:
// - Records probe result in watcher_checks and updates probe metrics on watcher.
// - Does NOT alter failure streaks, down_since, or state.
// - Does NOT trigger Telegram outage or recovery alerts.
func (s *Service) CheckWatcher(ctx context.Context, id int64, isManual bool) (*CheckResult, error) {
	w, err := s.store.GetWatcher(ctx, id)
	if err != nil {
		return nil, err
	}

	res := s.checker.CheckCycle(ctx, w.URL, w.ExpectedStatus)

	// Persist probe evidence into watcher_checks
	if err := s.store.RecordCheck(ctx, w.ID, res); err != nil {
		s.logger.Printf("Failed to record check for watcher %d: %v", w.ID, err)
	}

	// Update telemetry metrics on watcher record
	if err := s.store.UpdateWatcherProbeMetrics(ctx, w.ID, res); err != nil {
		s.logger.Printf("Failed to update watcher probe metrics for %d: %v", w.ID, err)
	}

	// If diagnostic check, return immediately without perturbing state machine
	if isManual {
		return &res, nil
	}

	// Evaluate state machine transitions
	s.evaluateTransition(ctx, w, res)

	return &res, nil
}

// evaluateTransition evaluates health state and manages alert decoupling.
func (s *Service) evaluateTransition(ctx context.Context, w *Watcher, res CheckResult) {
	sanitizedURL := SanitizeURL(w.URL)

	if res.Success {
		// Target is UP
		if w.State == StateDown {
			// RECOVERY TRANSITION
			var downtimeDuration time.Duration
			if w.DownSince != nil {
				downtimeDuration = time.Since(*w.DownSince)
			}
			downtimeStr := FormatDowntime(downtimeDuration)

			// 1. Commit state transition to database FIRST
			if err := s.store.UpdateWatcherState(ctx, w.ID, StateUp, 0, nil); err != nil {
				s.logger.Printf("Failed to update recovery state for watcher %d: %v", w.ID, err)
			}

			// 2. Decoupled Telegram alert delivery
			if s.notifier != nil {
				msg := fmt.Sprintf(
					"🟢 <b>Watchtower Recovery: %s</b>\n"+
						"Target: <code>%s</code>\n"+
						"Status: HTTP %d (Expected %d)\n"+
						"Latency: %dms\n"+
						"Downtime: %s\n"+
						"Vantage: %s",
					w.Name, sanitizedURL, res.StatusCode, w.ExpectedStatus, res.LatencyMs, downtimeStr, DefaultTargetLabel,
				)

				alertCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				sendErr := s.notifier.Send(alertCtx, msg)
				cancel()

				if sendErr != nil {
					// Log failure to event_logs; state remains UP and intact!
					s.logEvent(ctx, "ERROR", fmt.Sprintf("Failed to send recovery alert for %s: %v", w.Name, sendErr), sanitizedURL)
				} else {
					s.logEvent(ctx, "INFO", fmt.Sprintf("Recovery alert sent for %s (downtime: %s)", w.Name, downtimeStr), sanitizedURL)
				}
			}
		} else {
			// Normal up or initial check success
			if err := s.store.UpdateWatcherState(ctx, w.ID, StateUp, 0, nil); err != nil {
				s.logger.Printf("Failed to update state for watcher %d: %v", w.ID, err)
			}
		}
	} else {
		// Target is FAILING
		if w.State == StateDown {
			// Already down — increment streak, NO duplicate alert!
			newStreak := w.ConsecutiveFailures + 1
			if err := s.store.UpdateWatcherState(ctx, w.ID, StateDown, newStreak, w.DownSince); err != nil {
				s.logger.Printf("Failed to update failure streak for watcher %d: %v", w.ID, err)
			}
		} else {
			// Transitioning towards failure
			newStreak := w.ConsecutiveFailures + 1
			downSince := w.DownSince
			if downSince == nil {
				t := res.CheckedAt.UTC()
				downSince = &t
			}

			if newStreak >= FailureStreakThreshold {
				// OUTAGE TRANSITION: 3 consecutive failed cycles
				if err := s.store.UpdateWatcherState(ctx, w.ID, StateDown, newStreak, downSince); err != nil {
					s.logger.Printf("Failed to update outage state for watcher %d: %v", w.ID, err)
				}

				// Decoupled Telegram outage alert delivery
				if s.notifier != nil {
					statusDesc := fmt.Sprintf("HTTP %d", res.StatusCode)
					if res.StatusCode == 0 {
						statusDesc = "Connection Failed"
					}
					reason := SanitizeError(res.Error)
					if reason == "" {
						reason = fmt.Sprintf("HTTP %d (Expected %d)", res.StatusCode, w.ExpectedStatus)
					}

					startTimeStr := downSince.Format("15:04:05 MST")
					msg := fmt.Sprintf(
						"🔴 <b>Watchtower Outage Detected: %s</b>\n"+
							"Target: <code>%s</code>\n"+
							"Status: %s\n"+
							"Reason: %s\n"+
							"Latency: %dms\n"+
							"Consecutive Failures: %d cycles\n"+
							"Outage Started: %s\n"+
							"Vantage: %s",
						w.Name, sanitizedURL, statusDesc, reason, res.LatencyMs, newStreak, startTimeStr, DefaultTargetLabel,
					)

					if s.incidentSummarizer != nil {
						summaryCtx, cancelSum := context.WithTimeout(context.Background(), 10*time.Second)
						aiSummary := s.incidentSummarizer(summaryCtx, w, res)
						cancelSum()
						if aiSummary != "" {
							msg += "\n\n" + aiSummary
						}
					}

					alertCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					sendErr := s.notifier.Send(alertCtx, msg)
					cancel()

					if sendErr != nil {
						// Log failure to event_logs; state remains DOWN and intact!
						s.logEvent(ctx, "ERROR", fmt.Sprintf("Failed to send outage alert for %s: %v", w.Name, sendErr), sanitizedURL)
					} else {
						s.logEvent(ctx, "WARNING", fmt.Sprintf("Outage alert sent for %s (%s)", w.Name, reason), sanitizedURL)
					}
				}
			} else {
				// Failures < 3: increment streak, keep state intact (up or unknown), do not alert
				if err := s.store.UpdateWatcherState(ctx, w.ID, w.State, newStreak, downSince); err != nil {
					s.logger.Printf("Failed to update failure streak for watcher %d: %v", w.ID, err)
				}
			}
		}
	}
}

// RunCycle iterates over all enabled watchers and checks them concurrently (up to 3).
func (s *Service) RunCycle(ctx context.Context) {
	watchers, err := s.store.ListWatchers(ctx)
	if err != nil {
		s.logger.Printf("Watchtower cycle error listing watchers: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, w := range watchers {
		if !w.Enabled || w.State == StatePaused {
			continue
		}

		watcherID := w.ID
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Concurrency semaphore (max 3 concurrent checks)
			s.sem <- struct{}{}
			defer func() { <-s.sem }()

			checkCtx, cancel := context.WithTimeout(ctx, CheckTimeout+5*time.Second)
			defer cancel()

			_, _ = s.CheckWatcher(checkCtx, watcherID, false)
		}()
	}

	wg.Wait()
}

// Start starts background monitoring cycles (alias for StartScheduler).
func (s *Service) Start(ctx context.Context) {
	s.StartScheduler(ctx)
}

// StartScheduler starts background monitoring cycle (5 minutes) with startup jitter (0-60s)
// and scheduled 7-day retention purge.
func (s *Service) StartScheduler(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// 1. Startup Jitter (0-60s) to avoid thundering herd on boot
		jitter := time.Duration(rand.Intn(60)) * time.Second
		s.logger.Printf("Watchtower scheduler starting in %v (startup jitter)...", jitter)

		select {
		case <-time.After(jitter):
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}

		// Initial cycle after jitter
		s.RunCycle(ctx)

		// 5-minute periodic check ticker
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		// 1-hour retention purge ticker
		purgeTicker := time.NewTicker(1 * time.Hour)
		defer purgeTicker.Stop()

		for {
			select {
			case <-ticker.C:
				s.RunCycle(ctx)
			case <-purgeTicker.C:
				if purged, err := s.store.PurgeOldChecks(ctx, RetentionDays); err == nil && purged > 0 {
					s.logger.Printf("Purged %d old watchtower checks (> %d days)", purged, RetentionDays)
				}
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the scheduler and waits for in-flight checks to finish.
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}
