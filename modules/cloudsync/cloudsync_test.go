package cloudsync

import (
	"context"
	"strings"
	"testing"
	"time"

	"v11-server/modules/digest"
	"v11-server/modules/telemetry"
)

func TestSanitizeTelemetry_NilSample(t *testing.T) {
	services := ServicesStatus{
		Server:    "healthy",
		Gatus:     "healthy",
		Tailscale: "connected",
	}

	res := SanitizeTelemetry(nil, services)
	if res.Device.BatteryLevel != 0 {
		t.Errorf("expected battery 0, got %d", res.Device.BatteryLevel)
	}
	if res.Services.Server != "healthy" {
		t.Errorf("expected server healthy, got %s", res.Services.Server)
	}
}

func TestSanitizeTelemetry_ValidSample(t *testing.T) {
	batLevel := 78.6
	batStatus := "Charging"
	totalMB := 3800.0
	usedMB := 2400.0
	sample := &telemetry.HealthSample{
		Timestamp:         time.Now().UTC(),
		UptimeSeconds:     36000,
		BatteryLevel:      &batLevel,
		BatteryStatus:     &batStatus,
		HostMemoryTotalMB: &totalMB,
		HostMemoryUsedMB:  &usedMB,
		DiskTotalGB:       64.0,
		DiskFreeGB:        32.0,
	}

	services := ServicesStatus{
		Server:    "online",
		Gatus:     "healthy",
		Tailscale: "connected",
	}

	res := SanitizeTelemetry(sample, services)

	if res.Device.BatteryLevel != 79 {
		t.Errorf("expected battery 79, got %d", res.Device.BatteryLevel)
	}
	if !res.Device.IsCharging {
		t.Errorf("expected charging true")
	}
	if res.Device.DiskUsedPercent != 50.0 {
		t.Errorf("expected disk 50.0%%, got %.1f", res.Device.DiskUsedPercent)
	}
	if res.Device.UptimeHours != 10.0 {
		t.Errorf("expected uptime 10.0h, got %.1f", res.Device.UptimeHours)
	}
}

func TestSanitizeDigest(t *testing.T) {
	run := &digest.RunRecord{
		StartedAt:  time.Now().Add(-5 * time.Minute),
		FinishedAt: time.Now(),
		SummaryMD:  "### สรุปข่าว AI\nทดสอบ",
		GoldJSON:   `{"world_spot": 2685.5, "thai_bar_buy": 43800, "thai_bar_sell": 43900, "updated_at": "14:00"}`,
	}

	res := SanitizeDigest(run)
	if !strings.Contains(res.SummaryMD, "สรุปข่าว AI") {
		t.Errorf("expected summary, got %s", res.SummaryMD)
	}
	if res.Gold == nil || res.Gold.WorldSpot != 2685.5 {
		t.Errorf("expected gold spot 2685.5, got %+v", res.Gold)
	}
}

func TestActionDispatcher_Whitelist(t *testing.T) {
	dispatcher := NewActionDispatcher(func(ctx context.Context) (string, error) {
		return "digest ran successfully", nil
	}, func(ctx context.Context, prompt, model string) (string, error) {
		return "answer: " + prompt, nil
	}, nil)

	ctx := context.Background()

	// 1. Allowed: ping
	pingRes, err := dispatcher.Dispatch(ctx, ActionRequest{Action: "ping"})
	if err != nil || !strings.Contains(pingRes, "pong") {
		t.Fatalf("expected ping success, got %v / %s", err, pingRes)
	}

	// 2. Allowed: refresh_digest
	digRes, err := dispatcher.Dispatch(ctx, ActionRequest{Action: "refresh_digest"})
	if err != nil || !strings.Contains(digRes, "digest ran successfully") {
		t.Fatalf("expected digest success, got %v / %s", err, digRes)
	}

	// 3. Allowed: ask
	askRes, err := dispatcher.Dispatch(ctx, ActionRequest{Action: "ask", Prompt: "test question"})
	if err != nil || !strings.Contains(askRes, "answer: test question") {
		t.Fatalf("expected ask success, got %v / %s", err, askRes)
	}

	// 4. Disallowed: dangerous command
	_, err = dispatcher.Dispatch(ctx, ActionRequest{Action: "rm -rf /"})
	if err == nil {
		t.Fatalf("expected error for disallowed action, got nil")
	}
}

func TestDryRunClient(t *testing.T) {
	client, err := NewClient("", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !client.IsDryRun() {
		t.Errorf("expected dry run mode true")
	}

	ctx := context.Background()
	if err := client.SyncStatus(ctx, PublicStatusPayload{}); err != nil {
		t.Errorf("expected nil error on dry run, got %v", err)
	}
	if err := client.SyncDigest(ctx, PublicDigestPayload{}); err != nil {
		t.Errorf("expected nil error on dry run, got %v", err)
	}
	actions, err := client.GetPendingActions(ctx)
	if err != nil || len(actions) != 0 {
		t.Errorf("expected empty actions, got %v, %v", actions, err)
	}
}

type mockTelemetry struct {
	sample *telemetry.HealthSample
}

func (m *mockTelemetry) GetLatestSample(ctx context.Context) (*telemetry.HealthSample, error) {
	return m.sample, nil
}

type mockDigest struct {
	run *digest.RunRecord
}

func (m *mockDigest) GetLatestRun(ctx context.Context) (*digest.RunRecord, error) {
	return m.run, nil
}

func TestWorker_SyncOnce(t *testing.T) {
	client := &DryRunClient{}
	tel := &mockTelemetry{sample: &telemetry.HealthSample{UptimeSeconds: 120}}
	dig := &mockDigest{run: &digest.RunRecord{SummaryMD: "Mock"}}

	w := NewWorker(client, tel, dig, 10*time.Second)
	if err := w.SyncOnce(context.Background()); err != nil {
		t.Errorf("expected nil error on sync once, got %v", err)
	}
}
