package location

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStoreUpdateGeofence(t *testing.T) {
	_, store := openLocationTestStore(t)
	ctx := context.Background()
	zone, err := store.CreateGeofence(ctx, GeofenceInput{Name: "บ้าน", Latitude: 13.7, Longitude: 100.5, Radius: 150, NotifyExit: boolPtr(false)})
	if err != nil {
		t.Fatal(err)
	}
	updated, found, err := store.UpdateGeofence(ctx, zone.ID, GeofenceInput{Name: " ที่ทำงาน ", Latitude: 13.8, Longitude: 100.6, Radius: 300})
	if err != nil || !found {
		t.Fatalf("update = %+v found=%v err=%v", updated, found, err)
	}
	if updated.Name != "ที่ทำงาน" || updated.Latitude != 13.8 || updated.Longitude != 100.6 || updated.Radius != 300 {
		t.Fatalf("unexpected updated values: %+v", updated)
	}
	if !updated.NotifyEnter || updated.NotifyExit {
		t.Fatalf("nil notify flags must keep stored values, got enter=%v exit=%v", updated.NotifyEnter, updated.NotifyExit)
	}
	zones, err := store.ListGeofences(ctx)
	if err != nil || len(zones) != 1 || zones[0].Name != "ที่ทำงาน" || zones[0].Radius != 300 {
		t.Fatalf("listed = %+v err=%v", zones, err)
	}
	if _, found, err := store.UpdateGeofence(ctx, zone.ID+99, GeofenceInput{Name: "x", Latitude: 1, Longitude: 1, Radius: 10}); err != nil || found {
		t.Fatalf("missing id: found=%v err=%v", found, err)
	}
	if _, _, err := store.UpdateGeofence(ctx, zone.ID, GeofenceInput{Name: "x", Latitude: 1, Longitude: 1, Radius: 0}); err == nil {
		t.Fatal("radius 0 must be rejected")
	}
}

func TestUpdateGeofenceRebaselinesSilently(t *testing.T) {
	_, store := openLocationTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	service := NewService(store, nil, WithClock(func() time.Time { return now }))
	zone, err := store.CreateGeofence(ctx, GeofenceInput{Name: "บ้าน", Latitude: 0, Longitude: 0, Radius: 100})
	if err != nil {
		t.Fatal(err)
	}
	point := samplePoint(DeviceFriend, now, 0)
	point.Longitude = 0
	if events, err := service.ProcessPoint(ctx, point); err != nil || len(events) != 1 || events[0].Kind != EventGeofenceEnter {
		t.Fatalf("setup enter events = %+v err=%v", events, err)
	}

	// Move the place 5 km away: the friend did not move, so no exit may fire.
	if _, found, err := service.UpdateGeofence(ctx, zone.ID, GeofenceInput{Name: "บ้าน", Latitude: metersToLatitude(5000), Longitude: 0, Radius: 100}); err != nil || !found {
		t.Fatalf("update err=%v found=%v", err, found)
	}
	state, err := store.GetGeofenceState(ctx, DeviceFriend, zone.ID)
	if err != nil || state.IsInside {
		t.Fatalf("state after moving away = %+v err=%v, want outside", state, err)
	}
	now = now.Add(time.Minute)
	point.RecordedAt = now
	if events, err := service.ProcessPoint(ctx, point); err != nil || len(events) != 0 {
		t.Fatalf("events after moving place away = %+v err=%v, want none", events, err)
	}

	// Move the place back onto the friend: no enter may fire either.
	if _, _, err := service.UpdateGeofence(ctx, zone.ID, GeofenceInput{Name: "บ้าน", Latitude: 0, Longitude: 0, Radius: 100}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	point.RecordedAt = now
	if events, err := service.ProcessPoint(ctx, point); err != nil || len(events) != 0 {
		t.Fatalf("events after moving place onto friend = %+v err=%v, want none", events, err)
	}
}

func TestUpdateGeofenceRebaselinesWithoutPoints(t *testing.T) {
	_, store := openLocationTestStore(t)
	ctx := context.Background()
	service := NewService(store, nil)
	zone, err := store.CreateGeofence(ctx, GeofenceInput{Name: "บ้าน", Latitude: 13.7, Longitude: 100.5, Radius: 150})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := service.UpdateGeofence(ctx, zone.ID, GeofenceInput{Name: "บ้าน", Latitude: 13.71, Longitude: 100.5, Radius: 150}); err != nil || !found {
		t.Fatalf("update without points: found=%v err=%v", found, err)
	}
}

func TestUpdateGeofenceEndpoint(t *testing.T) {
	router, module, _, _ := newLocationTestRouter(t)
	zone, err := module.store.CreateGeofence(context.Background(), GeofenceInput{Name: "บ้าน", Latitude: 13.7, Longitude: 100.5, Radius: 150})
	if err != nil {
		t.Fatal(err)
	}
	put := func(pin, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Relay-PIN", pin)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	path := fmt.Sprintf("/api/location/geofences/%d", zone.ID)
	good := `{"name":"คอนโด","latitude":13.75,"longitude":100.55,"radius_meters":200}`
	if rr := put("wrong", path, good); rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong PIN = %d", rr.Code)
	}
	if rr := put("1804", path, good); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "คอนโด") {
		t.Fatalf("update = %d %s", rr.Code, rr.Body.String())
	}
	if rr := put("1804", fmt.Sprintf("/api/location/geofences/%d", zone.ID+50), good); rr.Code != http.StatusNotFound {
		t.Fatalf("missing id = %d", rr.Code)
	}
	if rr := put("1804", path, `{"name":`); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json = %d", rr.Code)
	}
	if rr := put("1804", "/api/location/geofences/abc", good); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad id = %d", rr.Code)
	}
}
