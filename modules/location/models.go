package location

import "time"

const (
	DeviceMe     = "me"
	DeviceFriend = "friend"
)

// LocationReport is the OwnTracks HTTP location payload.
type LocationReport struct {
	Type          string  `json:"_type"`
	Latitude      float64 `json:"lat"`
	Longitude     float64 `json:"lon"`
	Accuracy      float64 `json:"acc"`
	Timestamp     int64   `json:"tst"`
	Battery       *int    `json:"batt"`
	BatteryStatus int     `json:"bs"`
	Speed         float64 `json:"vel"`
	Altitude      float64 `json:"alt"`
	Course        float64 `json:"cog"`
}

// LocationPoint is a persisted OwnTracks sample.
type LocationPoint struct {
	ID            int64     `json:"id"`
	DeviceID      string    `json:"device_id"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	Accuracy      float64   `json:"accuracy"`
	Speed         float64   `json:"speed"`
	Altitude      float64   `json:"altitude"`
	Course        float64   `json:"course"`
	Battery       int       `json:"battery_level"`
	BatteryStatus int       `json:"battery_status"`
	RecordedAt    time.Time `json:"recorded_at"`
}

// Geofence is a circular place monitored for device arrival and departure.
type Geofence struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Radius      float64   `json:"radius_meters"`
	NotifyEnter bool      `json:"notify_enter"`
	NotifyExit  bool      `json:"notify_exit"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GeofenceState tracks whether one device is inside one geofence.
type GeofenceState struct {
	DeviceID       string
	GeofenceID     int64
	IsInside       bool
	LastTransition time.Time
}

// CoLocationState stores the singleton rendezvous state for me and friend.
type CoLocationState struct {
	IsCoLocated      bool
	LastDistance     float64
	LastTransitionAt time.Time
	LastNotifiedAt   time.Time
}

// BatteryState persists critical-battery alert throttling for one device.
type BatteryState struct {
	DeviceID    string
	IsCritical  bool
	LastAlertAt time.Time
}

// LocationSettings stores notification preferences and internal credentials.
// Credentials are deliberately excluded from JSON responses.
type LocationSettings struct {
	SecretToken      string  `json:"-"`
	ViewerToken      string  `json:"-"`
	NotifyCoLocation bool    `json:"notify_colocation"`
	NotifyGeofence   bool    `json:"notify_geofence"`
	NotifyBattery    bool    `json:"notify_battery"`
	EnterMeters      float64 `json:"colocation_enter_meters"`
	ExitMeters       float64 `json:"colocation_exit_meters"`
	CooldownSeconds  int     `json:"cooldown_seconds"`
}

// NotificationSettings is the safe subset exposed to the live viewer.
type NotificationSettings struct {
	NotifyCoLocation bool    `json:"notify_colocation"`
	NotifyGeofence   bool    `json:"notify_geofence"`
	NotifyBattery    bool    `json:"notify_battery"`
	EnterMeters      float64 `json:"colocation_enter_meters"`
	ExitMeters       float64 `json:"colocation_exit_meters"`
	CooldownSeconds  int     `json:"cooldown_seconds"`
}

// LiveResponse contains the latest device positions and viewer overlays.
type LiveResponse struct {
	Me             *LocationPoint             `json:"me,omitempty"`
	Friend         *LocationPoint             `json:"friend,omitempty"`
	DistanceMeters *float64                   `json:"distance_meters,omitempty"`
	IsCoLocated    bool                       `json:"is_colocated"`
	Geofences      []Geofence                 `json:"geofences"`
	Breadcrumbs    map[string][]LocationPoint `json:"breadcrumbs"`
	Settings       NotificationSettings       `json:"settings"`
}

// GeofenceInput is accepted from the PIN-protected geofence management API.
type GeofenceInput struct {
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Radius      float64 `json:"radius_meters"`
	NotifyEnter *bool   `json:"notify_enter"`
	NotifyExit  *bool   `json:"notify_exit"`
}

// SettingsInput changes only notification behavior, never credentials.
type SettingsInput struct {
	NotifyCoLocation *bool `json:"notify_colocation"`
	NotifyGeofence   *bool `json:"notify_geofence"`
	NotifyBattery    *bool `json:"notify_battery"`
}
