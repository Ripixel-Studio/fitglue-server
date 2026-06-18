package distance_milestones

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
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func runActivity(distanceM float64) *pbactivity.StandardizedActivity {
	return &pbactivity.StandardizedActivity{
		Type:     pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Sessions: []*pbactivity.Session{{TotalDistance: distanceM}},
	}
}

func TestDistanceMilestones_IsIdempotent(t *testing.T) {
	if NewDistanceMilestones().IsIdempotent() {
		t.Error("distance_milestones should not be idempotent")
	}
}

func TestDistanceMilestones_Enrich_NoDistance(t *testing.T) {
	p := NewDistanceMilestones()
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	res, err := p.Enrich(context.Background(), discardLogger(), &pbactivity.StandardizedActivity{}, u, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["milestone_status"] != "no_distance" {
		t.Errorf("expected no_distance, got %v", res.Metadata)
	}
}

func TestDistanceMilestones_Enrich_SportFiltered(t *testing.T) {
	p := NewDistanceMilestones()
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	// Run activity, but filter is cycling → filtered.
	res, err := p.Enrich(context.Background(), discardLogger(), runActivity(5000), u, map[string]string{"sport": "cycling"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["milestone_status"] != "filtered" {
		t.Errorf("expected filtered, got %v", res.Metadata)
	}
}

func TestDistanceMilestones_Enrich_Progress_NoService(t *testing.T) {
	p := NewDistanceMilestones()
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	// 50km, no prior lifetime (no DB) → no milestone crossed, progress shown.
	res, err := p.Enrich(context.Background(), discardLogger(), runActivity(50000), u, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["lifetime_distance"] != "50.0" {
		t.Errorf("expected lifetime 50.0, got %q", res.Metadata["lifetime_distance"])
	}
	if !strings.Contains(res.Description, "Next milestone") {
		t.Errorf("expected progress description, got %q", res.Description)
	}
	if res.Enrichments == nil || res.Enrichments.DistanceMilestone == nil {
		t.Error("expected DistanceMilestone enrichment")
	}
	if res.Enrichments.DistanceMilestone.NextMilestoneKm == nil {
		t.Error("expected NextMilestoneKm set on progress result")
	}
}

func TestDistanceMilestones_Enrich_MilestoneCrossed(t *testing.T) {
	// Lifetime at 95km, +10km run → crosses 100km milestone.
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{"lifetime_distance": float64(95)}, nil
		},
	}
	p := NewDistanceMilestones()
	p.SetService(&bootstrap.Service{DB: mockDB})

	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	res, err := p.Enrich(context.Background(), discardLogger(), runActivity(10000), u, map[string]string{"sport": "running"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["milestone_reached"] != "100" {
		t.Errorf("expected milestone_reached=100, got %v", res.Metadata)
	}
	if !strings.Contains(res.Description, "MILESTONE") {
		t.Errorf("expected MILESTONE in description, got %q", res.Description)
	}
	if res.Enrichments == nil || res.Enrichments.DistanceMilestone == nil ||
		res.Enrichments.DistanceMilestone.MilestoneKm != 100 {
		t.Errorf("expected milestone summary at 100km, got %+v", res.Enrichments.GetDistanceMilestone())
	}
}

func TestDistanceMilestones_Enrich_SameSourceDedup(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"lifetime_distance":       float64(120),
				"last_external_id":        "ext-9",
				"last_result_description": "cached desc",
				"last_result_metadata":    map[string]interface{}{"lifetime_distance": "120.0"},
			}, nil
		},
	}
	p := NewDistanceMilestones()
	p.SetService(&bootstrap.Service{DB: mockDB})

	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	res, err := p.Enrich(context.Background(), discardLogger(), runActivity(5000), u, map[string]string{"external_id": "ext-9"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Description != "cached desc" {
		t.Errorf("expected cached description, got %q", res.Description)
	}
	if res.Metadata["dedup"] != "true" {
		t.Errorf("expected dedup flag, got %v", res.Metadata)
	}
}
