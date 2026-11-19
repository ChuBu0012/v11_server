package location

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLocationReportDecodesOwnTracksFields(t *testing.T) {
	const payload = `{"_type":"location","lat":13.7462,"lon":100.5348,"acc":15,"tst":1700394000,"batt":45,"bs":1,"vel":24,"alt":18,"cog":180}`
	var report LocationReport
	if err := json.Unmarshal([]byte(payload), &report); err != nil {
		t.Fatalf("decode OwnTracks report: %v", err)
	}
	if report.Type != "location" || report.Latitude != 13.7462 || report.Longitude != 100.5348 {
		t.Fatalf("decoded location fields = %+v", report)
	}
	if report.Accuracy != 15 || report.Timestamp != 1700394000 || report.Battery == nil || *report.Battery != 45 || report.BatteryStatus != 1 {
		t.Fatalf("decoded telemetry fields = %+v", report)
	}
	if report.Speed != 24 || report.Altitude != 18 || report.Course != 180 {
		t.Fatalf("decoded motion fields = %+v", report)
	}
}

func TestLocationReportPreservesMissingBatteryAsUnavailable(t *testing.T) {
	var report LocationReport
	if err := json.Unmarshal([]byte(`{"_type":"location","lat":0,"lon":0,"acc":5,"tst":1700394000}`), &report); err != nil {
		t.Fatal(err)
	}
	if report.Battery != nil {
		t.Fatalf("missing battery should remain unavailable, got %d", *report.Battery)
	}
}

func TestLocationSettingsJSONDoesNotExposeTokens(t *testing.T) {
	data, err := json.Marshal(LocationSettings{SecretToken: "secret-value", ViewerToken: "viewer-value"})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if strings.Contains(string(data), "secret-value") || strings.Contains(string(data), "viewer-value") {
		t.Fatalf("serialized settings leaked a credential: %s", data)
	}
}
