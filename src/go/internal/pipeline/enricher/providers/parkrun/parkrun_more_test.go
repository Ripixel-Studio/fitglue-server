package parkrun

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestParkrun_NameAndType(t *testing.T) {
	p := NewParkrunProvider()
	if p.Name() != "parkrun" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PARKRUN {
		t.Errorf("unexpected provider type %v", p.ProviderType())
	}
}

func TestGetStartLocation(t *testing.T) {
	// No sessions.
	if _, _, found := getStartLocation(&pbactivity.StandardizedActivity{}); found {
		t.Error("expected not found for empty activity")
	}

	// Sessions but no GPS.
	noGPS := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{{}}}}}},
	}
	if _, _, found := getStartLocation(noGPS); found {
		t.Error("expected not found when all records are zero")
	}

	// Valid GPS in second record.
	withGPS := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{
			{},
			{PositionLat: 51.4106, PositionLong: -0.3421},
		}}}}},
	}
	lat, long, found := getStartLocation(withGPS)
	if !found || lat != 51.4106 || long != -0.3421 {
		t.Errorf("getStartLocation = (%v,%v,%v)", lat, long, found)
	}
}

func TestDistanceMeters(t *testing.T) {
	// Same point → ~0.
	if d := distanceMeters(51.4106, -0.3421, 51.4106, -0.3421); d > 0.001 {
		t.Errorf("distance for same point = %v, want ~0", d)
	}
	// Known approximate distance: 1 degree latitude ≈ 111km.
	d := distanceMeters(0, 0, 1, 0)
	if d < 110000 || d > 112000 {
		t.Errorf("distance for 1 deg lat = %v, want ~111km", d)
	}
}

func TestResolveTimezone(t *testing.T) {
	cases := []struct {
		country string
		lng     float64
		want    string
	}{
		{"www.parkrun.org.uk", -0.3, "Europe/London"},
		{"www.parkrun.ie", -6, "Europe/Dublin"},
		{"www.parkrun.jp", 139, "Asia/Tokyo"},
		{"www.parkrun.com.au", 115, "Australia/Perth"},
		{"www.parkrun.com.au", 151, "Australia/Sydney"},
		{"www.parkrun.us", -74, "America/New_York"},
		{"www.parkrun.us", -120, "America/Los_Angeles"},
		{"www.parkrun.ca", -73, "America/Toronto"},
		{"www.parkrun.ca", -79, "America/Winnipeg"},
		{"www.parkrun.unknown", 0, ""},
	}
	for _, c := range cases {
		got := resolveTimezone(ParkrunLocation{CountryURL: c.country, Longitude: c.lng})
		if got != c.want {
			t.Errorf("resolveTimezone(%s,%v) = %q, want %q", c.country, c.lng, got, c.want)
		}
	}
}

func TestLocalTimeForLocation(t *testing.T) {
	utc := time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC)
	// UK in December = GMT (UTC+0).
	uk := localTimeForLocation(utc, ParkrunLocation{CountryURL: "www.parkrun.org.uk"})
	if uk.Hour() != 9 {
		t.Errorf("UK local hour = %d, want 9", uk.Hour())
	}
	// Unknown country falls back to longitude offset (+15deg ≈ +1h).
	fallback := localTimeForLocation(utc, ParkrunLocation{CountryURL: "x", Longitude: 15})
	if fallback.Hour() != 10 {
		t.Errorf("longitude fallback hour = %d, want 10", fallback.Hour())
	}
}

func TestParkrun_EnrichResume(t *testing.T) {
	p := NewParkrunProvider()
	// Auto-resolved input: parkrun_checker carries the full stats (total run count +
	// PB flags) in InputData so EnrichResume can rebuild the complete summary card.
	pending := &pbpipeline.PendingInput{
		InputData: map[string]string{
			"description":     "🏃 Parkrun Results:\nGreat run!",
			"position":        "42",
			"time":            "25:30",
			"age_grade":       "55.5%",
			"total_parkruns":  "137",
			"is_time_pb":      "true",
			"is_age_grade_pb": "false",
		},
		ProviderMetadata: map[string]string{"parkrun_event_name": "Bushy Park Parkrun"},
	}
	res, err := p.EnrichResume(context.Background(), &pbactivity.StandardizedActivity{}, &user.Record{}, pending)
	if err != nil {
		t.Fatalf("EnrichResume error: %v", err)
	}
	if res.SectionHeader != "🏃 Parkrun Results:" {
		t.Errorf("unexpected section header %q", res.SectionHeader)
	}
	if res.Metadata["parkrun_position"] != "42" || res.Metadata["parkrun_results_state"] != "COMPLETE" {
		t.Errorf("unexpected metadata %v", res.Metadata)
	}
	if res.Metadata["parkrun_total_parkruns"] != "137" {
		t.Errorf("expected parkrun_total_parkruns=137, got %q", res.Metadata["parkrun_total_parkruns"])
	}
	if res.Enrichments == nil || res.Enrichments.Parkrun == nil {
		t.Fatal("expected Parkrun enrichment")
	}
	pr := res.Enrichments.Parkrun
	if pr.Position != 42 || pr.EventName != "Bushy Park Parkrun" {
		t.Errorf("unexpected parkrun summary %+v", pr)
	}
	if pr.TotalParkruns != 137 {
		t.Errorf("expected TotalParkruns=137, got %d", pr.TotalParkruns)
	}
	if !pr.IsTimePb || pr.IsAgeGradePb {
		t.Errorf("expected IsTimePb=true IsAgeGradePb=false, got %v/%v", pr.IsTimePb, pr.IsAgeGradePb)
	}
}

