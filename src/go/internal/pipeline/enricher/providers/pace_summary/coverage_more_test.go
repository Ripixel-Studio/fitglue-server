package pace_summary

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newTestUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}
}

// buildKmLapSession builds a session with km laps from per-lap (distance, elapsedSeconds) pairs.
// Each lap is given a single speed record so the avg/best pace path also runs.
func buildKmLapSession(start time.Time, sessionElapsed float64, laps [][2]float64) *pbactivity.StandardizedActivity {
	sess := &pbactivity.Session{
		StartTime:        timestamppb.New(start),
		TotalElapsedTime: sessionElapsed,
	}
	cursor := start
	for _, l := range laps {
		dist, elapsed := l[0], l[1]
		speed := 0.0
		if elapsed > 0 {
			speed = dist / elapsed
		}
		sess.Laps = append(sess.Laps, &pbactivity.Lap{
			StartTime:        timestamppb.New(cursor),
			TotalDistance:    dist,
			TotalElapsedTime: elapsed,
			Records:          []*pbactivity.Record{{Speed: speed}},
		})
		cursor = cursor.Add(time.Duration(elapsed) * time.Second)
	}
	return &pbactivity.StandardizedActivity{
		StartTime:   timestamppb.New(start),
		Description: "Run",
		Sessions:    []*pbactivity.Session{sess},
	}
}

func TestPaceSummary_SetService(t *testing.T) {
	p := NewPaceSummary()
	p.SetService(nil)
	if p.Service != nil {
		t.Errorf("Expected nil service after SetService(nil)")
	}
	svc := &bootstrap.Service{}
	p.SetService(svc)
	if p.Service != svc {
		t.Errorf("Expected service to be set")
	}
}

// TestPaceSummary_NegativeSplitAndSplitsMarkers exercises the splits, fastest/slowest
// marker, and negative-split branches of Enrich.
func TestPaceSummary_NegativeSplitAndSplitsMarkers(t *testing.T) {
	start := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	// 4 km, second half clearly faster than first -> negative split.
	activity := buildKmLapSession(start, 1500, [][2]float64{
		{1000, 420}, // 7:00/km
		{1000, 410}, // 6:50/km
		{1000, 360}, // 6:00/km
		{1000, 350}, // 5:50/km (fastest)
	})

	p := NewPaceSummary()
	p.Service = &bootstrap.Service{}
	inputs := map[string]string{
		"show_splits":          "true",
		"negative_split_alert": "true",
	}
	res, err := p.Enrich(context.Background(), slog.Default(), activity, newTestUser(), inputs, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}
	if !strings.Contains(res.Description, "Splits") {
		t.Errorf("Expected splits section in description, got:\n%s", res.Description)
	}
	if !strings.Contains(res.Description, "🏆") || !strings.Contains(res.Description, "🐢") {
		t.Errorf("Expected fastest/slowest markers in description, got:\n%s", res.Description)
	}
	if !strings.Contains(res.Description, "Negative Split") {
		t.Errorf("Expected negative split callout, got:\n%s", res.Description)
	}
	if res.Metadata["splits_count"] != "4" {
		t.Errorf("Expected splits_count=4, got %s", res.Metadata["splits_count"])
	}
	if res.Enrichments == nil || res.Enrichments.Pace == nil {
		t.Fatal("Expected pace enrichments populated")
	}
	if len(res.Enrichments.Pace.Splits) != 4 {
		t.Errorf("Expected 4 pace splits, got %d", len(res.Enrichments.Pace.Splits))
	}
}

// TestPaceSummary_FatigueDrop drives the fatigue branch where the last quarter is
// slower than the first quarter (positive pace drop).
func TestPaceSummary_FatigueDrop(t *testing.T) {
	start := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	// 8 km getting progressively slower -> fatigue.
	activity := buildKmLapSession(start, 3000, [][2]float64{
		{1000, 300},
		{1000, 305},
		{1000, 310},
		{1000, 320},
		{1000, 340},
		{1000, 360},
		{1000, 380},
		{1000, 400},
	})

	p := NewPaceSummary()
	p.Service = &bootstrap.Service{}
	inputs := map[string]string{"show_fatigue": "true"}
	res, err := p.Enrich(context.Background(), slog.Default(), activity, newTestUser(), inputs, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}
	if !strings.Contains(res.Description, "Fatigue") {
		t.Errorf("Expected fatigue callout, got:\n%s", res.Description)
	}
	if res.Enrichments.Pace.PaceDropPercent <= 0 {
		t.Errorf("Expected positive pace drop percent, got %f", res.Enrichments.Pace.PaceDropPercent)
	}
}

