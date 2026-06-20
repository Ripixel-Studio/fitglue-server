package pace_summary

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPaceSummary_Enrich_Success(t *testing.T) {
	provider := NewPaceSummary()
	provider.Service = &bootstrap.Service{}

	// Create activity with speed data (5 m/s = 3:20/km, 4 m/s = 4:10/km, 6 m/s = 2:47/km)
	activity := &pbactivity.StandardizedActivity{
		StartTime:   timestamppb.New(time.Now()),
		Description: "Morning Run",
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 3600,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{Speed: 4.0}, // 4:10/km
							{Speed: 5.0}, // 3:20/km
							{Speed: 5.0}, // 3:20/km
							{Speed: 6.0}, // 2:47/km (best)
							{Speed: 5.0}, // 3:20/km
						},
					},
				},
			},
		},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Verify metadata
	if result.Metadata["pace_summary_status"] != "success" {
		t.Errorf("Expected pace_summary_status=success, got %s", result.Metadata["pace_summary_status"])
	}

	// Verify best pace (6 m/s = ~2:47/km)
	if result.Metadata["pace_best"] != "2:46" && result.Metadata["pace_best"] != "2:47" {
		t.Errorf("Expected pace_best around 2:46-2:47, got %s", result.Metadata["pace_best"])
	}

	// Verify description is appended
	if result.Description == "" {
		t.Error("Expected non-empty description")
	}
	if result.Description == "Morning Run" {
		t.Error("Expected description to be appended with pace summary")
	}
}

func TestPaceSummary_Enrich_NoSpeedData(t *testing.T) {
	provider := NewPaceSummary()
	provider.Service = &bootstrap.Service{}

	// Create activity without speed data
	activity := &pbactivity.StandardizedActivity{
		StartTime:   timestamppb.New(time.Now()),
		Description: "Strength Workout",
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 3600,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{Speed: 0},
							{Speed: 0},
						},
					},
				},
			},
		},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if result.Metadata["pace_summary_status"] != "skipped" {
		t.Errorf("Expected pace_summary_status=skipped, got %s", result.Metadata["pace_summary_status"])
	}
}

func TestPaceSummary_Name(t *testing.T) {
	provider := NewPaceSummary()
	expected := "pace-summary"
	if provider.Name() != expected {
		t.Errorf("Expected provider name %q, got %q", expected, provider.Name())
	}
}

func TestPaceSummary_ProviderType(t *testing.T) {
	provider := NewPaceSummary()
	expected := pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PACE_SUMMARY
	if provider.ProviderType() != expected {
		t.Errorf("Expected provider type %v, got %v", expected, provider.ProviderType())
	}
}

func TestFormatPace(t *testing.T) {
	tests := []struct {
		paceMinutes float64
		expected    string
	}{
		{5.5, "5:30"},
		{4.0, "4:00"},
		{3.333, "3:19"},
		{6.75, "6:45"},
	}

	for _, tt := range tests {
		result := formatPace(tt.paceMinutes)
		if result != tt.expected {
			t.Errorf("formatPace(%.3f) = %s, want %s", tt.paceMinutes, result, tt.expected)
		}
	}
}

// rec builds a record at a given offset from start with a cumulative distance.
func rec(start time.Time, offsetSec int, distance, speed float64) *pbactivity.Record {
	return &pbactivity.Record{
		Timestamp: timestamppb.New(start.Add(time.Duration(offsetSec) * time.Second)),
		Distance:  distance,
		Speed:     speed,
	}
}

