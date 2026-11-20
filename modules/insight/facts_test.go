package insight

import (
	"math"
	"testing"
	"time"
)

var bkk = time.FixedZone("Asia/Bangkok", 7*3600)

// healthSeries builds one sample per minute going back `hours`, with battery
// changing by `ratePerHour` (negative = draining).
func healthSeries(now time.Time, hours int, start, ratePerHour float64, charging bool, ram float64) []HealthPoint {
	n := hours * 60
	out := make([]HealthPoint, 0, n)
	for i := n; i >= 0; i-- {
		at := now.Add(-time.Duration(i) * time.Minute)
		elapsedH := float64(n-i) / 60
		out = append(out, HealthPoint{At: at, RAMUsedPct: ram, HasRAM: true, DiskFreeGB: 70, Battery: start + ratePerHour*elapsedH, HasBattery: true, Charging: charging})
	}
	return out
}

func TestServerFactsBatteryAbnormal(t *testing.T) {
	now := time.Date(2026, 11, 21, 7, 0, 0, 0, bkk)
	// 48 h baseline draining 1 %/h (battery kept in range by restarting level), then 24 h at 4 %/h.
	base := healthSeries(now.Add(-24*time.Hour), 48, 100, -1, false, 50)
	recent := healthSeries(now, 23, 100, -4, false, 50)
	f := ComputeServer(append(base, recent...), now)
	if f.BatteryVerdict != VerdictAbnormal || math.Abs(f.BatteryRate-4) > 0.3 || math.Abs(f.BatteryBaseline-1) > 0.3 {
		t.Fatalf("battery = %+v", f)
	}
	if len(f.Attention) == 0 {
		t.Fatal("abnormal battery must be listed in Attention")
	}
}

func TestServerFactsBatteryNormal(t *testing.T) {
	now := time.Date(2026, 11, 21, 7, 0, 0, 0, bkk)
	base := healthSeries(now.Add(-24*time.Hour), 48, 100, -1, false, 50)
	recent := healthSeries(now, 23, 100, -1.2, false, 50)
	f := ComputeServer(append(base, recent...), now)
	if f.BatteryVerdict != VerdictNormal {
		t.Fatalf("battery verdict = %s (%+v)", f.BatteryVerdict, f)
	}
}

func TestServerFactsAlwaysCharging(t *testing.T) {
	now := time.Date(2026, 11, 21, 7, 0, 0, 0, bkk)
	f := ComputeServer(healthSeries(now, 72, 100, 0, true, 50), now)
	if !f.AlwaysCharging || f.BatteryVerdict != VerdictUnknown {
		t.Fatalf("always charging = %+v", f)
	}
}

func TestServerFactsRAMAndDisk(t *testing.T) {
	now := time.Date(2026, 11, 21, 7, 0, 0, 0, bkk)
	base := healthSeries(now.Add(-24*time.Hour), 48, 100, 0, true, 60)
	recent := healthSeries(now, 23, 100, 0, true, 80)
	f := ComputeServer(append(base, recent...), now)
	if f.RAMVerdict != VerdictAbnormal {
		t.Fatalf("ram verdict = %s (%+v)", f.RAMVerdict, f)
	}
	low := healthSeries(now, 72, 100, 0, true, 50)
	for i := range low {
		low[i].DiskFreeGB = 4
	}
	if g := ComputeServer(low, now); g.DiskVerdict != VerdictAbnormal {
		t.Fatalf("disk verdict = %s (%+v)", g.DiskVerdict, g)
	}
}

// track walks north from (lat0, 100.5) at speedKmh for minutes, one point per minute.
func track(start time.Time, lat0, speedKmh float64, minutes int) []GeoPoint {
	const kmPerDegLat = 111.195
	out := make([]GeoPoint, 0, minutes+1)
	for i := 0; i <= minutes; i++ {
		lat := lat0 + (speedKmh*float64(i)/60)/kmPerDegLat
		out = append(out, GeoPoint{At: start.Add(time.Duration(i) * time.Minute), Lat: lat, Lng: 100.5, Accuracy: 10, Battery: 80})
	}
	return out
}

func TestMovementWalkVsVehicle(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, bkk)
	walk := track(day.Add(8*time.Hour), 13.70, 5, 24)  // 2 km
	ride := track(day.Add(9*time.Hour), 13.80, 40, 15) // 10 km
	f := ComputePerson("Chubu", append(walk, ride...), nil, day, bkk)
	if !f.HasData || math.Abs(f.WalkKm-2) > 0.1 || math.Abs(f.TotalKm-12) > 0.2 {
		t.Fatalf("movement = %+v", f)
	}
	if f.MovingMin < 38 || f.MovingMin > 40 {
		t.Fatalf("moving minutes = %d, want ~39", f.MovingMin)
	}
}

