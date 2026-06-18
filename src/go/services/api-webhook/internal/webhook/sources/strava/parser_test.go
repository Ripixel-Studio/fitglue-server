package strava

import (
	"encoding/json"
	"testing"
	"time"

	stravaapi "github.com/fitglue/server/src/go/pkg/api/strava"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func TestMapActivityType(t *testing.T) {
	if got := mapActivityType(stravaapi.ActivityTypeRun); got != activitypb.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("Run -> %v", got)
	}
	if got := mapActivityType(stravaapi.ActivityTypeRide); got != activitypb.ActivityType_ACTIVITY_TYPE_RIDE {
		t.Errorf("Ride -> %v", got)
	}
	if got := mapActivityType(stravaapi.ActivityType("Nonexistent")); got != activitypb.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		t.Errorf("unknown -> %v, want UNSPECIFIED", got)
	}
}

func TestMapToStandardizedActivity_InvalidJSON(t *testing.T) {
	if _, err := mapToStandardizedActivity([]byte("{not json"), "u1", nil); err == nil {
		t.Error("expected unmarshal error")
	}
}

func TestMapToStandardizedActivity_SummaryLaps(t *testing.T) {
	raw := []byte(`{
		"id": 12345,
		"name": "Morning Run",
		"description": "easy",
		"type": "Run",
		"distance": 5000,
		"elapsed_time": 1800,
		"calories": 320,
		"start_date": "2024-01-01T08:00:00Z",
		"laps": [
			{"elapsed_time": 900, "distance": 2500, "start_date": "2024-01-01T08:00:00Z"},
			{"elapsed_time": 900, "distance": 2500, "start_date": "2024-01-01T08:15:00Z"}
		]
	}`)
	act, err := mapToStandardizedActivity(raw, "user-1", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if act.UserId != "user-1" || act.ExternalId != "12345" || act.Name != "Morning Run" {
		t.Errorf("unexpected header: %+v", act)
	}
	if act.Type != activitypb.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("type = %v", act.Type)
	}
	if len(act.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(act.Sessions))
	}
	s := act.Sessions[0]
	if s.TotalDistance != 5000 || s.TotalElapsedTime != 1800 {
		t.Errorf("session totals: %+v", s)
	}
	if s.TotalCalories == nil || *s.TotalCalories != 320 {
		t.Errorf("calories: %v", s.TotalCalories)
	}
	// No streams -> summary laps fallback.
	if len(s.Laps) != 2 {
		t.Errorf("expected 2 summary laps, got %d", len(s.Laps))
	}
}

func TestMapToStandardizedActivity_WithStreams(t *testing.T) {
	raw := []byte(`{"id":1,"name":"Run","type":"Run","start_date":"2024-01-01T08:00:00Z","elapsed_time":3}`)

	var streams stravaapi.StreamSet
	streamJSON := []byte(`{
		"time": {"data": [0, 1, 2]},
		"heartrate": {"data": [100, 110, 120]},
		"velocity_smooth": {"data": [3.0, 3.1, 3.2]},
		"altitude": {"data": [10.0, 11.0, 12.0]},
		"latlng": {"data": [[51.5, -0.1], [51.6, -0.2], [51.7, -0.3]]},
		"cadence": {"data": [80, 81, 82]},
		"watts": {"data": [200, 210, 220]},
		"distance": {"data": [0.0, 3.0, 6.0]},
		"temp": {"data": [15, 16, 17]}
	}`)
	if err := json.Unmarshal(streamJSON, &streams); err != nil {
		t.Fatalf("stream unmarshal: %v", err)
	}

	act, err := mapToStandardizedActivity(raw, "u1", &streams)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := act.Sessions[0]
	if len(s.Laps) != 1 || !s.Laps[0].IsTelemetryContainerOnly {
		t.Fatalf("expected a single telemetry lap, got %+v", s.Laps)
	}
	recs := s.Laps[0].Records
	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}
	if recs[0].HeartRate != 100 || recs[2].HeartRate != 120 {
		t.Errorf("heart rate mapping wrong: %+v", recs)
	}
	if recs[1].Cadence != 81 || recs[1].Power != 210 {
		t.Errorf("cadence/power mapping wrong: %+v", recs[1])
	}
	if recs[0].PositionLat != 51.5 {
		t.Errorf("latlng mapping wrong: %+v", recs[0])
	}
	if recs[0].Temperature == nil || *recs[0].Temperature != 15 {
		t.Errorf("temp mapping wrong: %+v", recs[0].Temperature)
	}
	// avg HR = (100+110+120)/3 = 110, max = 120
	if s.AvgHeartRate == nil || *s.AvgHeartRate != 110 || s.MaxHeartRate == nil || *s.MaxHeartRate != 120 {
		t.Errorf("session HR wrong: avg=%v max=%v", s.AvgHeartRate, s.MaxHeartRate)
	}
}

func TestComputeSessionHR_NoData(t *testing.T) {
	if _, _, ok := computeSessionHR(nil); ok {
		t.Error("expected ok=false for nil streams")
	}
	var empty stravaapi.StreamSet
	if _, _, ok := computeSessionHR(&empty); ok {
		t.Error("expected ok=false for empty streams")
	}
}

func TestBuildRecordsFromStreams_Nil(t *testing.T) {
	if recs := buildRecordsFromStreams(nil, time.Time{}); recs != nil {
		t.Error("expected nil records for nil streams")
	}
}
