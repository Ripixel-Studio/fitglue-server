package enricher

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/encoding/protojson"
)

// fakeNonIdempotentProvider implements providers.Provider and
// providers.NonIdempotentProvider for journal/replay tests.
type fakeNonIdempotentProvider struct{}

func (f *fakeNonIdempotentProvider) Name() string { return "ai-companion" }

func (f *fakeNonIdempotentProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED
}

func (f *fakeNonIdempotentProvider) Enrich(_ context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
	return nil, nil
}

func (f *fakeNonIdempotentProvider) IsIdempotent() bool { return false }

// TestBuildBoosterMetadata_PersistsTypedEnrichments verifies that a
// non-idempotent provider's typed Enrichments (e.g. AiSummary) are serialized
// into the replay journal so they survive a resume. Without this, the AI
// summary was lost from showcase enrichment data on re-runs even though the
// description survived. (Regression test.)
func TestBuildBoosterMetadata_PersistsTypedEnrichments(t *testing.T) {
	res := &providers.EnrichmentResult{
		Description: "✨ AI Summary:\nGreat workout!",
		Metadata:    map[string]string{"status": "success", "mode": "description"},
		Enrichments: &pbactivity.ActivityEnrichments{
			AiSummary: &pbactivity.AiSummary{Html: "Great workout!"},
		},
	}

	m := buildBoosterMetadata(res, &fakeNonIdempotentProvider{})

	if m["replay_description"] != res.Description {
		t.Errorf("replay_description = %q, want %q", m["replay_description"], res.Description)
	}

	raw, ok := m["replay_enrichments"]
	if !ok || raw == "" {
		t.Fatalf("expected replay_enrichments to be set, got %q", raw)
	}

	restored := &pbactivity.ActivityEnrichments{}
	if err := protojson.Unmarshal([]byte(raw), restored); err != nil {
		t.Fatalf("failed to unmarshal replay_enrichments: %v", err)
	}
	if restored.GetAiSummary().GetHtml() != "Great workout!" {
		t.Errorf("restored AiSummary.Html = %q, want %q", restored.GetAiSummary().GetHtml(), "Great workout!")
	}
}

// TestBuildBoosterMetadata_NoEnrichments verifies the journal omits the key
// when the result carries no typed enrichments.
func TestBuildBoosterMetadata_NoEnrichments(t *testing.T) {
	res := &providers.EnrichmentResult{
		Name:     "Morning Run (#5)",
		Metadata: map[string]string{"status": "success"},
	}

	m := buildBoosterMetadata(res, &fakeNonIdempotentProvider{})

	if _, ok := m["replay_enrichments"]; ok {
		t.Error("replay_enrichments should not be set when Enrichments is nil")
	}
}

// TestStripReplayKeys ensures internal replay_* bookkeeping keys are removed
// before the journal metadata is merged back into EnrichmentMetadata on resume,
// so the large replay_enrichments JSON blob does not pollute the final event.
func TestStripReplayKeys(t *testing.T) {
	in := map[string]string{
		"status":             "success",
		"mode":               "description",
		"replay_completed":   "true",
		"replay_description": "some text",
		"replay_enrichments": `{"aiSummary":{"html":"x"}}`,
		"replay_name":        "Run",
	}

	out := stripReplayKeys(in)

	if out["status"] != "success" || out["mode"] != "description" {
		t.Errorf("expected provider metadata preserved, got %v", out)
	}
	for k := range out {
		if len(k) >= 7 && k[:7] == "replay_" {
			t.Errorf("replay_ key %q should have been stripped", k)
		}
	}
}