func TestMovementDropsInaccurateAndGaps(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, bkk)
	pts := track(day.Add(8*time.Hour), 13.70, 5, 12)                                                 // 1 km
	pts[6].Accuracy = 500                                                                            // dropped: neighbours still connect within the gap rule
	far := GeoPoint{At: pts[len(pts)-1].At.Add(2 * time.Hour), Lat: 14.70, Lng: 100.5, Accuracy: 10} // 2 h gap: not a segment
	f := ComputePerson("Chubu", append(pts, far), nil, day, bkk)
	if math.Abs(f.TotalKm-1) > 0.1 {
		t.Fatalf("total km = %.2f, want ~1 (gap and inaccurate point ignored)", f.TotalKm)
	}
}

func TestMovementPlacesVisited(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, bkk)
	home := Place{Name: "บ้าน", Lat: 13.70, Lng: 100.5, Radius: 150}
	work := Place{Name: "ที่ทำงาน", Lat: 13.90, Lng: 100.5, Radius: 150}
	pts := []GeoPoint{
		{At: day.Add(8 * time.Hour), Lat: 13.7001, Lng: 100.5, Accuracy: 10},
		{At: day.Add(8*time.Hour + time.Minute), Lat: 13.7002, Lng: 100.5, Accuracy: 10},
		{At: day.Add(9 * time.Hour), Lat: 13.9001, Lng: 100.5, Accuracy: 10}, // single point: not counted
	}
	f := ComputePerson("Chubu", pts, []Place{home, work}, day, bkk)
	if len(f.Places) != 1 || f.Places[0] != "บ้าน" {
		t.Fatalf("places = %v", f.Places)
	}
}

func TestMovementNoPoints(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, bkk)
	f := ComputePerson("Furturia", nil, nil, day, bkk)
	if f.HasData || f.TotalKm != 0 || f.WalkKm != 0 || math.IsNaN(f.WalkKmMedian7) {
		t.Fatalf("no points = %+v", f)
	}
}

func TestMovementWeeklyMedian(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, bkk)
	var pts []GeoPoint
	for d := 1; d <= 6; d++ {
		pts = append(pts, track(day.AddDate(0, 0, -d).Add(8*time.Hour), 13.70, 5, 12)...) // 1 km/day
	}
	pts = append(pts, track(day.Add(8*time.Hour), 13.70, 5, 36)...) // 3 km yesterday
	f := ComputePerson("Chubu", pts, nil, day, bkk)
	if math.Abs(f.WalkKmMedian7-1) > 0.1 || math.Abs(f.WalkKm-3) > 0.1 {
		t.Fatalf("weekly = walk %.2f median %.2f", f.WalkKm, f.WalkKmMedian7)
	}
}

func TestTogetherMinutes(t *testing.T) {
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, bkk)
	var me, friend []GeoPoint
	start := day.Add(12 * time.Hour)
	for i := 0; i <= 90; i++ {
		at := start.Add(time.Duration(i) * time.Minute)
		me = append(me, GeoPoint{At: at, Lat: 13.7, Lng: 100.5, Accuracy: 10})
		friend = append(friend, GeoPoint{At: at.Add(20 * time.Second), Lat: 13.7003, Lng: 100.5, Accuracy: 10}) // ~33 m
	}
	// Later: 200 m apart for an hour, must not count.
	for i := 0; i <= 60; i++ {
		at := start.Add(3*time.Hour + time.Duration(i)*time.Minute)
		me = append(me, GeoPoint{At: at, Lat: 13.7, Lng: 100.5, Accuracy: 10})
		friend = append(friend, GeoPoint{At: at, Lat: 13.7018, Lng: 100.5, Accuracy: 10})
	}
	f := ComputeTogether(me, friend, day, bkk)
	if !f.HasData || f.Minutes < 88 || f.Minutes > 92 {
		t.Fatalf("together = %+v", f)
	}
	if !f.LongestStart.Equal(start) {
		t.Fatalf("longest start = %v, want %v", f.LongestStart, start)
	}
}

func TestToday(t *testing.T) {
	now := time.Date(2026, 11, 21, 7, 0, 0, 0, bkk)
	items := []ReminderItem{
		{At: now.Add(3 * time.Hour), Title: "ประชุมทีม"},
		{At: now.Add(1 * time.Hour), Title: "โทรหาแม่"},
		{At: now.Add(30 * time.Hour), Title: "พรุ่งนี้"},
		{At: now.Add(-1 * time.Hour), Title: "ผ่านไปแล้ว"},
	}
	f := ComputeToday(items, now, bkk)
	if f.Count != 2 || f.NextTitle != "โทรหาแม่" {
		t.Fatalf("today = %+v", f)
	}
}
