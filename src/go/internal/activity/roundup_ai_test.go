package activity

import (
	"strings"
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSanitiseRoundupString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			name:  "short string passes through",
			input: "Alice Smith",
			check: func(s string) bool { return s == "Alice Smith" },
		},
		{
			name:  "injection delimiters stripped",
			input: "Alice <script>{injection}</script>",
			check: func(s string) bool { return !strings.ContainsAny(s, "<>{}") },
		},
		{
			name:  "long string truncated to 200",
			input: strings.Repeat("a", 300),
			check: func(s string) bool { return len(s) <= 200 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitiseRoundupString(tt.input)
			if !tt.check(got) {
				t.Errorf("sanitiseRoundupString(%q) = %q", tt.input, got)
			}
		})
	}
}

func TestBuildRoundupSummaryContext(t *testing.T) {
	start := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)

	roundup := &pbactivity.ShowcaseRoundup{
		PeriodType:           pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK,
		PeriodStart:          timestamppb.New(start),
		PeriodEnd:            timestamppb.New(end),
		OwnerDisplayName:     "Alice",
		TotalActivities:      5,
		TotalDurationSeconds: 18000, // 5h
		TotalDistanceMeters:  52000, // 52 km
		TotalCaloriesKcal:    2400,
		ActivityTypeBreakdowns: []*pbactivity.RoundupActivityTypeBreakdown{
			{ActivityType: pbactivity.ActivityType_ACTIVITY_TYPE_RUN, ActivityCount: 3},
			{ActivityType: pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, ActivityCount: 2},
		},
		PrsAchieved: []*pbactivity.ShowcaseTopPR{
			{},
		},
		LongestActivityDurationSeconds: 5400, // 1h 30m
		HighestAvgBpm:                  162,
		HighestAvgBpmActivityTitle:     "Tuesday Threshold Run",
		HighestCaloriesPerHourKcal:     720,
		EffortEasyCount:                2,
		EffortModerateCount:            2,
		EffortHardCount:                1,
	}

	ctx := buildRoundupSummaryContext(roundup)

	checks := []struct {
		label string
		want  string
	}{
		{"period type", "week"},
		{"athlete name", "Alice"},
		{"session count", "Sessions: 5"},
		{"total time", "5h"},
		{"distance", "52.0 km"},
		{"calories", "2400"},
		{"sport run", "run"},
		{"sport ride", "ride"},
		{"PR count", "Personal records broken: 1"},
		{"longest session", "1h 30m"},
		{"peak bpm", "162"},
		{"bpm activity title", "Tuesday Threshold Run"},
		{"burn rate", "720"},
		{"effort easy", "2 easy"},
		{"effort hard", "1 hard"},
	}

	for _, c := range checks {
		t.Run(c.label, func(t *testing.T) {
			if !strings.Contains(ctx, c.want) {
				t.Errorf("expected context to contain %q\ngot:\n%s", c.want, ctx)
			}
		})
	}
}

func TestBuildRoundupSummaryContext_MinimalData(t *testing.T) {
	roundup := &pbactivity.ShowcaseRoundup{
		PeriodType:      pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH,
		TotalActivities: 1,
	}
	ctx := buildRoundupSummaryContext(roundup)
	if !strings.Contains(ctx, "month") {
		t.Errorf("expected context to contain 'month', got: %s", ctx)
	}
	if !strings.Contains(ctx, "Sessions: 1") {
		t.Errorf("expected session count in context, got: %s", ctx)
	}
}

func TestBuildRoundupSummaryContext_NoPRLineWhenEmpty(t *testing.T) {
	roundup := &pbactivity.ShowcaseRoundup{
		TotalActivities: 3,
		PrsAchieved:     nil,
	}
	ctx := buildRoundupSummaryContext(roundup)
	if strings.Contains(ctx, "Personal records") {
		t.Errorf("expected no PR line when prsAchieved is empty, got: %s", ctx)
	}
}

func TestBuildRoundupSummaryContext_NoBPMLineWhenZero(t *testing.T) {
	roundup := &pbactivity.ShowcaseRoundup{
		TotalActivities: 2,
		HighestAvgBpm:   0,
	}
	ctx := buildRoundupSummaryContext(roundup)
	if strings.Contains(ctx, "heart rate") {
		t.Errorf("expected no BPM line when HighestAvgBpm is 0, got: %s", ctx)
	}
}

func TestBuildRoundupSummaryContext_ShortDuration(t *testing.T) {
	roundup := &pbactivity.ShowcaseRoundup{
		TotalActivities:      1,
		TotalDurationSeconds: 2700, // 45m — under 1h, no hours component
	}
	ctx := buildRoundupSummaryContext(roundup)
	if !strings.Contains(ctx, "45m") {
		t.Errorf("expected '45m' for sub-hour duration, got: %s", ctx)
	}
}

