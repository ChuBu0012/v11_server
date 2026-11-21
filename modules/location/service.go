package location

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

const (
	maximumColocationAccuracy = 100.0
	batteryCriticalPercent    = 15
	batteryRecoveryPercent    = 25
	batteryAlertCooldown      = 2 * time.Hour
	locationDeliveryTimeout   = 8 * time.Second
)

type EventKind string

const (
	EventMeet            EventKind = "meet"
	EventSeparate        EventKind = "separate"
	EventGeofenceEnter   EventKind = "geofence_enter"
	EventGeofenceExit    EventKind = "geofence_exit"
	EventBatteryCritical EventKind = "battery_critical"
)

// NotificationEvent contains the facts needed to render one Discord message.
type NotificationEvent struct {
	Kind           EventKind
	DeviceID       string
	Point          LocationPoint
	Partner        *LocationPoint
	Geofence       Geofence
	HasGeofence    bool
	DistanceMeters float64
	OccurredAt     time.Time
}

type MessageNotifier interface {
	Send(context.Context, string) error
}

type Service struct {
	store    *Store
	notifier MessageNotifier
	clock    func() time.Time
	mu       sync.Mutex
}

type ServiceOption func(*Service)

func WithClock(clock func() time.Time) ServiceOption {
	return func(service *Service) {
		if clock != nil {
			service.clock = clock
		}
	}
}

func NewService(store *Store, notifier MessageNotifier, opts ...ServiceOption) *Service {
	service := &Service{store: store, notifier: notifier, clock: time.Now}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// ProcessPoint saves one sample, evaluates transitions, and dispatches notices
// asynchronously so Discord latency does not hold up the OwnTracks ACK.
func (s *Service) ProcessPoint(ctx context.Context, point LocationPoint) ([]NotificationEvent, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("location service is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	latest, err := s.store.LatestPoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("read latest locations: %w", err)
	}
	if previous := latest[point.DeviceID]; previous != nil {
		if !point.RecordedAt.After(previous.RecordedAt) {
			return nil, nil
		}
		// Debounce high-frequency stationary updates: if moved < 5m, arrived within 10s,
		// battery is identical, and accuracy is not an improvement, skip redundant DB writes.
		dist := DistanceMeters(previous.Latitude, previous.Longitude, point.Latitude, point.Longitude)
		if dist < 5.0 && point.RecordedAt.Sub(previous.RecordedAt) < 10*time.Second && previous.Battery == point.Battery && point.Accuracy >= previous.Accuracy {
			return nil, nil
		}
	}
	if err := s.store.SavePoint(ctx, point); err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read location settings: %w", err)
	}
	var events []NotificationEvent
	geofenceEvents, err := s.evaluateGeofences(ctx, point, now, settings)
	if err != nil {
		return nil, err
	}
	events = append(events, geofenceEvents...)
	coLocationEvents, err := s.evaluateCoLocation(ctx, now, settings)
	if err != nil {
		return nil, err
	}
	events = append(events, coLocationEvents...)
	batteryEvents, err := s.evaluateBattery(ctx, point, now, settings)
	if err != nil {
		return nil, err
	}
	events = append(events, batteryEvents...)
	s.dispatchAsync(events)
	return events, nil
}

// UpdateGeofence saves the edit, then silently re-baselines each device's
// inside/outside state against the new centre and radius. An edit is not
// movement, so it must never produce an enter/exit notification.
func (s *Service) UpdateGeofence(ctx context.Context, id int64, input GeofenceInput) (Geofence, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	zone, found, err := s.store.UpdateGeofence(ctx, id, input)
	if err != nil || !found {
		return zone, found, err
	}
	latest, err := s.store.LatestPoints(ctx)
	if err != nil {
		return zone, true, fmt.Errorf("read latest locations: %w", err)
	}
	now := s.clock()
	for device, point := range latest {
		if point == nil {
			continue
		}
		inside := DistanceMeters(point.Latitude, point.Longitude, zone.Latitude, zone.Longitude) <= zone.Radius
		if err := s.store.SetGeofenceState(ctx, GeofenceState{DeviceID: device, GeofenceID: zone.ID, IsInside: inside, LastTransition: now}); err != nil {
			return zone, true, err
		}
	}
	return zone, true, nil
}

