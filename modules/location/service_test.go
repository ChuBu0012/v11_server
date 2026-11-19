package location

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceCoLocationHysteresisAndCooldown(t *testing.T) {
	_, store := openLocationTestStore(t)
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	ctx := context.Background()
	process := func(point LocationPoint) []NotificationEvent {
		t.Helper()
		events, err := service.ProcessPoint(ctx, point)
		if err != nil {
			t.Fatalf("ProcessPoint: %v", err)
		}
		return events
	}

	if events := process(samplePoint(DeviceMe, now, 0)); len(events) != 0 {
		t.Fatalf("first device emitted events: %+v", events)
	}
	events := process(samplePoint(DeviceFriend, now, metersToLatitude(70)))
	if len(events) != 1 || events[0].Kind != EventMeet || events[0].DistanceMeters > 80 {
		t.Fatalf("meeting event = %+v", events)
	}

	for _, distance := range []float64{85, 110, 249, 260} {
		now = now.Add(time.Minute)
		events := process(samplePoint(DeviceFriend, now, metersToLatitude(distance)))
		if len(events) != 0 {
			t.Fatalf("distance %.0fm emitted during jitter/cooldown: %+v", distance, events)
		}
	}
	now = now.Add(time.Minute)
	events = process(samplePoint(DeviceFriend, now, metersToLatitude(260)))
	if len(events) != 1 || events[0].Kind != EventSeparate || events[0].DistanceMeters < 250 {
		t.Fatalf("separation event after cooldown = %+v", events)
	}
}

func TestServiceFiltersInaccurateFixesFromCoLocation(t *testing.T) {
	_, store := openLocationTestStore(t)
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	if _, err := service.ProcessPoint(context.Background(), samplePoint(DeviceMe, now, 0)); err != nil {
		t.Fatal(err)
	}
	inaccurate := samplePoint(DeviceFriend, now, metersToLatitude(40))
	inaccurate.Accuracy = maximumColocationAccuracy + 1
	events, err := service.ProcessPoint(context.Background(), inaccurate)
	if err != nil || len(events) != 0 {
		t.Fatalf("inaccurate fix emitted co-location event: %+v, err=%v", events, err)
	}
	inaccurate.Accuracy = maximumColocationAccuracy
	inaccurate.RecordedAt = now.Add(time.Second)
	events, err = service.ProcessPoint(context.Background(), inaccurate)
	if err != nil || len(events) != 1 || events[0].Kind != EventMeet {
		t.Fatalf("accurate fix did not trigger meeting: %+v, err=%v", events, err)
	}
}

