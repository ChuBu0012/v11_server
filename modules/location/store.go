package location

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const storeOperationTimeout = 3 * time.Second

var errInvalidLocationData = errors.New("invalid location data")

// Credentials are supplied by config. Explicit values replace persisted tokens;
// generated defaults are only used when the database has no saved token yet.
type Credentials struct {
	SecretToken    string
	ViewerToken    string
	SecretExplicit bool
	ViewerExplicit bool
}

// Store provides bounded SQLite operations for location data.
type Store struct {
	db          *sql.DB
	secretToken string
	viewerToken string
}

func NewStore(database *sql.DB, credentials Credentials) (*Store, error) {
	if database == nil {
		return nil, errors.New("location store requires a database")
	}
	if credentials.SecretToken == "" {
		var err error
		credentials.SecretToken, err = newToken()
		if err != nil {
			return nil, fmt.Errorf("generate location secret token: %w", err)
		}
	}
	if credentials.ViewerToken == "" {
		var err error
		credentials.ViewerToken, err = newToken()
		if err != nil {
			return nil, fmt.Errorf("generate location viewer token: %w", err)
		}
	}
	if len(credentials.SecretToken) < 32 || len(credentials.ViewerToken) < 32 {
		return nil, errors.New("location tokens must contain at least 32 characters")
	}

	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()

	var storedSecret, storedViewer string
	if err := database.QueryRowContext(ctx, `SELECT secret_token, viewer_token FROM location_settings WHERE id=1`).Scan(&storedSecret, &storedViewer); err != nil {
		return nil, fmt.Errorf("read location credentials: %w", err)
	}
	if !credentials.SecretExplicit && storedSecret != "" {
		credentials.SecretToken = storedSecret
	}
	if !credentials.ViewerExplicit && storedViewer != "" {
		credentials.ViewerToken = storedViewer
	}
	if credentials.SecretToken == credentials.ViewerToken {
		return nil, errors.New("location secret and viewer tokens must be distinct")
	}

	if _, err := database.ExecContext(ctx, `UPDATE location_settings SET secret_token=?, viewer_token=?, updated_at=? WHERE id=1`,
		credentials.SecretToken, credentials.ViewerToken, time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("persist location credentials: %w", err)
	}
	return &Store{db: database, secretToken: credentials.SecretToken, viewerToken: credentials.ViewerToken}, nil
}

func newToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Tokens returns the ingress and viewer tokens for route authentication.
func (s *Store) Tokens() (secret, viewer string) { return s.secretToken, s.viewerToken }