func (s *Service) Live(ctx context.Context) (LiveResponse, error) {
	latest, err := s.store.LatestPoints(ctx)
	if err != nil {
		return LiveResponse{}, err
	}
	zones, err := s.store.ListGeofences(ctx)
	if err != nil {
		return LiveResponse{}, err
	}
	trails, err := s.store.Breadcrumbs(ctx, s.clock().Add(-24*time.Hour), 2000)
	if err != nil {
		return LiveResponse{}, err
	}
	state, err := s.store.GetCoLocationState(ctx)
	if err != nil {
		return LiveResponse{}, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return LiveResponse{}, err
	}
	response := LiveResponse{
		Me: latest[DeviceMe], Friend: latest[DeviceFriend], IsCoLocated: state.IsCoLocated,
		Geofences: zones, Breadcrumbs: trails,
		Settings: NotificationSettings{NotifyCoLocation: settings.NotifyCoLocation,
			NotifyGeofence: settings.NotifyGeofence, NotifyBattery: settings.NotifyBattery,
			EnterMeters: settings.EnterMeters, ExitMeters: settings.ExitMeters,
			CooldownSeconds: settings.CooldownSeconds},
	}
	if response.Me != nil && response.Friend != nil {
		distance := DistanceMeters(response.Me.Latitude, response.Me.Longitude, response.Friend.Latitude, response.Friend.Longitude)
		response.DistanceMeters = &distance
	}
	return response, nil
}

func (s *Service) evaluateCoLocation(ctx context.Context, now time.Time, settings LocationSettings) ([]NotificationEvent, error) {
	latest, err := s.store.LatestPoints(ctx)
	if err != nil {
		return nil, err
	}
	me, friend := latest[DeviceMe], latest[DeviceFriend]
	if me == nil || friend == nil || me.Accuracy > maximumColocationAccuracy || friend.Accuracy > maximumColocationAccuracy {
		return nil, nil
	}
	distance := DistanceMeters(me.Latitude, me.Longitude, friend.Latitude, friend.Longitude)
	state, err := s.store.GetCoLocationState(ctx)
	if err != nil {
		return nil, err
	}
	state.LastDistance = distance
	cooldown := time.Duration(settings.CooldownSeconds) * time.Second
	transitionAllowed := !now.Before(state.LastTransitionAt) && now.Sub(state.LastTransitionAt) >= cooldown
	var event *NotificationEvent
	if !state.IsCoLocated && distance <= settings.EnterMeters && transitionAllowed {
		state.IsCoLocated = true
		state.LastTransitionAt = now
		if settings.NotifyCoLocation {
			event = &NotificationEvent{Kind: EventMeet, Point: *me, Partner: friend, DistanceMeters: distance, OccurredAt: now}
			zone, ok, err := s.matchingGeofence(ctx, me, friend)
			if err != nil {
				return nil, err
			}
			if ok {
				event.Geofence, event.HasGeofence = zone, true
			}
			state.LastNotifiedAt = now
		}
	} else if state.IsCoLocated && distance >= settings.ExitMeters && transitionAllowed {
		state.IsCoLocated = false
		state.LastTransitionAt = now
		if settings.NotifyCoLocation {
			event = &NotificationEvent{Kind: EventSeparate, Point: *me, Partner: friend, DistanceMeters: distance, OccurredAt: now}
			state.LastNotifiedAt = now
		}
	}
	if err := s.store.SetCoLocationState(ctx, state); err != nil {
		return nil, err
	}
	if event == nil {
		return nil, nil
	}
	return []NotificationEvent{*event}, nil
}

