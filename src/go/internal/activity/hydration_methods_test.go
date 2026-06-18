package activity

import (
	"context"
	"errors"
	"testing"

	"github.com/fitglue/server/src/go/pkg/domain/showcaseview"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var errInjected = errors.New("injected store error")

// marshalEnrichedEvent builds a GCS blob payload (a full EnrichedActivityEvent)
// the way PrepareForPublish stores it, for hydration tests.
func marshalEnrichedEvent(t *testing.T, ev *pbevents.EnrichedActivityEvent) []byte {
	t.Helper()
	data, err := protojson.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal enriched event: %v", err)
	}
	return data
}

// GetPublicShowcase: hydrate from GCS + owner metadata from profile.
func TestGetPublicShowcase_HydratesAndOwnerMetadata(t *testing.T) {
	ctx := context.Background()
	blobData := marshalEnrichedEvent(t, &pbevents.EnrichedActivityEvent{
		ActivityData: &pbactivity.StandardizedActivity{Name: "Public Run"},
		Enrichments:  &pbactivity.ActivityEnrichments{Calories: &pbactivity.CaloriesSummary{Kcal: 320}},
	})
	store := &MockActivityStore{
		GetPublicShowcaseFunc: func(_ context.Context, _ string) (*pbactivity.ShowcasedActivity, string, error) {
			return &pbactivity.ShowcasedActivity{ShowcaseId: "s1", ActivityDataUri: "gs://b/x.json"}, "owner-uid", nil
		},
		GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{
				DisplayName:       "Owner Name",
				ProfilePictureUrl: "https://pic",
				Slug:              "owner-slug",
			}, nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(_ context.Context, _, _ string) ([]byte, error) { return blobData, nil },
	}
	svc := newTestSvc(store, blob)
	res, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{ShowcaseId: "s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OwnerDisplayName != "Owner Name" || res.OwnerProfileSlug != "owner-slug" {
		t.Errorf("expected owner metadata hydrated, got name=%q slug=%q", res.OwnerDisplayName, res.OwnerProfileSlug)
	}
	if res.ActivityData == nil || res.ActivityData.Name != "Public Run" {
		t.Errorf("expected activity data hydrated from GCS, got %+v", res.ActivityData)
	}
}

// GetPublicShowcase: fallback display name when no profile.
func TestGetPublicShowcase_DefaultDisplayName(t *testing.T) {
	ctx := context.Background()
	store := &MockActivityStore{
		GetPublicShowcaseFunc: func(_ context.Context, _ string) (*pbactivity.ShowcasedActivity, string, error) {
			return &pbactivity.ShowcasedActivity{ShowcaseId: "s1"}, "owner-uid", nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	res, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{ShowcaseId: "s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OwnerDisplayName != "FitGlue Athlete" {
		t.Errorf("expected fallback display name, got %q", res.OwnerDisplayName)
	}
}

// AddShowcaseEntry: hydrate ActivityData + enrichments from GCS and project them.
func TestAddShowcaseEntry_HydratesEnrichments(t *testing.T) {
	ctx := context.Background()
	avg := int32(155)
	blobData := marshalEnrichedEvent(t, &pbevents.EnrichedActivityEvent{
		ActivityData: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{TotalDistance: 8000, TotalElapsedTime: 2400, AvgHeartRate: &avg},
			},
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			HeartRate: &pbactivity.HeartRateSummary{AvgBpm: 160},
			Calories:  &pbactivity.CaloriesSummary{Kcal: 600},
			Elevation: &pbactivity.ElevationSummary{TotalGainM: 120},
			Location:  &pbactivity.LocationSummary{LocationName: "Park", Country: "UK"},
			Weather:   &pbactivity.WeatherSummary{TempC: 12, WeatherDescription: "Cloudy"},
			HeartRateZones: &pbactivity.HeartRateZonesSummary{
				Zones: []*pbactivity.HeartRateZoneBucket{{ZoneIndex: 2, Minutes: 30}},
			},
			PersonalRecords: &pbactivity.PersonalRecordsSummary{
				Records: []*pbactivity.PersonalRecord{{RecordType: "5k", NewValue: 1500, Unit: "seconds"}},
			},
		},
	})

	var entry *pbactivity.ShowcaseProfileEntry
	store := &MockActivityStore{
		GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
			return &pbactivity.ShowcasedActivity{
				ShowcaseId:      "s1",
				Title:           "Run",
				ActivityType:    pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
				ActivityDataUri: "gs://b/x.json",
			}, nil
		},
		SetShowcaseProfileEntryFunc: func(_ context.Context, _ string, e *pbactivity.ShowcaseProfileEntry) error {
			entry = e
			return nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(_ context.Context, _, _ string) ([]byte, error) { return blobData, nil },
	}
	svc := newTestSvc(store, blob)
	_, err := svc.AddShowcaseEntry(ctx, &pbsvc.AddShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry to be set")
	}
	if entry.DistanceMeters != 8000 {
		t.Errorf("expected distance 8000, got %v", entry.DistanceMeters)
	}
	if entry.AvgHeartRate == nil || *entry.AvgHeartRate != 160 {
		t.Errorf("expected avg HR from enrichment (160), got %v", entry.AvgHeartRate)
	}
	if entry.CaloriesKcal == nil || *entry.CaloriesKcal != 600 {
		t.Errorf("expected calories 600, got %v", entry.CaloriesKcal)
	}
	if entry.ElevationGainM == nil || *entry.ElevationGainM != 120 {
		t.Errorf("expected elevation gain 120, got %v", entry.ElevationGainM)
	}
	if entry.LocationName == nil || *entry.LocationName != "Park" {
		t.Errorf("expected location name 'Park', got %v", entry.LocationName)
	}
	if entry.PrLabel == nil {
		t.Error("expected PR label to be set")
	}
	if len(entry.HrZoneMinutes) != 6 || entry.HrZoneMinutes[2] != 30 {
		t.Errorf("expected zone minutes projected, got %v", entry.HrZoneMinutes)
	}
}

