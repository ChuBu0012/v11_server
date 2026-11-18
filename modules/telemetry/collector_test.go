package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"v11-server/modules/alerts"
)

type fakeSystemProvider struct {
	uptime int64
	alloc  float64
	sys    float64
	total  float64
	free   float64
	err    error
}

func (f *fakeSystemProvider) GetMetrics() (int64, float64, float64, float64, float64, error) {
	return f.uptime, f.alloc, f.sys, f.total, f.free, f.err
}

type fakeBatteryProvider struct {
	level  *float64
	status *string
	err    error
}

func (f *fakeBatteryProvider) GetBattery(ctx context.Context) (*float64, *string, error) {
	return f.level, f.status, f.err
}

type fakeAlertProcessor struct {
	received []alerts.Sample
}

func (f *fakeAlertProcessor) ProcessSample(sample alerts.Sample) error {
	f.received = append(f.received, sample)
	return nil
}

func TestCollectorWithNormalProviders(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	batLevel := 85.0
	batStatus := "CHARGING"
	sysP := &fakeSystemProvider{
		uptime: 300,
		alloc:  2.5,
		sys:    12.0,
		total:  100.0,
		free:   45.0,
	}
	batP := &fakeBatteryProvider{
		level:  &batLevel,
		status: &batStatus,
	}
	alertP := &fakeAlertProcessor{}

	collector := NewCollector(repo, time.Now(),
		WithSystemProvider(sysP),
		WithBatteryProvider(batP),
		WithAlertProcessor(alertP),
	)

	sample, err := collector.CollectOnce(context.Background())
	if err != nil {
		t.Fatalf("CollectOnce failed: %v", err)
	}

	if sample.UptimeSeconds != 300 {
		t.Errorf("Expected uptime 300, got %d", sample.UptimeSeconds)
	}
	if sample.BatteryLevel == nil || *sample.BatteryLevel != 85.0 {
		t.Errorf("Expected battery level 85.0, got %v", sample.BatteryLevel)
	}
	if len(alertP.received) != 1 {
		t.Fatalf("Expected 1 alert sample received, got %d", len(alertP.received))
	}
	if alertP.received[0].DiskFreeGB != 45.0 {
		t.Errorf("Expected alert sample disk free 45.0, got %f", alertP.received[0].DiskFreeGB)
	}
}

func TestCollectorDegradedBattery(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	sysP := &fakeSystemProvider{
		uptime: 500,
		alloc:  3.0,
		sys:    15.0,
		total:  100.0,
		free:   40.0,
	}
	// Battery provider returns error (e.g. termux-battery-status not available)
	batP := &fakeBatteryProvider{
		err: errors.New("command not found: termux-battery-status"),
	}
	alertP := &fakeAlertProcessor{}

	collector := NewCollector(repo, time.Now(),
		WithSystemProvider(sysP),
		WithBatteryProvider(batP),
		WithAlertProcessor(alertP),
	)

	sample, err := collector.CollectOnce(context.Background())
	if err != nil {
		t.Fatalf("Expected collection to succeed despite battery failure, got error: %v", err)
	}

	if sample.BatteryLevel != nil {
		t.Errorf("Expected nil battery level in degraded mode, got %v", *sample.BatteryLevel)
	}
	if sample.DiskFreeGB != 40.0 {
		t.Errorf("Expected disk free 40.0, got %f", sample.DiskFreeGB)
	}
	if len(alertP.received) != 1 {
		t.Fatalf("Expected alert processor to still receive sample, got %d", len(alertP.received))
	}
	if alertP.received[0].BatteryLevel != nil {
		t.Errorf("Expected nil battery level in alert sample, got %v", alertP.received[0].BatteryLevel)
	}
}

type fakeHostMemProvider struct {
	total float64
	used  float64
	free  float64
	err   error
}

func (f *fakeHostMemProvider) GetHostMemory() (*float64, *float64, *float64, error) {
	if f.err != nil {
		return nil, nil, nil, f.err
	}
	return &f.total, &f.used, &f.free, nil
}

func TestCollectorWithHostMemoryProvider(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	sysP := &fakeSystemProvider{uptime: 100, alloc: 2.0, sys: 8.0, total: 50.0, free: 25.0}
	batP := &fakeBatteryProvider{}
	hostMemP := &fakeHostMemProvider{total: 5800.0, used: 2900.0, free: 2900.0}

	collector := NewCollector(repo, time.Now(),
		WithSystemProvider(sysP),
		WithBatteryProvider(batP),
		WithHostMemoryProvider(hostMemP),
	)

	sample, err := collector.CollectOnce(context.Background())
	if err != nil {
		t.Fatalf("CollectOnce failed: %v", err)
	}

	if sample.HostMemoryTotalMB == nil || *sample.HostMemoryTotalMB != 5800.0 {
		t.Errorf("Expected HostMemoryTotalMB 5800.0, got %v", sample.HostMemoryTotalMB)
	}
	if sample.HostMemoryUsedMB == nil || *sample.HostMemoryUsedMB != 2900.0 {
		t.Errorf("Expected HostMemoryUsedMB 2900.0, got %v", sample.HostMemoryUsedMB)
	}
	if sample.HostMemoryFreeMB == nil || *sample.HostMemoryFreeMB != 2900.0 {
		t.Errorf("Expected HostMemoryFreeMB 2900.0, got %v", sample.HostMemoryFreeMB)
	}
}
