package location

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"v11-server/shared/db"
)

func openLocationTestStore(t *testing.T) (*sql.DB, *Store) {
	t.Helper()
	database, err := db.InitDB(filepath.Join(t.TempDir(), "location.db"))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database, Credentials{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return database, store
}

func samplePoint(device string, at time.Time, lat float64) LocationPoint {
	return LocationPoint{
		DeviceID: device, Latitude: lat, Longitude: 100.5, Accuracy: 8,
		Speed: 2.5, Altitude: 17, Course: 90, Battery: 61,
		BatteryStatus: 1, RecordedAt: at,
	}
}

func boolPtr(value bool) *bool { return &value }

func TestNewStoreGeneratesAndPersistsLocationTokens(t *testing.T) {
	database, store := openLocationTestStore(t)
	secret, viewer := store.Tokens()
	if len(secret) != 32 || len(viewer) != 32 || secret == viewer {
		t.Fatalf("generated tokens must be distinct 32-character values: secret=%q viewer=%q", secret, viewer)
	}

	second, err := NewStore(database, Credentials{})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	gotSecret, gotViewer := second.Tokens()
	if gotSecret != secret || gotViewer != viewer {
		t.Fatalf("tokens changed across store initialization: (%q, %q) != (%q, %q)", gotSecret, gotViewer, secret, viewer)
	}
}

func TestNewStoreUsesExplicitEnvironmentTokens(t *testing.T) {
	database, _ := openLocationTestStore(t)
	wantSecret := strings.Repeat("s", 32)
	wantViewer := strings.Repeat("v", 32)
	store, err := NewStore(database, Credentials{
		SecretToken: wantSecret, ViewerToken: wantViewer,
		SecretExplicit: true, ViewerExplicit: true,
	})
	if err != nil {
		t.Fatalf("NewStore with credentials: %v", err)
	}
	gotSecret, gotViewer := store.Tokens()
	if gotSecret != wantSecret || gotViewer != wantViewer {
		t.Fatalf("tokens = (%q, %q), want explicit credentials", gotSecret, gotViewer)
	}
}

func TestStorePointLatestBreadcrumbsAndRetention(t *testing.T) {
	_, store := openLocationTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for _, point := range []LocationPoint{
		samplePoint(DeviceMe, now.Add(-8*24*time.Hour), 13.70),
		samplePoint(DeviceMe, now.Add(-2*time.Hour), 13.71),
		samplePoint(DeviceMe, now.Add(-time.Hour), 13.72),
		samplePoint(DeviceFriend, now.Add(-30*time.Minute), 13.73),
	} {
		if err := store.SavePoint(ctx, point); err != nil {
			t.Fatalf("SavePoint: %v", err)
		}
	}

	latest, err := store.LatestPoints(ctx)
	if err != nil {
		t.Fatalf("LatestPoints: %v", err)
	}
	if latest[DeviceMe] == nil || latest[DeviceMe].Latitude != 13.72 || latest[DeviceFriend] == nil {
		t.Fatalf("unexpected latest points: %+v", latest)
	}

	trails, err := store.Breadcrumbs(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("Breadcrumbs: %v", err)
	}
	if len(trails[DeviceMe]) != 2 || trails[DeviceMe][0].Latitude != 13.71 || trails[DeviceMe][1].Latitude != 13.72 {
		t.Fatalf("breadcrumbs should be chronological and limited to 24h: %+v", trails[DeviceMe])
	}
	deleted, err := store.PruneOld(ctx, now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("PruneOld: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("PruneOld deleted %d points, want 1", deleted)
	}
	latest, err = store.LatestPoints(ctx)
	if err != nil || latest[DeviceMe].Latitude != 13.72 {
		t.Fatalf("latest after retention prune = %+v, err=%v", latest, err)
	}
}

func TestStoreRejectsInvalidLocationPoint(t *testing.T) {
	_, store := openLocationTestStore(t)
	point := samplePoint(DeviceMe, time.Now(), 91)
	if err := store.SavePoint(context.Background(), point); err == nil {
		t.Fatal("SavePoint accepted invalid latitude")
	}
	point = samplePoint("unknown", time.Now(), 13.7)
	if err := store.SavePoint(context.Background(), point); err == nil {
		t.Fatal("SavePoint accepted unknown device")
	}
}

func TestStoreGeofencesAndStates(t *testing.T) {
	_, store := openLocationTestStore(t)
	ctx := context.Background()
	created, err := store.CreateGeofence(ctx, GeofenceInput{
		Name: "บ้านเพื่อน", Latitude: 13.7, Longitude: 100.5, Radius: 150,
		NotifyEnter: boolPtr(true), NotifyExit: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("CreateGeofence: %v", err)
	}
	zones, err := store.ListGeofences(ctx)
	if err != nil || len(zones) != 1 || zones[0].ID != created.ID {
		t.Fatalf("ListGeofences = %+v, err=%v", zones, err)
	}

	state := GeofenceState{DeviceID: DeviceFriend, GeofenceID: created.ID, IsInside: true, LastTransition: time.Now().UTC()}
	if err := store.SetGeofenceState(ctx, state); err != nil {
		t.Fatalf("SetGeofenceState: %v", err)
	}
	got, err := store.GetGeofenceState(ctx, DeviceFriend, created.ID)
	if err != nil || !got.IsInside {
		t.Fatalf("GetGeofenceState = %+v, err=%v", got, err)
	}
	deleted, err := store.DeleteGeofence(ctx, created.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteGeofence = %v, err=%v", deleted, err)
	}
	if _, err := store.GetGeofenceState(ctx, DeviceFriend, created.ID); err == nil {
		t.Fatal("geofence state remains after geofence cascade delete")
	}
}

func TestStoreSettingsPatchAndCoLocationState(t *testing.T) {
	_, store := openLocationTestStore(t)
	ctx := context.Background()
	enabled := false
	settings, err := store.UpdateSettings(ctx, SettingsInput{NotifyBattery: &enabled})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if settings.NotifyBattery || !settings.NotifyCoLocation || !settings.NotifyGeofence {
		t.Fatalf("unexpected partial settings update: %+v", settings)
	}

	state := CoLocationState{IsCoLocated: true, LastDistance: 42, LastTransitionAt: time.Now().UTC(), LastNotifiedAt: time.Now().UTC()}
	if err := store.SetCoLocationState(ctx, state); err != nil {
		t.Fatalf("SetCoLocationState: %v", err)
	}
	got, err := store.GetCoLocationState(ctx)
	if err != nil || !got.IsCoLocated || got.LastDistance != 42 {
		t.Fatalf("GetCoLocationState = %+v, err=%v", got, err)
	}
}
