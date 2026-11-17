package hostmem

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
)

// Info holds host memory metrics in megabytes and percentage.
type Info struct {
	TotalMB     float64 `json:"total_mb"`
	UsedMB      float64 `json:"used_mb"`
	AvailableMB float64 `json:"available_mb"`
	FreeMB      float64 `json:"free_mb"`
	UsedPercent float64 `json:"used_percent"`
}

// Read reads and parses host memory metrics from /proc/meminfo.
// If /proc/meminfo is not available (e.g. non-Linux systems), it returns an empty Info and nil error.
func Read() (Info, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		if os.IsNotExist(err) {
			return Info{}, nil
		}
		return Info{}, err
	}
	return Parse(string(data))
}

// Parse extracts memory metrics from /proc/meminfo formatted content.
func Parse(content string) (Info, error) {
	var totalKB, freeKB, availKB, buffersKB, cachedKB float64
	hasAvail := false

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon == -1 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		valStr := strings.TrimSpace(line[colon+1:])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)

		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}

		switch key {
		case "MemTotal":
			totalKB = val
		case "MemFree":
			freeKB = val
		case "MemAvailable":
			availKB = val
			hasAvail = true
		case "Buffers":
			buffersKB = val
		case "Cached":
			cachedKB = val
		}
	}

	if totalKB <= 0 {
		return Info{}, nil
	}

	totalMB := totalKB / 1024.0
	freeMB := freeKB / 1024.0
	var availMB float64
	if hasAvail {
		availMB = availKB / 1024.0
	} else {
		availMB = (freeKB + buffersKB + cachedKB) / 1024.0
	}
	if availMB > totalMB {
		availMB = totalMB
	}

	usedMB := totalMB - availMB
	if usedMB < 0 {
		usedMB = 0
	}
	usedPercent := 0.0
	if totalMB > 0 {
		usedPercent = (usedMB / totalMB) * 100.0
	}

	return Info{
		TotalMB:     math.Round(totalMB*100) / 100,
		UsedMB:      math.Round(usedMB*100) / 100,
		AvailableMB: math.Round(availMB*100) / 100,
		FreeMB:      math.Round(freeMB*100) / 100,
		UsedPercent: math.Round(usedPercent*10) / 10,
	}, nil
}
