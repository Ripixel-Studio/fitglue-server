package personal_records

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPersonalRecords_NameAndType(t *testing.T) {
	p := NewPersonalRecordsProvider()
	if p.Name() != "personal-records" {
		t.Errorf("Name() = %q, want personal-records", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PERSONAL_RECORDS {
		t.Errorf("unexpected provider type %v", p.ProviderType())
	}
}

func TestIsCardioActivity(t *testing.T) {
	cardio := []pbactivity.ActivityType{
		pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_SWIM,
		pbactivity.ActivityType_ACTIVITY_TYPE_ROWING,
		pbactivity.ActivityType_ACTIVITY_TYPE_WALK,
		pbactivity.ActivityType_ACTIVITY_TYPE_HIKE,
	}
	for _, at := range cardio {
		if !IsCardioActivity(at) {
			t.Errorf("IsCardioActivity(%v) = false, want true", at)
		}
	}
	if IsCardioActivity(pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING) {
		t.Error("weight training should not be cardio")
	}
	if IsCardioActivity(pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED) {
		t.Error("unspecified should not be cardio")
	}
}

func TestIsRunningAndCyclingAndStrength(t *testing.T) {
	if !IsRunningActivity(pbactivity.ActivityType_ACTIVITY_TYPE_RUN) {
		t.Error("run should be running")
	}
	if IsRunningActivity(pbactivity.ActivityType_ACTIVITY_TYPE_RIDE) {
		t.Error("ride should not be running")
	}
	if !IsCyclingActivity(pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE) {
		t.Error("gravel ride should be cycling")
	}
	if IsCyclingActivity(pbactivity.ActivityType_ACTIVITY_TYPE_RUN) {
		t.Error("run should not be cycling")
	}
	if !IsStrengthActivity(pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT) {
		t.Error("crossfit should be strength")
	}
	if IsStrengthActivity(pbactivity.ActivityType_ACTIVITY_TYPE_RUN) {
		t.Error("run should not be strength")
	}
}

func TestHybridRaceRecordTypeRoundTrip(t *testing.T) {
	key := FormatHybridRaceRecordType("hyrox", "skierg")
	if key != "hybrid_race_hyrox_skierg" {
		t.Errorf("FormatHybridRaceRecordType = %q", key)
	}
	rt, cat := ParseHybridRaceRecordType(key)
	if rt != "hyrox" || cat != "skierg" {
		t.Errorf("ParseHybridRaceRecordType = (%q,%q), want (hyrox,skierg)", rt, cat)
	}

	rt2, cat2 := ParseHybridRaceRecordType(FormatHybridRaceRecordType("athx", "total_time"))
	if rt2 != "athx" || cat2 != "total_time" {
		t.Errorf("ParseHybridRaceRecordType athx = (%q,%q)", rt2, cat2)
	}

	// Non-hybrid input returns empty.
	if rt3, cat3 := ParseHybridRaceRecordType("fastest_5k"); rt3 != "" || cat3 != "" {
		t.Errorf("expected empty for non-hybrid, got (%q,%q)", rt3, cat3)
	}
	if rt4, _ := ParseHybridRaceRecordType("hybrid_race_"); rt4 != "" {
		t.Errorf("expected empty for too-short prefix, got %q", rt4)
	}
}

func TestCyclingDistanceThresholds(t *testing.T) {
	thresholds := CyclingDistanceThresholds()
	if len(thresholds) == 0 {
		t.Fatal("expected non-empty cycling thresholds")
	}
	for _, th := range thresholds {
		if th.RecordType == "" || th.Display == "" || th.DistanceM <= 0 {
			t.Errorf("invalid cycling threshold %+v", th)
		}
		if !strings.Contains(string(th.RecordType), "ride") {
			t.Errorf("cycling record type should reference ride: %s", th.RecordType)
		}
	}
}

func TestFormatPRValue(t *testing.T) {
	cases := []struct {
		value float64
		unit  string
		want  string
	}{
		{1425, "seconds", "23:45"},
		{21097, "meters", "21.10km"},
		{800, "meters", "800m"},
		{100, "kg", "100kg"},
		{15, "reps", "15"},
		{42.5, "custom", "42.50 custom"},
	}
	for _, c := range cases {
		if got := formatPRValue(c.value, c.unit); got != c.want {
			t.Errorf("formatPRValue(%v,%q) = %q, want %q", c.value, c.unit, got, c.want)
		}
	}
}

func TestPersonalRecords_Enrich_NoNewPRs_NoService(t *testing.T) {
	p := NewPersonalRecordsProvider()
	// Service nil and not a cardio/strength/hybrid activity → no PRs.
	activity := &pbactivity.StandardizedActivity{
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_YOGA,
	}
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}

	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["pr_status"] != "no_new_prs" {
		t.Errorf("expected pr_status=no_new_prs, got %q", res.Metadata["pr_status"])
	}
}

func TestPersonalRecords_Enrich_StrengthFirstPR(t *testing.T) {
	// MockDatabase.GetPersonalRecord returns (nil,nil) → treated as first record (new PR).
	p := NewPersonalRecordsProvider()
	p.SetService(&bootstrap.Service{DB: &mocks.MockDatabase{}})

	activity := &pbactivity.StandardizedActivity{
		Name: "Leg Day",
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
		Sessions: []*pbactivity.Session{
			{
				StrengthSets: []*pbactivity.StrengthSet{
					{ExerciseName: "Squat", WeightKg: 140, Reps: 5},
					{ExerciseName: "Squat", WeightKg: 140, Reps: 5},
				},
			},
		},
	}
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}

	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, map[string]string{"celebrate_in_title": "true"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["pr_status"] != "pr_detected" {
		t.Fatalf("expected pr_detected, got %q", res.Metadata["pr_status"])
	}
	if !strings.Contains(res.Description, "Personal Records") {
		t.Errorf("expected PR description, got %q", res.Description)
	}
	if !strings.HasPrefix(res.Name, "🎉 ") {
		t.Errorf("expected celebrated name, got %q", res.Name)
	}
	if res.Enrichments == nil || res.Enrichments.PersonalRecords == nil || len(res.Enrichments.PersonalRecords.Records) == 0 {
		t.Error("expected PersonalRecords enrichments populated")
	}
}

func TestPersonalRecords_Enrich_CardioRunFirstPR(t *testing.T) {
	p := NewPersonalRecordsProvider()
	p.SetService(&bootstrap.Service{DB: &mocks.MockDatabase{}})

	// 10K run in 50 minutes (session-only totals exercise proportional fallback).
	activity := &pbactivity.StandardizedActivity{
		Name: "Morning Run",
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Sessions: []*pbactivity.Session{
			{TotalDistance: 10000, TotalElapsedTime: 3000},
		},
	}
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}

	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["pr_status"] != "pr_detected" {
		t.Errorf("expected pr_detected for first cardio run, got %q", res.Metadata["pr_status"])
	}
}
