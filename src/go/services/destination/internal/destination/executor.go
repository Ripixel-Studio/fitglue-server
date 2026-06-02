// nolint:proto-json
package destination

import (
	"context"
	"encoding/json"
	"errors"
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
	fitglueerrors "github.com/fitglue/server/src/go/pkg/errors"
	"github.com/fitglue/server/src/go/pkg/loopprevention"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	pbactivitymodel "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UploadExecutor handles the consumption of Pub/Sub messages
// and orchestrating the destination interface lifecycle.
type UploadExecutor struct {
	registry       *Registry
	userClient     userpb.UserServiceClient
	activityClient activitypb.ActivityServiceClient
	db             shared.Database
	store          shared.BlobStore
	publisher      shared.Publisher
	logger         infra.Logger
}

// NewUploadExecutor creates an orchestrator initialized with dependencies.
func NewUploadExecutor(
	registry *Registry,
	userClient userpb.UserServiceClient,
	activityClient activitypb.ActivityServiceClient,
	db shared.Database,
	store shared.BlobStore,
	publisher shared.Publisher,
	logger infra.Logger,
) *UploadExecutor {
	return &UploadExecutor{
		registry:       registry,
		userClient:     userClient,
		activityClient: activityClient,
		db:             db,
		store:          store,
		publisher:      publisher,
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

	// The router publishes one event per destination (type = "com.fitglue.job.DESTINATION_X").
	// Extract the target destination so we only process the one this message was routed for.
	// Without this filter the executor processes ALL destinations from every routed message,
	// causing N uploads per destination when a pipeline has N destinations.
	targetDest := pbplugin.DestinationType_DESTINATION_UNSPECIFIED
	if t := ce.Type(); strings.HasPrefix(t, "com.fitglue.job.") {
		destStr := strings.TrimPrefix(t, "com.fitglue.job.")
		if val, ok := pbplugin.DestinationType_value[destStr]; ok {
			targetDest = pbplugin.DestinationType(val)
		}
	}

	e.logger.Info(ctx, "Processing upload for activity", "activity_id", payload.ActivityId, "user_id", payload.UserId, "destinations_count", len(payload.Destinations), "target_dest", targetDest.String())

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
		e.writeFailureForTargetedDestinations(ctx, &payload, targetDest, pipelineRunId, fmt.Sprintf("Failed to fetch user profile: %v", err))
		return fmt.Errorf("getting user profile: %w", err)
	}

	integrationsResp, err := e.userClient.ListIntegrations(ctx, &userpb.ListIntegrationsRequest{UserId: payload.UserId})
	if err != nil {
		e.logger.Error(ctx, "Failed to fetch user integrations", "error", err)
		e.writeFailureForTargetedDestinations(ctx, &payload, targetDest, pipelineRunId, fmt.Sprintf("Failed to fetch user integrations: %v", err))
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
		var err error
		pr, err = e.db.GetPipelineRun(ctx, payload.UserId, pipelineRunId)
		if err != nil {
			e.logger.Warn(ctx, "failed to fetch pipeline run for update/create decision", "error", err, "pipelineRunId", pipelineRunId)
		}
	}

	// Pre-fetch destination outcomes once; used below to skip already-succeeded destinations
	// on Pub/Sub redelivery (prevents double-posting).
	var priorOutcomes []*pbpipeline.DestinationOutcome
	if pipelineRunId != "" {
		var err error
		priorOutcomes, err = e.db.GetDestinationOutcomes(ctx, payload.UserId, pipelineRunId)
		if err != nil {
			e.logger.Error(ctx, "failed to fetch prior destination outcomes; dedup check skipped, activity may re-upload", "error", err, "pipelineRunId", pipelineRunId)
		}
	}

destinations:
	for _, destEnum := range payload.Destinations {
		if destEnum == pbplugin.DestinationType_DESTINATION_UNSPECIFIED {
			continue
		}

		// Each routed message targets exactly one destination; skip the rest.
		// Falls back to processing all if the event type is not destination-specific.
		if targetDest != pbplugin.DestinationType_DESTINATION_UNSPECIFIED && destEnum != targetDest {
			e.logger.Debug(ctx, "Skipping destination not targeted by this event", "destination", destEnum.String(), "target", targetDest.String())
			continue
		}

		uploader, ok := e.registry.Get(destEnum)
		if !ok {
			e.logger.Warn(ctx, "No uploader registered for destination", "destination", destEnum.String())
			if pipelineRunId != "" {
				destination.UpdateStatus(ctx, e.db, e.publisher, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED, "", "Uploader not registered", payload.Name, payload.ActivityId, e.logger)
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

		if effectiveIsUpdate {
			// Pre-write bounceback record before Update so the webhook that follows
			// is suppressed even if it arrives before the response returns.
			if loopprevention.DestinationHasSource(destEnum) {
				if existID := existingExternalID(destEnum, pr, metadata, activityPayload); existID != "" {
					e.storeBouncebackRecord(ctx, payload.UserId, payload.Source, activityPayload, destEnum, existID)
				}
			}
			uploadErr = uploader.Update(ctx, activityPayload, userRecord, pr)
		} else {
			// Pre-write a pending bounceback record before Create so the webhook cannot
			// arrive before we know the destination ID. Keyed by startTime since the
			// real destination ID is unknown until the API responds.
			if loopprevention.DestinationHasSource(destEnum) && activityPayload.Timestamp != nil {
				pendingID := fmt.Sprintf("pending:%d", activityPayload.Timestamp.AsTime().Unix())
				e.storeBouncebackRecord(ctx, payload.UserId, payload.Source, activityPayload, destEnum, pendingID)
			}
			externalId, uploadErr = uploader.Create(ctx, activityPayload, userRecord)
			// Write the real record now that we have the destination ID.
			if uploadErr == nil && externalId != "" && loopprevention.DestinationHasSource(destEnum) {
				e.storeBouncebackRecord(ctx, payload.UserId, payload.Source, activityPayload, destEnum, externalId)
			}
		}

		if uploadErr != nil {
			e.logger.Error(ctx, "Destination uploader failed", "destination", destEnum.String(), "error", uploadErr)
			var fgErr *fitglueerrors.FitGlueError
			if errors.As(uploadErr, &fgErr) && fgErr.Code == fitglueerrors.CodeIntegrationAuthFailed {
				e.enqueueConnectionActionNotification(ctx, payload.UserId, destEnum)
			}
			if pipelineRunId != "" {
				destination.UpdateStatus(ctx, e.db, e.publisher, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED, externalId, uploadErr.Error(), payload.Name, payload.ActivityId, e.logger)
			}
			continue
		}

		// Success
		if pipelineRunId != "" {
			destination.UpdateStatus(ctx, e.db, e.publisher, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS, externalId, "", payload.Name, payload.ActivityId, e.logger)
		}
		if err := e.db.RecordBillingEvent(ctx, payload.UserId, shared.BillingEvent{
			ActivityID:    payload.ActivityId,
			PipelineRunID: payload.GetPipelineExecutionId(),
			PipelineID:    payload.PipelineId,
			Source:        payload.Source.String(),
			Destination:   destEnum.String(),
		}); err != nil {
			e.logger.Warn(ctx, "Failed to record billing event", "error", err, "destination", destEnum.String())
		}

		e.logger.Info(ctx, "Destination uploader completed successfully", "destination", destEnum.String())
	}

	return nil
}

// writeFailureForTargetedDestinations writes DESTINATION_STATUS_FAILED for the destinations
// this event was routed for. If targetDest is UNSPECIFIED (legacy / non-routed event),
// falls back to marking all destinations failed so the user never sees "pending" forever.
func (e *UploadExecutor) writeFailureForTargetedDestinations(ctx context.Context, payload *pbevents.EnrichedActivityEvent, targetDest pbplugin.DestinationType, pipelineRunId string, errMsg string) {
	if pipelineRunId == "" {
		return
	}
	for _, destEnum := range payload.Destinations {
		if destEnum == pbplugin.DestinationType_DESTINATION_UNSPECIFIED {
			continue
		}
		if targetDest != pbplugin.DestinationType_DESTINATION_UNSPECIFIED && destEnum != targetDest {
			continue
		}
		destination.UpdateStatus(ctx, e.db, e.publisher, payload.UserId, pipelineRunId, destEnum, pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED, "", errMsg, payload.Name, payload.ActivityId, e.logger)
	}
}

// storeBouncebackRecord writes an UploadedActivityRecord to Firestore so the webhook
// processor can recognise this activity as a bounceback and skip re-ingestion.
// Errors are logged but never fatal — a missing record causes a duplicate, not a data loss.
func (e *UploadExecutor) storeBouncebackRecord(
	ctx context.Context,
	userID string,
	source pbactivitymodel.ActivitySource,
	payload *pbevents.ActivityPayload,
	dest pbplugin.DestinationType,
	destinationID string,
) {
	record := &pbactivitymodel.UploadedActivityRecord{
		Id:            loopprevention.BuildUploadedActivityID(dest, destinationID),
		UserId:        userID,
		Source:        source,
		ExternalId:    payload.StandardizedActivity.GetExternalId(),
		StartTime:     payload.Timestamp,
		Destination:   dest,
		DestinationId: destinationID,
		UploadedAt:    timestamppb.Now(),
	}
	if err := e.db.SetUploadedActivity(ctx, userID, record); err != nil {
		e.logger.Warn(ctx, "Failed to store bounceback record", "destination", dest.String(), "destination_id", destinationID, "error", err)
	}
}

// existingExternalID returns the destination-side activity ID that was created in a prior
// pipeline run, used to pre-write the bounceback record before an Update call.
func existingExternalID(
	dest pbplugin.DestinationType,
	pr *pbpipeline.PipelineRun,
	metadata map[string]string,
	payload *pbevents.ActivityPayload,
) string {
	if pr != nil {
		for _, d := range pr.Destinations {
			if d.Destination == dest && d.ExternalId != nil && *d.ExternalId != "" {
				return *d.ExternalId
			}
		}
	}
	// Same-source flow: the source's external ID is also the destination ID.
	destName := strings.ToLower(strings.TrimPrefix(dest.String(), "DESTINATION_"))
	if metadata["same_source_destination_"+destName] == "true" && payload.StandardizedActivity != nil {
		return payload.StandardizedActivity.GetExternalId()
	}
	return ""
}

// enqueueConnectionActionNotification publishes a CONNECTION_ACTION notification so the user
// knows their destination OAuth needs re-authorisation. The notification service handles channel dispatch.
func (e *UploadExecutor) enqueueConnectionActionNotification(ctx context.Context, userID string, dest pbplugin.DestinationType) {
	if e.publisher == nil {
		return
	}
	sourceID := strings.ToLower(strings.TrimPrefix(dest.String(), "DESTINATION_"))
	destName := destination.FormatDestinationName(dest)
	req := &pbnotification.NotificationRequest{
		UserId: userID,
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_CONNECTION_ACTION,
		Title:  fmt.Sprintf("Reconnect %s", destName),
		Body:   fmt.Sprintf("Your %s connection needs to be re-authorised.", destName),
		Data:   map[string]string{"sourceId": sourceID},
	}
	if err := notificationpub.Enqueue(ctx, e.publisher, req); err != nil {
		e.logger.Warn(ctx, "Failed to enqueue connection action notification", "error", err, "user_id", userID, "destination", dest.String())
	}
}
