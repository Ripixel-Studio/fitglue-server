package destination

import (
	"context"
	"fmt"
	"strings"

	"github.com/fitglue/server/src/go/internal/infra"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	"github.com/fitglue/server/src/go/pkg/types/formatters"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Database interface subset needed for destination updates
type Database interface {
	UpdatePipelineRun(ctx context.Context, userId string, id string, data map[string]interface{}) error
	SetDestinationOutcome(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error
	GetDestinationOutcomes(ctx context.Context, userId string, pipelineRunId string) ([]*pbpipeline.DestinationOutcome, error)
}

// UpdateStatus updates a single destination's status using the subcollection pattern.
// Each destination is written as a separate document in the destination_outcomes subcollection,
// eliminating race conditions between parallel uploaders.
// When all destinations have reached a terminal status, a push notification is sent to the user.
// Parameters:
//   - db: the Database interface for Firestore operations
//   - notifications: the notification service for sending push notifications (can be nil)
//   - userId: the user's ID
//   - pipelineRunId: the ID of the pipeline run (same as pipelineExecutionId)
//   - dest: the destination enum value (e.g., DESTINATION_STRAVA)
//   - status: the new status (e.g., DESTINATION_STATUS_SUCCESS, DESTINATION_STATUS_FAILED)
//   - externalId: optional external ID from the destination (e.g., Strava activity ID)
//   - errMsg: optional error message if status is FAILED
//   - activityName: the activity name for the push notification title
//   - logger: logger for debugging
//   - nonBlockingPendingIDs: IDs of non-blocking pending inputs still awaiting user input
//   - nonBlockingUpdate: true when this upload is an update post triggered by a resolved
//     non-blocking input. The user was already notified at the initial sync, so a success
//     notification is suppressed here — only a failure (PARTIAL) is worth notifying about.
func UpdateStatus(ctx context.Context, db Database, publisher shared.Publisher, userId string, pipelineRunId string, dest pbplugin.DestinationType, status pbpipeline.DestinationStatus, externalId string, errMsg string, activityName string, activityId string, logger infra.Logger, nonBlockingPendingIDs []string, nonBlockingUpdate bool) {
	if pipelineRunId == "" {
		return // No pipeline run to update - legacy flow
	}

	// Build the outcome
	outcome := &pbpipeline.DestinationOutcome{
		Destination: dest,
		Status:      status,
		CompletedAt: timestamppb.Now(),
	}
	if externalId != "" {
		outcome.ExternalId = &externalId
	}
	if errMsg != "" {
		outcome.Error = &errMsg
	}

	// Write directly to subcollection - each destination has its own document
	// No read-modify-write needed, eliminating race conditions
	if err := db.SetDestinationOutcome(ctx, userId, pipelineRunId, outcome); err != nil {
		logger.Error(ctx, "Failed to set destination outcome", "error", err, "pipeline_run_id", pipelineRunId, "destination", dest.String())
		return
	}

	logger.Debug(ctx, "Set destination outcome in subcollection", "pipeline_run_id", pipelineRunId, "destination", dest.String(), "status", status.String())

	// Now compute and update the overall pipeline status
	// Read all destination outcomes from subcollection
	outcomes, err := db.GetDestinationOutcomes(ctx, userId, pipelineRunId)
	if err != nil {
		logger.Warn(ctx, "Failed to get destination outcomes for status computation", "error", err, "pipeline_run_id", pipelineRunId)
		return
	}

	newStatus := ComputePipelineRunStatus(outcomes, nonBlockingPendingIDs)

	// Convert outcomes to Firestore-compatible format for the inline destinations array
	// This keeps the inline array in sync with the subcollection for UI consumers
	destinationsData := make([]map[string]interface{}, len(outcomes))
	for i, o := range outcomes {
		destData := map[string]interface{}{
			"destination": int32(o.Destination),
			"status":      int32(o.Status),
		}
		if o.ExternalId != nil {
			destData["external_id"] = *o.ExternalId
		}
		if o.Error != nil {
			destData["error"] = *o.Error
		}
		if o.CompletedAt != nil {
			destData["completed_at"] = o.CompletedAt.AsTime()
		}
		destinationsData[i] = destData
	}

	// Update the parent pipeline run's overall status AND inline destinations array.
	// Always write non_blocking_pending_input_ids so resolution clears them correctly.
	updateData := map[string]interface{}{
		"status":                         int32(newStatus),
		"updated_at":                     timestamppb.Now(),
		"destinations":                   destinationsData,
		"non_blocking_pending_input_ids": nonBlockingPendingIDs,
	}

	if err := db.UpdatePipelineRun(ctx, userId, pipelineRunId, updateData); err != nil {
		logger.Error(ctx, "Failed to update pipeline run status", "error", err, "pipeline_run_id", pipelineRunId)
	} else {
		logger.Debug(ctx, "Updated pipeline run status and destinations", "pipeline_run_id", pipelineRunId, "status", newStatus.String(), "destinations_count", len(destinationsData))
	}

	// Send push notification when all destinations have reached a terminal status.
	// SYNCED_WITH_PENDING is terminal for destinations (enricher resolution notifies separately).
	if newStatus == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED ||
		newStatus == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING ||
		newStatus == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PARTIAL {
		// Update posts triggered by a resolved non-blocking input would otherwise re-notify
		// on every resolution (the user already got the initial sync notification). Only the
		// failure case (PARTIAL) is worth a second notification.
		if nonBlockingUpdate && newStatus != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PARTIAL {
			logger.Debug(ctx, "Suppressing success notification for non-blocking input update post", "pipeline_run_id", pipelineRunId, "status", newStatus.String())
		} else {
			sendSyncNotification(ctx, publisher, userId, activityName, activityId, newStatus, outcomes, logger)
		}
	}
}

// sendSyncNotification enqueues a pipeline sync notification.
// The notification service resolves the user's enabled channels and dispatches accordingly.
func sendSyncNotification(ctx context.Context, publisher shared.Publisher, userId string, activityName string, activityId string, status pbpipeline.PipelineRunStatus, outcomes []*pbpipeline.DestinationOutcome, logger infra.Logger) {
	if publisher == nil {
		return
	}

	var succeeded []string
	var failed []string
	for _, o := range outcomes {
		name := FormatDestinationName(o.Destination)
		switch o.Status {
		case pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS:
			succeeded = append(succeeded, name)
		case pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED:
			failed = append(failed, name)
		}
	}

	var notifType pbnotification.NotificationType
	var title, body string

	// SYNCED_WITH_PENDING means every destination synced successfully and only a
	// non-blocking input is still outstanding (prompted via a separate PENDING_INPUT
	// notification), so it's a success — not a "Partial Sync" failure.
	if status == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED ||
		status == pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING {
		notifType = pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS
		title = fmt.Sprintf("Activity Synced: %s", activityName)
		body = fmt.Sprintf("Successfully synced to: %s", strings.Join(succeeded, ", "))
	} else {
		notifType = pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE
		title = fmt.Sprintf("Partial Sync: %s", activityName)
		if len(succeeded) > 0 && len(failed) > 0 {
			body = fmt.Sprintf("Synced to %s, but %s failed", strings.Join(succeeded, ", "), strings.Join(failed, ", "))
		} else if len(failed) > 0 {
			body = fmt.Sprintf("Failed to sync to: %s", strings.Join(failed, ", "))
		}
	}

	req := &pbnotification.NotificationRequest{
		UserId: userId,
		Type:   notifType,
		Title:  title,
		Body:   body,
		Data:   map[string]string{"activity_id": activityId},
	}
	if err := notificationpub.Enqueue(ctx, publisher, req); err != nil {
		logger.Warn(ctx, "Failed to enqueue sync notification", "error", err, "user_id", userId)
	}
}

// FormatDestinationName returns a human-readable name for a destination.
// Delegates to the generated formatters package for consistency across Go and TypeScript.
func FormatDestinationName(dest pbplugin.DestinationType) string {
	return formatters.FormatDestination(dest)
}

// ComputePipelineRunStatus determines overall status from destination outcomes and any
// non-blocking pending inputs still awaiting user input.
func ComputePipelineRunStatus(destinations []*pbpipeline.DestinationOutcome, nonBlockingPendingIDs []string) pbpipeline.PipelineRunStatus {
	if len(destinations) == 0 {
		return pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING
	}

	anyFailed := false
	allComplete := true

	for _, d := range destinations {
		switch d.Status {
		case pbpipeline.DestinationStatus_DESTINATION_STATUS_PENDING:
			allComplete = false
		case pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED:
			anyFailed = true
		case pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS:
			// Good
		case pbpipeline.DestinationStatus_DESTINATION_STATUS_SKIPPED:
			// Skipped doesn't count as failure
		}
	}

	if !allComplete {
		return pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING
	}
	if anyFailed {
		return pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PARTIAL
	}
	if len(nonBlockingPendingIDs) > 0 {
		return pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING
	}
	return pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED
}