// TestPaceSummary_SplitsFromRecords covers the core fix: a run delivered as a
// single lap (as Strava does) must produce real, varying per-km splits derived
// from the distance/time stream — not the overall average repeated per km — and
// the "best split" must be the fastest actual kilometre, not the fastest noisy
// instantaneous GPS sample.
func TestPaceSummary_SplitsFromRecords(t *testing.T) {
	provider := NewPaceSummary()
	provider.Service = &bootstrap.Service{}
	start := time.Now()

	// One lap covering 3 km: km1 = 300s (5:00), km2 = 240s (4:00, fastest),
	// km3 = 360s (6:00, slowest). Includes a 10 m/s spike that would falsely
	// read as ~1:40/km if best split were taken from instantaneous speed.
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions: []*pbactivity.Session{
			{
				StartTime:        timestamppb.New(start),
				TotalElapsedTime: 900,
				TotalDistance:    3000,
				Laps: []*pbactivity.Lap{
					{
						StartTime:        timestamppb.New(start),
						TotalElapsedTime: 900,
						TotalDistance:    3000,
						Records: []*pbactivity.Record{
							rec(start, 0, 0, 3.0),
							rec(start, 150, 500, 3.3),
							rec(start, 300, 1000, 3.3),  // end km1 @ 300s
							rec(start, 420, 1500, 10.0), // spike mid-km2
							rec(start, 540, 2000, 4.2),  // end km2 @ 540s
							rec(start, 720, 2500, 2.8),
							rec(start, 900, 3000, 2.8), // end km3 @ 900s
						},
					},
				},
			},
		},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}
	inputs := map[string]string{"show_splits": "true"}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, inputs, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	splits := result.Enrichments.GetPace().GetSplits()
	if len(splits) != 3 {
		t.Fatalf("Expected 3 km splits, got %d", len(splits))
	}

	wantSeconds := []float64{300, 240, 360}
	for i, want := range wantSeconds {
		if got := splits[i].Seconds; got < want-1 || got > want+1 {
			t.Errorf("split %d: expected ~%.0fs/km, got %.1fs/km", i+1, want, got)
		}
	}

	// Splits must not all be identical (the original bug).
	if splits[0].Seconds == splits[1].Seconds && splits[1].Seconds == splits[2].Seconds {
		t.Errorf("splits are all identical (%.1f) — average pace was extrapolated per km", splits[0].Seconds)
	}

	// Best split = fastest real km (240s = 4:00), NOT the 10 m/s spike (~100s).
	best := result.Enrichments.GetPace().GetBestSplitSecondsPerKm()
	if best < 239 || best > 241 {
		t.Errorf("expected best split ~240s/km (fastest real km), got %.1fs/km", best)
	}
}

// TestPaceSummary_SplitsFallBackToLaps verifies that when records carry no
// distance/timestamp stream (e.g. treadmill / structured workouts), splits are
// still derived from per-km lap data.
func TestPaceSummary_SplitsFallBackToLaps(t *testing.T) {
	provider := NewPaceSummary()
	provider.Service = &bootstrap.Service{}
	now := time.Now()

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(now),
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 700,
				Laps: []*pbactivity.Lap{
					{StartTime: timestamppb.New(now), TotalElapsedTime: 360, TotalDistance: 1000, Records: []*pbactivity.Record{{Speed: 2.78}}},
					{StartTime: timestamppb.New(now.Add(6 * time.Minute)), TotalElapsedTime: 340, TotalDistance: 1000, Records: []*pbactivity.Record{{Speed: 2.94}}},
				},
			},
		},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}
	inputs := map[string]string{"show_splits": "true"}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, inputs, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	splits := result.Enrichments.GetPace().GetSplits()
	if len(splits) != 2 {
		t.Fatalf("Expected 2 lap-derived splits, got %d", len(splits))
	}
	// 1000m in 360s = 6:00/km = 360s; 1000m in 340s = 5:40/km = 340s.
	if splits[0].Seconds < 359 || splits[0].Seconds > 361 {
		t.Errorf("expected lap 1 ~360s/km, got %.1f", splits[0].Seconds)
	}
}

func TestPaceSummary_SplitTimeMarkers(t *testing.T) {
	provider := NewPaceSummary()
	provider.Service = &bootstrap.Service{}
	now := time.Now()

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(now),
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 1800,
				Laps: []*pbactivity.Lap{
					{
						StartTime:        timestamppb.New(now),
						TotalElapsedTime: 360,
						TotalDistance:    1000,
						Records:          []*pbactivity.Record{{Speed: 2.78}},
					},
					{
						StartTime:        timestamppb.New(now.Add(6 * time.Minute)),
						TotalElapsedTime: 340,
						TotalDistance:    1000,
						Records:          []*pbactivity.Record{{Speed: 2.94}},
					},
					{
						StartTime:        timestamppb.New(now.Add(12 * time.Minute)),
						TotalElapsedTime: 350,
						TotalDistance:    1000,
						Records:          []*pbactivity.Record{{Speed: 2.86}},
					},
				},
			},
		},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}
	inputs := map[string]string{"show_splits": "true"}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, inputs, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// 3 km splits = 3 time markers
	if len(result.TimeMarkers) != 3 {
		t.Errorf("Expected 3 time markers, got %d", len(result.TimeMarkers))
	}
	if len(result.TimeMarkers) > 0 {
		if result.TimeMarkers[0].Label != "Km 1" {
			t.Errorf("Expected first marker label 'Km 1', got %q", result.TimeMarkers[0].Label)
		}
		if result.TimeMarkers[0].MarkerType != "split" {
			t.Errorf("Expected marker type 'split', got %q", result.TimeMarkers[0].MarkerType)
		}
	}
	if result.Metadata["time_markers"] != "3" {
		t.Errorf("Expected metadata time_markers='3', got %q", result.Metadata["time_markers"])
	}
}