// A genuinely manual entry has no total_parkruns / PB flags in InputData (the fetch
// never resolved). Those must default cleanly to 0/false rather than erroring, so the
// card can omit the "TOTAL RUNS" tile instead of showing a bogus zero.
func TestParkrun_EnrichResume_ManualEntryNoStats(t *testing.T) {
	p := NewParkrunProvider()
	pending := &pbpipeline.PendingInput{
		InputData: map[string]string{
			"description": "Ran it, felt good",
			"position":    "42",
			"time":        "25:30",
			"age_grade":   "55.5%",
		},
		ProviderMetadata: map[string]string{"parkrun_event_name": "Bushy Park Parkrun"},
	}
	res, err := p.EnrichResume(context.Background(), &pbactivity.StandardizedActivity{}, &user.Record{}, pending)
	if err != nil {
		t.Fatalf("EnrichResume error: %v", err)
	}
	pr := res.Enrichments.Parkrun
	if pr.TotalParkruns != 0 || pr.IsTimePb || pr.IsAgeGradePb {
		t.Errorf("expected zero-value stats for manual entry, got total=%d timePB=%v agPB=%v",
			pr.TotalParkruns, pr.IsTimePb, pr.IsAgeGradePb)
	}
	if res.Metadata["parkrun_total_parkruns"] != "0" {
		t.Errorf("expected parkrun_total_parkruns=0, got %q", res.Metadata["parkrun_total_parkruns"])
	}
}

func TestParkrun_Enrich_NotRunActivity(t *testing.T) {
	p := NewParkrunProviderWithService(createMockLocationsService())
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		StartTime: timestamppb.New(time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC)),
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, nil, map[string]string{}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "skipped" || res.Metadata["reason"] != "not_run_activity_type" {
		t.Errorf("expected not_run_activity_type skip, got %v", res.Metadata)
	}
}

func TestParkrun_Enrich_NoGPS(t *testing.T) {
	p := NewParkrunProviderWithService(createMockLocationsService())
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC)),
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, nil, map[string]string{}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "skipped" || res.Metadata["reason"] != "no_gps_data" {
		t.Errorf("expected no_gps_data skip, got %v", res.Metadata)
	}
}

func TestParkrun_Enrich_DisabledResultsState(t *testing.T) {
	// A genuine parkrun match but the user has no parkrun integration → DISABLED state.
	p := NewParkrunProviderWithService(createMockLocationsService())
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC)),
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{{PositionLat: 51.4106, PositionLong: -0.3421}}}}},
		},
	}
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, map[string]string{}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "success" || res.Metadata["is_parkrun"] != "true" {
		t.Fatalf("expected parkrun match, got %v", res.Metadata)
	}
	if res.Metadata["parkrun_results_state"] != "DISABLED" {
		t.Errorf("expected DISABLED results state, got %q", res.Metadata["parkrun_results_state"])
	}
	if res.Metadata["debug_integration_nil"] != "true" {
		t.Errorf("expected debug_integration_nil flag, got %v", res.Metadata)
	}
}

func TestParkrun_Enrich_FetchResultsDisabled(t *testing.T) {
	p := NewParkrunProviderWithService(createMockLocationsService())
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC)),
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{{PositionLat: 51.4106, PositionLong: -0.3421}}}}},
		},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, nil, map[string]string{"fetch_results": "false"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["parkrun_results_state"] != "IMMEDIATE" {
		t.Errorf("expected IMMEDIATE when fetch disabled, got %q", res.Metadata["parkrun_results_state"])
	}
}
