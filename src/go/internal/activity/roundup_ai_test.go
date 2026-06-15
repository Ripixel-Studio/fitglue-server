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
