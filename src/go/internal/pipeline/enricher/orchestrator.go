// nolint:proto-json
package enricher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	formatters "github.com/fitglue/server/src/go/pkg/types/formatters"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"

	fit "github.com/fitglue/server/src/go/pkg/domain/file_generators"
	"github.com/fitglue/server/src/go/pkg/domain/tier"

	"github.com/fitglue/server/src/go/pkg/framework"
	infrasentry "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"

	pendinginput "github.com/fitglue/server/src/go/pkg/pending_input"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/location_naming"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// temporarilyUnavailableEnrichers is a skip-list for enrichers that are awaiting API access.
// When an enricher is added here, it will be skipped during pipeline execution even if configured.
// Remove entries from this map once API access is granted and the enricher is ready.
var temporarilyUnavailableEnrichers = map[pbplugin.EnricherProviderType]bool{
	// Example: pb.EnricherProviderType_ENRICHER_PROVIDER_POLAR_TRACKS: true,
}

type Orchestrator struct {
	database        shared.Database
	storage         shared.BlobStore
	bucketName      string
	providersByName map[string]providers.Provider
	providersByType map[pbplugin.EnricherProviderType]providers.Provider
	publisher       shared.Publisher
	// geocode reverse-geocodes coordinates for the always-on implicit location step (see the
	// finalizer). Left nil by NewOrchestrator so unit tests never make external calls; the
	// production wiring in function.go sets it to location_naming.ReverseGeocode. When nil, the
	// implicit location carries coordinates only (no place name).
	geocode location_naming.GeocodeFunc
}

func NewOrchestrator(db shared.Database, storage shared.BlobStore, bucketName string, publisher shared.Publisher) *Orchestrator {
	return &Orchestrator{
		database:        db,
		storage:         storage,
		bucketName:      bucketName,
		providersByName: make(map[string]providers.Provider),
		providersByType: make(map[pbplugin.EnricherProviderType]providers.Provider),
		publisher:       publisher,
	}
}

func (o *Orchestrator) Register(p providers.Provider) {
	o.providersByName[p.Name()] = p
	if t := p.ProviderType(); t != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED {
		o.providersByType[t] = p
	}
}

// ProcessResult contains detailed information about the enrichment process
type ProcessResult struct {
	Events             []*pbevents.EnrichedActivityEvent
	ProviderExecutions []ProviderExecution
	Status             pbpipeline.ExecutionStatus
}

// ProviderExecution tracks a single provider's execution
type ProviderExecution struct {
	ProviderName string
	ExecutionID  string
	Status       string
	Error        string
	DurationMs   int64
	Metadata     map[string]string
}

