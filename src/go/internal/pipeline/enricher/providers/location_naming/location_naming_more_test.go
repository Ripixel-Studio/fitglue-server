package location_naming

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGetActivityTypeStr_More(t *testing.T) {
	cases := []struct {
		at   pbactivity.ActivityType
		want string
	}{
		{pbactivity.ActivityType_ACTIVITY_TYPE_SWIM, "Swim"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING, "Workout"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_YOGA, "Yoga"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE, "Virtual Ride"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN, "Virtual Run"},
	}
	for _, c := range cases {
		if got := getActivityTypeStr(c.at); got != c.want {
			t.Errorf("getActivityTypeStr(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}

func TestGetSolarTimeContext(t *testing.T) {
	sunrise := time.Date(2025, 6, 21, 5, 0, 0, 0, time.UTC)
	sunset := time.Date(2025, 6, 21, 21, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		t          time.Time
		wantPrefix string
		wantEmoji  string
	}{
		{"deep night", time.Date(2025, 6, 21, 2, 0, 0, 0, time.UTC), "Night", "🌙"},
		{"sunrise window", time.Date(2025, 6, 21, 5, 10, 0, 0, time.UTC), "Sunrise", "🌅"},
		{"morning", time.Date(2025, 6, 21, 8, 0, 0, 0, time.UTC), "Morning", "☀️"},
		{"afternoon", time.Date(2025, 6, 21, 15, 0, 0, 0, time.UTC), "Afternoon", "☀️"},
		{"sunset window", time.Date(2025, 6, 21, 20, 50, 0, 0, time.UTC), "Sunset", "🌄"},
		{"after sunset", time.Date(2025, 6, 21, 23, 0, 0, 0, time.UTC), "Night", "🌙"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prefix, emoji := getSolarTimeContext(c.t, sunrise, sunset)
			if prefix != c.wantPrefix || emoji != c.wantEmoji {
				t.Errorf("getSolarTimeContext = (%q,%q), want (%q,%q)", prefix, emoji, c.wantPrefix, c.wantEmoji)
			}
		})
	}
}

func TestCalculateSunriseSunset(t *testing.T) {
	date := time.Date(2025, 6, 21, 12, 0, 0, 0, time.UTC)
	// London (~51.5N, 0W) midsummer: long day, sunrise before sunset.
	sunrise, sunset := calculateSunriseSunset(date, 51.5, 0)
	if !sunrise.Before(sunset) {
		t.Errorf("sunrise %v should be before sunset %v", sunrise, sunset)
	}
	if sunset.Sub(sunrise) < 12*time.Hour {
		t.Errorf("midsummer London day should be long, got %v", sunset.Sub(sunrise))
	}

	// Polar night clamp: extreme northern latitude in deep winter should not panic.
	winter := time.Date(2025, 12, 21, 12, 0, 0, 0, time.UTC)
	calculateSunriseSunset(winter, 89, 0)
}

func TestLocationNaming_Enrich_NoGPS_Skips(t *testing.T) {
	p := NewLocationNaming()
	activity := &pbactivity.StandardizedActivity{
		Type:     pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Sessions: []*pbactivity.Session{{TotalElapsedTime: 1800}},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, nil, nil, true)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if !res.Skipped || res.Metadata["location_naming_status"] != "skipped" {
		t.Errorf("expected skip for no GPS, got %v", res.Metadata)
	}
}

func TestLocationNaming_Enrich_HintLabel_DescriptionMode(t *testing.T) {
	p := NewLocationNaming()
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_YOGA,
		StartTime: timestamppb.New(time.Date(2025, 6, 21, 8, 0, 0, 0, time.UTC)),
		HintLocation: &pbactivity.LocationSummary{
			Latitude:     52.99,
			Longitude:    -0.78,
			LocationName: "Code Fitness, Newark",
		},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, nil, map[string]string{
		"mode":         "description",
		"time_context": "solar",
	}, true)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Skipped {
		t.Fatalf("expected enrichment, got skip: %s", res.SkipReason)
	}
	if !strings.Contains(res.Description, "Code Fitness, Newark") {
		t.Errorf("expected location line in description, got %q", res.Description)
	}
	if !strings.HasPrefix(res.Description, "📍 Location:") {
		t.Errorf("expected location pin prefix, got %q", res.Description)
	}
	if res.Metadata["location_naming_status"] != "success" {
		t.Errorf("expected success status, got %q", res.Metadata["location_naming_status"])
	}
}
