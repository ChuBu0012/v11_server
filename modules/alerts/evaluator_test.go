package alerts

import (
	"testing"
	"time"
)

func TestEvaluateThresholds(t *testing.T) {
	thresh := Thresholds{
		BatteryPercent: 20.0,
		DiskFreeGB:     5.0,
	}

	tests := []struct {
		name          string
		sample        Sample
		expectBattery *bool // nil if battery condition not expected
		expectDisk    bool
		expectService bool
	}{
		{
			name: "all healthy",
			sample: Sample{
				BatteryLevel: ptrFloat(50.0),
				DiskFreeGB:   15.0,
				ServiceOK:    true,
			},
			expectBattery: ptrBool(false),
			expectDisk:    false,
			expectService: false,
		},
		{
			name: "low battery exactly at threshold",
			sample: Sample{
				BatteryLevel: ptrFloat(20.0),
				DiskFreeGB:   10.0,
				ServiceOK:    true,
			},
			expectBattery: ptrBool(true),
			expectDisk:    false,
			expectService: false,
		},
		{
			name: "battery below threshold",
			sample: Sample{
				BatteryLevel: ptrFloat(15.0),
				DiskFreeGB:   10.0,
				ServiceOK:    true,
			},
			expectBattery: ptrBool(true),
			expectDisk:    false,
			expectService: false,
		},
		{
			name: "low disk below threshold",
			sample: Sample{
				BatteryLevel: ptrFloat(80.0),
				DiskFreeGB:   3.5,
				ServiceOK:    true,
			},
			expectBattery: ptrBool(false),
			expectDisk:    true,
			expectService: false,
		},
		{
			name: "battery unavailable (nil) - battery condition omitted",
			sample: Sample{
				BatteryLevel: nil,
				DiskFreeGB:   10.0,
				ServiceOK:    true,
			},
			expectBattery: nil, // omitted
			expectDisk:    false,
			expectService: false,
		},
		{
			name: "service health failed",
			sample: Sample{
				BatteryLevel: ptrFloat(90.0),
				DiskFreeGB:   20.0,
				ServiceOK:    false,
			},
			expectBattery: ptrBool(false),
			expectDisk:    false,
			expectService: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conds := Evaluate(tt.sample, thresh)
			byType := make(map[ConditionType]Condition)
			for _, c := range conds {
				byType[c.Type] = c
			}

			if tt.expectBattery != nil {
				c, ok := byType[LowBattery]
				if !ok {
					t.Errorf("Expected LowBattery condition to be evaluated")
				} else if c.Active != *tt.expectBattery {
					t.Errorf("LowBattery active=%v, expected %v", c.Active, *tt.expectBattery)
				}
			} else {
				if _, ok := byType[LowBattery]; ok {
					t.Errorf("Expected LowBattery to be omitted when battery is nil")
				}
			}

			if c, ok := byType[LowDisk]; !ok {
				t.Errorf("Expected LowDisk condition to be evaluated")
			} else if c.Active != tt.expectDisk {
				t.Errorf("LowDisk active=%v, expected %v", c.Active, tt.expectDisk)
			}

			if c, ok := byType[ServiceFailure]; !ok {
				t.Errorf("Expected ServiceFailure condition to be evaluated")
			} else if c.Active != tt.expectService {
				t.Errorf("ServiceFailure active=%v, expected %v", c.Active, tt.expectService)
			}
		})
	}
}

func TestTransitions(t *testing.T) {
	now := time.Now().UTC()

	// 1. OK -> Problem: TransitionToAlert
	condProblem := Condition{Type: LowDisk, Active: true, Value: 3.0, Threshold: 5.0}
	kind, state := Transition(nil, condProblem, now)
	if kind != TransitionToAlert {
		t.Errorf("Expected TransitionToAlert, got %v", kind)
	}
	if !state.Active {
		t.Errorf("Expected state to be active")
	}

	// 2. Problem -> Problem: NoTransition (suppression)
	prevState := &state
	kind2, state2 := Transition(prevState, condProblem, now.Add(time.Minute))
	if kind2 != NoTransition {
		t.Errorf("Expected NoTransition (suppressed repeat), got %v", kind2)
	}
	if !state2.Active {
		t.Errorf("Expected state to remain active")
	}

	// 3. Problem -> OK: TransitionToRecovery
	condOK := Condition{Type: LowDisk, Active: false, Value: 10.0, Threshold: 5.0}
	kind3, state3 := Transition(&state2, condOK, now.Add(2*time.Minute))
	if kind3 != TransitionToRecovery {
		t.Errorf("Expected TransitionToRecovery, got %v", kind3)
	}
	if state3.Active {
		t.Errorf("Expected state to be inactive after recovery")
	}

	// 4. OK -> OK: NoTransition
	kind4, _ := Transition(&state3, condOK, now.Add(3*time.Minute))
	if kind4 != NoTransition {
		t.Errorf("Expected NoTransition, got %v", kind4)
	}
}

func ptrFloat(f float64) *float64 { return &f }
func ptrBool(b bool) *bool        { return &b }
