package activity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------- GetPublicRoundup ----------------

func TestGetPublicRoundup_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetPublicRoundup(ctx, &pbsvc.GetPublicRoundupRequest{Slug: "x"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetRoundupFunc = func(_ context.Context, _, _ string) (*pbactivity.ShowcaseRoundup, error) {
			return nil, errors.New("db error")
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetPublicRoundup(ctx, &pbsvc.GetPublicRoundupRequest{Slug: "x", PeriodKey: "week-01-2024"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetRoundupFunc = func(_ context.Context, _, _ string) (*pbactivity.ShowcaseRoundup, error) {
			return nil, nil
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetPublicRoundup(ctx, &pbsvc.GetPublicRoundupRequest{Slug: "x", PeriodKey: "week-01-2024"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetRoundupFunc = func(_ context.Context, _, _ string) (*pbactivity.ShowcaseRoundup, error) {
			return &pbactivity.ShowcaseRoundup{RoundupId: "r1"}, nil
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.GetPublicRoundup(ctx, &pbsvc.GetPublicRoundupRequest{Slug: "x", PeriodKey: "week-01-2024"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RoundupId != "r1" {
			t.Errorf("expected r1, got %q", res.RoundupId)
		}
	})
}

// ---------------- GetRecentPublicRoundups ----------------

func TestGetRecentPublicRoundups_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetRecentPublicRoundups(ctx, &pbsvc.GetRecentPublicRoundupsRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.ListRecentRoundupsFunc = func(_ context.Context, _ string, _ int) ([]*pbactivity.ShowcaseRoundup, error) {
			return nil, errors.New("db error")
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetRecentPublicRoundups(ctx, &pbsvc.GetRecentPublicRoundupsRequest{Slug: "x"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("DefaultLimitSuccess", func(t *testing.T) {
		var gotLimit int
		store := &MockActivityStore{}
		store.ListRecentRoundupsFunc = func(_ context.Context, _ string, limit int) ([]*pbactivity.ShowcaseRoundup, error) {
			gotLimit = limit
			return []*pbactivity.ShowcaseRoundup{{RoundupId: "r1"}}, nil
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.GetRecentPublicRoundups(ctx, &pbsvc.GetRecentPublicRoundupsRequest{Slug: "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotLimit != 3 {
			t.Errorf("expected default limit 3, got %d", gotLimit)
		}
		if len(res.Roundups) != 1 {
			t.Errorf("expected 1 roundup, got %d", len(res.Roundups))
		}
	})
}

// ---------------- RecomputeRoundup ----------------

func TestRecomputeRoundup_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.RecomputeRoundup(ctx, &pbsvc.RecomputeRoundupRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("BadPeriodKey", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.RecomputeRoundup(ctx, &pbsvc.RecomputeRoundupRequest{UserId: "u1", PeriodKey: "garbage-key"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for bad period key, got %v", err)
		}
	})

	t.Run("GenerateError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return nil, errors.New("db error")
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.RecomputeRoundup(ctx, &pbsvc.RecomputeRoundupRequest{UserId: "u1", PeriodKey: "week-01-2024"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{
				UserId:          "u1",
				Slug:            "athlete",
				RoundupSettings: &pbactivity.RoundupSettings{EnabledWeekly: true},
			}, nil
		}
		store.ListShowcaseEntriesInRangeFunc = func(_ context.Context, _ string, _, _ time.Time) ([]*pbactivity.ShowcaseProfileEntry, error) {
			return []*pbactivity.ShowcaseProfileEntry{
				{ShowcaseId: "s1", Title: "Run", ActivityType: pbactivity.ActivityType_ACTIVITY_TYPE_RUN, DurationSeconds: 1800, DistanceMeters: 5000},
			}, nil
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.RecomputeRoundup(ctx, &pbsvc.RecomputeRoundupRequest{UserId: "u1", PeriodKey: "week-01-2024"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil || res.TotalActivities != 1 {
			t.Errorf("expected a roundup with 1 activity, got %+v", res)
		}
	})

	t.Run("NoProfileNotFound", func(t *testing.T) {
		// generateRoundup returns nil,nil when there is no profile/slug → NotFound.
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return nil, nil
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.RecomputeRoundup(ctx, &pbsvc.RecomputeRoundupRequest{UserId: "u1", PeriodKey: "week-01-2024"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})
}

// ---------------- UpdateRoundupSettings ----------------

func TestUpdateRoundupSettings_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateRoundupSettings(ctx, &pbsvc.UpdateRoundupSettingsRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("EnsureProfileError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return nil, errors.New("db error")
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.UpdateRoundupSettings(ctx, &pbsvc.UpdateRoundupSettingsRequest{
			UserId:   "u1",
			Settings: &pbactivity.RoundupSettings{EnabledWeekly: true},
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		var saved *pbactivity.ShowcaseProfile
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{UserId: "u1"}, nil
		}
		store.UpdateShowcasePreferencesFunc = func(_ context.Context, _ string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
			saved = prefs
			return prefs, nil
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.UpdateRoundupSettings(ctx, &pbsvc.UpdateRoundupSettingsRequest{
			UserId:   "u1",
			Settings: &pbactivity.RoundupSettings{EnabledMonthly: true},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if saved == nil || saved.RoundupSettings == nil || !saved.RoundupSettings.EnabledMonthly {
			t.Errorf("expected roundup settings persisted, got %+v", saved)
		}
	})
}

// ---------------- Pure helpers ----------------

func TestDeriveEffortLevel(t *testing.T) {
	tests := []struct {
		name string
		in   *pbactivity.ShowcaseProfileEntry
		want int32
	}{
		{"zero", &pbactivity.ShowcaseProfileEntry{}, 0},
		{"duration easy", &pbactivity.ShowcaseProfileEntry{DurationSeconds: 600}, 1},
		{"duration moderate", &pbactivity.ShowcaseProfileEntry{DurationSeconds: 2400}, 2},
		{"duration hard", &pbactivity.ShowcaseProfileEntry{DurationSeconds: 5000}, 3},
		{"duration peak", &pbactivity.ShowcaseProfileEntry{DurationSeconds: 8000}, 4},
		{"hr peak Z5", &pbactivity.ShowcaseProfileEntry{HrZoneMinutes: []int32{0, 0, 0, 0, 0, 100}}, 4},
		{"hr easy Z1Z2", &pbactivity.ShowcaseProfileEntry{HrZoneMinutes: []int32{0, 50, 50, 0, 0, 0}}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveEffortLevel(tc.in); got != tc.want {
				t.Errorf("deriveEffortLevel(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatEffortClock(t *testing.T) {
	cases := map[float64]string{
		0:    "0:00",
		65:   "1:05",
		600:  "10:00",
		3661: "1:01:01",
	}
	for in, want := range cases {
		if got := formatEffortClock(in); got != want {
			t.Errorf("formatEffortClock(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatPRLabel(t *testing.T) {
	prev := 175.0
	weightPR := &pbactivity.PersonalRecord{
		RecordType:    "bench_press_1rm",
		NewValue:      180,
		Unit:          "kg",
		PreviousValue: &prev,
	}
	label := formatPRLabel(weightPR)
	if !strings.Contains(label, "BENCH PRESS 1RM") || !strings.Contains(label, "180KG") || !strings.Contains(label, "+5KG") {
		t.Errorf("unexpected weight PR label: %q", label)
	}

	prevSec := 320.0
	timePR := &pbactivity.PersonalRecord{
		RecordType:    "5k",
		NewValue:      300, // 5:00
		Unit:          "seconds",
		PreviousValue: &prevSec,
	}
	tlabel := formatPRLabel(timePR)
	if !strings.Contains(tlabel, "5:00") {
		t.Errorf("expected time PR formatted as clock, got %q", tlabel)
	}
}

func TestCamelToSnake(t *testing.T) {
	cases := map[string]string{
		"displayName": "display_name",
		"Bio":         "bio",
		"userId":      "user_id",
		"slug":        "slug",
	}
	for in, want := range cases {
		if got := camelToSnake(in); got != want {
			t.Errorf("camelToSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestActivityTypeHasDistance(t *testing.T) {
	if !activityTypeHasDistance(pbactivity.ActivityType_ACTIVITY_TYPE_RUN) {
		t.Error("expected RUN to have distance")
	}
	if activityTypeHasDistance(pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING) {
		t.Error("expected WEIGHT_TRAINING not to have distance")
	}
}

func TestDownsample(t *testing.T) {
	if got := downsample(nil, 5); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
	short := []float64{1, 2, 3}
	if got := downsample(short, 5); len(got) != 3 {
		t.Errorf("expected unchanged short slice, got %v", got)
	}
	long := make([]float64, 100)
	for i := range long {
		long[i] = float64(i)
	}
	if got := downsample(long, 10); len(got) != 10 {
		t.Errorf("expected 10 downsampled points, got %d", len(got))
	}
}

func TestBuildSparkline(t *testing.T) {
	t.Run("NoData", func(t *testing.T) {
		sess := &pbactivity.Session{}
		if got := buildSparkline(sess); got != nil {
			t.Errorf("expected nil sparkline with no records, got %v", got)
		}
	})

	t.Run("PrefersHeartRate", func(t *testing.T) {
		sess := &pbactivity.Session{
			Laps: []*pbactivity.Lap{
				{Records: []*pbactivity.Record{
					{HeartRate: 120, Power: 200},
					{HeartRate: 130, Power: 210},
				}},
			},
		}
		got := buildSparkline(sess)
		if got == nil || got.Metric != "heart_rate" {
			t.Errorf("expected heart_rate metric, got %+v", got)
		}
	})
}

func TestRecomputeAndSaveProfileStats(t *testing.T) {
	ctx := context.Background()
	var saved *pbactivity.ShowcaseProfile
	store := &MockActivityStore{
		GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{UserId: "u1"}, nil
		},
		ListShowcaseProfileEntriesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
			return []*pbactivity.ShowcaseProfileEntry{
				{ShowcaseId: "s1", DistanceMeters: 5000, DurationSeconds: 1800, StartTime: timestamppb.Now(), HrZoneMinutes: []int32{0, 30, 40, 5, 5, 5}},
			}, nil
		},
		UpdateShowcasePreferencesFunc: func(_ context.Context, _ string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
			saved = prefs
			return prefs, nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	svc.recomputeAndSaveProfileStats(ctx, "u1")

	if saved == nil {
		t.Fatal("expected profile to be saved")
	}
	if saved.TotalActivities != 1 || saved.TotalDistanceMeters != 5000 {
		t.Errorf("expected recomputed totals, got activities=%d distance=%v", saved.TotalActivities, saved.TotalDistanceMeters)
	}
	if saved.StreakHistory == nil {
		t.Error("expected streak history to be computed")
	}
	if saved.ZoneSplit == nil {
		t.Error("expected zone split to be computed")
	}
}
