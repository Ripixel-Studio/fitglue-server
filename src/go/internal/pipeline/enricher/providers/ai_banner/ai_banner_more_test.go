package ai_banner

import (
	"context"
	"io"
	"log/slog"
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

func TestAIBanner_FlagMethods(t *testing.T) {
	p := NewAIBannerProvider()
	if p.IsIdempotent() {
		t.Error("ai_banner should not be idempotent")
	}
	if !p.ShouldDefer() {
		t.Error("ai_banner should defer")
	}
}

func TestTruncatePrompt(t *testing.T) {
	if got := truncatePrompt("hello", 10); got != "hello" {
		t.Errorf("short prompt should be unchanged, got %q", got)
	}
	if got := truncatePrompt("abcdefghij", 5); got != "abcde..." {
		t.Errorf("truncatePrompt = %q, want abcde...", got)
	}
}

func athleteUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{
		UserId: "u1",
		Tier:   pbuser.UserTier_USER_TIER_ATHLETE,
	}}
}

func TestAIBanner_Enrich_NoAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	p := NewAIBannerProvider()
	p.Service = &bootstrap.Service{}

	activity := &pbactivity.StandardizedActivity{ExternalId: "act-1", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}

	res, err := p.Enrich(context.Background(), discardLogger(), activity, athleteUser(), nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "skipped" || res.Metadata["reason"] != "api_key_not_configured" {
		t.Errorf("expected api_key_not_configured skip, got %v", res.Metadata)
	}
}

func TestAIBanner_Enrich_RepostCacheHit(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake-key")
	const cachedURL = "https://assets.example/banner.png"

	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"cached_external_id": "ext-42",
				"cached_banner_url":  cachedURL,
			}, nil
		},
	}
	p := NewAIBannerProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{ExternalId: "act-1", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}
	inputs := map[string]string{
		"is_repost":   "true",
		"external_id": "ext-42",
		"style":       "dramatic",
	}

	res, err := p.Enrich(context.Background(), discardLogger(), activity, athleteUser(), inputs, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["status"] != "success" || res.Metadata["cache"] != "hit" {
		t.Fatalf("expected cache hit success, got %v", res.Metadata)
	}
	if res.Metadata["asset_ai_banner"] != cachedURL {
		t.Errorf("expected cached URL, got %q", res.Metadata["asset_ai_banner"])
	}
	if res.Metadata["style"] != "dramatic" {
		t.Errorf("expected style passthrough, got %q", res.Metadata["style"])
	}
	if res.Enrichments == nil || res.Enrichments.AiBanner == nil || res.Enrichments.AiBanner.ImageUrl != cachedURL {
		t.Error("expected AiBanner enrichment with cached URL")
	}
}

func TestAIBanner_SetService(t *testing.T) {
	p := NewAIBannerProvider()
	svc := &bootstrap.Service{}
	p.SetService(svc)
	if p.Service != svc {
		t.Error("SetService did not set Service")
	}
}