// ListShowcaseViewStats: full aggregation across profile, roundups, and showcases.
func TestListShowcaseViewStats_FullAggregation(t *testing.T) {
	ctx := context.Background()
	store := &MockActivityStore{
		GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{UserId: "u1", Slug: "athlete"}, nil
		},
		ListRecentRoundupsFunc: func(_ context.Context, _ string, _ int) ([]*pbactivity.ShowcaseRoundup, error) {
			return []*pbactivity.ShowcaseRoundup{{PeriodKey: "week-01-2024"}, {PeriodKey: ""}}, nil
		},
		ListShowcasedActivitiesByUserFunc: func(_ context.Context, _ string, _ int32, _ int32) ([]*pbactivity.ShowcasedActivity, int32, error) {
			return []*pbactivity.ShowcasedActivity{{ShowcaseId: "s1"}}, 1, nil
		},
		GetShowcaseViewStatsFunc: func(_ context.Context, key string) (*pbactivity.ShowcaseViewStats, error) {
			return &pbactivity.ShowcaseViewStats{TargetKey: key, Views: 10, Visitors: 5}, nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	res, err := svc.ListShowcaseViewStats(ctx, &pbsvc.ListShowcaseViewStatsRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// profile (10) + 1 roundup with period key (10) + 1 showcase (10) = 30 views.
	if res.TotalViews != 30 {
		t.Errorf("expected 30 total views, got %d", res.TotalViews)
	}
	if res.Profile == nil || len(res.Roundups) != 1 || len(res.Showcases) != 1 {
		t.Errorf("expected profile + 1 roundup + 1 showcase, got %+v", res)
	}
	// Sanity: the profile key is derived from the slug.
	if res.Profile.TargetKey != showcaseview.ProfileKey("athlete") {
		t.Errorf("unexpected profile target key: %q", res.Profile.TargetKey)
	}
}

// RecordShowcaseView store error path.
func TestRecordShowcaseView_StoreError(t *testing.T) {
	store := &MockActivityStore{
		RecordShowcaseViewFunc: func(_ context.Context, _, _ string) error {
			return errInjected
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.RecordShowcaseView(context.Background(), &pbsvc.RecordShowcaseViewRequest{TargetKey: "k"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

// GetShowcaseViewStats store error after key resolution.
func TestGetShowcaseViewStats_StoreError(t *testing.T) {
	store := &MockActivityStore{
		GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{UserId: "u1", Slug: "athlete"}, nil
		},
		GetShowcaseViewStatsFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseViewStats, error) {
			return nil, errInjected
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.GetShowcaseViewStats(context.Background(), &pbsvc.GetShowcaseViewStatsRequest{
		UserId: "u1",
		Target: pbactivity.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_PROFILE,
	})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

// GetPublicShowcaseProfile with personal records exercises the medal-wall sort branch.
func TestGetPublicShowcaseProfile_WithPersonalRecords(t *testing.T) {
	store := &MockActivityStore{
		GetShowcaseProfileBySlugFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{
				UserId:        "u1",
				Slug:          "athlete",
				Visible:       true,
				StreakHistory: &pbactivity.WeeklyStreakHistory{},
			}, nil
		},
		ListUserPersonalRecordsFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseTopPR, error) {
			return []*pbactivity.ShowcaseTopPR{
				{RecordType: "bench_press_1rm", Unit: "kg", Value: 180, AchievedAt: timestamppb.Now()},
				{RecordType: "squat_1rm", Unit: "kg", Value: 200, AchievedAt: timestamppb.Now()},
				{RecordType: "5k", Unit: "seconds", Value: 1500, AchievedAt: timestamppb.Now()},
			}, nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	res, err := svc.GetPublicShowcaseProfile(context.Background(), &pbsvc.GetPublicShowcaseProfileRequest{Slug: "athlete", Page: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Profile.TopPrs) != 3 {
		t.Errorf("expected 3 top PRs (1 time + 2 weight 1RM), got %d", len(res.Profile.TopPrs))
	}
}