// Process executes the enrichment pipelines for the activity
func (o *Orchestrator) Process(ctx context.Context, logger *slog.Logger, payload *pbevents.ActivityPayload, parentExecutionID string, basePipelineExecutionID string, doNotRetry bool) (*ProcessResult, error) {
	// 1. Fetch User Config
	userRec, err := o.database.GetUser(ctx, payload.UserId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user config: %w", err)
	}

	// 1.1. Check Tier Limits
	if tier.ShouldResetSyncCount(userRec) {
		// Reset monthly counter
		if err := o.database.ResetSyncCount(ctx, payload.UserId); err != nil {
			logger.Warn("Failed to reset sync count", "error", err, "userId", payload.UserId)
		}
		userRec.SyncCountThisMonth = 0
	}

	allowed, reason := tier.CanSync(userRec)
	if !allowed {
		logger.Info("Sync blocked by tier limit", "userId", payload.UserId, "reason", reason)
		// Track prevented sync
		if err := o.database.IncrementPreventedSyncCount(ctx, payload.UserId); err != nil {
			logger.Warn("Failed to increment prevented sync count", "error", err, "userId", payload.UserId)
		}

		// Create a visible TIER_BLOCKED PipelineRun so user sees the blocked activity
		// and can be prompted to upgrade
		if payload.StandardizedActivity != nil &&
			len(payload.StandardizedActivity.Sessions) > 0 &&
			payload.PipelineId != nil && *payload.PipelineId != "" {

			activity := payload.StandardizedActivity
			activityId := uuid.NewString()

			blockedRun := &pbpipeline.PipelineRun{
				Id:               basePipelineExecutionID,
				PipelineId:       *payload.PipelineId,
				ActivityId:       activityId,
				Source:           payload.Source.String(),
				SourceActivityId: activity.GetExternalId(),
				Title:            activity.GetName(),
				Description:      activity.GetDescription(),
				Type:             activity.GetType(),
				StartTime:        activity.GetSessions()[0].GetStartTime(),
				Status:           pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_TIER_BLOCKED,
				StatusMessage:    &reason,
				CreatedAt:        timestamppb.Now(),
				UpdatedAt:        timestamppb.Now(),
				Destinations:     []*pbpipeline.DestinationOutcome{}, // No destinations for blocked runs
			}

			if err := o.database.CreatePipelineRun(ctx, payload.UserId, blockedRun); err != nil {
				logger.Warn("Failed to create tier-blocked pipeline run", "error", err)
			} else {
				logger.Info("Created tier-blocked pipeline run", "pipeline_run_id", blockedRun.Id, "activity_id", activityId)
			}
		}

		return &ProcessResult{
			Events:             []*pbevents.EnrichedActivityEvent{},
			ProviderExecutions: []ProviderExecution{},
			Status:             pbpipeline.ExecutionStatus_STATUS_SKIPPED,
		}, nil // Return nil error - this is a controlled halt, not an exception
	}

	// 1.5. Validate Payload
	if payload.StandardizedActivity == nil {
		return nil, framework.NewTerminalError("standardized activity is nil")
	}
	if len(payload.StandardizedActivity.Sessions) != 1 {
		logger.Error("Activity does not have exactly one session", "count", len(payload.StandardizedActivity.Sessions))
		return nil, framework.NewTerminalError("multiple sessions not supported")
	}
	if payload.StandardizedActivity.Sessions[0].TotalElapsedTime == 0 {
		logger.Error("Activity session has 0 elapsed time")
		return nil, framework.NewTerminalError("session total elapsed time is 0")
	}

	// 2. MANDATORY: Pipeline ID is required (Rule E25: Per-Pipeline Isolation via Splitter)
	// The enricher ONLY receives targeted messages from the pipeline-splitter.
	// Each invocation processes exactly one pipeline with clean memory and a dedicated trace.
	if payload.PipelineId == nil || *payload.PipelineId == "" {
		logger.Error("pipeline_id is required - enricher only accepts targeted messages from splitter")
		return nil, framework.NewTerminalError("pipeline_id is required")
	}

	pipelineID := *payload.PipelineId
	logger.Info("Processing targeted pipeline", "pipeline_id", pipelineID, "is_resume", payload.IsResume)

	// 2.1 Resolve the targeted pipeline by ID
	pipeline, err := o.resolvePipeline(ctx, pipelineID, userRec.UserId, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve pipeline: %w", err)
	}
	if pipeline == nil {
		logger.Error("Targeted pipeline not found or disabled", "pipeline_id", pipelineID)
		return &ProcessResult{
			Events:             []*pbevents.EnrichedActivityEvent{},
			ProviderExecutions: []ProviderExecution{},
			Status:             pbpipeline.ExecutionStatus_STATUS_SKIPPED,
		}, nil
	}

	// 2.2 Handle Resume Mode flags
	isResumeMode := payload.IsResume
	useUpdateMethod := payload.UseUpdateMethod

	if isResumeMode {
		logger.Info("Resume mode activated",
			"use_update_method", useUpdateMethod,
			"resume_pending_input_id", payload.ResumePendingInputId,
			"pipeline_id", pipelineID)
	}

	var providerExecutions []ProviderExecution

	// 3. Execute the Pipeline (Single Pipeline Mode)
	// Note: basePipelineExecutionID already contains the pipeline ID (appended by pipeline-splitter)
	pipelineExecutionID := basePipelineExecutionID
	logger.Info("Executing pipeline", "id", pipeline.ID, "pipelineExecutionId", pipelineExecutionID)

	// Load the existing pipeline run once. Used to:
	//  (a) guard against re-running a cancelled pipeline (Pub/Sub redelivery)
	//  (b) build the replay journal for non-idempotent enrichers on resume
	completedJournal := map[string]map[string]string{} // provider name → replay metadata
	if existingRun, runErr := o.database.GetPipelineRun(ctx, payload.UserId, pipelineExecutionID); runErr == nil && existingRun != nil {
		if existingRun.Status == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_CANCELLED {
			logger.Info("Pipeline run already cancelled — skipping enrichment", "pipeline_execution_id", pipelineExecutionID)
			return &ProcessResult{
				Events:             []*pbevents.EnrichedActivityEvent{},
				ProviderExecutions: nil,
				Status:             pbpipeline.ExecutionStatus_STATUS_UNSPECIFIED,
			}, nil
		}
		if isResumeMode {
			for _, b := range existingRun.GetBoosters() {
				if b.GetMetadata()["replay_completed"] == "true" {
					completedJournal[b.GetProviderName()] = b.GetMetadata()
				}
			}
			logger.Info("Resume mode: loaded enricher journal", "completed_count", len(completedJournal))
		}
	}

	// Pre-generate ActivityId so enrichers can use it for pending input linking
	// In resume mode, use the provided ActivityId; otherwise generate a new one
	var activityId string
	if isResumeMode {
		if payload.ActivityId == nil || *payload.ActivityId == "" {
			return nil, fmt.Errorf("resume mode requires activity_id to be set")
		}
		activityId = *payload.ActivityId
	} else {
		activityId = uuid.NewString()
	}
	logger.Debug("Activity ID for pipeline", "activity_id", activityId, "is_resume", isResumeMode)

	activeDestinations := pipeline.Destinations
	if payload.IsRepost && payload.RepostMode != "full-pipeline" && payload.RepostDestination != "" {
		if dest := formatters.ParseDestination(payload.RepostDestination); dest != pbplugin.DestinationType_DESTINATION_UNSPECIFIED {
			activeDestinations = []pbplugin.DestinationType{dest}
			logger.Info("Targeted repost mode active, filtered destinations", "repost_mode", payload.RepostMode, "repost_destination", payload.RepostDestination)
		} else {
			logger.Warn("Invalid repost destination provided", "repost_destination", payload.RepostDestination)
		}
	}

	// Create initial pipeline run document for lifecycle tracking (RUNNING status)
	// This ensures we track the pipeline execution even if it fails partway through
	o.createInitialPipelineRun(ctx, logger, payload.UserId, pipelineExecutionID, pipeline.ID, activityId, payload, activeDestinations, isResumeMode)

	// Upload original payload to GCS for Magic Actions (retry/repost) BEFORE any mutations
	// This ensures the stored payload has the clean original description (Rule E22: Reset-on-Repost)
	originalPayloadUri := ""
	if o.storage != nil && o.bucketName != "" {
		payloadPath := fmt.Sprintf("payloads/%s/%s.json", payload.UserId, activityId)
		payloadBytes, err := protojson.Marshal(payload)
		if err != nil {
			logger.Warn("Failed to marshal original payload for GCS", "error", err)
		} else if err := o.storage.Write(ctx, o.bucketName, payloadPath, payloadBytes); err != nil {
			logger.Warn("Failed to upload original payload to GCS", "error", err)
		} else {
			originalPayloadUri = fmt.Sprintf("gs://%s/%s", o.bucketName, payloadPath)
			logger.Debug("Uploaded original payload to GCS", "uri", originalPayloadUri)

			// Update pipeline run with GCS URI immediately so it's available even if pipeline fails early
			// This ensures full-pipeline repost can always retrieve the original payload
			if err := o.database.UpdatePipelineRun(ctx, payload.UserId, pipelineExecutionID, map[string]interface{}{
				"original_payload_uri": originalPayloadUri,
			}); err != nil {
				logger.Warn("Failed to update pipeline run with original payload URI", "error", err)
			}
		}
	}

	// 3a. Execute Enrichers Sequentially
	configs := pipeline.Enrichers
	results := make([]*providers.EnrichmentResult, len(configs))

	// Use the activity directly - no cloning needed since we process exactly one pipeline
	currentActivity := payload.StandardizedActivity

	// Save the original description and build enriched description separately
	// to prevent stacking across reposts.
	// Use slot-based description to preserve pipeline ordering when deferred enrichers
	// are executed out of order (Phase 2). Each enricher writes to its pipeline index.
	originalDescription := currentActivity.Description
	descriptionSlots := make([]string, len(configs)+1) // +1 for original description slot
	if originalDescription != "" {
		descriptionSlots[0] = originalDescription
	}

	// Collect deferred enrichers during Phase 1 for Phase 2 execution
	type deferredEnricher struct {
		index    int
		cfg      configuredEnricher
		provider providers.Provider
	}

	var deferredEnrichers []deferredEnricher

	// Non-blocking enrichers that raised WaitForInputError — pipeline continues without them.
	// Their pending input IDs are attached to the event so the destination service records
	// them on the PipelineRun and sets SYNCED_WITH_PENDING status.
	var nonBlockingPendingIDs []string

	// Map to track excluded downstream enrichers (type -> excluder name)
	excludedEnrichers := make(map[pbplugin.EnricherProviderType]string)

	// ---- Phase 1: Execute non-deferred enrichers, collect deferred ones ----
	for i, cfg := range configs {
		var provider providers.Provider
		var ok bool

		// Lookup by Type
		provider, ok = o.providersByType[cfg.ProviderType]
		if !ok {
			logger.Warn("Provider not found for type", "type", cfg.ProviderType)
			// Send Sentry warning - this is a configuration issue that should be investigated
			infrasentry.CaptureMessage(
				fmt.Sprintf("Enricher provider not registered: %s", cfg.ProviderType),
				"warning",
				map[string]interface{}{
					"provider_type": cfg.ProviderType.String(),
					"pipeline_id":   pipeline.ID,
					"user_id":       payload.UserId,
				},
				logger,
			)
			providerExecutions = append(providerExecutions, ProviderExecution{
				ProviderName: fmt.Sprintf("TYPE:%s", cfg.ProviderType),
				Status:       "SKIPPED",
				Error:        "provider not registered",
			})
			continue
		}

		// Skip temporarily unavailable enrichers
		if temporarilyUnavailableEnrichers[cfg.ProviderType] {
			logger.Info("Skipping temporarily unavailable enricher", "type", cfg.ProviderType, "name", provider.Name())
			providerExecutions = append(providerExecutions, ProviderExecution{
				ProviderName: provider.Name(),
				Status:       "SKIPPED",
				Error:        "temporarily unavailable",
				Metadata:     map[string]string{"skip_reason": "temporarily_unavailable"},
			})
			continue
		}

		// Skip explicitly excluded enrichers by upstream providers
		if reason, excluded := excludedEnrichers[cfg.ProviderType]; excluded {
			logger.Info("Skipping explicitly excluded enricher", "type", cfg.ProviderType, "name", provider.Name(), "reason", reason)
			providerExecutions = append(providerExecutions, ProviderExecution{
				ProviderName: provider.Name(),
				Status:       "SKIPPED",
				Metadata:     map[string]string{"skip_reason": fmt.Sprintf("excluded_by_upstream: %s", reason)},
			})
			continue
		}

		// 3a.1 Resume Mode: replay non-idempotent enrichers that already completed.
		// This prevents stateful side-effects (counter increments, accumulated totals,
		// external API calls) from running twice when a pipeline resumes after a
		// WaitForInputError. Pure/idempotent enrichers are not in completedJournal and
		// will run normally.
		if isResumeMode {
			if replayMeta, alreadyRan := completedJournal[provider.Name()]; alreadyRan {
				if ni, ok := provider.(providers.NonIdempotentProvider); ok && !ni.IsIdempotent() {
					if v := replayMeta["replay_name"]; v != "" {
						currentActivity.Name = v
					}
					if v := replayMeta["replay_name_suffix"]; v != "" {
						currentActivity.Name += v
					}
					if v := replayMeta["replay_activity_type"]; v != "" {
						if n, err := strconv.ParseInt(v, 10, 32); err == nil && n != 0 {
							currentActivity.Type = pbactivity.ActivityType(int32(n))
						}
					}
					if v := replayMeta["replay_tags"]; v != "" {
						var tags []string
						if json.Unmarshal([]byte(v), &tags) == nil && len(tags) > 0 {
							currentActivity.Tags = append(currentActivity.Tags, tags...)
						}
					}
					if v := replayMeta["replay_description"]; v != "" {
						descriptionSlots[i+1] = v
					}
					// Restore typed enrichments (e.g. AiSummary) so the final merge
					// loop re-attaches them to finalEvent.Enrichments. Populating
					// results[i] also re-adds this enricher to AppliedEnrichments.
					if v := replayMeta["replay_enrichments"]; v != "" {
						restored := &pbactivity.ActivityEnrichments{}
						if err := protojson.Unmarshal([]byte(v), restored); err != nil {
							logger.Warn("Resume mode: failed to unmarshal replayed enrichments", "provider", provider.Name(), "error", err)
						} else {
							results[i] = &providers.EnrichmentResult{
								Enrichments: restored,
								Metadata:    stripReplayKeys(replayMeta),
							}
						}
					}
					// Restore a previously-fetched heart rate stream rather than letting the
					// provider re-run against the clean pre-enrichment payload (which has no
					// existing heart rate data, so its own "already enriched" skip guard never
					// triggers on resume).
					if v := replayMeta["replay_heart_rate_stream"]; v != "" {
						var stream []int
						if err := json.Unmarshal([]byte(v), &stream); err != nil {
							logger.Warn("Resume mode: failed to unmarshal replayed heart rate stream", "provider", provider.Name(), "error", err)
						} else if len(stream) > 0 {
							applyEnrichmentStreams(currentActivity, &providers.EnrichmentResult{HeartRateStream: stream})
							if results[i] == nil {
								results[i] = &providers.EnrichmentResult{
									HeartRateStream: stream,
									Metadata:        stripReplayKeys(replayMeta),
								}
							} else {
								results[i].HeartRateStream = stream
							}
						}
					}
					providerExecutions = append(providerExecutions, ProviderExecution{
						ProviderName: provider.Name(),
						Status:       "REPLAYED",
						Metadata:     replayMeta,
					})
					logger.Info("Resume mode: replayed non-idempotent enricher", "provider", provider.Name())
					continue
				}
			}
		}

		// 3a.2 Deferred Execution: Collect deferrable providers for Phase 2
		if deferrable, isDeferrable := provider.(providers.DeferrableProvider); isDeferrable && deferrable.ShouldDefer() && !isResumeMode {
			logger.Info("Deferring enricher to Phase 2", "name", provider.Name(), "index", i)
			deferredEnrichers = append(deferredEnrichers, deferredEnricher{
				index:    i,
				cfg:      cfg,
				provider: provider,
			})
			continue
		}

		startTime := time.Now()
		execID := uuid.NewString()

		pe := ProviderExecution{
			ProviderName: provider.Name(),
			ExecutionID:  execID,
			Status:       "STARTED",
		}

		// Merge pipelineExecutionID, pipelineID, and activityId into config for providers
		enricherConfig := make(map[string]string)
		for k, v := range cfg.TypedConfig {
			enricherConfig[k] = v
		}
		enricherConfig["pipeline_execution_id"] = pipelineExecutionID
		enricherConfig["pipeline_id"] = pipeline.ID
		enricherConfig["activity_id"] = activityId                         // For pending input linking
		enricherConfig["external_id"] = currentActivity.GetExternalId()    // For same-source dedup
		enricherConfig["is_repost"] = strconv.FormatBool(payload.IsRepost) // For repost guards
		enricherConfig["is_resume"] = strconv.FormatBool(isResumeMode)     // Re-arm data-lag retries on resume

		// Clear stale pending inputs when re-running (not resuming)
		// This allows users to provide different input on a fresh re-run.
		if !isResumeMode {
			staleInputID := pendinginput.GenerateID(currentActivity.Source.String(), currentActivity.ExternalId, provider.Name())
			existingInput, fetchErr := o.database.GetPendingInput(ctx, payload.UserId, staleInputID)
			if fetchErr == nil && existingInput != nil && existingInput.Status == pbpipeline.PendingInput_STATUS_WAITING {
				logger.Info("Clearing stale pending input for re-run", "provider", provider.Name(), "pending_input_id", staleInputID)
				if delErr := o.database.DeletePendingInput(ctx, payload.UserId, staleInputID); delErr != nil {
					logger.Warn("Failed to delete stale pending input", "error", delErr, "pending_input_id", staleInputID)
				}
			}
		}

		// Execute
		// TODO: Get logger from FrameworkContext when orchestrator is refactored
		providerLogger := logger.With("provider", provider.Name())

		var res *providers.EnrichmentResult
		var err error

		// Resume Mode: Check if provider supports EnrichResume and we have a pending input to resolve
		if isResumeMode && payload.ResumePendingInputId != nil && *payload.ResumePendingInputId != "" {
			if resumable, ok := provider.(providers.ResumableProvider); ok {
				// Fetch the resolved pending input from database
				pendingInput, fetchErr := o.database.GetPendingInput(ctx, payload.UserId, *payload.ResumePendingInputId)
				if fetchErr != nil {
					logger.Warn("Failed to fetch pending input for resume", "error", fetchErr, "pending_input_id", *payload.ResumePendingInputId)
					// Fall back to regular Enrich
					res, err = provider.Enrich(ctx, providerLogger, currentActivity, userRec, enricherConfig, doNotRetry)
				} else if pendingInput == nil || pendingInput.Status != pbpipeline.PendingInput_STATUS_COMPLETED {
					logger.Warn("Pending input not found or not completed", "pending_input_id", *payload.ResumePendingInputId, "status", pendingInput.GetStatus())
					// Fall back to regular Enrich
					res, err = provider.Enrich(ctx, providerLogger, currentActivity, userRec, enricherConfig, doNotRetry)
				} else if pendingInput.EnricherProviderId != provider.Name() {
					// The resolved pending input belongs to a different provider — use regular
					// Enrich so this provider can run normally (or raise its own WaitForInputError).
					logger.Debug("Pending input belongs to different provider, using regular Enrich",
						"provider", provider.Name(), "pending_input_provider", pendingInput.EnricherProviderId)
					res, err = provider.Enrich(ctx, providerLogger, currentActivity, userRec, enricherConfig, doNotRetry)
				} else {
					// Call EnrichResume with the resolved pending input
					logger.Info("Calling EnrichResume with resolved pending input", "provider", provider.Name(), "pending_input_id", *payload.ResumePendingInputId)
					res, err = resumable.EnrichResume(ctx, currentActivity, userRec, pendingInput)
				}
			} else {
				// Provider doesn't support resume mode, use regular Enrich
				res, err = provider.Enrich(ctx, providerLogger, currentActivity, userRec, enricherConfig, doNotRetry)
			}
		} else {
			// Normal mode: call regular Enrich
			res, err = provider.Enrich(ctx, providerLogger, currentActivity, userRec, enricherConfig, doNotRetry)
		}
		duration := time.Since(startTime).Milliseconds()
		pe.DurationMs = duration

		if err != nil {
			// Check for expected control flow errors BEFORE logging at ERROR level
			// to prevent Sentry from capturing them as exceptions.
			if retryErr, ok := err.(*providers.RetryableError); ok {
				logger.Info(fmt.Sprintf("Provider requires retry: %v", provider.Name()), "name", provider.Name(), "reason", retryErr.Reason, "retry_after", retryErr.RetryAfter, "duration_ms", duration, "execution_id", execID)
				pe.Status = "RETRY"
				pe.Error = retryErr.Reason
				pe.Metadata = map[string]string{
					"retry_after":  retryErr.RetryAfter.String(),
					"retry_reason": retryErr.Reason,
				}
				providerExecutions = append(providerExecutions, pe)
				// Keep RUNNING status - retry is in progress, will be retried automatically
				o.updatePipelineRunStatus(ctx, logger, payload.UserId, pipelineExecutionID,
					pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING,
					fmt.Sprintf("Retry scheduled: %s", retryErr.Reason),
					providerExecutions)
				return &ProcessResult{
					Events:             []*pbevents.EnrichedActivityEvent{},
					ProviderExecutions: providerExecutions,
					Status:             pbpipeline.ExecutionStatus_STATUS_LAGGED_RETRY,
				}, retryErr
			}
			if waitErr, ok := err.(*user_input.WaitForInputError); ok {
				_, supportsNonBlocking := provider.(providers.SupportsNonBlocking)
				if supportsNonBlocking && cfg.NonBlocking {
					// Non-blocking: create the pending input, continue the pipeline.
					// Destinations run normally; this enricher's output arrives later via Update().
					logger.Info("Non-blocking enricher deferred pending user input",
						"provider", provider.Name(),
						"activity_id", waitErr.ActivityID,
						"duration_ms", duration,
					)
					pendingID := o.createNonBlockingPendingInput(ctx, logger, payload, waitErr, activityId, originalPayloadUri)
					if pendingID != "" {
						nonBlockingPendingIDs = append(nonBlockingPendingIDs, pendingID)
					}
					pe.Status = "WAITING_NON_BLOCKING"
					pe.Metadata = map[string]string{
						"activity_id":     waitErr.ActivityID,
						"required_fields": strings.Join(waitErr.RequiredFields, ","),
						"non_blocking":    "true",
					}
					providerExecutions = append(providerExecutions, pe)
					continue
				}

				logger.Info(fmt.Sprintf("Provider waiting for user input: %v", provider.Name()), "name", provider.Name(), "activity_id", waitErr.ActivityID, "required_fields", waitErr.RequiredFields, "duration_ms", duration, "execution_id", execID)
				pe.Status = "WAITING"
				pe.Metadata = map[string]string{
					"activity_id":     waitErr.ActivityID,
					"required_fields": strings.Join(waitErr.RequiredFields, ","),
				}
				providerExecutions = append(providerExecutions, pe)
				// Update pipeline run to PENDING status - waiting for user input
				o.updatePipelineRunStatus(ctx, logger, payload.UserId, pipelineExecutionID,
					pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PENDING,
					buildPendingInputStatusMessage(waitErr),
					providerExecutions)
				// Write pending_input_id and any already-accumulated non-blocking IDs so the UI
				// can offer a cancel button and show all related pending inputs for this run.
				if err := o.database.UpdatePipelineRun(ctx, payload.UserId, pipelineExecutionID, map[string]interface{}{
					"pending_input_id":               waitErr.ActivityID,
					"non_blocking_pending_input_ids": nonBlockingPendingIDs,
				}); err != nil {
					logger.Warn("Failed to link pending input IDs to pipeline run", "error", err)
				}
				return o.handleWaitError(ctx, logger, payload, providerExecutions, waitErr, activityId, originalPayloadUri)
			}

			// This is a genuine error - log at ERROR level for Sentry capture
			logger.Error(fmt.Sprintf("Provider failed: %v", provider.Name()), "name", provider.Name(), "error", err, "duration_ms", duration, "execution_id", execID)
			pe.Status = "FAILED"
			pe.Error = err.Error()
			providerExecutions = append(providerExecutions, pe)

			// Update pipeline run to FAILED status
			o.updatePipelineRunStatus(ctx, logger, payload.UserId, pipelineExecutionID,
				pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_FAILED,
				fmt.Sprintf("Enricher failed: %s - %v", provider.Name(), err),
				providerExecutions)

			// Enqueue pipeline failure notification
			o.enqueuePipelineFailureNotification(logger, ctx, payload.UserId, activityId, currentActivity.Name, provider.Name())

			// Fail pipeline
			return &ProcessResult{
				Events:             []*pbevents.EnrichedActivityEvent{},
				ProviderExecutions: providerExecutions,
			}, fmt.Errorf("enricher failed: %s: %v", provider.Name(), err)
		}

		if res == nil {
			logger.Warn(fmt.Sprintf("Provider returned nil result: %v", provider.Name()), "name", provider.Name())
			pe.Status = "SKIPPED"
			pe.Error = "nil result"
			providerExecutions = append(providerExecutions, pe)
			continue
		}

		// Check if provider wants to halt the pipeline
		if res.HaltPipeline {
			logger.Info(fmt.Sprintf("Provider halted pipeline: %v", provider.Name()), "name", provider.Name(), "reason", res.HaltReason)
			pe.Status = "SKIPPED"
			pe.Metadata = res.Metadata
			if res.HaltReason != "" {
				pe.Metadata["halt_reason"] = res.HaltReason
			}
			providerExecutions = append(providerExecutions, pe)

			// Update pipeline run to SKIPPED status
			statusMsg := fmt.Sprintf("Pipeline halted by %s", provider.Name())
			if res.HaltReason != "" {
				statusMsg = fmt.Sprintf("Pipeline halted by %s: %s", provider.Name(), res.HaltReason)
			}
			o.updatePipelineRunStatus(ctx, logger, payload.UserId, pipelineExecutionID,
				pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SKIPPED,
				statusMsg,
				providerExecutions)

			// Skip remaining enrichers and don't publish events for this pipeline
			return &ProcessResult{
				Events:             []*pbevents.EnrichedActivityEvent{},
				ProviderExecutions: providerExecutions,
				Status:             pbpipeline.ExecutionStatus_STATUS_SKIPPED,
			}, nil
		}

		// Check if provider skipped (ran but didn't apply)
		if res.Skipped {
			logger.Info(fmt.Sprintf("Provider skipped: %v", provider.Name()), "name", provider.Name(), "reason", res.SkipReason, "duration_ms", duration)
			pe.Status = "SKIPPED"
			pe.Metadata = res.Metadata
			if res.SkipReason != "" {
				if pe.Metadata == nil {
					pe.Metadata = map[string]string{}
				}
				pe.Metadata["skip_reason"] = res.SkipReason
			}
			providerExecutions = append(providerExecutions, pe)
			continue
		}

		pe.Status = "SUCCESS"
		pe.Metadata = buildBoosterMetadata(res, provider)
		results[i] = res
		providerExecutions = append(providerExecutions, pe)

		logger.Info(fmt.Sprintf("Provider completed: %v", provider.Name()), "name", provider.Name(), "duration_ms", duration, "execution_id", execID)

		// Apply changes to currentActivity immediately so next provider sees them
		if res.Name != "" {
			currentActivity.Name = res.Name
		}
		if res.NameSuffix != "" {
			currentActivity.Name += res.NameSuffix
		}
		if res.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
			currentActivity.Type = res.ActivityType
		}
		if len(res.Tags) > 0 {
			currentActivity.Tags = append(currentActivity.Tags, res.Tags...)
		}
		if len(res.TimeMarkers) > 0 {
			currentActivity.TimeMarkers = append(currentActivity.TimeMarkers, res.TimeMarkers...)
		}
		if res.HybridRaceSummary != nil {
			currentActivity.HybridRaceSummary = res.HybridRaceSummary
		}
		// Transient location hint (set by location-pinner) so downstream enrichers
		// (weather, location-naming) can use it as a GPS fallback. Not written to
		// records, so it never appears in the generated FIT/uploaded track.
		if res.HintLocation != nil {
			currentActivity.HintLocation = res.HintLocation
		}

		// Apply description to slot (preserves pipeline ordering for deferred enrichers)
		logger.Debug(fmt.Sprintf("Applying description from provider: %v, length: %v", provider.Name(), len(res.Description)), "name", provider.Name())
		if res.Description != "" {
			trimmed := strings.TrimSpace(res.Description)
			if trimmed != "" {
				descriptionSlots[i+1] = trimmed // +1 because slot 0 is original description
			}
		}

		// Track downstream excluded enrichers
		if len(res.ExcludeEnrichers) > 0 {
			for _, pType := range res.ExcludeEnrichers {
				excludedEnrichers[pType] = provider.Name()
			}
		}

		// Apply stream data immediately to currentActivity so downstream enrichers can see it
		applyEnrichmentStreams(currentActivity, res)
	}

	// ---- Phase 2: Execute deferred enrichers with full context ----
	if len(deferredEnrichers) > 0 {
		// Build the Phase 1 accumulated description to inject into deferred enricher configs
		phase1Description := buildDescriptionFromSlots(descriptionSlots)
		logger.Info("Starting Phase 2: deferred enricher execution",
			"deferred_count", len(deferredEnrichers),
			"phase1_description_length", len(phase1Description),
		)

		for _, deferred := range deferredEnrichers {
			provider := deferred.provider
			cfg := deferred.cfg
			i := deferred.index

			// Skip explicitly excluded enrichers (even deferred ones)
			if reason, excluded := excludedEnrichers[cfg.ProviderType]; excluded {
				logger.Info("Skipping explicitly excluded deferred enricher", "type", cfg.ProviderType, "name", provider.Name(), "reason", reason)
				providerExecutions = append(providerExecutions, ProviderExecution{
					ProviderName: provider.Name(),
					Status:       "SKIPPED",
					Metadata:     map[string]string{"skip_reason": fmt.Sprintf("excluded_by_upstream: %s", reason)},
				})
				continue
			}

			startTime := time.Now()
			execID := uuid.NewString()

			pe := ProviderExecution{
				ProviderName: provider.Name(),
				ExecutionID:  execID,
				Status:       "STARTED",
			}

			// Build enricher config with injected enriched_description
			enricherConfig := make(map[string]string)
			for k, v := range cfg.TypedConfig {
				enricherConfig[k] = v
			}
			enricherConfig["pipeline_execution_id"] = pipelineExecutionID
			enricherConfig["pipeline_id"] = pipeline.ID
			enricherConfig["activity_id"] = activityId
			enricherConfig["enriched_description"] = phase1Description         // Phase 2 context injection
			enricherConfig["is_repost"] = strconv.FormatBool(payload.IsRepost) // For repost guards

			// Execute
			providerLogger := logger.With("provider", provider.Name(), "phase", "deferred")
			res, err := provider.Enrich(ctx, providerLogger, currentActivity, userRec, enricherConfig, doNotRetry)
			duration := time.Since(startTime).Milliseconds()
			pe.DurationMs = duration

			if err != nil {
				// Check for expected control flow errors
				if retryErr, ok := err.(*providers.RetryableError); ok {
					logger.Info(fmt.Sprintf("Deferred provider requires retry: %v", provider.Name()), "name", provider.Name(), "reason", retryErr.Reason)
					pe.Status = "RETRY"
					pe.Error = retryErr.Reason
					providerExecutions = append(providerExecutions, pe)
					o.updatePipelineRunStatus(ctx, logger, payload.UserId, pipelineExecutionID,
						pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING,
						fmt.Sprintf("Retry scheduled: %s", retryErr.Reason),
						providerExecutions)
					return &ProcessResult{
						Events:             []*pbevents.EnrichedActivityEvent{},
						ProviderExecutions: providerExecutions,
						Status:             pbpipeline.ExecutionStatus_STATUS_LAGGED_RETRY,
					}, retryErr
				}

				// Genuine error
				logger.Error(fmt.Sprintf("Deferred provider failed: %v", provider.Name()), "name", provider.Name(), "error", err, "duration_ms", duration)
				pe.Status = "FAILED"
				pe.Error = err.Error()
				providerExecutions = append(providerExecutions, pe)

				o.updatePipelineRunStatus(ctx, logger, payload.UserId, pipelineExecutionID,
					pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_FAILED,
					fmt.Sprintf("Enricher failed: %s - %v", provider.Name(), err),
					providerExecutions)

				return &ProcessResult{
					Events:             []*pbevents.EnrichedActivityEvent{},
					ProviderExecutions: providerExecutions,
				}, fmt.Errorf("enricher failed: %s: %v", provider.Name(), err)
			}

			if res == nil {
				logger.Warn(fmt.Sprintf("Deferred provider returned nil result: %v", provider.Name()))
				pe.Status = "SKIPPED"
				pe.Error = "nil result"
				providerExecutions = append(providerExecutions, pe)
				continue
			}

			// Check if deferred provider skipped
			if res.Skipped {
				logger.Info(fmt.Sprintf("Deferred provider skipped: %v", provider.Name()), "name", provider.Name(), "reason", res.SkipReason, "duration_ms", duration)
				pe.Status = "SKIPPED"
				pe.Metadata = res.Metadata
				if res.SkipReason != "" {
					if pe.Metadata == nil {
						pe.Metadata = map[string]string{}
					}
					pe.Metadata["skip_reason"] = res.SkipReason
				}
				providerExecutions = append(providerExecutions, pe)
				continue
			}

			pe.Status = "SUCCESS"
			pe.Metadata = res.Metadata
			results[i] = res
			providerExecutions = append(providerExecutions, pe)

			logger.Info(fmt.Sprintf("Deferred provider completed: %v", provider.Name()), "name", provider.Name(), "duration_ms", duration)

			// Apply mutations from deferred enricher
			if res.Name != "" {
				currentActivity.Name = res.Name
			}
			if res.NameSuffix != "" {
				currentActivity.Name += res.NameSuffix
			}
			if res.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
				currentActivity.Type = res.ActivityType
			}
			if len(res.Tags) > 0 {
				currentActivity.Tags = append(currentActivity.Tags, res.Tags...)
			}
			if len(res.TimeMarkers) > 0 {
				currentActivity.TimeMarkers = append(currentActivity.TimeMarkers, res.TimeMarkers...)
			}

			// Apply description to correct slot
			if res.Description != "" {
				trimmed := strings.TrimSpace(res.Description)
				if trimmed != "" {
					descriptionSlots[i+1] = trimmed
				}
			}
		}
	}

	// Post-enrichment: Reconcile TimeMarker labels with StrengthSet exercise names.
	// After all enrichers have run, the StrengthSets may have better names than
	// the generic FIT category-based labels on the TimeMarkers (e.g., from Hevy data).
	reconcileTimeMarkerLabels(currentActivity)

	brandingApplied := false
	// Run branding provider last (for non-paying users only)
	if brandingProvider, ok := o.providersByName["branding"]; ok && tier.ShouldShowBranding(userRec) {
		brandingLogger := logger.With("provider", "branding")
		brandingRes, err := brandingProvider.Enrich(ctx, brandingLogger, currentActivity, userRec, map[string]string{}, doNotRetry)
		if err != nil {
			logger.Warn("Branding provider failed", "error", err)
		} else if brandingRes != nil && brandingRes.Description != "" {
			logger.Debug(fmt.Sprintf("Applying description from provider: %v, length: %v", brandingProvider.Name(), len(brandingRes.Description)), "name", brandingProvider.Name(), "description", brandingRes.Description)
			trimmed := strings.TrimSpace(brandingRes.Description)
			if trimmed != "" {
				// Branding goes in the last slot (after all enrichers)
				descriptionSlots = append(descriptionSlots, trimmed)
				brandingApplied = true
			}
		}
	}

	// Build final description from slots
	finalDescription := buildDescriptionFromSlots(descriptionSlots)
	// Backfill mode (cmd/showcase-reboost): the source description is the
	// description the user already has on Strava/Hevy — for re-ingested history it
	// typically already contains the previously rendered booster text — so keep it
	// verbatim rather than stacking freshly generated sections on top of it.
	// Typed enrichments are unaffected; only the prose is pinned.
	if payload.GetMetadata()["backfill_verbatim_description"] == "true" {
		finalDescription = originalDescription
	}
	currentActivity.Description = finalDescription

	// Build final event structure (no Fan-In needed - currentActivity is already fully enriched)
	finalEvent := &pbevents.EnrichedActivityEvent{
		UserId:              payload.UserId,
		Source:              payload.Source,
		ActivityId:          activityId,      // Use pre-generated ID (or preserved resume ID)
		ActivityData:        currentActivity, // Already fully enriched
		ActivityType:        currentActivity.Type,
		Name:                currentActivity.Name,
		Description:         finalDescription,
		AppliedEnrichments:  []string{},
		EnrichmentMetadata:  make(map[string]string),
		Destinations:        activeDestinations,
		PipelineId:          pipeline.ID,
		PipelineExecutionId: &pipelineExecutionID,
		StartTime:           currentActivity.Sessions[0].StartTime,
	}

	// Attach non-blocking pending input IDs so the destination service can record them
	// on the PipelineRun and set SYNCED_WITH_PENDING status.
	if len(nonBlockingPendingIDs) > 0 {
		finalEvent.NonBlockingPendingInputIds = nonBlockingPendingIDs
	}

	// Resume Mode: Add update metadata
	if isResumeMode {
		if useUpdateMethod {
			finalEvent.EnrichmentMetadata["use_update_method"] = "true"
		}
		// Flag every resume so the destination executor refreshes destinations that
		// already succeeded on an earlier pass (with Update) instead of skipping them
		// via the redelivery idempotency guard. Without this, late-arriving non-blocking
		// data (e.g. photo uploads resolved after a separate blocking input synced the
		// pipeline) never reaches an already-created showcase/destination.
		finalEvent.EnrichmentMetadata["pipeline_resumed"] = "true"
	}

	// Propagate targeted-repost intent to the destination executor so it can bypass the
	// already-uploaded idempotency guard for an explicit retry/missed-destination Magic
	// Action (these reuse the original run, whose prior SUCCESS outcome would otherwise
	// make the repost a silent no-op).
	// Forward backfill_* hints to the destination service (e.g. the showcase
	// uploader's in-place target). Only this namespace is forwarded so arbitrary
	// caller metadata cannot collide with enricher/destination config keys.
	for k, v := range payload.GetMetadata() {
		if strings.HasPrefix(k, "backfill_") {
			finalEvent.EnrichmentMetadata[k] = v
		}
	}

	if payload.IsRepost {
		finalEvent.EnrichmentMetadata["is_repost"] = "true"
		if payload.RepostMode != "" {
			finalEvent.EnrichmentMetadata["repost_mode"] = payload.RepostMode
		}
	}

	// Same-Source Detection: When the activity's source platform matches a destination,
	// signal uploaders to overwrite title/description instead of section-based merge.
	// Use payload.Source (the actual webhook source) — correct for multi-source pipelines.
	sourceDestName := strings.ToLower(strings.TrimPrefix(payload.Source.String(), "SOURCE_"))
	for _, dest := range activeDestinations {
		destName := strings.ToLower(strings.TrimPrefix(dest.String(), "DESTINATION_"))
		if sourceDestName == destName {
			finalEvent.EnrichmentMetadata["same_source_destination_"+destName] = "true"
		}
	}

	// Build AppliedEnrichments list and merge metadata + typed enrichments from results
	for i, res := range results {
		if res == nil {
			continue
		}

		cfgName := configs[i].ProviderType.String()
		finalEvent.AppliedEnrichments = append(finalEvent.AppliedEnrichments, cfgName)

		// Merge metadata
		for k, v := range res.Metadata {
			finalEvent.EnrichmentMetadata[k] = v
		}

		// Propagate section header for replaceable description sections
		if res.SectionHeader != "" {
			finalEvent.EnrichmentMetadata["section_header_"+cfgName] = res.SectionHeader
		}

		// Merge typed enrichments: copy non-nil sub-messages from this result
		if res.Enrichments != nil {
			finalEvent.Enrichments = mergeEnrichments(finalEvent.Enrichments, res.Enrichments)
		}
	}

	// Ensure every GPS-tracked activity carries a location, independent of whether the
	// opt-in, title-generating location_naming enricher ran. Without this, activities that
	// have GPS (and often weather, which reads the same records) show no place on the
	// showcased activity page or in the roundup "where it happened" section, because
	// Enrichments.Location was only ever produced by location_naming. Best-effort and
	// idempotent: skipped when a location is already set, and never blocks the pipeline.
	if finalEvent.Enrichments == nil || finalEvent.Enrichments.Location == nil {
		if loc := location_naming.ResolveLocationSummary(ctx, logger, currentActivity, o.geocode); loc != nil {
			finalEvent.Enrichments = mergeEnrichments(finalEvent.Enrichments, &pbactivity.ActivityEnrichments{Location: loc})
			logger.Info("Attached implicit location to GPS-tracked activity",
				"location_name", loc.LocationName, "lat", loc.Latitude, "lng", loc.Longitude)
		}
	}

	// Add branding if it was applied
	if brandingApplied {
		finalEvent.AppliedEnrichments = append(finalEvent.AppliedEnrichments, "branding")
	}

	// Inject source config into metadata (with user default fallback)
	sourceConfig := pipeline.SourceConfig
	if len(sourceConfig) == 0 {
		// Fall back to user plugin default for this source
		sourcePluginId := strings.ToLower(strings.TrimPrefix(payload.Source.String(), "SOURCE_"))
		if def, err := o.database.GetPluginDefault(ctx, payload.UserId, sourcePluginId); err == nil && def != nil {
			sourceConfig = def.Config
			logger.Info("Using user default for source config", "plugin", sourcePluginId)
		}
	}
	for k, v := range sourceConfig {
		finalEvent.EnrichmentMetadata[k] = v
	}

	// Inject destination configs into metadata (prefixed with destination ID)
	// For each destination, merge pipeline config with user default (pipeline wins)
	// Track which destinations have been processed via DestinationConfigs
	processedDests := make(map[string]bool)
	for destId, destCfg := range pipeline.DestinationConfigs {
		processedDests[destId] = true
		if destCfg != nil && len(destCfg.Config) > 0 {
			for k, v := range destCfg.Config {
				finalEvent.EnrichmentMetadata[destId+"_"+k] = v
			}
		} else {
			// Fall back to user plugin default for this destination
			if def, err := o.database.GetPluginDefault(ctx, payload.UserId, destId); err == nil && def != nil {
				for k, v := range def.Config {
					finalEvent.EnrichmentMetadata[destId+"_"+k] = v
				}
				logger.Info("Using user default for destination config", "destination", destId)
			}
		}
	}

	// Also check activeDestinations for any destinations not in DestinationConfigs
	// These destinations have no per-pipeline config, so fall back to plugin_defaults
	for _, dest := range activeDestinations {
		destId := strings.ToLower(strings.TrimPrefix(dest.String(), "DESTINATION_"))
		if processedDests[destId] {
			continue // Already handled above
		}
		// Fall back to user plugin default
		if def, err := o.database.GetPluginDefault(ctx, payload.UserId, destId); err == nil && def != nil {
			for k, v := range def.Config {
				finalEvent.EnrichmentMetadata[destId+"_"+k] = v
			}
			logger.Info("Using user default for destination config (from Destinations list)", "destination", destId)
		}
	}

	// Generate FIT file artifact
	fitBytes, err := fit.GenerateFitFile(currentActivity)
	if err != nil {
		logger.Error("Failed to generate FIT file", "error", err) // Don't fail the whole event, just log
	} else if len(fitBytes) > 0 {
		objName := fmt.Sprintf("activities/%s/%s.fit", payload.UserId, finalEvent.ActivityId)
		if err := o.storage.Write(ctx, o.bucketName, objName, fitBytes); err != nil {
			logger.Error("Failed to write FIT file artifact", "error", err)
		} else {
			finalEvent.FitFileUri = fmt.Sprintf("gs://%s/%s", o.bucketName, objName)
		}
	}

	// Finalize PipelineRun with enriched data (initial run was created at start)
	o.finalizePipelineRun(ctx, logger, payload.UserId, finalEvent, providerExecutions, originalPayloadUri)

	// Note: Success/partial notifications are now sent by destination.UpdateStatus
	// when all destinations have reported their final status (SYNCED or PARTIAL).

	// --- Destination-specific enricher exclusions ---
	// Group destinations by their exclusion sets. Destinations with identical
	// ExcludedEnrichers lists share a single event; different sets get separate events
	// with filtered descriptions and appliedEnrichments.
	groups := groupDestinationsByExclusions(activeDestinations, pipeline.DestinationConfigs)

	if len(groups) <= 1 {
		// No exclusion diversity — all destinations get the same event (common case)
		return &ProcessResult{
			Events:             []*pbevents.EnrichedActivityEvent{finalEvent},
			ProviderExecutions: providerExecutions,
			Status:             pbpipeline.ExecutionStatus_STATUS_SUCCESS,
		}, nil
	}

	// Multiple exclusion groups — emit one event per group
	var events []*pbevents.EnrichedActivityEvent
	for exclusionKey, dests := range groups {
		if exclusionKey == "" {
			// Default group (no exclusions) — use the full event with narrowed destinations
			evt := cloneEnrichedEvent(finalEvent)
			evt.Destinations = dests
			events = append(events, evt)
			continue
		}

		// Build excluded set from the comma-separated key
		excludedSet := make(map[string]bool)
		for _, e := range strings.Split(exclusionKey, ",") {
			excludedSet[e] = true
		}

		// Build filtered description by zeroing excluded slots
		filteredSlots := make([]string, len(descriptionSlots))
		copy(filteredSlots, descriptionSlots)
		for i, cfg := range configs {
			if excludedSet[cfg.ProviderType.String()] {
				filteredSlots[i+1] = "" // Zero the excluded enricher's slot
			}
		}
		filteredDesc := buildDescriptionFromSlots(filteredSlots)

		// Filter appliedEnrichments
		var filteredApplied []string
		for _, ae := range finalEvent.AppliedEnrichments {
			if !excludedSet[ae] {
				filteredApplied = append(filteredApplied, ae)
			}
		}

		evt := cloneEnrichedEvent(finalEvent)
		evt.Description = filteredDesc
		if payload.GetMetadata()["backfill_verbatim_description"] == "true" {
			evt.Description = originalDescription
		}
		evt.AppliedEnrichments = filteredApplied
		evt.Destinations = dests
		events = append(events, evt)

		logger.Info("Emitting filtered event for destination group",
			"excluded", exclusionKey,
			"destinations", len(dests),
			"appliedEnrichments", len(filteredApplied))
	}

	return &ProcessResult{
		Events:             events,
		ProviderExecutions: providerExecutions,
		Status:             pbpipeline.ExecutionStatus_STATUS_SUCCESS,
	}, nil
}

