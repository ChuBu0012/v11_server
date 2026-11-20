// Package insight builds the owner's morning report. Every number and every
// "abnormal" verdict is computed here, deterministically; AI only rephrases.
package insight

import (
	"math"
	"sort"
	"time"
)

// Thresholds from docs/superpowers/specs/2026-11-20-morning-insight-design.md.
const (
	maxAccuracyM       = 100.0
	maxSegmentGap      = 30 * time.Minute
	walkMinKmh         = 1.5
	walkMaxKmh         = 7.0
	togetherMaxM       = 80.0
	togetherPairWindow = 2 * time.Minute
	togetherMaxStep    = 5 * time.Minute
	batteryMaxGap      = 5 * time.Minute
	batteryMinWindow   = 30 * time.Minute
	ramJumpPP          = 15.0
	ramCriticalPct     = 90.0
	diskLossGBPerDay   = 1.0
	diskLossFactor     = 3.0
	diskCriticalGB     = 5.0
	phoneLowPct        = 15
)

// Verdict is the outcome of comparing a metric with its own baseline.
type Verdict string

const (
	VerdictNormal   Verdict = "normal"
	VerdictAbnormal Verdict = "abnormal"
	VerdictUnknown  Verdict = "unknown"
)

// HealthPoint is one telemetry sample of the v11 server.
type HealthPoint struct {
	At         time.Time
	RAMUsedPct float64
	HasRAM     bool
	DiskFreeGB float64
	Battery    float64
	HasBattery bool
	Charging   bool
}

// GeoPoint is one location fix of a person's phone.
type GeoPoint struct {
	At       time.Time
	Lat, Lng float64
	Accuracy float64
	Battery  int
	Charging bool
}

// Place is a saved geofence.
type Place struct {
	Name     string
	Lat, Lng float64
	Radius   float64
}

// ReminderItem is one scheduled reminder event.
type ReminderItem struct {
	At    time.Time
	Title string
}

type ServerFacts struct {
	BatteryRate        float64  `json:"battery_drain_pct_per_h"`
	BatteryBaseline    float64  `json:"battery_baseline_pct_per_h"`
	BatteryVerdict     Verdict  `json:"battery_verdict"`
	AlwaysCharging     bool     `json:"always_charging"`
	RAMNow             float64  `json:"ram_now_pct"`
	RAMAvg             float64  `json:"ram_avg_pct"`
	RAMBaseline        float64  `json:"ram_baseline_pct"`
	RAMVerdict         Verdict  `json:"ram_verdict"`
	DiskFree           float64  `json:"disk_free_gb"`
	DiskPerDay         float64  `json:"disk_loss_gb_per_day"`
	DiskBaselinePerDay float64  `json:"disk_baseline_gb_per_day"`
	DiskVerdict        Verdict  `json:"disk_verdict"`
	Attention          []string `json:"attention,omitempty"`
}

type PersonFacts struct {
	Name          string   `json:"name"`
	HasData       bool     `json:"has_data"`
	WalkKm        float64  `json:"walk_km"`
	TotalKm       float64  `json:"total_km"`
	MovingMin     int      `json:"moving_min"`
	Places        []string `json:"places,omitempty"`
	MinBattery    int      `json:"min_battery"`
	LowBattery    bool     `json:"low_battery"`
	WalkKmMedian7 float64  `json:"walk_km_median_7d"`
}

type TogetherFacts struct {
	HasData        bool      `json:"has_data"`
	Minutes        int       `json:"minutes"`
	MedianMinutes7 int       `json:"median_minutes_7d"`
	LongestStart   time.Time `json:"longest_start"`
	LongestEnd     time.Time `json:"longest_end"`
}

type TodayFacts struct {
	Count     int       `json:"count"`
	NextAt    time.Time `json:"next_at"`
	NextTitle string    `json:"next_title,omitempty"`
}

// Facts is the whole report input; it is also what the AI sees.
type Facts struct {
	Date     string        `json:"date"`
	Server   ServerFacts   `json:"server"`
	Me       PersonFacts   `json:"me"`
	Friend   PersonFacts   `json:"friend"`
	Together TogetherFacts `json:"together"`
	Today    TodayFacts    `json:"today"`
}

// ---------------------------------------------------------------- server