// TestPaceSummary_StrongFinish drives the fatigue branch's "else" (strong finish) path.
func TestPaceSummary_StrongFinish(t *testing.T) {
	start := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	// 8 km getting progressively faster -> strong finish.
	activity := buildKmLapSession(start, 3000, [][2]float64{
		{1000, 400},
		{1000, 380},
		{1000, 360},
		{1000, 340},
		{1000, 320},
		{1000, 310},
		{1000, 305},
		{1000, 300},
	})

	p := NewPaceSummary()
	p.Service = &bootstrap.Service{}
	inputs := map[string]string{"show_fatigue": "true"}
	res, err := p.Enrich(context.Background(), slog.Default(), activity, newTestUser(), inputs, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}
	if !strings.Contains(res.Description, "Strong Finish") {
		t.Errorf("Expected strong finish callout, got:\n%s", res.Description)
	}
	// Last quarter faster than first -> no pace drop recorded.
	if res.Enrichments.Pace.PaceDropPercent != 0 {
		t.Errorf("Expected zero pace drop on strong finish, got %f", res.Enrichments.Pace.PaceDropPercent)
	}
}

// TestLapElapsedSeconds covers all four return paths of lapElapsedSeconds.
func TestLapElapsedSeconds(t *testing.T) {
	base := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)

	t.Run("uses TotalElapsedTime when set", func(t *testing.T) {
		lap := &pbactivity.Lap{TotalElapsedTime: 123}
		if got := lapElapsedSeconds(lap, nil, nil); got != 123 {
			t.Errorf("Expected 123, got %f", got)
		}
	})

	t.Run("returns 0 when no start time", func(t *testing.T) {
		lap := &pbactivity.Lap{}
		if got := lapElapsedSeconds(lap, nil, nil); got != 0 {
			t.Errorf("Expected 0, got %f", got)
		}
	})

	t.Run("falls back to next lap start", func(t *testing.T) {
		lap := &pbactivity.Lap{StartTime: timestamppb.New(base)}
		next := timestamppb.New(base.Add(90 * time.Second))
		if got := lapElapsedSeconds(lap, next, nil); got != 90 {
			t.Errorf("Expected 90, got %f", got)
		}
	})

	t.Run("falls back to session end for final lap", func(t *testing.T) {
		lap := &pbactivity.Lap{StartTime: timestamppb.New(base)}
		end := timestamppb.New(base.Add(120 * time.Second))
		if got := lapElapsedSeconds(lap, nil, end); got != 120 {
			t.Errorf("Expected 120, got %f", got)
		}
	})

	t.Run("returns 0 when no fallbacks available", func(t *testing.T) {
		lap := &pbactivity.Lap{StartTime: timestamppb.New(base)}
		if got := lapElapsedSeconds(lap, nil, nil); got != 0 {
			t.Errorf("Expected 0, got %f", got)
		}
	})
}

// TestCalculateSplitsFromLaps_LongLapSplitting drives the >1100m lap path that
// estimates multiple km splits within a single long lap.
func TestCalculateSplitsFromLaps_LongLapSplitting(t *testing.T) {
	start := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions: []*pbactivity.Session{
			{
				StartTime:        timestamppb.New(start),
				TotalElapsedTime: 1500,
				Laps: []*pbactivity.Lap{
					{
						StartTime:        timestamppb.New(start),
						TotalDistance:    3000, // one long lap -> ~3 km splits
						TotalElapsedTime: 900,
					},
				},
			},
		},
	}

	splits := calculateSplitsFromLaps(activity)
	if len(splits) != 3 {
		t.Fatalf("Expected 3 estimated km splits from a 3000m lap, got %d", len(splits))
	}
	for i, s := range splits {
		if s.Distance != 1000 {
			t.Errorf("split %d: expected 1000m distance, got %f", i, s.Distance)
		}
		if s.StartTime == nil {
			t.Errorf("split %d: expected non-nil start time", i)
		}
	}
}

// TestCalculateSplitsFromLaps_SkipsZeroElapsedKmLap covers the elapsed<=0 continue
// branch for a km-distance lap with no usable timing fallback.
func TestCalculateSplitsFromLaps_SkipsZeroElapsedKmLap(t *testing.T) {
	activity := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{
			{
				// No session StartTime/TotalElapsedTime -> no sessionEndTime fallback.
				Laps: []*pbactivity.Lap{
					{TotalDistance: 1000}, // km lap, but no StartTime and no elapsed -> skipped
				},
			},
		},
	}
	splits := calculateSplitsFromLaps(activity)
	if len(splits) != 0 {
		t.Errorf("Expected 0 splits when km lap has no timing, got %d", len(splits))
	}
}

// TestCalculateSplitsFromLaps_FallbackTiming exercises the lapElapsedSeconds fallback
// path inside calculateSplitsFromLaps (laps with no TotalElapsedTime but a session end).
func TestCalculateSplitsFromLaps_FallbackTiming(t *testing.T) {
	start := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions: []*pbactivity.Session{
			{
				StartTime:        timestamppb.New(start),
				TotalElapsedTime: 720,
				Laps: []*pbactivity.Lap{
					{StartTime: timestamppb.New(start), TotalDistance: 1000},                        // -> next lap start
					{StartTime: timestamppb.New(start.Add(360 * time.Second)), TotalDistance: 1000}, // -> session end
				},
			},
		},
	}
	splits := calculateSplitsFromLaps(activity)
	if len(splits) != 2 {
		t.Fatalf("Expected 2 splits using timing fallbacks, got %d", len(splits))
	}
}
