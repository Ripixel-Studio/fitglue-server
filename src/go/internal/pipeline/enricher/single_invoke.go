// nolint:proto-json
package enricher

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Sentinel errors returned by SingleInvoker.InvokeSingle so callers (the gRPC service)
// can map them to the right status code without importing enricher internals.
var (
	// ErrProviderNotFound is returned when no enricher is registered under the given name.
	ErrProviderNotFound = errors.New("enricher provider not found")
	// ErrNotIndividuallyInvokable is returned for enrichers that cannot be safely re-run
	// out-of-band: non-idempotent enrichers (counters, external side-effects) and ones
	// that require user input (which the out-of-band, propose/accept flow cannot satisfy).
	ErrNotIndividuallyInvokable = errors.New("enricher is not individually invokable")
	// ErrRequiresInput is returned when an otherwise-invokable enricher paused for user
	// input during the out-of-band run.
	ErrRequiresInput = errors.New("enricher requires user input")
)

// SingleInvoker runs one registered enricher out-of-band (no pipeline, no destinations,
// no downstream publishing) against a persisted activity, producing a *proposed*
// EnricherRunLayer. It reuses the shared provider registry populated at init() so an
// individually-invoked enricher behaves exactly as it does inside the pipeline chain.
type SingleInvoker struct {
	svc    *bootstrap.Service
	logger *slog.Logger
}

// NewSingleInvoker builds a SingleInvoker. svc is handed to providers that implement
// SetService (AI, weather, geocoding, artifact-writing enrichers) exactly as the
// orchestrator does. logger may be nil, in which case slog.Default() is used.
func NewSingleInvoker(svc *bootstrap.Service, logger *slog.Logger) *SingleInvoker {
	if logger == nil {
		logger = slog.Default()
	}
	return &SingleInvoker{svc: svc, logger: logger.With("component", "enricher-invoke")}
}

// InvokeSingle runs the enricher registered under providerName against a clone of activity
// (which is never mutated) and returns the resulting proposed EnricherRunLayer. The layer
// carries the enricher's typed enrichments and its layerable field contribution — the same
// data the pipeline records for an in-chain run — so the caller can fold it into the
// derived activity on accept and attribute provenance.
//
// Guards: only idempotent, input-free enrichers are individually invokable. Non-idempotent
// enrichers are rejected with ErrNotIndividuallyInvokable up front; an enricher that pauses
// for input mid-run yields ErrRequiresInput. An unknown provider yields ErrProviderNotFound.
//
// A run that produced no layerable change and no typed enrichments returns (nil, nil): there
// is nothing worth proposing.
func (si *SingleInvoker) InvokeSingle(ctx context.Context, providerName string, activity *pbactivity.StandardizedActivity, userRec *user.Record) (*pbactivity.EnricherRunLayer, error) {
	provider, ok := providers.GetByName(providerName)
	if !ok {
		return nil, ErrProviderNotFound
	}

	// Reject enrichers whose side-effects must not repeat: running one out-of-band would
	// fire the side-effect at invoke time, before the user accepts — defeating the
	// propose/accept model. These stay pipeline-only.
	if ni, ok := provider.(providers.NonIdempotentProvider); ok && !ni.IsIdempotent() {
		return nil, ErrNotIndividuallyInvokable
	}

	if sp, ok := provider.(interface{ SetService(*bootstrap.Service) }); ok {
		sp.SetService(si.svc)
	}

	// Clone so the enricher's in-place mutations never touch the caller's activity.
	work := cloneActivity(activity)

	config := map[string]string{
		"external_id": work.GetExternalId(),
		"is_repost":   "false",
		"is_resume":   "false",
	}

	start := time.Now()
	execID := uuid.NewString()
	// doNotRetry=true: out-of-band runs cannot participate in the lag/retry queue, so ask
	// providers to return partial/success on transient data lag instead of a RetryableError.
	res, err := provider.Enrich(ctx, si.logger.With("provider", provider.Name()), work, userRec, config, true)
	duration := time.Since(start).Milliseconds()

	layer := &pbactivity.EnricherRunLayer{
		ProviderName: provider.Name(),
		ProviderType: provider.ProviderType().String(),
		ExecutionId:  execID,
		RunAt:        timestamppb.New(start),
		DurationMs:   duration,
	}

	if err != nil {
		if _, isWait := err.(*user_input.WaitForInputError); isWait {
			return nil, ErrRequiresInput
		}
		if retryErr, isRetry := err.(*providers.RetryableError); isRetry {
			// doNotRetry was set, so this is unusual; there is no out-of-band retry path.
			si.logger.Warn("enricher requested retry during out-of-band run", "provider", provider.Name(), "reason", retryErr.Reason)
			return nil, err
		}
		return nil, err
	}

	// Nothing produced (nil result, explicit skip, or a halt) → nothing to propose.
	if res == nil || res.Skipped || res.HaltPipeline {
		return nil, nil
	}

	contribution := enricherContribution(res)
	if contribution == nil && res.Enrichments == nil {
		return nil, nil
	}

	layer.Status = "SUCCESS"
	layer.Contribution = contribution
	layer.Enrichments = res.Enrichments
	if len(res.Metadata) > 0 {
		layer.Metadata = res.Metadata
	}
	return layer, nil
}
