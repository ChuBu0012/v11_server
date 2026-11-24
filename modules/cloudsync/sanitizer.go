package cloudsync

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"v11-server/modules/digest"
	"v11-server/modules/telemetry"
)

// SanitizeTelemetry transforms an internal health sample into a strictly sanitized PublicStatusPayload.
// Guarantees zero sensitive fields (no GPS, no internal IP addresses, no quota tokens).
func SanitizeTelemetry(sample *telemetry.HealthSample, services ServicesStatus) PublicStatusPayload {
	now := time.Now().UTC().Format(time.RFC3339)
	if sample == nil {
		return PublicStatusPayload{
			UpdatedAt: now,
			Device: DeviceStatus{
				BatteryLevel:    0,
				IsCharging:      false,
				RAMUsedPercent:  0,
				DiskUsedPercent: 0,
				UptimeSeconds:   0,
				UptimeHours:     0,
			},
			Services: services,
			Message:  "Waiting for first telemetry sample",
		}
	}

	batteryLevel := 0
	if sample.BatteryLevel != nil {
		batteryLevel = int(math.Round(*sample.BatteryLevel))
	}

	isCharging := false
	if sample.BatteryStatus != nil {
		st := strings.ToLower(strings.TrimSpace(*sample.BatteryStatus))
		isCharging = st == "charging" || st == "full" || strings.Contains(st, "charge")
	}

	ramPercent := 0.0
	if sample.HostMemoryTotalMB != nil && sample.HostMemoryUsedMB != nil && *sample.HostMemoryTotalMB > 0 {
		ramPercent = (*sample.HostMemoryUsedMB / *sample.HostMemoryTotalMB) * 100.0
	} else if sample.MemorySysMB > 0 {
		// Fallback to Go process sys memory approximation
		ramPercent = (sample.MemoryAllocMB / sample.MemorySysMB) * 100.0
	}
	ramPercent = math.Round(ramPercent*10) / 10

	diskPercent := 0.0
	if sample.DiskTotalGB > 0 {
		diskUsed := sample.DiskTotalGB - sample.DiskFreeGB
		if diskUsed < 0 {
			diskUsed = 0
		}
		diskPercent = (diskUsed / sample.DiskTotalGB) * 100.0
	}
	diskPercent = math.Round(diskPercent*10) / 10

	uptimeHours := math.Round((float64(sample.UptimeSeconds)/3600.0)*10) / 10

	updatedAt := sample.Timestamp.UTC().Format(time.RFC3339)
	if sample.Timestamp.IsZero() {
		updatedAt = now
	}

	return PublicStatusPayload{
		UpdatedAt: updatedAt,
		Device: DeviceStatus{
			BatteryLevel:    batteryLevel,
			IsCharging:      isCharging,
			RAMUsedPercent:  ramPercent,
			DiskUsedPercent: diskPercent,
			UptimeSeconds:   sample.UptimeSeconds,
			UptimeHours:     uptimeHours,
		},
		Services: services,
	}
}

// SanitizeDigest transforms a digest run record into a PublicDigestPayload.
func SanitizeDigest(run *digest.RunRecord) PublicDigestPayload {
	now := time.Now().UTC().Format(time.RFC3339)
	if run == nil {
		return PublicDigestPayload{
			UpdatedAt: now,
			SummaryMD: "ยังไม่มีสรุปข่าวประจำวัน",
		}
	}

	updatedAt := run.FinishedAt.UTC().Format(time.RFC3339)
	if run.FinishedAt.IsZero() {
		updatedAt = run.StartedAt.UTC().Format(time.RFC3339)
	}

	payload := PublicDigestPayload{
		UpdatedAt: updatedAt,
		SummaryMD: run.SummaryMD,
	}

	if run.GoldJSON != "" {
		var g struct {
			WorldSpot   float64 `json:"world_spot"`
			ThaiBarBuy  float64 `json:"thai_bar_buy"`
			ThaiBarSell float64 `json:"thai_bar_sell"`
			UpdatedAt   string  `json:"updated_at"`
		}
		if err := json.Unmarshal([]byte(run.GoldJSON), &g); err == nil && (g.WorldSpot > 0 || g.ThaiBarBuy > 0) {
			payload.Gold = &GoldPayload{
				WorldSpot: g.WorldSpot,
				BarBuy:    g.ThaiBarBuy,
				BarSell:   g.ThaiBarSell,
				UpdatedAt: g.UpdatedAt,
			}
		}
	}

	return payload
}