func TestServiceUsesMatchingGeofenceAsMeetingPlace(t *testing.T) {
	_, store := openLocationTestStore(t)
	zone, err := store.CreateGeofence(context.Background(), GeofenceInput{
		Name: "สยามพารากอน", Latitude: 13.7, Longitude: 100.5, Radius: 250,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	if _, err := service.ProcessPoint(context.Background(), samplePoint(DeviceMe, now, 13.7)); err != nil {
		t.Fatal(err)
	}
	friend := samplePoint(DeviceFriend, now, 13.7+metersToLatitude(30))
	events, err := service.ProcessPoint(context.Background(), friend)
	if err != nil {
		t.Fatalf("ProcessPoint: %v", err)
	}
	for _, event := range events {
		if event.Kind == EventMeet && event.HasGeofence && event.Geofence.ID == zone.ID {
			return
		}
	}
	t.Fatalf("meeting did not include matching geofence: %+v", events)
}

func TestServiceIgnoresOutOfOrderPoints(t *testing.T) {
	_, store := openLocationTestStore(t)
	zone, err := store.CreateGeofence(context.Background(), GeofenceInput{
		Name: "บ้าน", Latitude: 0, Longitude: 100.5, Radius: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	inside := samplePoint(DeviceFriend, now, 0)
	if events, err := service.ProcessPoint(context.Background(), inside); err != nil || len(events) != 1 || events[0].Kind != EventGeofenceEnter {
		t.Fatalf("initial point = events:%+v err:%v", events, err)
	}
	staleOutside := samplePoint(DeviceFriend, now.Add(-time.Minute), metersToLatitude(250))
	if events, err := service.ProcessPoint(context.Background(), staleOutside); err != nil || len(events) != 0 {
		t.Fatalf("stale point should be ignored: events:%+v err:%v", events, err)
	}
	latest, err := store.LatestPoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest[DeviceFriend] == nil || !latest[DeviceFriend].RecordedAt.Equal(now) || latest[DeviceFriend].Latitude != 0 || zone.ID <= 0 {
		t.Fatalf("stale point replaced latest location: %+v", latest[DeviceFriend])
	}
}

func TestServiceGeofenceEnterAndExit(t *testing.T) {
	_, store := openLocationTestStore(t)
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	zone, err := store.CreateGeofence(context.Background(), GeofenceInput{
		Name: "บ้านเพื่อน", Latitude: 0, Longitude: 0, Radius: 100, NotifyEnter: boolPtr(true), NotifyExit: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	inside := samplePoint(DeviceFriend, now, 0)
	inside.Longitude = 0
	events, err := service.ProcessPoint(context.Background(), inside)
	if err != nil || len(events) != 1 || events[0].Kind != EventGeofenceEnter || events[0].Geofence.ID != zone.ID {
		t.Fatalf("enter events = %+v, err=%v", events, err)
	}

	now = now.Add(time.Minute)
	outside := samplePoint(DeviceFriend, now, metersToLatitude(150))
	outside.Longitude = 0
	events, err = service.ProcessPoint(context.Background(), outside)
	if err != nil || len(events) != 1 || events[0].Kind != EventGeofenceExit {
		t.Fatalf("exit events = %+v, err=%v", events, err)
	}
}

func TestServiceBatteryAlertThrottleAndRecovery(t *testing.T) {
	_, store := openLocationTestStore(t)
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	point := samplePoint(DeviceFriend, now, 13.7)
	point.Battery, point.BatteryStatus = 15, 1
	events, err := service.ProcessPoint(context.Background(), point)
	if err != nil || len(events) != 1 || events[0].Kind != EventBatteryCritical {
		t.Fatalf("first battery event = %+v, err=%v", events, err)
	}

	now = now.Add(30 * time.Minute)
	point.RecordedAt = now
	events, err = service.ProcessPoint(context.Background(), point)
	if err != nil || len(events) != 0 {
		t.Fatalf("battery warning should be throttled: events=%+v err=%v", events, err)
	}

	now = now.Add(90 * time.Minute)
	point.RecordedAt = now
	events, err = service.ProcessPoint(context.Background(), point)
	if err != nil || len(events) != 1 || events[0].Kind != EventBatteryCritical {
		t.Fatalf("battery warning after 2h = %+v, err=%v", events, err)
	}

	now = now.Add(time.Minute)
	point.RecordedAt, point.Battery = now, 26
	if _, err := service.ProcessPoint(context.Background(), point); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	point.RecordedAt, point.Battery = now, 15
	events, err = service.ProcessPoint(context.Background(), point)
	if err != nil || len(events) != 1 || events[0].Kind != EventBatteryCritical {
		t.Fatalf("battery warning after recovery reset = %+v, err=%v", events, err)
	}
}

func TestServiceRespectsNotificationToggles(t *testing.T) {
	_, store := openLocationTestStore(t)
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	disabled := false
	if _, err := store.UpdateSettings(context.Background(), SettingsInput{NotifyCoLocation: &disabled}); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	if _, err := service.ProcessPoint(context.Background(), samplePoint(DeviceMe, now, 0)); err != nil {
		t.Fatal(err)
	}
	events, err := service.ProcessPoint(context.Background(), samplePoint(DeviceFriend, now, metersToLatitude(40)))
	if err != nil || len(events) != 0 {
		t.Fatalf("disabled co-location notifications emitted events: %+v, err=%v", events, err)
	}
	state, err := store.GetCoLocationState(context.Background())
	if err != nil || !state.IsCoLocated {
		t.Fatalf("notification toggle should not disable state tracking: %+v, err=%v", state, err)
	}
}

type blockingMessageNotifier struct {
	started chan struct{}
	release chan struct{}
}

func (n *blockingMessageNotifier) Send(ctx context.Context, _ string) error {
	close(n.started)
	select {
	case <-n.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestServiceDispatchesDiscordWithoutBlockingLocationProcessing(t *testing.T) {
	_, store := openLocationTestStore(t)
	now := time.Now().UTC()
	notifier := &blockingMessageNotifier{started: make(chan struct{}), release: make(chan struct{})}
	service := NewService(store, notifier, WithClock(func() time.Time { return now }))
	if _, err := service.ProcessPoint(context.Background(), samplePoint(DeviceMe, now, 0)); err != nil {
		t.Fatal(err)
	}
	events, err := service.ProcessPoint(context.Background(), samplePoint(DeviceFriend, now, metersToLatitude(40)))
	if err != nil || len(events) != 1 || events[0].Kind != EventMeet {
		t.Fatalf("meeting event = %+v, err=%v", events, err)
	}
	select {
	case <-notifier.started:
	case <-time.After(time.Second):
		t.Fatal("asynchronous notifier did not start")
	}
	close(notifier.release)
}

func TestServiceRejectsLocationBeforePersistingWhenStoreFails(t *testing.T) {
	service := NewService(nil, nil)
	_, err := service.ProcessPoint(context.Background(), samplePoint(DeviceMe, time.Now(), 0))
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("ProcessPoint without store error = %v, want configuration error", err)
	}
}

func metersToLatitude(meters float64) float64 { return meters / 111195.0 }
