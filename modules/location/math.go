package location

import (
	"fmt"
	"math"
)

const earthMeanRadiusMeters = 6371008.8

// ValidCoordinates reports whether a pair is a finite WGS84 coordinate.
func ValidCoordinates(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsInf(latitude, 0) &&
		!math.IsNaN(longitude) && !math.IsInf(longitude, 0) &&
		latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

// DistanceMeters returns the great-circle distance between two coordinates.
// Inputs are expected to have passed ValidCoordinates.
func DistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	toRadians := math.Pi / 180
	phi1 := lat1 * toRadians
	phi2 := lat2 * toRadians
	deltaPhi := (lat2 - lat1) * toRadians
	deltaLambda := (lon2 - lon1) * toRadians

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	// Floating-point rounding can move a extremely close to 1 at antipodal points.
	if a > 1 {
		a = 1
	}
	return 2 * earthMeanRadiusMeters * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// FormatDistance gives compact rounded meters for short distances and km otherwise.
func FormatDistance(meters float64) string {
	if meters <= 0 {
		return "0 ม."
	}
	if meters < 1000 {
		return fmt.Sprintf("%.0f ม.", meters)
	}
	return fmt.Sprintf("%.1f กม.", meters/1000)
}
