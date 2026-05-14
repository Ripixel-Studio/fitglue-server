// nolint:proto-json
package destination

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/destination"
	activityPkg "github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/protobuf/encoding/protojson"
)

// UploadExecutor handles the consumption of Pub/Sub messages
// and orchestrating the destination interface lifecycle.
type UploadExecutor struct {
	registry       *Registry
	userClient     userpb.UserServiceClient
	activityClient activitypb.ActivityServiceClient
	db             shared.Database
	store          shared.BlobStore
	notifications  shared.NotificationService
	logger         infra.Logger
}

// NewUploadExecutor creates an orchestrator initialized with dependencies.
func NewUploadExecutor(
	registry *Registry,
	userClient userpb.UserServiceClient,
	activityClient activitypb.ActivityServiceClient,
	db shared.Database,
	store shared.BlobStore,
	notifications shared.NotificationService,
	logger infra.Logger,
) *UploadExecutor {
	return &UploadExecutor{
		registry:       registry,
		userClient:     userClient,
		activityClient: activityClient,
		db:             db,
		store:          store,
		notifications:  notifications,
		logger:         logger,
	}
}

// HandlePubSubPush parses an HTTP request sent via Pub/Sub Push Subscription.
func (e *UploadExecutor) HandlePubSubPush(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		e.logger.Error(ctx, "Failed to read request body", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the Pub/Sub Push message envelope
	var msg struct {
		Message struct {
			Data []byte `json:"data"`
			ID   string `json:"messageId"`
		} `json:"message"`
		Subscription string `json:"subscription"`
	}

	if err := json.Unmarshal(body, &msg); err != nil {
		e.logger.Error(ctx, "Failed to unmarshal pub/sub envelope", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Unmarshal the inner payload as a CloudEvent
	var ce event.Event
	if err := json.Unmarshal(msg.Message.Data, &ce); err != nil {
		e.logger.Error(ctx, "Failed to unmarshal inner CloudEvent", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := e.Process(ctx, &ce); err != nil {
		e.logger.Error(ctx, "Failed to process destination upload event", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// Process unmarshals the event to an EnrichedActivityEvent and distributes it to the correct Destinations
func (e *UploadExecutor) Process(ctx context.Context, ce *event.Event) error {
	var payload pbevents.EnrichedActivityEvent

	// Use protojson to unmarshal to handle enum strings correctly
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
		AllowPartial:   true,
	}
	if err := unmarshaler.Unmarshal(ce.Data(), &payload); err != nil {
		e.logger.Error(ctx, "Failed to unmarshal EnrichedActivityEvent via protojson", "error", err)
		return nil // Return nil so pubsub ack's it as a bad payload
	}

	e.logger.Info(ctx, "Processing upload for activity", "activity_id", payload.ActivityId, "user_id", payload.UserId, "destinations_count", len(payload.Destinations))

	if len(payload.Destinations) == 0 {
		return nil
	}

	// Extract pipelineRunId early so failure-state writes can use it
	var pipelineRunId string
	if payload.PipelineExecutionId != nil {
		pipelineRunId = *payload.PipelineExecutionId
	}

	// Fetch User Record
	profileResp, err := e.userClient.GetProfile(ctx, &userpb.GetProfileRequest{UserId: payload.UserId})
	if err != nil {
		e.logger.Error(ctx, "Failed to fetch user profile", "error", err)
		e.writeFailureForAllDestinations(ctx, &payload, pipelineRunId, fmt.Sprintf("Failed to fetch user profile: %v", err))
		return fmt.Errorf("getting user profile: %w", err)
	}

	integrationsResp, err := e.userClient.ListIntegrations(ctx, &userpb.ListIntegrationsRequest{UserId: payload.UserId})
	if err != nil {
		e.logger.Error(ctx, "Failed to fetch user integrations", "error", err)
		e.writeFailureForAllDestinations(ctx, &payload, pipelineRunId, fmt.Sprintf("Failed to fetch user integrations: %v", err))
		return fmt.Errorf("getting user integrations: %w", err)
	}

	userRecord := &user.Record{
		UserProfile:  profileResp,
		Integrations: integrationsResp,
	}

	// Merge EnrichmentMetadata into a new Metadata map
	metadata := make(map[string]string)
	if payload.EnrichmentMetadata != nil {
		for k, v := range payload.EnrichmentMetadata {
			metadata[k] = v
		}
	}

	// Inject fields required by uploaders that aren't native to ActivityPayload
	metadata["fit_file_uri"] = payload.FitFileUri
	metadata["activity_name"] = payload.Name
	metadata["description"] = payload.Description
	metadata["strava_sport_type"] = activityPkg.GetStravaActivityType(payload.ActivityType)
	metadata["activity_type"] = payload.ActivityType.String()

	// Inject activity_data_uri so destination uploaders can persist GCS references
	if payload.ActivityDataUri != "" {
		metadata["activity_data_uri"] = payload.ActivityDataUri
	}

	// Inject applied enrichments and tags (comma-separated for metadata map)
	if len(payload.AppliedEnrichments) > 0 {
		metadata["applied_enrichments"] = strings.Join(payload.AppliedEnrichments, ",")
	}
	if len(payload.Tags) > 0 {
		metadata["tags"] = strings.Join(payload.Tags, ",")
	}

	// Resolve StandardizedActivity: the enricher always offloads ActivityData to GCS
	// and clears it from the Pub/Sub message. Fetch it back now so uploaders (e.g. Hevy)
	// can access the full session/lap/strength-set data written by enrichers like
	// HybridRaceTagger. Without this, every activity hits the single-cardio fallback.
	resolvedActivityData, resolveErr := activityPkg.ResolveActivityData(ctx, &payload, e.store)
	if resolveErr != nil {
		e.logger.Warn(ctx, "Failed to resolve activity data from GCS, proceeding with inline data",
			"error", resolveErr,
			"activity_data_uri", payload.ActivityDataUri)
		resolvedActivityData = payload.ActivityData // Fallback to whatever is inline (may be nil)
	}

	// Construct generic ActivityPayload for Destination Uploaders
	activityPayload := &pbevents.ActivityPayload{
		Source:               payload.Source,
		UserId:               payload.UserId,
		ActivityId:           &payload.ActivityId,
		StandardizedActivity: resolvedActivityData, // Resolved from GCS or inline
		OriginalPayloadJson:  "",
		Metadata:             metadata, // Injected Metadata
		PipelineExecutionId:  payload.PipelineExecutionId,
		Timestamp:            payload.StartTime, // Activity start time, not upload time
	}

	isUpdate := false
	if useUpdate, ok := payload.EnrichmentMetadata["use_update_method"]; ok && useUpdate == "true" {
		isUpdate = true
	}

	// Fetch the parent PipelineRun — needed by Update() to find the existing destination activity ID
	var pr *pbpipeline.PipelineRun
	if pipelineRunId != "" {
		pr, _ = e.db.GetPipelineRun(ctx, payload.UserId, pipelineRunId)
	}

	// Pre-fetch destination outcomes once; used below to skip already-succeeded destinations
	// on Pub/Sub redelivery (prevents double-posting).
	var priorOutcomes []*pbpipeline.DestinationOutcome
	if pipelineRunId != "" {
		priorOutcomes, _ = e.db.GetDestinationOutcomes(ctx, payload.UserId, pipelineRunId)
	}

destinations:
	for _, destEnum := range payload.Destinations {
		if destEnum == pbplugin.DestinationType_DESTINATION_UNSPECIFIED {
			continue
		}

		uploader, ok := e.registry.Get(destEnum)
		if !ok {
			e.logger.Warn(ctx, "No uploader registered for destination", "destination", destEnum.String())
			if pipelineRunId != "" {
				destination.UpdateStatus(ctx, e.db, e.notifications, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED, "", "Uploader not registered", payload.Name, payload.ActivityId, e.logger)
			}
			continue
		}

		// Same-source-destination flows (e.g. Hevy→Hevy) must use Update, not Create.
		// The orchestrator sets same_source_destination_{name}=true in EnrichmentMetadata
		// for these flows, but use_update_method is only set in resume mode. Without this
		// per-destination check, Create() is called for every initial same-source webhook,
		// producing a duplicate activity in the destination platform.
		effectiveIsUpdate := isUpdate
		if !effectiveIsUpdate {
			destName := strings.ToLower(strings.TrimPrefix(destEnum.String(), "DESTINATION_"))
			if val, ok := metadata["same_source_destination_"+destName]; ok && val == "true" {
				effectiveIsUpdate = true
			}
		}

		// Idempotency guard: if this destination already succeeded in a prior delivery of
		// the same pipeline run, skip rather than double-posting.
		if !effectiveIsUpdate {
			for _, outcome := range priorOutcomes {
				if outcome.Destination == destEnum && outcome.Status == pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS {
					e.logger.Info(ctx, "Skipping already-uploaded destination (Pub/Sub redelivery)", "destination", destEnum.String())
					continue destinations
				}
			}
		}

		e.logger.Info(ctx, "Triggering destination uploader", "destination", destEnum.String(), "is_update", effectiveIsUpdate)

		var externalId string
		var uploadErr error

		// Create or Update
		if effectiveIsUpdate {
			uploadErr = uploader.Update(ctx, activityPayload, userRecord, pr)
		} else {
			externalId, uploadErr = uploader.Create(ctx, activityPayload, userRecord)
		}

		if uploadErr != nil {
			e.logger.Error(ctx, "Destination uploader failed", "destination", destEnum.String(), "error", uploadErr)
			if pipelineRunId != "" {
				destination.UpdateStatus(ctx, e.db, e.notifications, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED, externalId, uploadErr.Error(), payload.Name, payload.ActivityId, e.logger)
			}
			continue
		}

		// Success
		if pipelineRunId != "" {
			destination.UpdateStatus(ctx, e.db, e.notifications, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS, externalId, "", payload.Name, payload.ActivityId, e.logger)
		}

		e.logger.Info(ctx, "Destination uploader completed successfully", "destination", destEnum.String())
	}

	return nil
}

// writeFailureForAllDestinations writes DESTINATION_STATUS_FAILED for every destination
// in the payload. Used when a systemic error (e.g. user service 403) prevents any
// uploader from running, so the user sees "failed" instead of "pending" forever.
func (e *UploadExecutor) writeFailureForAllDestinations(ctx context.Context, payload *pbevents.EnrichedActivityEvent, pipelineRunId string, errMsg string) {
	if pipelineRunId == "" {
		return
	}
	for _, destEnum := range payload.Destinations {
		if destEnum == pbplugin.DestinationType_DESTINATION_UNSPECIFIED {
			continue
		}
		destination.UpdateStatus(ctx, e.db, e.notifications, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED, "", errMsg, payload.Name, payload.ActivityId, e.logger)
	}
}
