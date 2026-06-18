package fit_parser

import (
	"testing"
	"time"

	"github.com/muktihari/fit/profile/typedef"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func i32(v int32) *int32 { return &v }

func TestMergeConsecutiveLapInfos(t *testing.T) {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)

	t.Run("empty", func(t *testing.T) {
		got := mergeConsecutiveLapInfos(nil)
		if !got.startTime.IsZero() || got.totalElapsedTime != 0 {
			t.Errorf("expected zero lapInfo, got %+v", got)
		}
	})

	t.Run("single returned as-is", func(t *testing.T) {
		in := lapInfo{startTime: base, totalElapsedTime: 30, totalDistance: 100}
		got := mergeConsecutiveLapInfos([]lapInfo{in})
		if got != in {
			t.Errorf("single lap should be returned unchanged, got %+v", got)
		}
	})

	t.Run("multiple sums distance and time", func(t *testing.T) {
		group := []lapInfo{
			{startTime: base, totalElapsedTime: 30, totalDistance: 100, wktStepIndex: i32(1)},
			{startTime: base.Add(30 * time.Second), totalElapsedTime: 20, totalDistance: 50},
		}
		got := mergeConsecutiveLapInfos(group)
		if got.totalElapsedTime != 50 {
			t.Errorf("elapsed = %v, want 50", got.totalElapsedTime)
		}
		if got.totalDistance != 150 {
			t.Errorf("distance = %v, want 150", got.totalDistance)
		}
		if !got.startTime.Equal(base) {
			t.Errorf("start time should be first lap's")
		}
		if got.wktStepIndex == nil || *got.wktStepIndex != 1 {
			t.Errorf("wktStepIndex should come from first lap")
		}
	})
}