// applyEnrichmentStreams writes an enricher's raw data streams (heart rate, power,
// GPS position) onto currentActivity's records by timestamp-based offset matching.
// Shared between live provider execution and resume-mode journal replay so a
// replayed stream (e.g. a previously-fetched Fitbit heart rate stream) lands on the
// activity the same way a freshly-computed one would.
func applyEnrichmentStreams(currentActivity *pbactivity.StandardizedActivity, res *providers.EnrichmentResult) {
	// Ensure Laps/Records exist
	enricherSession := currentActivity.Sessions[0]
	if len(enricherSession.Laps) == 0 {
		enricherSession.Laps = append(enricherSession.Laps, &pbactivity.Lap{
			StartTime:        enricherSession.StartTime,
			TotalElapsedTime: enricherSession.TotalElapsedTime,
			Records:          []*pbactivity.Record{},
		})
	}

	// Check if enricher provides any stream data that needs to be applied
	hasStreamData := len(res.HeartRateStream) > 0 || len(res.PowerStream) > 0 ||
		len(res.PositionLatStream) > 0 || len(res.PositionLongStream) > 0

	// Count total existing records across ALL laps to detect multi-lap activities
	// (e.g., from FIT file uploads where records are properly distributed)
	totalExistingRecords := 0
	for _, lap := range enricherSession.Laps {
		totalExistingRecords += len(lap.Records)
	}

	// Only expand Laps[0] with placeholder records if:
	// 1. An enricher provides stream data that needs to be applied, AND
	// 2. The activity doesn't already have substantial records (less than 25% coverage)
	//
	// This protects multi-lap FIT file uploads from having their rich record data
	// destroyed by placeholder expansion, while still supporting API-sourced activities
	// (e.g., Strava) where HR/power streams need to be applied to sparse records.
	enricherDuration := int(enricherSession.TotalElapsedTime)
	// Use max(duration/4, 1) to handle short durations properly
	threshold := enricherDuration / 4
	if threshold < 1 {
		threshold = 1
	}
	needsRecordExpansion := hasStreamData && totalExistingRecords < threshold

	if needsRecordExpansion {
		enricherLap := enricherSession.Laps[0]
		enricherCurrentLen := len(enricherLap.Records)
		if enricherCurrentLen < enricherDuration {
			enricherStartTime := enricherSession.StartTime.AsTime()
			for k := enricherCurrentLen; k < enricherDuration; k++ {
				ts := timestamppb.New(enricherStartTime.Add(time.Duration(k) * time.Second))
				enricherLap.Records = append(enricherLap.Records, &pbactivity.Record{Timestamp: ts})
			}
		}
	}

	// ALWAYS apply stream data when available - regardless of record expansion
	// For activities with existing records (like FIT files), apply to those records
	// For newly expanded activities, apply to the expanded placeholder records
	if !hasStreamData {
		return
	}

	// Apply stream data to ALL laps' records using timestamp-based matching
	// This handles both single-lap expanded activities and multi-lap FIT activities
	activityStart := enricherSession.StartTime.AsTime()

	for _, lap := range enricherSession.Laps {
		for _, record := range lap.Records {
			if record.Timestamp == nil {
				continue
			}
			// Calculate the second offset from activity start
			offsetSec := int(record.Timestamp.AsTime().Sub(activityStart).Seconds())
			if offsetSec < 0 {
				continue
			}

			// Apply HR stream value at this offset
			if len(res.HeartRateStream) > 0 && offsetSec < len(res.HeartRateStream) {
				val := res.HeartRateStream[offsetSec]
				if val > 0 {
					record.HeartRate = int32(val)
				}
			}

			// Apply Power stream value at this offset
			if len(res.PowerStream) > 0 && offsetSec < len(res.PowerStream) {
				val := res.PowerStream[offsetSec]
				if val > 0 {
					record.Power = int32(val)
				}
			}

			// Apply GPS position streams at this offset
			if len(res.PositionLatStream) > 0 && offsetSec < len(res.PositionLatStream) {
				record.PositionLat = res.PositionLatStream[offsetSec]
			}
			if len(res.PositionLongStream) > 0 && offsetSec < len(res.PositionLongStream) {
				record.PositionLong = res.PositionLongStream[offsetSec]
			}
		}
	}
}

