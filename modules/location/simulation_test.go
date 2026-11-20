package location

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type recordingNotifier struct {
	mu       sync.Mutex
	messages []string
}

func (r *recordingNotifier) Send(ctx context.Context, msg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
	return nil
}

func (r *recordingNotifier) Messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := make([]string, len(r.messages))
	copy(copied, r.messages)
	return copied
}

func TestSimulation_FullLife360Story(t *testing.T) {
	dbConn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer dbConn.Close()

	// Initialize tables
	_, err = dbConn.Exec(`
		CREATE TABLE location_settings (
			id INTEGER PRIMARY KEY CHECK (id=1),
			secret_token TEXT NOT NULL,
			viewer_token TEXT NOT NULL,
			notify_colocation BOOLEAN NOT NULL DEFAULT 1,
			notify_geofence BOOLEAN NOT NULL DEFAULT 1,
			notify_battery BOOLEAN NOT NULL DEFAULT 1,
			colocation_enter_meters REAL NOT NULL DEFAULT 80,
			colocation_exit_meters REAL NOT NULL DEFAULT 250,
			cooldown_seconds INTEGER NOT NULL DEFAULT 300,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO location_settings (id, secret_token, viewer_token) VALUES (1, 'secret-token-32-chars-long-abc12', 'viewer-token-32-chars-long-xyz12');

		CREATE TABLE location_points (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			latitude REAL NOT NULL,
			longitude REAL NOT NULL,
			accuracy REAL NOT NULL,
			speed REAL NOT NULL DEFAULT 0,
			altitude REAL NOT NULL DEFAULT 0,
			course REAL NOT NULL DEFAULT 0,
			battery_level INTEGER NOT NULL DEFAULT -1,
			battery_status INTEGER NOT NULL DEFAULT 0,
			recorded_at DATETIME NOT NULL
		);

		CREATE TABLE location_geofences (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			latitude REAL NOT NULL,
			longitude REAL NOT NULL,
			radius_meters REAL NOT NULL,
			notify_enter BOOLEAN NOT NULL DEFAULT 1,
			notify_exit BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE location_geofence_states (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			geofence_id INTEGER NOT NULL REFERENCES location_geofences(id) ON DELETE CASCADE,
			is_inside BOOLEAN NOT NULL DEFAULT 0,
			last_transition_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(device_id, geofence_id)
		);

		CREATE TABLE location_colocation_state (
			id INTEGER PRIMARY KEY CHECK (id=1),
			is_colocated BOOLEAN NOT NULL DEFAULT 0,
			last_distance_meters REAL NOT NULL DEFAULT 0,
			last_transition_at DATETIME,
			last_notified_at DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO location_colocation_state (id, is_colocated, last_distance_meters, last_transition_at, last_notified_at)
		VALUES (1, 0, 9999, '2026-01-01 00:00:00', '2026-01-01 00:00:00');

		CREATE TABLE location_battery_states (
			device_id TEXT PRIMARY KEY,
			is_critical BOOLEAN NOT NULL DEFAULT 0,
			last_alert_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	store, err := NewStore(dbConn, Credentials{
		SecretToken: "secret-token-32-chars-long-abc12",
		ViewerToken: "viewer-token-32-chars-long-xyz12",
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	notifier := &recordingNotifier{}
	simTime := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	service := NewService(store, notifier, WithClock(func() time.Time { return simTime }))
	ctx := context.Background()

	// 1. Create Geofence "มหาวิทยาลัย KMUTT" (lat: 13.6515, lng: 100.4939, radius: 150m)
	trueVal := true
	kmutt, err := store.CreateGeofence(ctx, GeofenceInput{
		Name:        "มหาวิทยาลัย KMUTT",
		Latitude:    13.6515,
		Longitude:   100.4939,
		Radius:      150,
		NotifyEnter: &trueVal,
		NotifyExit:  &trueVal,
	})
	if err != nil {
		t.Fatalf("create geofence: %v", err)
	}
	if kmutt.ID == 0 {
		t.Fatalf("expected valid geofence id")
	}

	// 2. Scenario 1: Chubu arrives at KMUTT
	events1, err := service.ProcessPoint(ctx, LocationPoint{
		DeviceID:      DeviceMe,
		Latitude:      13.65152,
		Longitude:     100.49391,
		Accuracy:      10,
		Battery:       85,
		BatteryStatus: 1,
		Speed:         0,
		RecordedAt:    simTime,
	})
	if err != nil {
		t.Fatalf("process point Chubu: %v", err)
	}
	if len(events1) == 0 {
		t.Fatalf("expected geofence enter event for Chubu")
	}

	// Give async dispatch a moment
	time.Sleep(50 * time.Millisecond)

	msgs := notifier.Messages()
	if len(msgs) == 0 {
		t.Fatalf("expected geofence enter notification for Chubu in notifier")
	}
	if !strings.Contains(msgs[0], "📍 **Chubu ถึง มหาวิทยาลัย KMUTT**") {
		t.Errorf("expected arrival message, got: %s", msgs[0])
	}

	// 3. Scenario 2: Furturia arrives at KMUTT and enters co-location with Chubu (distance ~8m)
	simTime = simTime.Add(6 * time.Minute)
	events2, err := service.ProcessPoint(ctx, LocationPoint{
		DeviceID:      DeviceFriend,
		Latitude:      13.65158,
		Longitude:     100.49395,
		Accuracy:      8,
		Battery:       70,
		BatteryStatus: 1,
		Speed:         0,
		RecordedAt:    simTime,
	})
	if err != nil {
		t.Fatalf("process point Furturia: %v", err)
	}
	if len(events2) < 2 {
		t.Fatalf("expected both arrival and meeting events, got %d events: %+v", len(events2), events2)
	}

	time.Sleep(50 * time.Millisecond)
	msgs = notifier.Messages()

	foundMeet := false
	foundArrival := false
	for _, m := range msgs {
		if strings.Contains(m, "📍 **Furturia ถึง มหาวิทยาลัย KMUTT**") {
			foundArrival = true
		}
		if strings.Contains(m, "🤝 **Chubu & Furturia อยู่ด้วยกันที่ มหาวิทยาลัย KMUTT**") {
			foundMeet = true
		}
	}
	if !foundArrival {
		t.Errorf("missing Furturia arrival notification: %+v", msgs)
	}
	if !foundMeet {
		t.Errorf("missing meeting notification: %+v", msgs)
	}

	// Verify Co-location state in Store is now true
	state, err := store.GetCoLocationState(ctx)
	if err != nil || !state.IsCoLocated {
		t.Fatalf("expected is_colocated=true, got %+v (err=%v)", state, err)
	}

	// 4. Scenario 3: Furturia departs from KMUTT to Siam Paragon (~11km away, speed 50 km/h) after cooldown
	simTime = simTime.Add(6 * time.Minute)
	events3, err := service.ProcessPoint(ctx, LocationPoint{
		DeviceID:      DeviceFriend,
		Latitude:      13.7462,
		Longitude:     100.5348,
		Accuracy:      12,
		Battery:       68,
		BatteryStatus: 1,
		Speed:         50,
		RecordedAt:    simTime,
	})
	if err != nil {
		t.Fatalf("process point Furturia departure: %v", err)
	}
	if len(events3) < 2 {
		t.Fatalf("expected both exit and separation events, got %d events: %+v", len(events3), events3)
	}

	time.Sleep(50 * time.Millisecond)
	msgs = notifier.Messages()

	foundExit := false
	foundSeparation := false
	for _, m := range msgs {
		if strings.Contains(m, "🚗 **Furturia ออกจาก มหาวิทยาลัย KMUTT**") {
			foundExit = true
		}
		if strings.Contains(m, "👋 **Chubu & Furturia แยกย้ายกัน**") {
			foundSeparation = true
		}
	}
	if !foundExit {
		t.Errorf("missing Furturia exit notification: %+v", msgs)
	}
	if !foundSeparation {
		t.Errorf("missing separation notification: %+v", msgs)
	}

	// Verify Co-location state in Store is now false
	state, err = store.GetCoLocationState(ctx)
	if err != nil || state.IsCoLocated {
		t.Fatalf("expected is_colocated=false, got %+v (err=%v)", state, err)
	}

	// 5. Scenario 4: Chubu low battery alert (10%, unplugged)
	simTime = simTime.Add(10 * time.Minute)
	events4, err := service.ProcessPoint(ctx, LocationPoint{
		DeviceID:      DeviceMe,
		Latitude:      13.65152,
		Longitude:     100.49391,
		Accuracy:      10,
		Battery:       10,
		BatteryStatus: 1, // unplugged
		Speed:         0,
		RecordedAt:    simTime,
	})
	if err != nil {
		t.Fatalf("process point Chubu low battery: %v", err)
	}
	if len(events4) == 0 {
		t.Fatalf("expected battery alert event")
	}

	time.Sleep(50 * time.Millisecond)
	msgs = notifier.Messages()

	foundBattery := false
	for _, m := range msgs {
		if strings.Contains(m, "🪫 **แบตเตอรี่ Chubu เหลือ 10%**") {
			foundBattery = true
			break
		}
	}
	if !foundBattery {
		t.Errorf("missing low battery alert for Chubu: %+v", msgs)
	}
}