func TestActivityTypeShortLabel(t *testing.T) {
	tests := []struct {
		input pbactivity.ActivityType
		want  string
	}{
		{pbactivity.ActivityType_ACTIVITY_TYPE_RUN, "run"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, "ride"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SWIM, "swim"},
	}
	for _, tt := range tests {
		got := activityTypeShortLabel(tt.input)
		if got != tt.want {
			t.Errorf("activityTypeShortLabel(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPeriodTypeName(t *testing.T) {
	tests := []struct {
		input pbactivity.RoundupPeriodType
		want  string
	}{
		{pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, "week"},
		{pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH, "month"},
		{pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR, "year"},
		{pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_UNSPECIFIED, "period"},
	}
	for _, tt := range tests {
		got := periodTypeName(tt.input)
		if got != tt.want {
			t.Errorf("periodTypeName(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCollectRoundupMedia(t *testing.T) {
	mkTime := func(day int) *timestamppb.Timestamp {
		return timestamppb.New(time.Date(2025, 9, day, 8, 0, 0, 0, time.UTC))
	}
	entries := []*pbactivity.ShowcaseProfileEntry{
		{
			Title:             "Morning Run",
			ActivityType:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			StartTime:         mkTime(14),
			DistanceMeters:    10000,
			ShowcaseId:        "sc-run-1",
			PhotoUrls:         []string{"https://cdn/p1.jpg", "", "https://cdn/p2.jpg"},
			RouteThumbnailUrl: "https://cdn/route1.png",
		},
		{
			Title:        "Leg Day",
			ActivityType: pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
			StartTime:    mkTime(15),
			PhotoUrls:    []string{"https://cdn/p3.jpg"},
			// no route thumbnail
		},
	}

	t.Run("gallery enabled collects photos and routes", func(t *testing.T) {
		photos, routes := collectRoundupMedia(entries, true)
		if len(photos) != 3 {
			t.Fatalf("photos = %d, want 3 (empty url skipped)", len(photos))
		}
		if photos[0].Url != "https://cdn/p1.jpg" || photos[0].ActivityTitle != "Morning Run" || photos[0].Date != "14 Sep" {
			t.Errorf("unexpected first photo: %+v", photos[0])
		}
		if photos[0].ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
			t.Errorf("photo activity type not carried: %v", photos[0].ActivityType)
		}
		if photos[0].ShowcaseId != "sc-run-1" {
			t.Errorf("photo showcase_id not carried: %q", photos[0].ShowcaseId)
		}
		if len(routes) != 1 {
			t.Fatalf("routes = %d, want 1", len(routes))
		}
		if routes[0].ThumbnailUrl != "https://cdn/route1.png" || routes[0].DistanceMeters != 10000 || routes[0].Date != "14 Sep" {
			t.Errorf("unexpected route: %+v", routes[0])
		}
		if routes[0].ShowcaseId != "sc-run-1" {
			t.Errorf("route showcase_id not carried: %q", routes[0].ShowcaseId)
		}
	})

	t.Run("gallery disabled omits photos but keeps routes", func(t *testing.T) {
		photos, routes := collectRoundupMedia(entries, false)
		if len(photos) != 0 {
			t.Errorf("photos = %d, want 0 when gallery disabled", len(photos))
		}
		if len(routes) != 1 {
			t.Errorf("routes = %d, want 1 (routes are not gated)", len(routes))
		}
	})

	t.Run("photos are capped", func(t *testing.T) {
		var many []*pbactivity.ShowcaseProfileEntry
		for i := 0; i < 40; i++ {
			many = append(many, &pbactivity.ShowcaseProfileEntry{
				Title:     "x",
				StartTime: mkTime(1),
				PhotoUrls: []string{"https://cdn/a.jpg", "https://cdn/b.jpg"},
			})
		}
		photos, _ := collectRoundupMedia(many, true)
		if len(photos) != maxRoundupPhotos {
			t.Errorf("photos = %d, want cap %d", len(photos), maxRoundupPhotos)
		}
	})
}

func TestIsWetWeather(t *testing.T) {
	wet := []string{"Rain", "Light drizzle", "Heavy Showers", "Snow", "Thunderstorm"}
	dry := []string{"Clear", "Partly Cloudy", "Sunny", ""}
	for _, d := range wet {
		if !isWetWeather(d) {
			t.Errorf("isWetWeather(%q) = false, want true", d)
		}
	}
	for _, d := range dry {
		if isWetWeather(d) {
			t.Errorf("isWetWeather(%q) = true, want false", d)
		}
	}
}

func TestAggregateRoundupEnrichments(t *testing.T) {
	strptr := func(s string) *string { return &s }
	fptr := func(f float64) *float64 { return &f }

	entries := []*pbactivity.ShowcaseProfileEntry{
		{
			LocationName:       strptr("Bushy Park, London"),
			Country:            strptr("United Kingdom"),
			TempC:              fptr(8),
			WeatherDescription: strptr("Light Rain"),
			BestEfforts: []*pbactivity.BestEffort{
				{DistanceKey: "5k", Display: "5K", DistanceM: 5000, TimeSeconds: 1400},
			},
			PrimaryMuscles: []string{"quads", "glutes"},
		},
		{
			LocationName:       strptr("Bushy Park, London"),
			Country:            strptr("United Kingdom"),
			TempC:              fptr(-2),
			WeatherDescription: strptr("Clear"),
			BestEfforts: []*pbactivity.BestEffort{
				{DistanceKey: "5k", Display: "5K", DistanceM: 5000, TimeSeconds: 1360}, // faster
				{DistanceKey: "10k", Display: "10K", DistanceM: 10000, TimeSeconds: 2900},
			},
			PrimaryMuscles: []string{"quads"},
		},
		{
			LocationName:   strptr("Richmond Park"),
			Country:        strptr("United Kingdom"),
			TempC:          fptr(15),
			PrimaryMuscles: []string{"chest"},
		},
	}

	places, weather, efforts, muscles := aggregateRoundupEnrichments(entries)

	// Places: Bushy Park (2) ahead of Richmond (1)
	if len(places) != 2 {
		t.Fatalf("places = %d, want 2", len(places))
	}
	if places[0].Name != "Bushy Park, London" || places[0].ActivityCount != 2 {
		t.Errorf("top place = %+v, want Bushy Park x2", places[0])
	}
	if places[0].Country != "United Kingdom" {
		t.Errorf("place country = %q", places[0].Country)
	}

	// Weather: 3 sessions w/ temp, 1 rainy, coldest -2, hottest 15
	if weather == nil {
		t.Fatal("weather is nil, want populated")
	}
	if weather.SessionCount != 3 || weather.RainCount != 1 {
		t.Errorf("weather counts = session %d rain %d, want 3/1", weather.SessionCount, weather.RainCount)
	}
	if weather.ColdestTempC != -2 || weather.HottestTempC != 15 {
		t.Errorf("weather temps = cold %v hot %v, want -2/15", weather.ColdestTempC, weather.HottestTempC)
	}

	// Best efforts: 5k fastest is 1360, sorted by distance asc (5k before 10k)
	if len(efforts) != 2 {
		t.Fatalf("efforts = %d, want 2", len(efforts))
	}
	if efforts[0].DistanceKey != "5k" || efforts[0].TimeSeconds != 1360 {
		t.Errorf("fastest 5k = %+v, want 1360s", efforts[0])
	}
	if efforts[1].DistanceKey != "10k" {
		t.Errorf("second effort = %s, want 10k", efforts[1].DistanceKey)
	}

	// Muscles: quads (2) ahead of glutes/chest (1)
	if len(muscles) != 3 || muscles[0].Name != "quads" || muscles[0].Count != 2 {
		t.Errorf("top muscle = %+v (n=%d), want quads x2", muscles[0], len(muscles))
	}
}

func TestAggregateRoundupEnrichments_Empty(t *testing.T) {
	places, weather, efforts, muscles := aggregateRoundupEnrichments(nil)
	if places != nil || weather != nil || efforts != nil || muscles != nil {
		t.Errorf("empty input should yield all nil, got places=%v weather=%v efforts=%v muscles=%v", places, weather, efforts, muscles)
	}
}

func TestComputeSessionPeaks(t *testing.T) {
	cal := func(v int32) *int32 { return &v }
	entries := []*pbactivity.ShowcaseProfileEntry{
		{DistanceMeters: 10000, CaloriesKcal: cal(600), TotalWeightKg: 0},
		{DistanceMeters: 42195, CaloriesKcal: cal(2800), TotalWeightKg: 0},
		{DistanceMeters: 0, CaloriesKcal: cal(450), TotalWeightKg: 4800},
		{DistanceMeters: 5000, TotalWeightKg: 6200},
	}
	furthest, mostCal, biggestVol := computeSessionPeaks(entries)
	if furthest != 42195 {
		t.Errorf("furthest = %v, want 42195", furthest)
	}
	if mostCal != 2800 {
		t.Errorf("mostCal = %d, want 2800", mostCal)
	}
	if biggestVol != 6200 {
		t.Errorf("biggestVol = %v, want 6200", biggestVol)
	}

	f, c, v := computeSessionPeaks(nil)
	if f != 0 || c != 0 || v != 0 {
		t.Errorf("empty input should be zero, got %v/%d/%v", f, c, v)
	}
}
