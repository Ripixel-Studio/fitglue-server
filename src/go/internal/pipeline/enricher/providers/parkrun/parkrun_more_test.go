package parkrun

import (
	"context"
	"io"
	"log/slog"
	"strings"
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

// Structured manual entry (Option B): the form submits structured fields but NO freeform
// description. EnrichResume must synthesize the description server-side so the manual entry
// renders the same formatted card as an auto-fetched one, and populate the ParkrunSummary
// that drives the web card.
func TestParkrun_EnrichResume_StructuredManual_Full(t *testing.T) {
	p := NewParkrunProvider()
	pending := &pbpipeline.PendingInput{
		InputData: map[string]string{
			// note: no "description" — the structured form doesn't collect freeform text
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
	// Description synthesized from structured fields, matching the auto-fetched layout.
	for _, want := range []string{
		"🏃 Parkrun Results:",
		"Position: 42nd",
		"Time: 25:30 · 🏆 New all-time PB!",
		"Age Grade: 55.5%",
		"Location: Bushy Park Parkrun (137 total)",
	} {
		if !strings.Contains(res.Description, want) {
			t.Errorf("synthesized description missing %q\nfull:\n%s", want, res.Description)
		}
	}
	// age grade is NOT a PB here — no badge on that line
	if strings.Contains(res.Description, "Age Grade: 55.5% · 🏆") {
		t.Errorf("age grade should have no PB badge, got:\n%s", res.Description)
	}
	pr := res.Enrichments.Parkrun
	if pr.EventName != "Bushy Park Parkrun" || pr.Position != 42 || pr.FinishTime != "25:30" || pr.TotalParkruns != 137 || !pr.IsTimePb || pr.IsAgeGradePb {
		t.Errorf("unexpected ParkrunSummary %+v", pr)
	}
}

// A partial manual entry has only some fields — total_parkruns / PB flags absent. They
// must default cleanly to 0/false, the synthesized description must not render a bogus
// "(0 total)", and the card can omit the "TOTAL RUNS" tile instead of showing a zero.
func TestParkrun_EnrichResume_StructuredManual_Partial(t *testing.T) {
	p := NewParkrunProvider()
	pending := &pbpipeline.PendingInput{
		InputData: map[string]string{
			"time": "25:30", // only the mandatory field supplied
		},
		ProviderMetadata: map[string]string{"parkrun_event_name": "Bushy Park Parkrun"},
	}
	res, err := p.EnrichResume(context.Background(), &pbactivity.StandardizedActivity{}, &user.Record{}, pending)
	if err != nil {
		t.Fatalf("EnrichResume error: %v", err)
	}
	pr := res.Enrichments.Parkrun
	if pr.TotalParkruns != 0 || pr.IsTimePb || pr.IsAgeGradePb {
		t.Errorf("expected zero-value stats for partial entry, got total=%d timePB=%v agPB=%v",
			pr.TotalParkruns, pr.IsTimePb, pr.IsAgeGradePb)
	}
	if res.Metadata["parkrun_total_parkruns"] != "0" {
		t.Errorf("expected parkrun_total_parkruns=0, got %q", res.Metadata["parkrun_total_parkruns"])
	}
	if !strings.Contains(res.Description, "Time: 25:30") {
		t.Errorf("expected synthesized time line, got:\n%s", res.Description)
	}
	if strings.Contains(res.Description, "total") {
		t.Errorf("blank total must not render a bogus count, got:\n%s", res.Description)
	}
}

// The auto-resolve path (parkrun_checker) still supplies a pre-built freeform description
// in InputData. EnrichResume must keep using it verbatim rather than overwriting it with a
// synthesized one, so this path is unchanged from PR #24.
func TestParkrun_EnrichResume_AutoDescriptionPreserved(t *testing.T) {
	p := NewParkrunProvider()
	pending := &pbpipeline.PendingInput{
		InputData: map[string]string{
			"description":    "🏃 Parkrun Results:\n• Location: Newark, 5th Parkrun here (37 total)",
			"time":           "25:30",
			"total_parkruns": "37",
		},
		ProviderMetadata: map[string]string{"parkrun_event_name": "Newark Parkrun"},
	}
	res, err := p.EnrichResume(context.Background(), &pbactivity.StandardizedActivity{}, &user.Record{}, pending)
	if err != nil {
		t.Fatalf("EnrichResume error: %v", err)
	}
	if !strings.Contains(res.Description, "5th Parkrun here") {
		t.Errorf("auto-supplied description should be preserved verbatim, got:\n%s", res.Description)
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