func (s *Service) matchingGeofence(ctx context.Context, me, friend *LocationPoint) (Geofence, bool, error) {
	zones, err := s.store.ListGeofences(ctx)
	if err != nil {
		return Geofence{}, false, err
	}
	var match Geofence
	matchDistance := math.MaxFloat64
	for _, zone := range zones {
		meDistance := DistanceMeters(me.Latitude, me.Longitude, zone.Latitude, zone.Longitude)
		friendDistance := DistanceMeters(friend.Latitude, friend.Longitude, zone.Latitude, zone.Longitude)
		if meDistance <= zone.Radius && friendDistance <= zone.Radius {
			midLat := (me.Latitude + friend.Latitude) / 2
			midLon := (me.Longitude + friend.Longitude) / 2
			centerDistance := DistanceMeters(midLat, midLon, zone.Latitude, zone.Longitude)
			if centerDistance < matchDistance {
				match, matchDistance = zone, centerDistance
			}
		}
	}
	return match, matchDistance < math.MaxFloat64, nil
}

func (s *Service) evaluateGeofences(ctx context.Context, point LocationPoint, now time.Time, settings LocationSettings) ([]NotificationEvent, error) {
	zones, err := s.store.ListGeofences(ctx)
	if err != nil {
		return nil, err
	}
	events := make([]NotificationEvent, 0)
	for _, zone := range zones {
		distance := DistanceMeters(point.Latitude, point.Longitude, zone.Latitude, zone.Longitude)
		inside := distance <= zone.Radius
		previous, err := s.store.GetGeofenceState(ctx, point.DeviceID, zone.ID)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("read geofence state: %w", err)
		}
		wasInside := err == nil && previous.IsInside
		if wasInside == inside {
			continue
		}
		state := GeofenceState{DeviceID: point.DeviceID, GeofenceID: zone.ID, IsInside: inside, LastTransition: now}
		if err := s.store.SetGeofenceState(ctx, state); err != nil {
			return nil, err
		}
		notify := settings.NotifyGeofence && ((inside && zone.NotifyEnter) || (!inside && zone.NotifyExit))
		if !notify {
			continue
		}
		kind := EventGeofenceExit
		if inside {
			kind = EventGeofenceEnter
		}
		events = append(events, NotificationEvent{Kind: kind, DeviceID: point.DeviceID,
			Point: point, Geofence: zone, HasGeofence: true, OccurredAt: now})
	}
	return events, nil
}

func (s *Service) evaluateBattery(ctx context.Context, point LocationPoint, now time.Time, settings LocationSettings) ([]NotificationEvent, error) {
	if point.Battery < 0 {
		return nil, nil
	}
	state, err := s.store.GetBatteryState(ctx, point.DeviceID)
	if err != nil {
		return nil, err
	}
	if point.Battery > batteryRecoveryPercent {
		if state.IsCritical || !state.LastAlertAt.IsZero() {
			state.IsCritical = false
			state.LastAlertAt = time.Unix(0, 0).UTC()
			if err := s.store.SetBatteryState(ctx, state); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if point.Battery > batteryCriticalPercent || point.BatteryStatus != 1 || !settings.NotifyBattery {
		return nil, nil
	}
	if state.IsCritical && now.Sub(state.LastAlertAt) < batteryAlertCooldown {
		return nil, nil
	}
	state.IsCritical = true
	state.LastAlertAt = now
	if err := s.store.SetBatteryState(ctx, state); err != nil {
		return nil, err
	}
	return []NotificationEvent{{Kind: EventBatteryCritical, DeviceID: point.DeviceID, Point: point, OccurredAt: now}}, nil
}

func (s *Service) dispatchAsync(events []NotificationEvent) {
	if s.notifier == nil || len(events) == 0 {
		return
	}
	for _, event := range events {
		event := event
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), locationDeliveryTimeout)
			defer cancel()
			if err := s.notifier.Send(ctx, FormatDiscordEvent(event)); err != nil {
				log.Printf("[location] notification delivery failed (%s): %v", event.Kind, err)
			}
		}()
	}
}
