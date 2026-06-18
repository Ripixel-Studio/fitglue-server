package ai_banner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// canceledCtx is an already-canceled context so the genai client's
// GenerateContent fails fast instead of performing a real network call.
func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestAIBanner_Enrich_NoAssetFolderID covers the skip branch where neither a
// pipeline_execution_id input nor an activity ExternalId is available.
func TestAIBanner_Enrich_NoAssetFolderID(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake-key")
	p := NewAIBannerProvider()
	p.Service = &bootstrap.Service{}

	activity := &pbactivity.StandardizedActivity{Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN} // no ExternalId
	res, err := p.Enrich(context.Background(), discardLogger(), activity, athleteUser(), nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "skipped" || res.Metadata["reason"] != "no_asset_folder_id" {
		t.Errorf("expected no_asset_folder_id skip, got %v", res.Metadata)
	}
}

// TestAIBanner_Enrich_RepostCacheMiss exercises the repost path where the cached
// external_id does not match, so the provider falls through to the LLM step,
// which fails fast under a canceled context (prompt_generation_failed).
func TestAIBanner_Enrich_RepostCacheMiss(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake-key")
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"cached_external_id": "different-id",
				"cached_banner_url":  "https://x/y.png",
			}, nil
		},
	}
	p := NewAIBannerProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{ExternalId: "act-1", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}
	inputs := map[string]string{
		"is_repost":            "true",
		"external_id":          "ext-42",
		"enriched_description": "muscle heatmap + HR data",
	}
	res, err := p.Enrich(canceledCtx(), discardLogger(), activity, athleteUser(), inputs, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "error" || res.Metadata["reason"] != "prompt_generation_failed" {
		t.Errorf("expected prompt_generation_failed, got %v", res.Metadata)
	}
}

// TestAIBanner_Enrich_RepostBoosterDataError covers the path where GetBoosterData
// returns an error: the cache lookup is skipped and the provider proceeds to the
// (canceled) LLM step.
func TestAIBanner_Enrich_RepostBoosterDataError(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake-key")
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return nil, errors.New("db down")
		},
	}
	p := NewAIBannerProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{ExternalId: "act-1", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}
	inputs := map[string]string{"is_repost": "true", "external_id": "ext-42"}
	res, err := p.Enrich(canceledCtx(), discardLogger(), activity, athleteUser(), inputs, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "error" {
		t.Errorf("expected error status after LLM failure, got %v", res.Metadata)
	}
}

// TestAIBanner_Enrich_DefaultStyleSubjectAndLLMFailure drives the default
// style/subject branches and the LLM error path using pipeline_execution_id as
// the asset folder.
func TestAIBanner_Enrich_DefaultStyleSubjectAndLLMFailure(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake-key")
	p := NewAIBannerProvider()
	p.SetService(&bootstrap.Service{})

	activity := &pbactivity.StandardizedActivity{Type: pbactivity.ActivityType_ACTIVITY_TYPE_RIDE}
	inputs := map[string]string{"pipeline_execution_id": "exec-9"} // no style/subject -> defaults
	res, err := p.Enrich(canceledCtx(), discardLogger(), activity, athleteUser(), inputs, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "error" || res.Metadata["reason"] != "prompt_generation_failed" {
		t.Errorf("expected prompt_generation_failed, got %v", res.Metadata)
	}
}

// TestBuildActivityContext_ScaleAndIntensity adds activity shapes (long swim,
// fast ride) to push the distance-scale and intensity switch branches.
func TestBuildActivityContext_ScaleAndIntensity(t *testing.T) {
	cases := []struct {
		name     string
		activity *pbactivity.StandardizedActivity
		want     string
	}{
		{
			name: "ironman_swim",
			activity: &pbactivity.StandardizedActivity{
				Type:     pbactivity.ActivityType_ACTIVITY_TYPE_SWIM,
				Sessions: []*pbactivity.Session{{TotalElapsedTime: 4200, TotalDistance: 3900}},
			},
			want: "ironman swim distance",
		},
		{
			name: "fast_ride",
			activity: &pbactivity.StandardizedActivity{
				Type:     pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
				Sessions: []*pbactivity.Session{{TotalElapsedTime: 3600, TotalDistance: 40000}},
			},
			want: "race pace",
		},
		{
			name: "short_swim_pool",
			activity: &pbactivity.StandardizedActivity{
				Type:     pbactivity.ActivityType_ACTIVITY_TYPE_SWIM,
				Sessions: []*pbactivity.Session{{TotalElapsedTime: 1800, TotalDistance: 500}},
			},
			want: "pool training session",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildActivityContext(tc.activity)
			if !strings.Contains(got, tc.want) {
				t.Errorf("buildActivityContext(%s) = %q, want substring %q", tc.name, got, tc.want)
			}
		})
	}
}

// nonAthleteUser returns a hobbyist user record for completeness.
func nonAthleteUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u2", Tier: pbuser.UserTier_USER_TIER_HOBBYIST}}
}

func TestAIBanner_Enrich_TierRestrictedRound2(t *testing.T) {
	p := NewAIBannerProvider()
	p.Service = &bootstrap.Service{}
	activity := &pbactivity.StandardizedActivity{ExternalId: "act-1"}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, nonAthleteUser(), nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["reason"] != "tier_restricted" {
		t.Errorf("expected tier_restricted, got %v", res.Metadata)
	}
}