func (s *Store) SavePoint(ctx context.Context, point LocationPoint) error {
	if !validDevice(point.DeviceID) || !ValidCoordinates(point.Latitude, point.Longitude) ||
		!finite(point.Accuracy) || point.Accuracy <= 0 ||
		!finite(point.Speed) || point.Speed < 0 ||
		!finite(point.Altitude) || !finite(point.Course) ||
		point.Battery < -1 || point.Battery > 100 || point.BatteryStatus < 0 || point.BatteryStatus > 3 || point.RecordedAt.IsZero() {
		return errInvalidLocationData
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO location_points
		(device_id, latitude, longitude, accuracy, speed, altitude, course, battery_level, battery_status, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, point.DeviceID, point.Latitude, point.Longitude,
		point.Accuracy, point.Speed, point.Altitude, point.Course, point.Battery,
		point.BatteryStatus, point.RecordedAt.UTC())
	if err != nil {
		return fmt.Errorf("save location point: %w", err)
	}
	return nil
}

func (s *Store) LatestPoints(ctx context.Context) (map[string]*LocationPoint, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT p.id, p.device_id, p.latitude, p.longitude, p.accuracy,
		p.speed, p.altitude, p.course, p.battery_level, p.battery_status, p.recorded_at
		FROM location_points p JOIN (
			SELECT device_id, MAX(recorded_at) AS latest_at FROM location_points GROUP BY device_id
		) latest ON p.device_id=latest.device_id AND p.recorded_at=latest.latest_at
		ORDER BY p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query latest locations: %w", err)
	}
	defer rows.Close()
	result := make(map[string]*LocationPoint, 2)
	for rows.Next() {
		point, err := scanPoint(rows)
		if err != nil {
			return nil, fmt.Errorf("scan latest location: %w", err)
		}
		if result[point.DeviceID] == nil {
			result[point.DeviceID] = &point
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) Breadcrumbs(ctx context.Context, since time.Time, limitPerDevice int) (map[string][]LocationPoint, error) {
	if limitPerDevice <= 0 || limitPerDevice > 2000 {
		limitPerDevice = 2000
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	result := map[string][]LocationPoint{DeviceMe: {}, DeviceFriend: {}}
	for _, device := range []string{DeviceMe, DeviceFriend} {
		rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, latitude, longitude, accuracy,
			speed, altitude, course, battery_level, battery_status, recorded_at
			FROM location_points WHERE device_id=? AND recorded_at>=?
			ORDER BY recorded_at DESC LIMIT ?`, device, since.UTC(), limitPerDevice)
		if err != nil {
			return nil, fmt.Errorf("query %s breadcrumbs: %w", device, err)
		}
		points := make([]LocationPoint, 0, min(limitPerDevice, 256))
		for rows.Next() {
			point, err := scanPoint(rows)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan %s breadcrumb: %w", device, err)
			}
			points = append(points, point)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		for left, right := 0, len(points)-1; left < right; left, right = left+1, right-1 {
			points[left], points[right] = points[right], points[left]
		}
		result[device] = points
	}
	return result, nil
}

// PointsBetween returns one device's points in [from, to), oldest first.
// It is capped at 20000 rows (about two weeks at one point per minute).
func (s *Store) PointsBetween(ctx context.Context, device string, from, to time.Time) ([]LocationPoint, error) {
	if !validDevice(device) {
		return nil, fmt.Errorf("unknown device %q", device)
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, latitude, longitude, accuracy,
		speed, altitude, course, battery_level, battery_status, recorded_at
		FROM location_points WHERE device_id=? AND recorded_at>=? AND recorded_at<?
		ORDER BY recorded_at ASC LIMIT 20000`, device, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("query %s points: %w", device, err)
	}
	defer rows.Close()
	points := []LocationPoint{}
	for rows.Next() {
		point, err := scanPoint(rows)
		if err != nil {
			return nil, fmt.Errorf("scan %s point: %w", device, err)
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (s *Store) CreateGeofence(ctx context.Context, input GeofenceInput) (Geofence, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 80 || !ValidCoordinates(input.Latitude, input.Longitude) ||
		!finite(input.Radius) || input.Radius <= 0 || input.Radius > 100000 {
		return Geofence{}, errors.New("invalid geofence name, coordinates, or radius")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	notifyEnter := true
	if input.NotifyEnter != nil {
		notifyEnter = *input.NotifyEnter
	}
	notifyExit := true
	if input.NotifyExit != nil {
		notifyExit = *input.NotifyExit
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO location_geofences
		(name, latitude, longitude, radius_meters, notify_enter, notify_exit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, input.Name, input.Latitude, input.Longitude,
		input.Radius, notifyEnter, notifyExit, now, now)
	if err != nil {
		return Geofence{}, fmt.Errorf("create geofence: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Geofence{}, err
	}
	return Geofence{ID: id, Name: input.Name, Latitude: input.Latitude, Longitude: input.Longitude,
		Radius: input.Radius, NotifyEnter: notifyEnter, NotifyExit: notifyExit, CreatedAt: now, UpdatedAt: now}, nil
}

// UpdateGeofence replaces a geofence's name, centre, radius, and (when non-nil)
// notification flags. found is false when no geofence has the given id.
func (s *Store) UpdateGeofence(ctx context.Context, id int64, input GeofenceInput) (Geofence, bool, error) {
	if id <= 0 {
		return Geofence{}, false, errors.New("geofence id must be positive")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 80 || !ValidCoordinates(input.Latitude, input.Longitude) ||
		!finite(input.Radius) || input.Radius <= 0 || input.Radius > 100000 {
		return Geofence{}, false, errors.New("invalid geofence name, coordinates, or radius")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	var zone Geofence
	err := s.db.QueryRowContext(ctx, `SELECT id, name, latitude, longitude, radius_meters,
		notify_enter, notify_exit, created_at, updated_at FROM location_geofences WHERE id=?`, id).
		Scan(&zone.ID, &zone.Name, &zone.Latitude, &zone.Longitude, &zone.Radius,
			&zone.NotifyEnter, &zone.NotifyExit, &zone.CreatedAt, &zone.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Geofence{}, false, nil
	}
	if err != nil {
		return Geofence{}, false, fmt.Errorf("read geofence: %w", err)
	}
	zone.Name, zone.Latitude, zone.Longitude, zone.Radius = input.Name, input.Latitude, input.Longitude, input.Radius
	if input.NotifyEnter != nil {
		zone.NotifyEnter = *input.NotifyEnter
	}
	if input.NotifyExit != nil {
		zone.NotifyExit = *input.NotifyExit
	}
	zone.UpdatedAt = time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE location_geofences SET name=?, latitude=?, longitude=?,
		radius_meters=?, notify_enter=?, notify_exit=?, updated_at=? WHERE id=?`, zone.Name, zone.Latitude,
		zone.Longitude, zone.Radius, zone.NotifyEnter, zone.NotifyExit, zone.UpdatedAt, id); err != nil {
		return Geofence{}, false, fmt.Errorf("update geofence: %w", err)
	}
	return zone, true, nil
}

func (s *Store) ListGeofences(ctx context.Context) ([]Geofence, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, latitude, longitude, radius_meters,
		notify_enter, notify_exit, created_at, updated_at FROM location_geofences ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list geofences: %w", err)
	}
	defer rows.Close()
	zones := make([]Geofence, 0)
	for rows.Next() {
		var zone Geofence
		if err := rows.Scan(&zone.ID, &zone.Name, &zone.Latitude, &zone.Longitude, &zone.Radius,
			&zone.NotifyEnter, &zone.NotifyExit, &zone.CreatedAt, &zone.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan geofence: %w", err)
		}
		zones = append(zones, zone)
	}
	return zones, rows.Err()
}

func (s *Store) DeleteGeofence(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, errors.New("geofence id must be positive")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin geofence delete: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM location_geofence_states WHERE geofence_id=?`, id); err != nil {
		return false, fmt.Errorf("delete geofence states: %w", err)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM location_geofences WHERE id=?`, id)
	if err != nil {
		return false, fmt.Errorf("delete geofence: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit geofence delete: %w", err)
	}
	return count > 0, nil
}

func (s *Store) GetGeofenceState(ctx context.Context, device string, geofenceID int64) (GeofenceState, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	var state GeofenceState
	state.DeviceID, state.GeofenceID = device, geofenceID
	err := s.db.QueryRowContext(ctx, `SELECT is_inside, last_transition_at FROM location_geofence_states
		WHERE device_id=? AND geofence_id=?`, device, geofenceID).Scan(&state.IsInside, &state.LastTransition)
	return state, err
}

func (s *Store) SetGeofenceState(ctx context.Context, state GeofenceState) error {
	if !validDevice(state.DeviceID) || state.GeofenceID <= 0 || state.LastTransition.IsZero() {
		return errors.New("invalid geofence state")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO location_geofence_states
		(device_id, geofence_id, is_inside, last_transition_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(device_id, geofence_id) DO UPDATE SET is_inside=excluded.is_inside,
		last_transition_at=excluded.last_transition_at, updated_at=excluded.updated_at`,
		state.DeviceID, state.GeofenceID, state.IsInside, state.LastTransition.UTC(), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save geofence state: %w", err)
	}
	return nil
}

func (s *Store) GetCoLocationState(ctx context.Context) (CoLocationState, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	var state CoLocationState
	err := s.db.QueryRowContext(ctx, `SELECT is_colocated, last_distance_meters, last_transition_at, last_notified_at
		FROM location_colocation_state WHERE id=1`).Scan(&state.IsCoLocated, &state.LastDistance,
		&state.LastTransitionAt, &state.LastNotifiedAt)
	return state, err
}

func (s *Store) SetCoLocationState(ctx context.Context, state CoLocationState) error {
	if !finite(state.LastDistance) || state.LastDistance < 0 || state.LastTransitionAt.IsZero() || state.LastNotifiedAt.IsZero() {
		return errors.New("invalid co-location state")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `UPDATE location_colocation_state SET is_colocated=?,
		last_distance_meters=?, last_transition_at=?, last_notified_at=?, updated_at=? WHERE id=1`,
		state.IsCoLocated, state.LastDistance, state.LastTransitionAt.UTC(), state.LastNotifiedAt.UTC(), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save co-location state: %w", err)
	}
	return nil
}

func (s *Store) GetBatteryState(ctx context.Context, device string) (BatteryState, error) {
	if !validDevice(device) {
		return BatteryState{}, errors.New("invalid device")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	state := BatteryState{DeviceID: device}
	err := s.db.QueryRowContext(ctx, `SELECT is_critical, last_alert_at FROM location_battery_states WHERE device_id=?`, device).
		Scan(&state.IsCritical, &state.LastAlertAt)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	return state, err
}

func (s *Store) SetBatteryState(ctx context.Context, state BatteryState) error {
	if !validDevice(state.DeviceID) || state.LastAlertAt.IsZero() {
		return errors.New("invalid battery state")
	}
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO location_battery_states
		(device_id, is_critical, last_alert_at, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET is_critical=excluded.is_critical,
		last_alert_at=excluded.last_alert_at, updated_at=excluded.updated_at`,
		state.DeviceID, state.IsCritical, state.LastAlertAt.UTC(), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save battery state: %w", err)
	}
	return nil
}

func (s *Store) GetSettings(ctx context.Context) (LocationSettings, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	var settings LocationSettings
	err := s.db.QueryRowContext(ctx, `SELECT secret_token, viewer_token, notify_colocation,
		notify_geofence, notify_battery, colocation_enter_meters, colocation_exit_meters, cooldown_seconds
		FROM location_settings WHERE id=1`).Scan(&settings.SecretToken, &settings.ViewerToken,
		&settings.NotifyCoLocation, &settings.NotifyGeofence, &settings.NotifyBattery,
		&settings.EnterMeters, &settings.ExitMeters, &settings.CooldownSeconds)
	return settings, err
}

func (s *Store) UpdateSettings(ctx context.Context, input SettingsInput) (LocationSettings, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	var colocation, geofence, battery any
	if input.NotifyCoLocation != nil {
		colocation = *input.NotifyCoLocation
	}
	if input.NotifyGeofence != nil {
		geofence = *input.NotifyGeofence
	}
	if input.NotifyBattery != nil {
		battery = *input.NotifyBattery
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE location_settings SET
		notify_colocation=COALESCE(?, notify_colocation),
		notify_geofence=COALESCE(?, notify_geofence),
		notify_battery=COALESCE(?, notify_battery), updated_at=? WHERE id=1`,
		colocation, geofence, battery, time.Now().UTC()); err != nil {
		return LocationSettings{}, fmt.Errorf("update location settings: %w", err)
	}
	return s.GetSettings(ctx)
}

func (s *Store) PruneOld(ctx context.Context, cutoff time.Time) (int64, error) {
	ctx, cancel := s.operationContext(ctx)
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM location_points WHERE recorded_at < ?`, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("prune old locations: %w", err)
	}
	return result.RowsAffected()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPoint(row rowScanner) (LocationPoint, error) {
	var point LocationPoint
	err := row.Scan(&point.ID, &point.DeviceID, &point.Latitude, &point.Longitude, &point.Accuracy,
		&point.Speed, &point.Altitude, &point.Course, &point.Battery, &point.BatteryStatus, &point.RecordedAt)
	return point, err
}

func (s *Store) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, storeOperationTimeout)
}

func validDevice(device string) bool { return device == DeviceMe || device == DeviceFriend }

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