// ComputeServer compares the last 24 h with the 48 h before it.
func ComputeServer(samples []HealthPoint, now time.Time) ServerFacts {
	sorted := append([]HealthPoint(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].At.Before(sorted[j].At) })
	recentStart := now.Add(-24 * time.Hour)
	var recent, base []HealthPoint
	for _, s := range sorted {
		switch {
		case s.At.After(now):
		case !s.At.Before(recentStart):
			recent = append(recent, s)
		case !s.At.Before(now.Add(-72 * time.Hour)):
			base = append(base, s)
		}
	}
	f := ServerFacts{BatteryVerdict: VerdictUnknown, RAMVerdict: VerdictUnknown, DiskVerdict: VerdictUnknown}

	// Battery: drain only across consecutive discharging samples.
	rate, window, sawBattery := drainRate(recent)
	baseRate, baseWindow, _ := drainRate(base)
	f.BatteryRate, f.BatteryBaseline = round1(rate), round1(baseRate)
	switch {
	case sawBattery && window < batteryMinWindow:
		f.AlwaysCharging = true
	case window >= batteryMinWindow && baseWindow >= batteryMinWindow:
		if rate > math.Max(2*baseRate, baseRate+1) && rate > 0.5 {
			f.BatteryVerdict = VerdictAbnormal
			f.Attention = append(f.Attention, "แบตเตอรี่ v11 ลดเร็วผิดปกติ")
		} else {
			f.BatteryVerdict = VerdictNormal
		}
	}

	// RAM.
	if avg, ok := ramAverage(recent); ok {
		f.RAMAvg = round1(avg)
		f.RAMNow = round1(lastRAM(recent))
		baseAvg, hasBase := ramAverage(base)
		f.RAMBaseline = round1(baseAvg)
		switch {
		case f.RAMNow >= ramCriticalPct || (hasBase && avg-baseAvg > ramJumpPP):
			f.RAMVerdict = VerdictAbnormal
			f.Attention = append(f.Attention, "RAM ของ v11 สูงขึ้นผิดปกติ")
		default:
			f.RAMVerdict = VerdictNormal
		}
	}

	// Disk.
	if len(recent) > 0 {
		f.DiskFree = round1(recent[len(recent)-1].DiskFreeGB)
		f.DiskPerDay = round1(diskLossPerDay(recent))
		f.DiskBaselinePerDay = round1(diskLossPerDay(base))
		if f.DiskFree < diskCriticalGB || (f.DiskPerDay > diskLossGBPerDay && f.DiskPerDay > diskLossFactor*f.DiskBaselinePerDay) {
			f.DiskVerdict = VerdictAbnormal
			f.Attention = append(f.Attention, "พื้นที่ดิสก์ของ v11 ลดเร็วหรือเหลือน้อย")
		} else {
			f.DiskVerdict = VerdictNormal
		}
	}
	return f
}

// drainRate returns %/h lost while discharging, the discharging time it was
// measured over, and whether any battery reading was seen at all.
func drainRate(samples []HealthPoint) (float64, time.Duration, bool) {
	var lost float64
	var window time.Duration
	seen := false
	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1], samples[i]
		if a.HasBattery || b.HasBattery {
			seen = true
		}
		gap := b.At.Sub(a.At)
		if !a.HasBattery || !b.HasBattery || a.Charging || b.Charging || gap <= 0 || gap > batteryMaxGap {
			continue
		}
		lost += a.Battery - b.Battery
		window += gap
	}
	if window <= 0 {
		return 0, 0, seen
	}
	return lost / window.Hours(), window, seen
}

func ramAverage(samples []HealthPoint) (float64, bool) {
	var sum float64
	n := 0
	for _, s := range samples {
		if s.HasRAM {
			sum += s.RAMUsedPct
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

func lastRAM(samples []HealthPoint) float64 {
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].HasRAM {
			return samples[i].RAMUsedPct
		}
	}
	return 0
}

// diskLossPerDay is free-space lost per day between first and last sample (≥ 0).
func diskLossPerDay(samples []HealthPoint) float64 {
	if len(samples) < 2 {
		return 0
	}
	first, last := samples[0], samples[len(samples)-1]
	days := last.At.Sub(first.At).Hours() / 24
	if days <= 0 {
		return 0
	}
	return math.Max(0, (first.DiskFreeGB-last.DiskFreeGB)/days)
}

// ---------------------------------------------------------------- movement

type dayMovement struct {
	walkKm, totalKm float64
	movingMin       int
	points          int
}

// ComputePerson reports yesterday (day = Bangkok midnight) for one person.
func ComputePerson(name string, points []GeoPoint, places []Place, day time.Time, loc *time.Location) PersonFacts {
	clean := cleanPoints(points)
	day = startOfDay(day, loc)
	f := PersonFacts{Name: name}
	yesterday := inDay(clean, day)
	m := movement(yesterday)
	if m.points == 0 {
		return f
	}
	f.HasData = true
	f.WalkKm, f.TotalKm, f.MovingMin = round1(m.walkKm), round1(m.totalKm), m.movingMin
	f.Places = placesVisited(yesterday, places)
	f.MinBattery = 101
	for _, p := range yesterday {
		if p.Battery > 0 && p.Battery < f.MinBattery {
			f.MinBattery = p.Battery
		}
	}
	if f.MinBattery == 101 {
		f.MinBattery = 0
	}
	f.LowBattery = f.MinBattery > 0 && f.MinBattery < phoneLowPct
	var prev []float64
	for d := 1; d <= 6; d++ {
		if pm := movement(inDay(clean, day.AddDate(0, 0, -d))); pm.points > 0 {
			prev = append(prev, pm.walkKm)
		}
	}
	f.WalkKmMedian7 = round1(median(prev))
	return f
}

func movement(points []GeoPoint) dayMovement {
	m := dayMovement{points: len(points)}
	var movingDur time.Duration
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		gap := b.At.Sub(a.At)
		if gap <= 0 || gap > maxSegmentGap {
			continue
		}
		km := haversineKm(a.Lat, a.Lng, b.Lat, b.Lng)
		kmh := km / gap.Hours()
		if kmh < walkMinKmh {
			continue
		}
		m.totalKm += km
		movingDur += gap
		if kmh <= walkMaxKmh {
			m.walkKm += km
		}
	}
	m.movingMin = int(math.Round(movingDur.Minutes()))
	return m
}

