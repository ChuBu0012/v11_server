package location

import (
	"math"
	"testing"
)

func TestDistanceMeters(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		want, tolerance        float64
	}{
		{name: "same point", lat1: 13.7, lon1: 100.5, lat2: 13.7, lon2: 100.5, want: 0, tolerance: 0.001},
		{name: "anti-meridian", lat1: 0, lon1: 179, lat2: 0, lon2: -179, want: 222390, tolerance: 100},
		{name: "north pole", lat1: 90, lon1: 0, lat2: 90, lon2: 180, want: 0, tolerance: 0.001},
		{name: "Siam Paragon to CentralWorld", lat1: 13.7466, lon1: 100.5347, lat2: 13.7465, lon2: 100.5393, want: 496, tolerance: 60},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DistanceMeters(tc.lat1, tc.lon1, tc.lat2, tc.lon2)
			if math.Abs(got-tc.want) > tc.tolerance {
				t.Fatalf("DistanceMeters() = %.3f, want %.3f ± %.3f", got, tc.want, tc.tolerance)
			}
		})
	}
}

func TestValidCoordinates(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		lon  float64
		want bool
	}{
		{name: "origin", lat: 0, lon: 0, want: true},
		{name: "north east bounds", lat: 90, lon: 180, want: true},
		{name: "south west bounds", lat: -90, lon: -180, want: true},
		{name: "latitude too high", lat: 90.01, lon: 0},
		{name: "longitude too low", lat: 0, lon: -180.01},
		{name: "nan", lat: math.NaN(), lon: 0},
		{name: "infinity", lat: 0, lon: math.Inf(1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidCoordinates(tc.lat, tc.lon); got != tc.want {
				t.Fatalf("ValidCoordinates(%.3f, %.3f) = %v, want %v", tc.lat, tc.lon, got, tc.want)
			}
		})
	}
}

func TestFormatDistance(t *testing.T) {
	tests := []struct {
		meters float64
		want   string
	}{
		{meters: 0, want: "0 ม."},
		{meters: 35.4, want: "35 ม."},
		{meters: 999.6, want: "1000 ม."},
		{meters: 1234, want: "1.2 กม."},
	}
	for _, tc := range tests {
		if got := FormatDistance(tc.meters); got != tc.want {
			t.Errorf("FormatDistance(%.1f) = %q, want %q", tc.meters, got, tc.want)
		}
	}
}

func BenchmarkDistanceMeters(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DistanceMeters(13.7462, 100.5348, 13.7471, 100.5393)
	}
}