// buildDescriptionFromSlots joins non-empty description slots with double newlines.
// This preserves pipeline ordering: each enricher's description appears at its
// configured position regardless of execution order (Phase 1 vs Phase 2).
func buildDescriptionFromSlots(slots []string) string {
	var parts []string
	for _, s := range slots {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

// groupDestinationsByExclusions groups destinations by their exclusion sets.
// Returns a map from exclusion key (sorted, comma-joined provider type strings) to destinations.
// An empty key means no exclusions (the default group).
func groupDestinationsByExclusions(destinations []pbplugin.DestinationType, destConfigs map[string]*pbpipeline.DestinationConfig) map[string][]pbplugin.DestinationType {
	groups := map[string][]pbplugin.DestinationType{}
	for _, dest := range destinations {
		destId := strings.ToLower(strings.TrimPrefix(dest.String(), "DESTINATION_"))
		cfg := destConfigs[destId]
		key := "" // empty = no exclusions
		if cfg != nil && len(cfg.ExcludedEnrichers) > 0 {
			sorted := make([]string, len(cfg.ExcludedEnrichers))
			copy(sorted, cfg.ExcludedEnrichers)
			sort.Strings(sorted)
			key = strings.Join(sorted, ",")
		}
		groups[key] = append(groups[key], dest)
	}
	return groups
}

// cloneEnrichedEvent creates a deep copy of an EnrichedActivityEvent using proto.Clone.
// ActivityData is shared (not deep-cloned) since only description text is filtered.
func cloneEnrichedEvent(src *pbevents.EnrichedActivityEvent) *pbevents.EnrichedActivityEvent {
	return proto.Clone(src).(*pbevents.EnrichedActivityEvent)
}

type configuredPipeline struct {
	ID                 string
	Source             string
	Enrichers          []configuredEnricher
	Destinations       []pbplugin.DestinationType
	SourceConfig       map[string]string
	DestinationConfigs map[string]*pbpipeline.DestinationConfig
}

type configuredEnricher struct {
	ProviderType pbplugin.EnricherProviderType
	TypedConfig  map[string]string
	NonBlocking  bool
}

// resolvePipeline looks up a single pipeline by ID from the user's pipelines collection.
// Returns nil if the pipeline is not found or is disabled.
func (o *Orchestrator) resolvePipeline(ctx context.Context, pipelineID string, userID string, logger *slog.Logger) (*configuredPipeline, error) {
	userPipelines, err := o.database.GetUserPipelines(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user pipelines: %w", err)
	}

	for _, p := range userPipelines {
		if p.Id == pipelineID {
			if p.Disabled {
				logger.Info("Targeted pipeline is disabled", "pipeline_id", p.Id, "name", p.Name)
				return nil, nil
			}

			var enrichers []configuredEnricher
			for _, e := range p.Enrichers {
				enrichers = append(enrichers, configuredEnricher{
					ProviderType: e.ProviderType,
					TypedConfig:  e.TypedConfig,
					NonBlocking:  e.NonBlocking,
				})
			}
			// Prefer Sources (array) over the legacy Source field; service.go clears
			// Source when saving, so p.Source is "" for all recently-created pipelines.
			pipelineSource := p.Source
			if len(p.Sources) > 0 {
				pipelineSource = p.Sources[0]
			}
			return &configuredPipeline{
				ID:                 p.Id,
				Source:             pipelineSource,
				Enrichers:          enrichers,
				Destinations:       p.Destinations,
				SourceConfig:       p.SourceConfig,
				DestinationConfigs: p.DestinationConfigs,
			}, nil
		}
	}

	return nil, nil // Pipeline not found
}

// createNonBlockingPendingInput creates a PendingInput record for a non-blocking enricher.
// Unlike handleWaitError, it does not halt the pipeline — the caller continues enrichment.
// Returns the stable pending input ID, or "" if creation failed (pipeline still continues).
func (o *Orchestrator) createNonBlockingPendingInput(ctx context.Context, logger *slog.Logger, payload *pbevents.ActivityPayload, waitErr *user_input.WaitForInputError, linkedActivityId string, originalPayloadUri string) string {
	// SAFETY CHECK: if the pending input already exists (e.g. Pub/Sub redelivery or a
	// resume triggered by another non-blocking enricher resolving), return the existing ID.
	existing, fetchErr := o.database.GetPendingInput(ctx, payload.UserId, waitErr.ActivityID)
	if fetchErr == nil && existing != nil {
		return waitErr.ActivityID
	}

	payloadUri := originalPayloadUri
	if payloadUri == "" {
		if o.storage != nil && o.bucketName != "" {
			payloadPath := fmt.Sprintf("payloads/%s/%s.json", payload.UserId, waitErr.ActivityID)
			payloadBytes, err := protojson.Marshal(payload)
			if err != nil {
				logger.Warn("Failed to marshal payload for non-blocking pending input", "error", err)
			} else if err := o.storage.Write(ctx, o.bucketName, payloadPath, payloadBytes); err != nil {
				logger.Warn("Failed to upload payload for non-blocking pending input", "error", err)
			} else {
				payloadUri = fmt.Sprintf("gs://%s/%s", o.bucketName, payloadPath)
			}
		}
	}

	pi := &pbpipeline.PendingInput{
		ActivityId:         waitErr.ActivityID,
		UserId:             payload.UserId,
		Status:             pbpipeline.PendingInput_STATUS_WAITING,
		RequiredFields:     waitErr.RequiredFields,
		OriginalPayloadUri: payloadUri,
		EnricherProviderId: waitErr.EnricherProviderID,
		NonBlocking:        true,
		CreatedAt:          timestamppb.Now(),
		UpdatedAt:          timestamppb.Now(),
		ProviderMetadata:   waitErr.Metadata,
		LinkedActivityId:   linkedActivityId,
		PipelineId:         *payload.PipelineId,
	}
	if act := payload.GetStandardizedActivity(); act != nil {
		pi.SourceDisplayName = act.Name
		pi.SourceActivityType = act.Type.String()
		pi.SourceStartTime = act.StartTime
		pi.SourceActivitySource = act.Source.String()
	}
	if err := o.database.CreatePendingInput(ctx, payload.UserId, pi); err != nil {
		logger.Warn("Failed to create non-blocking pending input", "error", err, "activity_id", waitErr.ActivityID)
		return ""
	}

	logger.Info("Created non-blocking pending input", "pending_id", waitErr.ActivityID, "provider", waitErr.EnricherProviderID)
	return waitErr.ActivityID
}

func (o *Orchestrator) handleWaitError(ctx context.Context, logger *slog.Logger, payload *pbevents.ActivityPayload, allExecs []ProviderExecution, waitErr *user_input.WaitForInputError, linkedActivityId string, originalPayloadUri string) (*ProcessResult, error) {
	logger.Warn("Provider requested user input", "activity_id", waitErr.ActivityID, "linked_activity_id", linkedActivityId)

	// SAFETY CHECK: Don't overwrite an existing pending input (completed or already waiting).
	// Pub/Sub redelivery can cause this handler to fire multiple times for the same activity —
	// re-creating a WAITING input resets its timestamps and interferes with user dismissal.
	existingInput, fetchErr := o.database.GetPendingInput(ctx, payload.UserId, waitErr.ActivityID)
	if fetchErr == nil && existingInput != nil {
		switch existingInput.Status {
		case pbpipeline.PendingInput_STATUS_COMPLETED:
			logger.Warn("Pending input already completed - skipping creation to prevent overwrite",
				"activity_id", waitErr.ActivityID)
			return &ProcessResult{
				Events:             []*pbevents.EnrichedActivityEvent{},
				ProviderExecutions: allExecs,
				Status:             pbpipeline.ExecutionStatus_STATUS_WAITING,
			}, nil
		case pbpipeline.PendingInput_STATUS_WAITING:
			// Still WAITING — user hasn't acted yet. Re-send the notification in case it was
			// missed on the first delivery (e.g. FCM was temporarily unavailable). The browser
			// deduplicates via the notification tag so this won't spam the user.
			logger.Info("Pending input already waiting - skipping duplicate creation, re-sending notification",
				"activity_id", waitErr.ActivityID)
			o.sendPendingInputNotification(ctx, logger, payload.UserId, waitErr.ActivityID)
			return &ProcessResult{
				Events:             []*pbevents.EnrichedActivityEvent{},
				ProviderExecutions: allExecs,
				Status:             pbpipeline.ExecutionStatus_STATUS_WAITING,
			}, nil
		}
	}

	// Use the pre-enrichment payload URI captured before any enrichers mutated the activity.
	// Re-serialising payload here would capture the already-mutated name (e.g. "(#24)" already
	// appended by auto_increment), causing it to be doubled on resume.
	payloadUri := originalPayloadUri
	if payloadUri == "" {
		// Fallback: upload current payload if no pre-enrichment URI is available.
		if o.storage != nil && o.bucketName != "" {
			payloadPath := fmt.Sprintf("payloads/%s/%s.json", payload.UserId, waitErr.ActivityID)
			payloadBytes, err := protojson.Marshal(payload)
			if err != nil {
				logger.Warn("Failed to marshal payload for GCS", "error", err)
			} else if err := o.storage.Write(ctx, o.bucketName, payloadPath, payloadBytes); err != nil {
				logger.Warn("Failed to upload payload to GCS", "error", err)
			} else {
				payloadUri = fmt.Sprintf("gs://%s/%s", o.bucketName, payloadPath)
				logger.Debug("Uploaded payload to GCS", "uri", payloadUri)
			}
		}
	}

	// Create Pending Input in DB
	pi := &pbpipeline.PendingInput{
		ActivityId:         waitErr.ActivityID,
		UserId:             payload.UserId,
		Status:             pbpipeline.PendingInput_STATUS_WAITING,
		RequiredFields:     waitErr.RequiredFields,
		OriginalPayloadUri: payloadUri, // GCS URI for payload retrieval
		EnricherProviderId: waitErr.EnricherProviderID,
		CreatedAt:          timestamppb.Now(),
		UpdatedAt:          timestamppb.Now(),
		ProviderMetadata:   waitErr.Metadata,    // Pass provider context to UI
		LinkedActivityId:   linkedActivityId,    // Activity ID for resume mode
		PipelineId:         *payload.PipelineId, // Pipeline that created this pending input
	}
	if act := payload.GetStandardizedActivity(); act != nil {
		pi.SourceDisplayName = act.Name
		pi.SourceActivityType = act.Type.String()
		pi.SourceStartTime = act.StartTime
		pi.SourceActivitySource = act.Source.String()
	}
	if err := o.database.CreatePendingInput(ctx, payload.UserId, pi); err != nil {
		logger.Warn("Failed to create pending input (might already exist)", "error", err)
	}

	o.sendPendingInputNotification(ctx, logger, payload.UserId, waitErr.ActivityID)

	return &ProcessResult{
		Events:             []*pbevents.EnrichedActivityEvent{},
		ProviderExecutions: allExecs,
		Status:             pbpipeline.ExecutionStatus_STATUS_WAITING,
	}, nil
}

// sendPendingInputNotification enqueues a PENDING_INPUT notification.
// Safe to call on retries — the browser deduplicates via the notification tag.
func (o *Orchestrator) sendPendingInputNotification(ctx context.Context, logger *slog.Logger, userID, activityID string) {
	if o.publisher == nil {
		logger.Warn("Publisher unavailable — PENDING_INPUT notification not sent", "user_id", userID)
		return
	}
	req := &pbnotification.NotificationRequest{
		UserId: userID,
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT,
		Title:  "Action Required: FitGlue",
		Body:   "An activity needs more information to be processed.",
		Data:   map[string]string{"activity_id": activityID},
	}
	if err := notificationpub.Enqueue(ctx, o.publisher, req); err != nil {
		logger.Error("Failed to enqueue pending input notification", "error", err, "user_id", userID)
	}
}

// enqueuePipelineFailureNotification enqueues a PIPELINE_FAILURE notification from the enricher.
func (o *Orchestrator) enqueuePipelineFailureNotification(logger *slog.Logger, ctx context.Context, userID, activityID, activityName, providerName string) {
	if o.publisher == nil {
		return
	}
	req := &pbnotification.NotificationRequest{
		UserId: userID,
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE,
		Title:  fmt.Sprintf("Activity Failed: %s", activityName),
		Body:   fmt.Sprintf("Enricher '%s' encountered an error", providerName),
		Data:   map[string]string{"activity_id": activityID},
	}
	if err := notificationpub.Enqueue(ctx, o.publisher, req); err != nil {
		logger.Warn("Failed to enqueue pipeline failure notification", "error", err, "user_id", userID)
	}
}

// createInitialPipelineRun creates a minimal PipelineRun document with RUNNING status
// Called early in the pipeline execution to ensure lifecycle tracking even if pipeline fails.
// In resume mode, only the status is reset to RUNNING — destination ExternalIds are preserved.
func (o *Orchestrator) createInitialPipelineRun(ctx context.Context, logger *slog.Logger, userId string, pipelineExecutionID string, pipelineID string, activityId string, payload *pbevents.ActivityPayload, destinations []pbplugin.DestinationType, isResume bool) {
	if isResume {
		// The pipeline run document already exists with valid destination ExternalIds from the
		// first run. Only reset status to RUNNING so the destination executor can call Update()
		// with the correct ExternalIds intact.
		if err := o.database.UpdatePipelineRun(ctx, userId, pipelineExecutionID, map[string]interface{}{
			"status":     int32(pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING),
			"updated_at": time.Now(),
		}); err != nil {
			logger.Warn("Failed to reset pipeline run status to RUNNING in resume mode", "error", err, "pipeline_run_id", pipelineExecutionID)
		} else {
			logger.Debug("Reset pipeline run status to RUNNING (resume mode)", "pipeline_run_id", pipelineExecutionID)
		}
		return
	}

	activity := payload.GetStandardizedActivity()

	// Build destination outcomes (all pending at this point)
	destOutcomes := make([]*pbpipeline.DestinationOutcome, 0, len(destinations))
	for _, dest := range destinations {
		destOutcomes = append(destOutcomes, &pbpipeline.DestinationOutcome{
			Destination: dest,
			Status:      pbpipeline.DestinationStatus_DESTINATION_STATUS_PENDING,
		})
	}

	pipelineRun := &pbpipeline.PipelineRun{
		Id:               pipelineExecutionID,
		PipelineId:       pipelineID,
		ActivityId:       activityId,
		Source:           payload.Source.String(),
		SourceActivityId: activity.GetExternalId(),
		Title:            activity.GetName(),
		Description:      activity.GetDescription(),
		Type:             activity.GetType(),
		StartTime:        activity.GetSessions()[0].GetStartTime(),
		Status:           pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING,
		CreatedAt:        timestamppb.Now(),
		UpdatedAt:        timestamppb.Now(),
		Destinations:     destOutcomes,
	}

	if err := o.database.CreatePipelineRun(ctx, userId, pipelineRun); err != nil {
		logger.Error("Failed to create initial pipeline run", "error", err, "pipeline_run_id", pipelineRun.Id)
	} else {
		logger.Debug("Created initial pipeline run", "pipeline_run_id", pipelineRun.Id, "activity_id", activityId)

		// Also write each destination outcome to the subcollection
		// This is required for the race-condition-free UpdateStatus pattern
		for _, outcome := range destOutcomes {
			if err := o.database.SetDestinationOutcome(ctx, userId, pipelineExecutionID, outcome); err != nil {
				logger.Error("Failed to create initial destination outcome", "error", err, "destination", outcome.Destination.String())
			}
		}
	}
}

// updatePipelineRunStatus updates the pipeline run with a new status and optional message
func (o *Orchestrator) updatePipelineRunStatus(ctx context.Context, logger *slog.Logger, userId string, pipelineRunId string, status pbpipeline.PipelineRunStatus, statusMessage string, providerExecs []ProviderExecution) {
	// Convert ProviderExecutions to snake_case maps for Firestore
	boosters := boostersToFirestoreMaps(providerExecs)

	gateSkipped := status == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SKIPPED && len(providerExecs) == 0
	enricherFailed := status == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_FAILED

	updateData := map[string]interface{}{
		"status":     int32(status),
		"updated_at": time.Now(),
		"boosters":   boosters,
		"steps":      buildAllSteps(pipelineRunId, providerExecs, gateSkipped, enricherFailed),
	}
	if statusMessage != "" {
		updateData["status_message"] = statusMessage
	}

	if err := o.database.UpdatePipelineRun(ctx, userId, pipelineRunId, updateData); err != nil {
		logger.Error("Failed to update pipeline run status", "error", err, "pipeline_run_id", pipelineRunId, "status", status)
	} else {
		logger.Debug("Updated pipeline run status", "pipeline_run_id", pipelineRunId, "status", status, "message", statusMessage)
	}
}

// finalizePipelineRun updates the pipeline run with final enriched data on success
func (o *Orchestrator) finalizePipelineRun(ctx context.Context, logger *slog.Logger, userId string, event *pbevents.EnrichedActivityEvent, providerExecs []ProviderExecution, originalPayloadUri string) {
	// Convert ProviderExecutions to snake_case maps for Firestore
	boosters := boostersToFirestoreMaps(providerExecs)

	// Note: destinations are now managed via subcollection (destination_outcomes)
	// and updated atomically by each uploader via SetDestinationOutcome.
	// We no longer write the destinations array on the parent document.

	// Update pipeline run with final enriched data
	// Note: status changes from PENDING -> RUNNING, and we clear any status_message
	// (e.g., "Waiting for user input: ...") since the input has been resolved.
	// The status will transition to SYNCED/PARTIAL/SYNCED_WITH_PENDING once destinations are processed.
	updateData := map[string]interface{}{
		"title":                          event.Name,
		"description":                    event.Description,
		"type":                           int32(event.ActivityType),
		"start_time":                     event.StartTime.AsTime(),
		"updated_at":                     time.Now(),
		"status":                         int32(pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING),
		"status_message":                 nil, // Clear pending input message on successful resume
		"boosters":                       boosters,
		"original_payload_uri":           originalPayloadUri,
		"steps":                          buildAllSteps(*event.PipelineExecutionId, providerExecs, false, false),
		"non_blocking_pending_input_ids": event.NonBlockingPendingInputIds,
	}

	if err := o.database.UpdatePipelineRun(ctx, userId, *event.PipelineExecutionId, updateData); err != nil {
		logger.Error("Failed to finalize pipeline run", "error", err, "pipeline_run_id", *event.PipelineExecutionId)
	} else {
		logger.Debug("Finalized pipeline run", "pipeline_run_id", *event.PipelineExecutionId, "activity_id", event.ActivityId)
	}
}

// buildSimpleStep returns a minimal ExecutionStep map for steps we don't time individually
// (source ingest, parse, gate, router). Duration and offset are 0 since they are not
// instrumented at this level — the enricher batch is the only timed step.
func buildSimpleStep(runID string, ordinal int32, kind pbpipeline.ExecutionStepKind, displayName, service string, status pbpipeline.ExecutionStepStatus, statusLabel string) map[string]interface{} {
	m := map[string]interface{}{
		"id":           fmt.Sprintf("%s_%d", runID, ordinal),
		"ordinal":      ordinal,
		"kind":         int32(kind),
		"display_name": displayName,
		"service":      service,
		"status":       int32(status),
		"offset_ms":    int64(0),
		"duration_ms":  int64(0),
		"metadata":     map[string]string{},
	}
	if statusLabel != "" {
		m["status_label"] = statusLabel
	}
	return m
}

// buildAllSteps builds the complete ExecutionStep array for a pipeline run.
// providerExecs should be non-nil when the enricher batch ran; gateSkipped=true
// when the activity was dropped by a condition matcher before enrichers ran.
func buildAllSteps(runID string, providerExecs []ProviderExecution, gateSkipped bool, enricherFailed bool) []map[string]interface{} {
	ok := pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK
	pass := pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_PASS
	skipped := pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_SKIPPED
	failed := pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_FAILED

	steps := []map[string]interface{}{
		buildSimpleStep(runID, 1, pbpipeline.ExecutionStepKind_EXECUTION_STEP_KIND_SOURCE, "Source Activity", "webhook", ok, "✓ OK"),
		buildSimpleStep(runID, 2, pbpipeline.ExecutionStepKind_EXECUTION_STEP_KIND_PARSE, "Parse & Normalise", "pipeline", ok, "✓ OK"),
	}

	if gateSkipped {
		steps = append(steps, buildSimpleStep(runID, 3, pbpipeline.ExecutionStepKind_EXECUTION_STEP_KIND_GATE, "Filter", "pipeline", skipped, "SKIP"))
		return steps
	}

	gateStatus := pass
	if enricherFailed && len(providerExecs) == 0 {
		gateStatus = failed
	}
	steps = append(steps, buildSimpleStep(runID, 3, pbpipeline.ExecutionStepKind_EXECUTION_STEP_KIND_GATE, "Filter", "pipeline", gateStatus, "✓ PASS"))

	if len(providerExecs) > 0 {
		enricherStatus := pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK
		if enricherFailed {
			enricherStatus = failed
		}
		steps = append(steps, buildEnricherBatchStep(runID, providerExecs, enricherStatus))
		if !enricherFailed {
			steps = append(steps, buildSimpleStep(runID, 5, pbpipeline.ExecutionStepKind_EXECUTION_STEP_KIND_ROUTER, "Route to Destinations", "pipeline", ok, "✓ OK"))
		}
	}

	return steps
}

// buildEnricherBatchStep converts ProviderExecutions into an ExecutionStep record
// for the enricher-batch stage. Duration is the sum of all provider DurationMs values.
func buildEnricherBatchStep(pipelineRunID string, providerExecs []ProviderExecution, stepStatus pbpipeline.ExecutionStepStatus) map[string]interface{} {
	var totalDurationMs int64
	okCount, failCount := 0, 0
	for _, pe := range providerExecs {
		totalDurationMs += pe.DurationMs
		switch pe.Status {
		case "SUCCESS":
			okCount++
		case "FAILED":
			failCount++
		}
	}
	total := len(providerExecs)
	statusLabel := fmt.Sprintf("✓ %d/%d", okCount, total)
	if failCount > 0 {
		statusLabel = fmt.Sprintf("⚠ %d/%d", okCount, total)
	}
	return map[string]interface{}{
		"id":           pipelineRunID + "_enricher",
		"ordinal":      int32(4),
		"kind":         int32(pbpipeline.ExecutionStepKind_EXECUTION_STEP_KIND_ENRICHER_BATCH),
		"display_name": "Enricher Batch",
		"service":      "pipeline",
		"status":       int32(stepStatus),
		"offset_ms":    int64(0),
		"duration_ms":  totalDurationMs,
		"status_label": statusLabel,
		"metadata":     map[string]string{},
	}
}

// pipelineRunStatusToStepStatus maps a PipelineRunStatus to the corresponding ExecutionStepStatus
// for the enricher batch step written alongside each status update.
func pipelineRunStatusToStepStatus(s pbpipeline.PipelineRunStatus) pbpipeline.ExecutionStepStatus {
	switch s {
	case pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_FAILED:
		return pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_FAILED
	case pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SKIPPED:
		return pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_SKIPPED
	case pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PENDING:
		return pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_QUEUED
	case pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING:
		return pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK
	default:
		// RUNNING (retry-in-progress) and any other transitional status.
		return pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_RETRIED
	}
}

// boostersToFirestoreMaps converts ProviderExecutions to snake_case maps for Firestore storage
// This ensures field names match what the web UI expects (provider_name, duration_ms, etc.)
func boostersToFirestoreMaps(providerExecs []ProviderExecution) []map[string]interface{} {
	boosters := make([]map[string]interface{}, 0, len(providerExecs))
	for _, pe := range providerExecs {
		booster := map[string]interface{}{
			"provider_name": pe.ProviderName,
			"status":        pe.Status,
			"duration_ms":   pe.DurationMs,
			"metadata":      pe.Metadata,
		}
		if pe.Error != "" {
			booster["error"] = pe.Error
		}
		boosters = append(boosters, booster)
	}
	return boosters
}

// stripReplayKeys returns a copy of the replay journal metadata with the internal
// replay_* bookkeeping keys removed, so only the provider's original metadata is
// merged back into EnrichedActivityEvent.EnrichmentMetadata on resume.
func stripReplayKeys(replayMeta map[string]string) map[string]string {
	clean := make(map[string]string, len(replayMeta))
	for k, v := range replayMeta {
		if strings.HasPrefix(k, "replay_") {
			continue
		}
		clean[k] = v
	}
	return clean
}

// buildBoosterMetadata returns the metadata map to store in a ProviderExecution on success.
// For non-idempotent providers it appends replay_* keys so the orchestrator can skip and
// replay the enricher's mutations if the pipeline is resumed later.
func buildBoosterMetadata(res *providers.EnrichmentResult, provider providers.Provider) map[string]string {
	m := make(map[string]string)
	for k, v := range res.Metadata {
		m[k] = v
	}
	ni, isNonIdempotent := provider.(providers.NonIdempotentProvider)
	if !isNonIdempotent || ni.IsIdempotent() {
		return m
	}
	m["replay_completed"] = "true"
	if res.NameSuffix != "" {
		m["replay_name_suffix"] = res.NameSuffix
	}
	if res.Name != "" {
		m["replay_name"] = res.Name
	}
	if trimmed := strings.TrimSpace(res.Description); trimmed != "" {
		m["replay_description"] = trimmed
	}
	if res.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		m["replay_activity_type"] = strconv.Itoa(int(res.ActivityType))
	}
	if len(res.Tags) > 0 {
		if tagsJSON, err := json.Marshal(res.Tags); err == nil {
			m["replay_tags"] = string(tagsJSON)
		}
	}
	// Persist typed enrichments (e.g. AiSummary) so they can be merged back into
	// the final event on resume. Without this, a non-idempotent enricher's typed
	// Enrichments are lost when the pipeline resumes (the enricher is replayed,
	// not re-run), even though its description survives via replay_description.
	if res.Enrichments != nil {
		if enrichJSON, err := protojson.Marshal(res.Enrichments); err == nil {
			m["replay_enrichments"] = string(enrichJSON)
		}
	}
	// Persist the heart rate stream so a resume replays the already-fetched data
	// instead of re-querying the source API. Without this, non-idempotent stream
	// providers (e.g. fitbit-heart-rate) re-run from the clean pre-enrichment
	// payload on every resume, re-fetching from the external API and sometimes
	// committing incomplete data if that fetch races the provider's own sync lag.
	if len(res.HeartRateStream) > 0 {
		if streamJSON, err := json.Marshal(res.HeartRateStream); err == nil {
			m["replay_heart_rate_stream"] = string(streamJSON)
		}
	}
	return m
}

// buildPendingInputStatusMessage creates a user-friendly status message for pending input.
// It uses the display.summary from the provider metadata if available, falling back
// to display.field_labels for humanized field names, and finally to Title-Cased field names.
func buildPendingInputStatusMessage(waitErr *user_input.WaitForInputError) string {
	// Priority 1: Use display.summary if the provider set it
	if summary, ok := waitErr.Metadata["display.summary"]; ok && summary != "" {
		return fmt.Sprintf("Waiting for user input: %s", summary)
	}

	// Priority 2: Use display.field_labels for humanized names
	if labelsJSON, ok := waitErr.Metadata["display.field_labels"]; ok && labelsJSON != "" {
		var labels map[string]string
		if err := json.Unmarshal([]byte(labelsJSON), &labels); err == nil && len(labels) > 0 {
			var friendly []string
			for _, field := range waitErr.RequiredFields {
				if label, exists := labels[field]; exists {
					friendly = append(friendly, label)
				} else {
					friendly = append(friendly, humanizeFieldName(field))
				}
			}
			return fmt.Sprintf("Waiting for user input: %s", strings.Join(friendly, ", "))
		}
	}

	// Priority 3: Humanize raw field names (e.g. fit_file_base64 -> Fit File Base64)
	var humanized []string
	for _, field := range waitErr.RequiredFields {
		humanized = append(humanized, humanizeFieldName(field))
	}
	return fmt.Sprintf("Waiting for user input: %s", strings.Join(humanized, ", "))
}

// mergeEnrichments merges non-nil fields from src into dst using proto.Merge,
// which walks all fields reflectively. New fields added to ActivityEnrichments
// in the proto are automatically included without any changes here.
func mergeEnrichments(dst, src *pbactivity.ActivityEnrichments) *pbactivity.ActivityEnrichments {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = &pbactivity.ActivityEnrichments{}
	}
	proto.Merge(dst, src)
	return dst
}

// humanizeFieldName converts snake_case to Title Case (e.g. "fit_file_base64" -> "Fit File Base64")
func humanizeFieldName(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
