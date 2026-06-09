package providers

import (
	"context"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// EnrichmentResult represents the outcome of an enrichment provider.
type EnrichmentResult struct {
	// Metadata overrides (if empty/unspecified, original is kept)
	ActivityType pbactivity.ActivityType
	Description  string

	// SectionHeader identifies this description as a replaceable section.
	// If set, uploaders in UPDATE mode will replace existing content
	// matching this header instead of appending.
	// Example: "🏃 Parkrun Results:"
	SectionHeader string

	Name       string
	NameSuffix string // Appended to the final name (e.g. " (#5)")
	Tags       []string

	// Raw Data Streams (for merging)
	HeartRateStream    []int
	PowerStream        []int
	PositionLatStream  []float64
	PositionLongStream []float64

	// TimeMarkers from enricher (e.g., exercise transitions from FIT file uploads)
	TimeMarkers []*pbactivity.TimeMarker

	// Dedicated UI structure for complex hybrid races
	HybridRaceSummary *pbactivity.HybridRaceSummary

	// Artifacts (Providers can still generate specific artifacts if independent)
	// But main FIT generation should normally happen in Orchestrator fan-in.
	FitFileContent []byte

	// Extra metadata to append
	Metadata map[string]string

	// HaltPipeline signals the orchestrator to stop processing this pipeline.
	// Not a failure - the activity is intentionally skipped (e.g., filtered out).
	HaltPipeline bool
	HaltReason   string // Human-readable reason for logging/display

	// Skipped signals that the provider ran but decided not to apply any enrichment.
	// Unlike HaltPipeline, the pipeline continues normally with the next enricher.
	Skipped    bool
	SkipReason string // Human-readable reason for logging/display

	// ExcludeEnrichers is a list of downstream provider types that should be
	// explicitly skipped during this pipeline run. This allows an upstream
	// provider (e.g. hybrid_race_tagger) to securely shape the execution
	// environment without downstream plugins needing defensive logic.
	ExcludeEnrichers []pbplugin.EnricherProviderType

	// Enrichments holds the typed enricher outputs for this result.
	// The orchestrator deep-merges non-nil sub-fields from all provider results
	// into EnrichedActivityEvent.Enrichments. Providers that don't produce a
	// particular sub-message leave it nil; the orchestrator skips nil fields.
	Enrichments *pbactivity.ActivityEnrichments
}

// Provider defines the interface for an enrichment service.
type Provider interface {
	// Name returns the unique identifier for the provider (e.g., "fitbit-hr", "ai-description").
	Name() string

	// ProviderType returns the protobuf enum type for this provider
	ProviderType() pbplugin.EnricherProviderType

	// Enrich applies the logic to the activity.
	// logger is the structured logger from FrameworkContext for debug/info logging.
	// inputConfig contains the user-specific input parameters for this provider.
	// doNotRetry indicates if the provider should return partial/success data instead of RetryableError on lag.
	Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*EnrichmentResult, error)
}

// ResumableProvider is an optional interface for providers that support resume mode.
// When the orchestrator is in resume mode and the provider is in the resume_only_enrichers list,
// if the provider implements this interface, EnrichResume will be called instead of Enrich.
// This allows providers to apply resolved pending input data directly.
type ResumableProvider interface {
	Provider
	// EnrichResume is called during resume mode to apply resolved pending input data.
	// The pendingInput contains the resolved InputData from the background polling service.
	EnrichResume(ctx context.Context, activity *pbactivity.StandardizedActivity, user *user.Record, pendingInput *pbpipeline.PendingInput) (*EnrichmentResult, error)
}

// DeferrableProvider is an optional interface for providers that benefit from
// running after all other enrichers have completed (e.g., AI providers).
// The orchestrator defers their execution to Phase 2 but preserves their
// pipeline position for description ordering. Deferred providers receive
// an "enriched_description" key in their config containing the accumulated
// description from all non-deferred enrichers.
type DeferrableProvider interface {
	Provider
	// ShouldDefer returns true if this provider should be deferred to Phase 2.
	ShouldDefer() bool
}

// SupportsNonBlocking marks an enricher that can run in non-blocking mode when
// its EnricherConfig.NonBlocking flag is set. Instead of halting the pipeline on
// a WaitForInputError, the orchestrator continues, runs destinations normally, and
// re-runs only this enricher (via EnrichResume) when the user submits input —
// updating destinations rather than creating new activities.
// Enrichers that implement this interface MUST also implement ResumableProvider.
type SupportsNonBlocking interface {
	Provider
}

// NonIdempotentProvider marks an enricher whose side-effects must not repeat
// within the same pipeline execution. When the orchestrator detects a resume
// and finds this enricher already completed in the stored execution journal,
// it skips the enricher call and replays the previously-stored mutations onto
// currentActivity instead.
//
// Implement this on any enricher that writes persistent state (counters,
// accumulated totals, external API calls with side-effects, etc.).
type NonIdempotentProvider interface {
	Provider
	// IsIdempotent returns false, signalling the orchestrator to skip and
	// replay this enricher rather than re-executing it on resume.
	IsIdempotent() bool
}