func TestMergeZeroDurationLaps(t *testing.T) {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)

	t.Run("empty", func(t *testing.T) {
		if got := mergeZeroDurationLaps(nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("transition lap merged into previous", func(t *testing.T) {
		in := []lapInfo{
			{startTime: base, totalElapsedTime: 60, totalDistance: 200},
			{startTime: base.Add(60 * time.Second), totalElapsedTime: 1, totalDistance: 5}, // transition
			{startTime: base.Add(61 * time.Second), totalElapsedTime: 60, totalDistance: 200},
		}
		got := mergeZeroDurationLaps(in)
		if len(got) != 2 {
			t.Fatalf("expected 2 laps after merge, got %d", len(got))
		}
		// First lap absorbed the transition lap.
		if got[0].totalElapsedTime != 61 || got[0].totalDistance != 205 {
			t.Errorf("first lap should absorb transition: %+v", got[0])
		}
	})

	t.Run("leading transition lap", func(t *testing.T) {
		in := []lapInfo{
			{startTime: base, totalElapsedTime: 1, totalDistance: 5}, // transition at start
			{startTime: base.Add(time.Second), totalElapsedTime: 60, totalDistance: 200},
		}
		got := mergeZeroDurationLaps(in)
		// Leading transition is skipped (continue), only the real lap remains.
		if len(got) != 1 {
			t.Fatalf("expected 1 lap, got %d", len(got))
		}
	})
}

func TestMergeLapInfosByWorkoutStep(t *testing.T) {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)

	t.Run("empty", func(t *testing.T) {
		if got := mergeLapInfosByWorkoutStep(nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("no step data falls back to zero-duration merge", func(t *testing.T) {
		in := []lapInfo{
			{startTime: base, totalElapsedTime: 60, totalDistance: 200},
			{startTime: base.Add(60 * time.Second), totalElapsedTime: 1}, // transition
		}
		got := mergeLapInfosByWorkoutStep(in)
		if len(got) != 1 {
			t.Fatalf("expected transition merged, got %d laps", len(got))
		}
	})

	t.Run("groups by workout step index", func(t *testing.T) {
		in := []lapInfo{
			{startTime: base, totalElapsedTime: 60, totalDistance: 100, wktStepIndex: i32(0)},
			{startTime: base.Add(60 * time.Second), totalElapsedTime: 60, totalDistance: 100, wktStepIndex: i32(0)},
			{startTime: base.Add(120 * time.Second), totalElapsedTime: 60, totalDistance: 100, wktStepIndex: i32(1)},
		}
		got := mergeLapInfosByWorkoutStep(in)
		if len(got) != 2 {
			t.Fatalf("expected 2 merged groups, got %d", len(got))
		}
		if got[0].totalElapsedTime != 120 {
			t.Errorf("step 0 should merge two laps to 120s, got %v", got[0].totalElapsedTime)
		}
	})

	t.Run("transition lap joins current group", func(t *testing.T) {
		in := []lapInfo{
			{startTime: base, totalElapsedTime: 60, wktStepIndex: i32(0)},
			{startTime: base.Add(60 * time.Second), totalElapsedTime: 1, wktStepIndex: i32(5)}, // transition, different step but <2s
		}
		got := mergeLapInfosByWorkoutStep(in)
		if len(got) != 1 {
			t.Fatalf("expected transition merged into group, got %d", len(got))
		}
		if got[0].totalElapsedTime != 61 {
			t.Errorf("expected merged elapsed 61, got %v", got[0].totalElapsedTime)
		}
	})
}

func TestGenerateActivityName_AllTimeBuckets(t *testing.T) {
	cases := []struct {
		hour int
		want string
	}{
		{8, "Morning Activity"},
		{14, "Afternoon Activity"},
		{18, "Evening Activity"},
		{23, "Night Activity"},
	}
	for _, c := range cases {
		ts := time.Date(2026, 1, 1, c.hour, 0, 0, 0, time.UTC)
		got := GenerateActivityName(pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, ts)
		if got != c.want {
			t.Errorf("hour %d: got %q, want %q", c.hour, got, c.want)
		}
	}

	// Walk and Hike named variants.
	if got := GenerateActivityName(pbactivity.ActivityType_ACTIVITY_TYPE_WALK, time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)); got != "Morning Walk" {
		t.Errorf("walk: got %q", got)
	}
	if got := GenerateActivityName(pbactivity.ActivityType_ACTIVITY_TYPE_HIKE, time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)); got != "Morning Hike" {
		t.Errorf("hike: got %q", got)
	}
}

func TestMapFitSportToActivityType_MoreSports(t *testing.T) {
	cases := []struct {
		sport    typedef.Sport
		subSport typedef.SubSport
		want     pbactivity.ActivityType
	}{
		{typedef.SportCycling, typedef.SubSportGravelCycling, pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE},
		{typedef.SportCycling, typedef.SubSportEBikeMountain, pbactivity.ActivityType_ACTIVITY_TYPE_EMOUNTAIN_BIKE_RIDE},
		{typedef.SportCycling, typedef.SubSportEBikeFitness, pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE},
		{typedef.SportTraining, typedef.SubSportHiit, pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING},
		{typedef.SportTraining, typedef.SubSportPilates, pbactivity.ActivityType_ACTIVITY_TYPE_PILATES},
		{typedef.SportTraining, typedef.SubSportElliptical, pbactivity.ActivityType_ACTIVITY_TYPE_ELLIPTICAL},
		{typedef.SportTraining, typedef.SubSportStairClimbing, pbactivity.ActivityType_ACTIVITY_TYPE_STAIR_STEPPER},
		{typedef.SportRowing, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_ROWING},
		{typedef.SportAlpineSkiing, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_ALPINE_SKI},
		{typedef.SportCrossCountrySkiing, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_NORDIC_SKI},
		{typedef.SportSnowboarding, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_SNOWBOARD},
		{typedef.SportSoccer, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_SOCCER},
		{typedef.SportTennis, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_TENNIS},
		{typedef.SportPaddling, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_KAYAKING},
		{typedef.SportStandUpPaddleboarding, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_STAND_UP_PADDLING},
		{typedef.SportSurfing, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_SURFING},
		{typedef.SportSailing, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_SAIL},
		{typedef.SportIceSkating, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_ICE_SKATE},
		{typedef.SportInlineSkating, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_INLINE_SKATE},
		{typedef.SportRockClimbing, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING},
	}
	for _, c := range cases {
		if got := mapFitSportToActivityType(c.sport, c.subSport); got != c.want {
			t.Errorf("mapFitSportToActivityType(%v,%v) = %v, want %v", c.sport, c.subSport, got, c.want)
		}
	}
}
