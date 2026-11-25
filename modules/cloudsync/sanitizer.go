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
		var g digest.GoldQuote
		if err := json.Unmarshal([]byte(run.GoldJSON), &g); err == nil && (g.SpotUSD > 0 || g.BarBuy > 0 || g.BarSell > 0) {
			updated := g.AnnouncedAt
			if updated == "" && !g.FetchedAt.IsZero() {
				updated = g.FetchedAt.UTC().Format(time.RFC3339)
			}
			payload.Gold = &GoldPayload{
				WorldSpot: g.SpotUSD,
				BarBuy:    g.BarBuy,
				BarSell:   g.BarSell,
				UpdatedAt: updated,
			}
		}
	}

	if run.SummaryMD != "" {
		sections := parseSummarySections(run.SummaryMD)
		for _, cat := range []string{"OVERVIEW", "AI", "POLITICS", "TRIVIA"} {
			if body, ok := sections[cat]; ok && strings.TrimSpace(body) != "" {
				payload.News = append(payload.News, NewsItem{
					Category: cat,
					Summary:  strings.TrimSpace(body),
				})
			}
		}
		if stocksBody, ok := sections["STOCKS"]; ok {
			for _, line := range strings.Split(stocksBody, "\n") {
				trimmed := strings.TrimSpace(line)
				trimmed = strings.TrimPrefix(trimmed, "- ")
				trimmed = strings.TrimPrefix(trimmed, "* ")
				trimmed = strings.TrimSpace(trimmed)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				payload.Stocks = append(payload.Stocks, StockItem{
					Title: trimmed,
				})
			}
		}
	}

	return payload
}

func parseSummarySections(md string) map[string]string {
	res := make(map[string]string)
	var currentKey string
	var buf []string
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "===SECTION:") && strings.HasSuffix(trimmed, "===") {
			if currentKey != "" {
				res[currentKey] = strings.Join(buf, "\n")
			}
			currentKey = strings.TrimSuffix(strings.TrimPrefix(trimmed, "===SECTION:"), "===")
			buf = nil
			continue
		}
		if currentKey != "" {
			buf = append(buf, line)
		}
	}
	if currentKey != "" {
		res[currentKey] = strings.Join(buf, "\n")
	}
	return res
}
