package ai_companion

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

func TestAICompanion_FlagMethods(t *testing.T) {
	p := NewAICompanionProvider()
	if p.IsIdempotent() {
		t.Error("ai_companion should not be idempotent")
	}
	if !p.ShouldDefer() {
		t.Error("ai_companion should defer")
	}
}

func TestSanitiseForPrompt(t *testing.T) {
	if got := sanitiseForPrompt("short"); got != "short" {
		t.Errorf("short string should be unchanged, got %q", got)
	}
	long := strings.Repeat("a", 700)
	if got := sanitiseForPrompt(long); len(got) != 500 {
		t.Errorf("expected truncation to 500, got %d", len(got))
	}
}

func TestBuildPrompt(t *testing.T) {
	for _, mode := range []string{"title", "both", "description", ""} {
		sys, userContent := buildPrompt(mode)
		if sys == "" || userContent == "" {
			t.Errorf("mode %q: empty prompt parts", mode)
		}
		if !strings.Contains(sys, "activity reviewer") {
			t.Errorf("mode %q: system instruction missing core text", mode)
		}
	}
	_, both := buildPrompt("both")
	if !strings.Contains(both, "TITLE:") || !strings.Contains(both, "DESCRIPTION:") {
		t.Errorf("both mode should request TITLE and DESCRIPTION, got %q", both)
	}
}

func TestParseAIResponse(t *testing.T) {
	// title mode
	if r := parseAIResponse("title", "  *Epic Morning Run*  "); r.Title != "Epic Morning Run" {
		t.Errorf("title parse = %q", r.Title)
	}
	// description mode
	if r := parseAIResponse("description", "Solid tempo effort."); r.Description != "Solid tempo effort." {
		t.Errorf("description parse = %q", r.Description)
	}
	// both mode
	r := parseAIResponse("both", "TITLE: Big Lift Day\nDESCRIPTION: Heavy squats all round.")
	if r.Title != "Big Lift Day" {
		t.Errorf("both title = %q", r.Title)
	}
	if r.Description != "Heavy squats all round." {
		t.Errorf("both description = %q", r.Description)
	}
}

func TestBuildActivityContext_Rich(t *testing.T) {
	activity := &pbactivity.StandardizedActivity{
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Name: "Tempo Run",
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 1800,
				TotalDistance:    5000,
				Laps: []*pbactivity.Lap{
					{Records: []*pbactivity.Record{
						{HeartRate: 150, Cadence: 170, Power: 200},
						{HeartRate: 160, Cadence: 180, Power: 250},
					}},
				},
			},
		},
	}
	ctx := buildActivityContext(activity)
	for _, want := range []string{"Tempo Run", "Duration", "Distance", "Heart Rate", "Cadence", "Power"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("context missing %q:\n%s", want, ctx)
		}
	}
}

func TestBuildActivityContext_Strength(t *testing.T) {
	activity := &pbactivity.StandardizedActivity{
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
		Sessions: []*pbactivity.Session{
			{StrengthSets: []*pbactivity.StrengthSet{
				{ExerciseName: "Bench Press", Reps: 8, WeightKg: 80},
				{ExerciseName: "Bench Press", Reps: 8, WeightKg: 80},
				{ExerciseName: "Pull Up", Reps: 10, WeightKg: 0},
			}},
		},
	}
	ctx := buildActivityContext(activity)
	if !strings.Contains(ctx, "Strength Exercises:") {
		t.Errorf("expected strength section:\n%s", ctx)
	}
	if !strings.Contains(ctx, "Bench Press") || !strings.Contains(ctx, "@ 80.0kg") {
		t.Errorf("expected grouped weighted set:\n%s", ctx)
	}
	if !strings.Contains(ctx, "Pull Up") {
		t.Errorf("expected bodyweight exercise:\n%s", ctx)
	}
}

func TestAICompanion_Enrich_RepostCacheHit(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake-key")
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"cached_external_id": "ext-7",
				"cached_description": "A great session.",
				"cached_name":        "Sweat Fest",
			}, nil
		},
	}
	p := NewAICompanionProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1", Tier: pbuser.UserTier_USER_TIER_ATHLETE}}
	inputs := map[string]string{"is_repost": "true", "external_id": "ext-7"}

	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, inputs, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "success" || res.Metadata["cache"] != "hit" {
		t.Fatalf("expected cache hit, got %v", res.Metadata)
	}
	if res.Description != "A great session." || res.Name != "Sweat Fest" {
		t.Errorf("unexpected cached content: name=%q desc=%q", res.Name, res.Description)
	}
	if res.Enrichments == nil || res.Enrichments.AiSummary == nil {
		t.Error("expected AiSummary enrichment")
	}
}
