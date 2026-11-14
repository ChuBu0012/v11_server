package alerts

import (
	"fmt"
	"time"
)

// ConditionType identifies a distinct alert condition.
type ConditionType string

const (
	LowBattery     ConditionType = "low_battery"
	LowDisk        ConditionType = "low_disk"
	ServiceFailure ConditionType = "service_failure"
)

// Thresholds holds the configurable alert thresholds.
type Thresholds struct {
	BatteryPercent float64 // alert when battery <= this (e.g. 20)
	DiskFreeGB     float64 // alert when free disk <= this (e.g. 5.0)
}

// Sample represents a single health snapshot fed to the evaluator.
// Fields use pointers for nullable values (e.g. battery unavailable).
type Sample struct {
	Timestamp     time.Time
	UptimeSeconds int64
	MemAllocMB    float64
	MemSysMB      float64
	DiskTotalGB   float64
	DiskFreeGB    float64
	BatteryLevel  *float64 // nil = unavailable
	BatteryStatus *string  // nil = unavailable
	ServiceOK     bool     // true = health check passed
}

// Condition is the result of evaluating a single alert rule against a sample.
type Condition struct {
	Type      ConditionType `json:"type"`
	Active    bool          `json:"active"`    // true = problem detected
	Value     float64       `json:"value"`     // observed value
	Threshold float64       `json:"threshold"` // configured threshold
	Message   string        `json:"message"`   // human-readable description
}

// Evaluate is a pure function: given a sample and thresholds, it returns
// the current condition state for every known alert type.
func Evaluate(s Sample, t Thresholds) []Condition {
	var conditions []Condition

	// Low battery — only evaluated when battery data is available.
	if s.BatteryLevel != nil {
		level := *s.BatteryLevel
		active := level <= t.BatteryPercent
		msg := fmt.Sprintf("Battery at %.0f%% (threshold: %.0f%%)", level, t.BatteryPercent)
		if active {
			msg = fmt.Sprintf("⚠️ Low battery: %.0f%% (threshold: %.0f%%)", level, t.BatteryPercent)
		}
		conditions = append(conditions, Condition{
			Type:      LowBattery,
			Active:    active,
			Value:     level,
			Threshold: t.BatteryPercent,
			Message:   msg,
		})
	}

	// Low disk
	{
		active := s.DiskFreeGB <= t.DiskFreeGB
		msg := fmt.Sprintf("Disk free: %.2f GB (threshold: %.2f GB)", s.DiskFreeGB, t.DiskFreeGB)
		if active {
			msg = fmt.Sprintf("⚠️ Low disk: %.2f GB free (threshold: %.2f GB)", s.DiskFreeGB, t.DiskFreeGB)
		}
		conditions = append(conditions, Condition{
			Type:      LowDisk,
			Active:    active,
			Value:     s.DiskFreeGB,
			Threshold: t.DiskFreeGB,
			Message:   msg,
		})
	}

	// Service failure
	{
		active := !s.ServiceOK
		var msg string
		if active {
			msg = "⚠️ Service health check failed"
		} else {
			msg = "Service healthy"
		}
		conditions = append(conditions, Condition{
			Type:      ServiceFailure,
			Active:    active,
			Value:     boolToFloat(s.ServiceOK),
			Threshold: 1, // 1 = must be healthy
			Message:   msg,
		})
	}

	return conditions
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// --- Edge-triggered state machine ---

// TransitionKind represents the type of state change.
type TransitionKind int

const (
	NoTransition TransitionKind = iota
	TransitionToAlert    // OK -> Problem
	TransitionToRecovery // Problem -> OK
)

func (t TransitionKind) String() string {
	switch t {
	case TransitionToAlert:
		return "alert"
	case TransitionToRecovery:
		return "recovery"
	default:
		return "none"
	}
}

// AlertState tracks the current state of a single condition for edge detection.
type AlertState struct {
	ConditionType ConditionType
	Active        bool
	TriggeredAt   time.Time
	LastCheckedAt time.Time
	ValueAtTrigger float64
}

// Transition computes the edge-triggered transition for a condition against
// its previously persisted state. Returns the kind of transition and the
// updated AlertState. The caller is responsible for persisting the new state.
func Transition(prev *AlertState, cond Condition, now time.Time) (TransitionKind, AlertState) {
	next := AlertState{
		ConditionType: cond.Type,
		Active:        cond.Active,
		LastCheckedAt: now,
	}

	wasActive := prev != nil && prev.Active

	switch {
	case cond.Active && !wasActive:
		// OK -> Problem
		next.TriggeredAt = now
		next.ValueAtTrigger = cond.Value
		return TransitionToAlert, next

	case !cond.Active && wasActive:
		// Problem -> OK (recovery)
		// Keep triggered_at for the recovery message context
		if prev != nil {
			next.TriggeredAt = prev.TriggeredAt
			next.ValueAtTrigger = prev.ValueAtTrigger
		}
		next.Active = false
		return TransitionToRecovery, next

	case cond.Active && wasActive:
		// Problem -> Problem (suppress)
		if prev != nil {
			next.TriggeredAt = prev.TriggeredAt
			next.ValueAtTrigger = prev.ValueAtTrigger
		}
		return NoTransition, next

	default:
		// OK -> OK (no change)
		return NoTransition, next
	}
}
