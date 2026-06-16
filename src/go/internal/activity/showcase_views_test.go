package activity

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newViewTestService(store *MockActivityStore) *Service {
	return NewService(store, &MockBlobStore{}, nil, "test-bucket", "test-showcase-bucket", infra.NewLogger())
}

func TestRecordShowcaseView_RequiresTargetKey(t *testing.T) {
	svc := newViewTestService(&MockActivityStore{})
	_, err := svc.RecordShowcaseView(context.Background(), &pbsvc.RecordShowcaseViewRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestRecordShowcaseView_ForwardsToStore(t *testing.T) {
	var gotKey, gotHash string
	store := &MockActivityStore{
		RecordShowcaseViewFunc: func(ctx context.Context, targetKey, visitorHash string) error {
			gotKey, gotHash = targetKey, visitorHash
			return nil
		},
	}
	svc := newViewTestService(store)

	if _, err := svc.RecordShowcaseView(context.Background(), &pbsvc.RecordShowcaseViewRequest{
		TargetKey:   "activity:s1",
		VisitorHash: "deadbeef",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKey != "activity:s1" || gotHash != "deadbeef" {
		t.Fatalf("store got (%q, %q)", gotKey, gotHash)
	}
}

func TestGetShowcaseViewStats_ActivityOwnershipEnforced(t *testing.T) {
	store := &MockActivityStore{
		GetPublicShowcaseFunc: func(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error) {
			return &pbactivity.ShowcasedActivity{ShowcaseId: showcaseID}, "owner-1", nil
		},
		GetShowcaseViewStatsFunc: func(ctx context.Context, targetKey string) (*pbactivity.ShowcaseViewStats, error) {
			return &pbactivity.ShowcaseViewStats{TargetKey: targetKey, Views: 7, Visitors: 3}, nil
		},
	}
	svc := newViewTestService(store)

	// Non-owner is rejected.
	_, err := svc.GetShowcaseViewStats(context.Background(), &pbsvc.GetShowcaseViewStatsRequest{
		UserId:   "intruder",
		Target:   pbactivity.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_ACTIVITY,
		TargetId: "s1",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied for non-owner, got %v", err)
	}

	// Owner reads stats for the correct key.
	stats, err := svc.GetShowcaseViewStats(context.Background(), &pbsvc.GetShowcaseViewStatsRequest{
		UserId:   "owner-1",
		Target:   pbactivity.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_ACTIVITY,
		TargetId: "s1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TargetKey != "activity:s1" || stats.Views != 7 || stats.Visitors != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestGetShowcaseViewStats_ProfileUsesOwnSlug(t *testing.T) {
	var readKey string
	store := &MockActivityStore{
		GetShowcasePreferencesFunc: func(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{UserId: userID, Slug: "jane"}, nil
		},
		GetShowcaseViewStatsFunc: func(ctx context.Context, targetKey string) (*pbactivity.ShowcaseViewStats, error) {
			readKey = targetKey
			return &pbactivity.ShowcaseViewStats{TargetKey: targetKey}, nil
		},
	}
	svc := newViewTestService(store)

	// target_id is ignored for profiles — the caller's own slug is always used.
	if _, err := svc.GetShowcaseViewStats(context.Background(), &pbsvc.GetShowcaseViewStatsRequest{
		UserId:   "owner-1",
		Target:   pbactivity.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_PROFILE,
		TargetId: "someone-elses-slug",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if readKey != "profile:jane" {
		t.Fatalf("expected profile:jane, got %q", readKey)
	}
}

func TestListShowcaseViewStats_Aggregates(t *testing.T) {
	store := &MockActivityStore{
		GetShowcasePreferencesFunc: func(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{UserId: userID, Slug: "jane"}, nil
		},
		ListShowcasedActivitiesByUserFunc: func(ctx context.Context, userID string, limit, offset int32) ([]*pbactivity.ShowcasedActivity, int32, error) {
			return []*pbactivity.ShowcasedActivity{{ShowcaseId: "s1"}, {ShowcaseId: "s2"}}, 2, nil
		},
		GetShowcaseViewStatsFunc: func(ctx context.Context, targetKey string) (*pbactivity.ShowcaseViewStats, error) {
			switch targetKey {
			case "profile:jane":
				return &pbactivity.ShowcaseViewStats{TargetKey: targetKey, Views: 100, Visitors: 40}, nil
			case "activity:s1":
				return &pbactivity.ShowcaseViewStats{TargetKey: targetKey, Views: 10, Visitors: 4}, nil
			case "activity:s2":
				return &pbactivity.ShowcaseViewStats{TargetKey: targetKey, Views: 5, Visitors: 2}, nil
			}
			return &pbactivity.ShowcaseViewStats{TargetKey: targetKey}, nil
		},
	}
	svc := newViewTestService(store)

	resp, err := svc.ListShowcaseViewStats(context.Background(), &pbsvc.ListShowcaseViewStatsRequest{UserId: "owner-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Profile == nil || resp.Profile.Views != 100 {
		t.Fatalf("expected profile stats, got %+v", resp.Profile)
	}
	if len(resp.Showcases) != 2 {
		t.Fatalf("expected 2 showcase stats, got %d", len(resp.Showcases))
	}
	if resp.TotalViews != 115 {
		t.Errorf("TotalViews = %d, want 115", resp.TotalViews)
	}
	if resp.TotalVisitors != 46 {
		t.Errorf("TotalVisitors = %d, want 46", resp.TotalVisitors)
	}
}
