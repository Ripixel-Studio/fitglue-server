package sourceplugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The orchestrator requires ActivityPayload.StandardizedActivity; the Strava plugin
// used to return only original_payload_json, so every Strava historical import died
// with "standardized activity is nil". This pins the fix: detail + streams → records.
func TestStravaFetchActivity_PopulatesStandardizedActivity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/activities/123/streams"):
			_, _ = w.Write([]byte(`{"time":{"data":[0,1,2]},"heartrate":{"data":[120,130,140]},"distance":{"data":[0,3,6]}}`))
		case strings.HasSuffix(r.URL.Path, "/activities/123"):
			_, _ = w.Write([]byte(`{"id":123,"name":"Morning Run","sport_type":"Run","start_date":"2026-01-05T08:00:00Z","elapsed_time":3,"distance":6.0,"description":"nice"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	prev := stravaAPIBase
	stravaAPIBase = srv.URL
	defer func() { stravaAPIBase = prev }()

	p, _ := ForSource("SOURCE_STRAVA")
	payload, err := p.FetchActivity(context.Background(), configuredIntegrations(), "user-1", "123")
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}
	act := payload.StandardizedActivity
	if act == nil {
		t.Fatal("StandardizedActivity is nil")
	}
	if act.Name != "Morning Run" || act.ExternalId != "123" || act.Description != "nice" {
		t.Errorf("summary not mapped: %+v", act)
	}
	if len(act.Sessions) != 1 || act.Sessions[0].TotalElapsedTime != 3 {
		t.Fatalf("expected one session with elapsed 3, got %+v", act.Sessions)
	}
	recs := 0
	for _, l := range act.Sessions[0].Laps {
		recs += len(l.Records)
	}
	if recs != 3 {
		t.Errorf("expected 3 stream records, got %d", recs)
	}
	if act.Sessions[0].AvgHeartRate == nil || *act.Sessions[0].AvgHeartRate != 130 {
		t.Errorf("avg HR not derived from streams: %v", act.Sessions[0].AvgHeartRate)
	}
	if payload.Timestamp == nil || !payload.Timestamp.AsTime().Equal(act.StartTime.AsTime()) {
		t.Errorf("payload timestamp should be the activity start")
	}
}

func TestStravaFetchActivity_NoStreamsStillMaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/streams") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Manual","type":"Workout","start_date":"2026-01-05T08:00:00Z","elapsed_time":600}`))
	}))
	defer srv.Close()
	prev := stravaAPIBase
	stravaAPIBase = srv.URL
	defer func() { stravaAPIBase = prev }()

	p, _ := ForSource("SOURCE_STRAVA")
	payload, err := p.FetchActivity(context.Background(), configuredIntegrations(), "user-1", "7")
	if err != nil {
		t.Fatalf("FetchActivity: %v", err)
	}
	if payload.StandardizedActivity == nil || payload.StandardizedActivity.Sessions[0].TotalElapsedTime != 600 {
		t.Fatalf("manual activity without streams should still map: %+v", payload.StandardizedActivity)
	}
}