func placesVisited(points []GeoPoint, places []Place) []string {
	type visit struct {
		name string
		at   time.Time
	}
	var visits []visit
	for _, place := range places {
		for i := 1; i < len(points); i++ {
			a, b := points[i-1], points[i]
			if haversineKm(a.Lat, a.Lng, place.Lat, place.Lng)*1000 <= place.Radius &&
				haversineKm(b.Lat, b.Lng, place.Lat, place.Lng)*1000 <= place.Radius {
				visits = append(visits, visit{place.Name, a.At})
				break
			}
		}
	}
	sort.Slice(visits, func(i, j int) bool { return visits[i].at.Before(visits[j].at) })
	names := make([]string, 0, len(visits))
	for _, v := range visits {
		names = append(names, v.name)
	}
	return names
}

// ---------------------------------------------------------------- together

// ComputeTogether sums yesterday's minutes the two phones were within 80 m.
func ComputeTogether(me, friend []GeoPoint, day time.Time, loc *time.Location) TogetherFacts {
	me, friend = cleanPoints(me), cleanPoints(friend)
	day = startOfDay(day, loc)
	f := TogetherFacts{}
	m, fr := inDay(me, day), inDay(friend, day)
	if len(m) == 0 || len(fr) == 0 {
		return f
	}
	f.HasData = true
	minutes, start, end := togetherMinutes(m, fr)
	f.Minutes, f.LongestStart, f.LongestEnd = minutes, start, end
	var prev []float64
	for d := 1; d <= 6; d++ {
		pm, pf := inDay(me, day.AddDate(0, 0, -d)), inDay(friend, day.AddDate(0, 0, -d))
		if len(pm) > 0 && len(pf) > 0 {
			mins, _, _ := togetherMinutes(pm, pf)
			prev = append(prev, float64(mins))
		}
	}
	f.MedianMinutes7 = int(math.Round(median(prev)))
	return f
}

func togetherMinutes(me, friend []GeoPoint) (int, time.Time, time.Time) {
	together := make([]bool, len(me))
	j := 0
	for i, p := range me {
		for j+1 < len(friend) && absDur(friend[j+1].At.Sub(p.At)) <= absDur(friend[j].At.Sub(p.At)) {
			j++
		}
		q := friend[j]
		together[i] = absDur(q.At.Sub(p.At)) <= togetherPairWindow &&
			haversineKm(p.Lat, p.Lng, q.Lat, q.Lng)*1000 <= togetherMaxM
	}
	var total, run time.Duration
	var runStart, bestStart, bestEnd time.Time
	var best time.Duration
	for i := 1; i < len(me); i++ {
		gap := me[i].At.Sub(me[i-1].At)
		if together[i-1] && together[i] && gap > 0 && gap <= togetherMaxStep {
			if run == 0 {
				runStart = me[i-1].At
			}
			run += gap
			total += gap
			if run > best {
				best, bestStart, bestEnd = run, runStart, me[i].At
			}
		} else {
			run = 0
		}
	}
	return int(math.Round(total.Minutes())), bestStart, bestEnd
}

// ---------------------------------------------------------------- today

// ComputeToday counts reminders still ahead today and picks the next one.
func ComputeToday(items []ReminderItem, now time.Time, loc *time.Location) TodayFacts {
	end := startOfDay(now, loc).AddDate(0, 0, 1)
	f := TodayFacts{}
	for _, it := range items {
		if it.At.Before(now) || !it.At.Before(end) {
			continue
		}
		f.Count++
		if f.NextAt.IsZero() || it.At.Before(f.NextAt) {
			f.NextAt, f.NextTitle = it.At, it.Title
		}
	}
	return f
}

// ---------------------------------------------------------------- helpers

func cleanPoints(points []GeoPoint) []GeoPoint {
	out := make([]GeoPoint, 0, len(points))
	for _, p := range points {
		if p.Accuracy <= maxAccuracyM && !math.IsNaN(p.Lat) && !math.IsNaN(p.Lng) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

func inDay(points []GeoPoint, day time.Time) []GeoPoint {
	end := day.AddDate(0, 0, 1)
	var out []GeoPoint
	for _, p := range points {
		if !p.At.Before(day) && p.At.Before(end) {
			out = append(out, p)
		}
	}
	return out
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
