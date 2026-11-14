package alerts

import (
	"context"
	"fmt"
	"log"
	"time"
)

// EventLogger is the interface for recording transition events.
// This matches the RecordLog method on modules/logs.Module without
// importing it directly, keeping the alerts module decoupled.
type EventLogger interface {
	RecordLog(level, message, metadata string) error
}

// AlertManager ties the evaluator, state store, and notifier together.
type AlertManager struct {
	store      *AlertStore
	notifier   Notifier
	thresholds Thresholds
	logger     EventLogger
	timeout    time.Duration // delivery timeout per notification
}

// AlertManagerOption allows customization of AlertManager.
type AlertManagerOption func(*AlertManager)

// WithDeliveryTimeout sets the timeout for each notification dispatch.
func WithDeliveryTimeout(d time.Duration) AlertManagerOption {
	return func(m *AlertManager) { m.timeout = d }
}

// NewAlertManager creates a new AlertManager.
func NewAlertManager(store *AlertStore, notifier Notifier, thresholds Thresholds, logger EventLogger, opts ...AlertManagerOption) *AlertManager {
	m := &AlertManager{
		store:      store,
		notifier:   notifier,
		thresholds: thresholds,
		logger:     logger,
		timeout:    15 * time.Second,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ProcessSample evaluates a sample against thresholds, detects state transitions,
// logs events, and dispatches notifications asynchronously.
// This method never blocks on notification delivery.
func (m *AlertManager) ProcessSample(sample Sample) error {
	conditions := Evaluate(sample, m.thresholds)

	// Load persisted state
	states, err := m.store.LoadState()
	if err != nil {
		log.Printf("[alerts] failed to load alert state: %v", err)
		// Continue with empty state — safer than skipping evaluation
		states = make(map[ConditionType]*AlertState)
	}

	now := time.Now().UTC()

	for _, cond := range conditions {
		prev := states[cond.Type]
		kind, newState := Transition(prev, cond, now)

		// Persist updated state
		if err := m.store.SaveState(cond.Type, newState); err != nil {
			log.Printf("[alerts] failed to save state for %s: %v", cond.Type, err)
			continue
		}

		// On recovery, clear the persisted state
		if kind == TransitionToRecovery {
			if err := m.store.ClearState(cond.Type); err != nil {
				log.Printf("[alerts] failed to clear state for %s: %v", cond.Type, err)
			}
		}

		if kind == NoTransition {
			continue
		}

		// Build notification message
		msg := m.buildMessage(kind, cond, newState)

		// Log the transition event
		eventLevel := "warning"
		if kind == TransitionToRecovery {
			eventLevel = "info"
		}
		if m.logger != nil {
			_ = m.logger.RecordLog(eventLevel, fmt.Sprintf("Alert %s: %s", kind, cond.Type), msg)
		}

		// Dispatch notification asynchronously — never blocks ProcessSample
		m.dispatchAsync(msg)
	}

	return nil
}

// buildMessage creates a human-readable notification message.
func (m *AlertManager) buildMessage(kind TransitionKind, cond Condition, state AlertState) string {
	switch kind {
	case TransitionToAlert:
		return fmt.Sprintf("🚨 <b>ALERT: %s</b>\n%s\nTriggered at: %s",
			cond.Type, cond.Message, state.TriggeredAt.Format(time.RFC3339))
	case TransitionToRecovery:
		return fmt.Sprintf("✅ <b>RECOVERED: %s</b>\n%s\nRecovered at: %s",
			cond.Type, cond.Message, state.LastCheckedAt.Format(time.RFC3339))
	default:
		return cond.Message
	}
}

// dispatchAsync sends a notification in a goroutine with a bounded timeout.
// Failed delivery is logged but does not crash or retry immediately.
func (m *AlertManager) dispatchAsync(msg string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()

		if err := m.notifier.Send(ctx, msg); err != nil {
			log.Printf("[alerts] notification delivery failed: %v", err)
			if m.logger != nil {
				_ = m.logger.RecordLog("error", "Alert notification delivery failed", err.Error())
			}
		}
	}()
}
