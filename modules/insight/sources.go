package insight

import (
	"context"
	"strings"
	"time"

	"v11-server/modules/location"
	"v11-server/modules/reminders"
	"v11-server/modules/telemetry"
)

// storeSources adapts the existing stores to the Sources interface.
// Any store may be nil; its section of the report is then empty.
type storeSources struct {
	tel *telemetry.Repository
	loc *location.Store
	rem *reminders.Store
}

// NewSources builds Sources over the telemetry, location and reminder stores.
func NewSources(tel *telemetry.Repository, loc *location.Store, rem *reminders.Store) Sources {
	return storeSources{tel: tel, loc: loc, rem: rem}
}

func (s storeSources) Health(ctx context.Context, since time.Time) ([]HealthPoint, error) {
	if s.tel == nil {
		return nil, nil
	}
	samples, err := s.tel.GetRecentSamples(ctx, 5000, time.Since(since))
	if err != nil {
		return nil, err
	}
	out := make([]HealthPoint, 0, len(samples))
	for _, x := range samples {
		hp := HealthPoint{At: x.Timestamp, DiskFreeGB: x.DiskFreeGB}
		if x.HostMemoryTotalMB != nil && x.HostMemoryUsedMB != nil && *x.HostMemoryTotalMB > 0 {
			hp.RAMUsedPct, hp.HasRAM = *x.HostMemoryUsedMB / *x.HostMemoryTotalMB * 100, true
		}
		if x.BatteryLevel != nil {
			hp.Battery, hp.HasBattery = *x.BatteryLevel, true
		}
		if x.BatteryStatus != nil {
			status := strings.ToUpper(*x.BatteryStatus)
			hp.Charging = status == "CHARGING" || status == "FULL"
		}
		out = append(out, hp)
	}
	return out, nil
}

func (s storeSources) Points(ctx context.Context, device string, from, to time.Time) ([]GeoPoint, error) {
	if s.loc == nil {
		return nil, nil
	}
	points, err := s.loc.PointsBetween(ctx, device, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]GeoPoint, 0, len(points))
	for _, p := range points {
		out = append(out, GeoPoint{At: p.RecordedAt, Lat: p.Latitude, Lng: p.Longitude, Accuracy: p.Accuracy,
			Battery: p.Battery, Charging: p.BatteryStatus == 2 || p.BatteryStatus == 3})
	}
	return out, nil
}

func (s storeSources) Places(ctx context.Context) ([]Place, error) {
	if s.loc == nil {
		return nil, nil
	}
	zones, err := s.loc.ListGeofences(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Place, 0, len(zones))
	for _, z := range zones {
		out = append(out, Place{Name: z.Name, Lat: z.Latitude, Lng: z.Longitude, Radius: z.Radius})
	}
	return out, nil
}

// Reminders expands today's occurrences: one-off reminders dated today and
// weekly reminders on today's weekday, skipping paused and completed ones.
func (s storeSources) Reminders(ctx context.Context, now time.Time) ([]ReminderItem, error) {
	if s.rem == nil {
		return nil, nil
	}
	list, err := s.rem.List(ctx)
	if err != nil {
		return nil, err
	}
	loc := bangkok()
	today := startOfDay(now, loc)
	var out []ReminderItem
	for _, r := range list {
		if r.Paused || !r.Enabled {
			continue
		}
		start := r.StartsAt.In(loc)
		var at time.Time
		switch {
		case r.Recurrence == "weekly":
			if start.After(today.AddDate(0, 0, 1)) || start.Weekday() != today.Weekday() {
				continue
			}
			at = time.Date(today.Year(), today.Month(), today.Day(), start.Hour(), start.Minute(), 0, 0, loc)
		case startOfDay(start, loc).Equal(today):
			at = start
		default:
			continue
		}
		done := false
		for _, c := range r.CompletedOccurrences {
			if startOfDay(c, loc).Equal(today) {
				done = true
				break
			}
		}
		if !done {
			out = append(out, ReminderItem{At: at, Title: r.Title})
		}
	}
	return out, nil
}
