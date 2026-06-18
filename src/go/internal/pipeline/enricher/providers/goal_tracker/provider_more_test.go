package goal_tracker

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGoalTracker_IsIdempotent(t *testing.T) {
	if NewGoalTracker().IsIdempotent() {
		t.Error("goal_tracker should not be idempotent")
	}
}

func TestGoalTracker_Enrich_CurrentPeriodProgress(t *testing.T) {
	p := NewGoalTracker()
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	// Activity dated now → current period; 30km toward 100km default, no service.
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(time.Now()),
		Sessions:  []*pbactivity.Session{{TotalDistance: 30000}},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["goal_status"] != "30%" {
		t.Errorf("expected goal_status=30%%, got %q", res.Metadata["goal_status"])
	}
	if res.Metadata["goal_target"] != "100" || res.Metadata["goal_metric"] != "distance" {
		t.Errorf("unexpected metadata %v", res.Metadata)
	}
	if !strings.Contains(res.Description, "Goal Progress") {
		t.Errorf("expected goal progress description, got %q", res.Description)
	}
	if res.Enrichments == nil || res.Enrichments.GoalTracker == nil || len(res.Enrichments.GoalTracker.Goals) != 1 {
		t.Fatal("expected one goal entry in enrichments")
	}
	g := res.Enrichments.GoalTracker.Goals[0]
	if g.Target != 100 || g.Current != 30 || g.OnPace {
		t.Errorf("unexpected goal entry %+v", g)
	}
}

func TestGoalTracker_Enrich_GoalComplete(t *testing.T) {
	p := NewGoalTracker()
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(time.Now()),
		Sessions:  []*pbactivity.Session{{TotalDistance: 60000}}, // 60km
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, map[string]string{"target": "50"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["goal_status"] != "100%" {
		t.Errorf("expected 100%% (capped), got %q", res.Metadata["goal_status"])
	}
	if !strings.Contains(res.Description, "Goal complete") {
		t.Errorf("expected goal-complete message, got %q", res.Description)
	}
	if !res.Enrichments.GoalTracker.Goals[0].OnPace {
		t.Error("expected OnPace true when goal met")
	}
}

func TestGoalTracker_Enrich_PastPeriod(t *testing.T) {
	p := NewGoalTracker()
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	// Year period; backdate to a previous year so it is not the current period.
	past := time.Date(2020, 3, 15, 9, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(past),
		Sessions:  []*pbactivity.Session{{TotalDistance: 10000}},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, map[string]string{"period": "year"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if !strings.Contains(res.Description, "Past activity") {
		t.Errorf("expected past-activity note, got %q", res.Description)
	}
	if res.Metadata["goal_period"] != "year" {
		t.Errorf("expected goal_period=year, got %q", res.Metadata["goal_period"])
	}
}

func TestGoalTracker_Enrich_SameSourceDedup(t *testing.T) {
	now := time.Now()
	periodKey := getPeriodKey("month", now)
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"period_key":              periodKey,
				"accumulated":             float64(40),
				"last_external_id":        "ext-3",
				"last_result_description": "cached goal desc",
				"last_result_metadata":    map[string]interface{}{"goal_status": "40%"},
			}, nil
		},
	}
	p := NewGoalTracker()
	p.SetService(&bootstrap.Service{DB: mockDB})

	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
	activity := &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(now),
		Sessions:  []*pbactivity.Session{{TotalDistance: 5000}},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, map[string]string{"external_id": "ext-3"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Description != "cached goal desc" {
		t.Errorf("expected cached description, got %q", res.Description)
	}
	if res.Metadata["dedup"] != "true" {
		t.Errorf("expected dedup flag, got %v", res.Metadata)
	}
}
