package hostmem

import (
	"testing"
)

const sampleMeminfoModern = `MemTotal:        5844352 kB
MemFree:         1207840 kB
MemAvailable:    2939876 kB
Buffers:           27400 kB
Cached:          1696552 kB
SwapCached:        29456 kB
Active:          1517760 kB
`

const sampleMeminfoLegacy = `MemTotal:        4000000 kB
MemFree:         1000000 kB
Buffers:          200000 kB
Cached:           800000 kB
`

func TestParseModern(t *testing.T) {
	info, err := Parse(sampleMeminfoModern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.TotalMB < 5700 || info.TotalMB > 5710 {
		t.Errorf("expected TotalMB ~5707.38, got %f", info.TotalMB)
	}
	if info.AvailableMB < 2870 || info.AvailableMB > 2872 {
		t.Errorf("expected AvailableMB ~2870.97, got %f", info.AvailableMB)
	}
	expectedUsed := info.TotalMB - info.AvailableMB
	if mathAbs(info.UsedMB-expectedUsed) > 0.1 {
		t.Errorf("expected UsedMB ~%f, got %f", expectedUsed, info.UsedMB)
	}
	if info.UsedPercent < 45 || info.UsedPercent > 55 {
		t.Errorf("expected UsedPercent ~49.7%%, got %f", info.UsedPercent)
	}
}

func TestParseLegacy(t *testing.T) {
	info, err := Parse(sampleMeminfoLegacy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Total: 4000000 kB = 3906.25 MB
	// Avail: (1000000 + 200000 + 800000) kB = 2000000 kB = 1953.13 MB
	// Used: 1953.12 MB (~50%)
	if info.TotalMB < 3900 || info.TotalMB > 3910 {
		t.Errorf("expected TotalMB ~3906, got %f", info.TotalMB)
	}
	if info.UsedPercent < 49 || info.UsedPercent > 51 {
		t.Errorf("expected UsedPercent ~50%%, got %f", info.UsedPercent)
	}
}

func TestParseEmpty(t *testing.T) {
	info, err := Parse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.TotalMB != 0 {
		t.Errorf("expected 0 total, got %f", info.TotalMB)
	}
}

func TestRead(t *testing.T) {
	info, err := Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	// On this Linux system, /proc/meminfo exists and has valid memory
	if info.TotalMB > 0 {
		if info.UsedMB <= 0 || info.AvailableMB <= 0 {
			t.Errorf("expected positive used and available, got used=%f avail=%f", info.UsedMB, info.AvailableMB)
		}
	}
}

func mathAbs(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}
