package distanceeffort

import (
	"math"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// buildActivityWithNativeDistance builds an activity whose records carry
// cumulative Distance and a timestamp at 1Hz, moving at a constant pace.
func buildActivityWithNativeDistance(paceMetersPerSec float64, durationSec int) *pbactivity.StandardizedActivity {
	var records []*pbactivity.Record
	base := int64(1_000_000)
	for i := 0; i <= durationSec; i++ {
		records = append(records, &pbactivity.Record{
			Timestamp: &timestamppb.Timestamp{Seconds: base + int64(i)},
			Distance:  paceMetersPerSec * float64(i),
			Speed:     paceMetersPerSec,
		})
	}
	return &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{{Records: records}}},
		},
	}
}

func TestFindFastestSegment_NativeDistance(t *testing.T) {
	// 4 m/s for 1000s -> 4000m total. A 1000m segment at 4m/s should take ~250s.
	act := buildActivityWithNativeDistance(4.0, 1000)
	got := FindFastestSegment(act, 1000)
	if math.Abs(got-250) > 2 {
		t.Errorf("expected ~250s for 1000m at 4m/s, got %v", got)
	}
}

func TestFindFastestSegment_TooShort(t *testing.T) {
	act := buildActivityWithNativeDistance(4.0, 100) // 400m total
	got := FindFastestSegment(act, 1000)
	if got != 0 {
		t.Errorf("expected 0 when activity is shorter than target, got %v", got)
	}
}

func TestFindFastestSegment_SpeedDerived(t *testing.T) {
	// No native Distance, only Speed -> buildFromSpeedDerived path.
	var records []*pbactivity.Record
	base := int64(2_000_000)
	for i := 0; i <= 600; i++ {
		records = append(records, &pbactivity.Record{
			Timestamp: &timestamppb.Timestamp{Seconds: base + int64(i)},
			Speed:     5.0, // 5 m/s
		})
	}
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{Laps: []*pbactivity.Lap{{Records: records}}}},
	}
	// 5 m/s -> 1000m in 200s
	got := FindFastestSegment(act, 1000)
	if math.Abs(got-200) > 5 {
		t.Errorf("expected ~200s for 1000m at 5m/s (speed-derived), got %v", got)
	}
}

func TestFindFastestSegment_LapLevel(t *testing.T) {
	// No records, only lap totals -> findFastestFromLaps path.
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{
				{TotalDistance: 1000, TotalElapsedTime: 300},
				{TotalDistance: 1000, TotalElapsedTime: 280},
				{TotalDistance: 1000, TotalElapsedTime: 320},
			}},
		},
	}
	got := FindFastestSegment(act, 1000)
	if got <= 0 {
		t.Errorf("expected a positive fastest-segment time from laps, got %v", got)
	}
	// The fastest contiguous 1000m should be no slower than the slowest single lap.
	if got > 320 {
		t.Errorf("fastest 1000m (%v) should not exceed slowest lap time", got)
	}
}

func TestFindFastestSegment_Proportional(t *testing.T) {
	// Only session totals (no laps, no records) -> proportional fallback.
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{
			{TotalDistance: 5000, TotalElapsedTime: 1500},
		},
	}
	got := FindFastestSegment(act, 1000)
	// (1000/5000)*1500 = 300
	if math.Abs(got-300) > 0.01 {
		t.Errorf("expected 300s proportional, got %v", got)
	}
}

func TestFindFastestProportional_TooShort(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{TotalDistance: 500, TotalElapsedTime: 200}},
	}
	if got := findFastestProportional(act, 1000); got != 0 {
		t.Errorf("expected 0 when total distance < target, got %v", got)
	}
}

func TestFindFastestProportional_ZeroDuration(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{TotalDistance: 5000, TotalElapsedTime: 0}},
	}
	if got := findFastestProportional(act, 1000); got != 0 {
		t.Errorf("expected 0 when duration is zero, got %v", got)
	}
}

func TestInterpolateTime(t *testing.T) {
	points := []DistanceTimePoint{
		{0, 0},
		{100, 20},
		{200, 40},
	}
	t.Run("midpoint", func(t *testing.T) {
		// At 150m, time should interpolate to 30s.
		if got := interpolateTime(points, 150); math.Abs(got-30) > 0.01 {
			t.Errorf("expected 30, got %v", got)
		}
	})
	t.Run("before first", func(t *testing.T) {
		if got := interpolateTime(points, -10); got != 0 {
			t.Errorf("expected first point time 0, got %v", got)
		}
	})
	t.Run("after last", func(t *testing.T) {
		if got := interpolateTime(points, 9999); got != 40 {
			t.Errorf("expected last point time 40, got %v", got)
		}
	})
}

func TestSlidingWindowMinTime_NoQualifyingWindow(t *testing.T) {
	points := []DistanceTimePoint{{0, 0}, {100, 50}}
	if got := slidingWindowMinTime(points, 1000); got != 0 {
		t.Errorf("expected 0 when no window covers target, got %v", got)
	}
}

func TestBuildDistanceTimePoints_Empty(t *testing.T) {
	act := &pbactivity.StandardizedActivity{}
	if pts := buildDistanceTimePoints(act); pts != nil {
		t.Errorf("expected nil points for empty activity, got %v", pts)
	}
}
