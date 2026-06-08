// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	"github.com/fitglue/server/src/go/pkg/sourceplugins"
	"github.com/fitglue/server/src/go/pkg/types/formatters"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service implements the pbsvc.PipelineServiceServer interface.
type Service struct {
	pbsvc.UnimplementedPipelineServiceServer
	store     PipelineStore
	publisher Publisher
	blobStore BlobStore
	logger    infra.Logger
	userSvc   userpb.UserServiceClient
}

func NewService(store PipelineStore, publisher Publisher, blobStore BlobStore, logger infra.Logger, userSvc userpb.UserServiceClient) *Service {
	return &Service{
		store:     store,
		publisher: publisher,
		blobStore: blobStore,
		logger:    logger,
		userSvc:   userSvc,
	}
}

func (s *Service) ListPipelines(ctx context.Context, req *pbsvc.ListPipelinesRequest) (*pbsvc.ListPipelinesResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	pipelines, err := s.store.ListPipelines(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list pipelines", "error", err, "userId", req.UserId)
		return nil, status.Error(codes.Internal, "failed to read pipelines")
	}

	return &pbsvc.ListPipelinesResponse{
		Pipelines: pipelines,
	}, nil
}

func (s *Service) GetPipeline(ctx context.Context, req *pbsvc.GetPipelineRequest) (*pipeline.PipelineConfig, error) {
	if req.UserId == "" || req.PipelineId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and pipeline_id are required")
	}

	cfg, err := s.store.GetPipeline(ctx, req.UserId, req.PipelineId)
	if err != nil {
		s.logger.Error(ctx, "failed to get pipeline", "error", err)
		return nil, status.Error(codes.Internal, "failed to read pipeline config")
	}
	if cfg == nil {
		return nil, status.Error(codes.NotFound, "pipeline not found")
	}

	return cfg, nil
}

// validateAndNormalizeSources validates each source string and returns canonical enum names.
// At least one source must be provided. Duplicate sources are not allowed.
func validateAndNormalizeSources(sources []string) ([]string, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("at least one source is required")
	}
	seen := make(map[string]bool, len(sources))
	normalized := make([]string, 0, len(sources))
	for _, s := range sources {
		parsed := formatters.ParseActivitySource(s)
		if parsed == pbactivity.ActivitySource_SOURCE_UNSPECIFIED {
			return nil, fmt.Errorf("unknown source: %q", s)
		}
		canonical := parsed.String()
		if seen[canonical] {
			return nil, fmt.Errorf("duplicate source: %q", canonical)
		}
		seen[canonical] = true
		normalized = append(normalized, canonical)
	}
	return normalized, nil
}

func (s *Service) CreatePipeline(ctx context.Context, req *pbsvc.CreatePipelineRequest) (*pipeline.PipelineConfig, error) {
	if req.UserId == "" || req.Pipeline == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and pipeline config are required")
	}

	// Accept either sources (new) or source (legacy); always store as sources.
	rawSources := req.Pipeline.Sources
	if len(rawSources) == 0 && req.Pipeline.Source != "" {
		rawSources = []string{req.Pipeline.Source}
	}
	normalizedSources, err := validateAndNormalizeSources(rawSources)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid source: %s", err))
	}
	req.Pipeline.Sources = normalizedSources
	req.Pipeline.Source = ""

	if len(req.Pipeline.Destinations) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Missing required field: destinations (must be non-empty array)")
	}

	// Generate pipeline ID
	req.Pipeline.Id = fmt.Sprintf("pipe_%d", time.Now().UnixMilli())
	req.Pipeline.Disabled = false

	created, err := s.store.CreatePipeline(ctx, req.UserId, req.Pipeline)
	if err != nil {
		s.logger.Error(ctx, "failed to create pipeline", "error", err)
		return nil, status.Error(codes.Internal, "failed to create pipeline")
	}

	return created, nil
}

func (s *Service) UpdatePipeline(ctx context.Context, req *pbsvc.UpdatePipelineRequest) (*pipeline.PipelineConfig, error) {
	pipelineID := req.PipelineId
	if pipelineID == "" {
		pipelineID = req.Pipeline.GetId()
	}
	if req.UserId == "" || pipelineID == "" {
		return nil, status.Error(codes.InvalidArgument, "valid user_id and pipeline_id are required")
	}

	// Fetch existing pipeline to support partial updates (e.g. toggle disabled)
	existing, err := s.store.GetPipeline(ctx, req.UserId, pipelineID)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch existing pipeline for update", "error", err)
		return nil, status.Error(codes.Internal, "failed to fetch existing pipeline")
	}
	if existing == nil {
		return nil, status.Error(codes.NotFound, "pipeline not found")
	}

	// Merge: apply non-default fields from request onto existing
	if req.Pipeline != nil {
		// Accept sources (new) or source (legacy); always store as sources.
		rawSources := req.Pipeline.Sources
		if len(rawSources) == 0 && req.Pipeline.Source != "" {
			rawSources = []string{req.Pipeline.Source}
		}
		if len(rawSources) > 0 {
			normalizedSources, err := validateAndNormalizeSources(rawSources)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid source: %s", err))
			}
			existing.Sources = normalizedSources
			existing.Source = ""
		}
		if len(req.Pipeline.Destinations) > 0 {
			existing.Destinations = req.Pipeline.Destinations
		}
		if len(req.Pipeline.Enrichers) > 0 {
			existing.Enrichers = req.Pipeline.Enrichers
		}
		if req.Pipeline.Name != "" {
			existing.Name = req.Pipeline.Name
		}
		if req.Pipeline.SourceConfig != nil {
			existing.SourceConfig = req.Pipeline.SourceConfig
		}
		if req.Pipeline.DestinationConfigs != nil {
			existing.DestinationConfigs = req.Pipeline.DestinationConfigs
		}
		// Disabled is a bool — always apply from request
		existing.Disabled = req.Pipeline.Disabled
	}

	updated, err := s.store.UpdatePipeline(ctx, req.UserId, existing)
	if err != nil {
		s.logger.Error(ctx, "failed to update pipeline", "error", err)
		return nil, status.Error(codes.Internal, "failed to update pipeline")
	}

	return updated, nil
}

func (s *Service) DeletePipeline(ctx context.Context, req *pbsvc.DeletePipelineRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.PipelineId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and pipeline_id are required")
	}

	if err := s.store.DeletePipeline(ctx, req.UserId, req.PipelineId); err != nil {
		s.logger.Error(ctx, "failed to delete pipeline", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete pipeline")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) SubmitInput(ctx context.Context, req *pbsvc.SubmitInputRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.PendingInputId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and pending_input_id are required")
	}

	input, err := s.store.GetPendingInput(ctx, req.UserId, req.PendingInputId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get pending input")
	}
	if input == nil {
		return nil, status.Error(codes.NotFound, "pending input not found")
	}

	if input.Status != pipeline.PendingInput_STATUS_WAITING {
		return nil, status.Error(codes.FailedPrecondition, "input is not in WAITING state")
	}

	if input.LinkedActivityId == "" {
		return nil, status.Error(codes.Internal, "pending input missing linked activity ID")
	}

	payloadUri := input.OriginalPayloadUri
	if payloadUri == "" {
		// Pending inputs created directly by enrichers (e.g. parkrun auto-poll) bypass
		// handleWaitError and never have OriginalPayloadUri set. Fall back to the pipeline
		// run stored for the linked activity.
		run, runErr := s.store.FindPipelineRunByActivityId(ctx, req.UserId, input.LinkedActivityId)
		if runErr != nil {
			s.logger.Error(ctx, "failed to look up pipeline run for pending input fallback", "error", runErr, "activity_id", input.LinkedActivityId)
			return nil, status.Error(codes.Internal, "pending input missing payload URI and pipeline run lookup failed")
		}
		if run == nil || run.OriginalPayloadUri == "" {
			return nil, status.Error(codes.Internal, "pending input missing payload URI and no pipeline run found")
		}
		payloadUri = run.OriginalPayloadUri
	}

	// Fetch payload from GCS
	payloadBytes, err := s.blobStore.Get(ctx, payloadUri)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch original payload from GCS", "error", err, "uri", payloadUri)
		return nil, status.Error(codes.Internal, "failed to fetch original payload")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, status.Error(codes.Internal, "failed to parse original payload")
	}

	// Update payload for resume
	payload["isResume"] = true
	payload["resumePendingInputId"] = req.PendingInputId
	payload["activityId"] = input.LinkedActivityId

	// Re-serialize payload
	updatedPayloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to serialize updated payload")
	}

	// Publish to topic-pipeline-activity
	ce := cloudevents.NewEvent()
	ce.SetID(fmt.Sprintf("%d", time.Now().UnixNano()))
	ce.SetSource("com.fitglue.inputs_handler")
	ce.SetType("com.fitglue.cloud_event.input_resolved")
	ce.SetData(cloudevents.ApplicationJSON, updatedPayloadBytes)

	if _, err := s.publisher.PublishCloudEvent(ctx, "topic-pipeline-activity", ce); err != nil {
		s.logger.Error(ctx, "failed to publish resume event", "error", err)
		return nil, status.Error(codes.Internal, "failed to publish resume event")
	}

	// Mark as resolved in store
	input.InputData = req.InputData
	input.Status = pipeline.PendingInput_STATUS_COMPLETED
	if err := s.store.UpdatePendingInput(ctx, req.UserId, input); err != nil {
		s.logger.Error(ctx, "failed to update pending input status", "error", err)
		// We still return success as the resume event was published
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) ListPendingInputs(ctx context.Context, req *pbsvc.ListPendingInputsRequest) (*pbsvc.ListPendingInputsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	inputs, err := s.store.ListPendingInputs(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list pending inputs", "error", err)
		return nil, status.Error(codes.Internal, "failed to list inputs")
	}

	return &pbsvc.ListPendingInputsResponse{
		Inputs: inputs,
	}, nil
}

func (s *Service) ResolvePendingInput(ctx context.Context, req *pbsvc.ResolvePendingInputRequest) (*emptypb.Empty, error) {
	// This acts as a dismiss action in the legacy TS code
	if req.UserId == "" || req.PendingInputId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and pending_input_id are required")
	}

	input, err := s.store.GetPendingInput(ctx, req.UserId, req.PendingInputId)
	if err != nil || input == nil {
		return nil, status.Error(codes.NotFound, "pending input not found")
	}

	input.Status = pipeline.PendingInput_STATUS_COMPLETED
	if err := s.store.UpdatePendingInput(ctx, req.UserId, input); err != nil {
		s.logger.Error(ctx, "failed to dismiss pending input", "error", err)
		return nil, status.Error(codes.Internal, "failed to dismiss pending input")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) CancelPipeline(ctx context.Context, req *pbsvc.CancelPipelineRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.PendingInputId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and pending_input_id are required")
	}

	input, err := s.store.GetPendingInput(ctx, req.UserId, req.PendingInputId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get pending input")
	}
	if input == nil {
		return nil, status.Error(codes.NotFound, "pending input not found")
	}
	if input.Status != pipeline.PendingInput_STATUS_WAITING {
		return nil, status.Error(codes.FailedPrecondition, "input is not in WAITING state")
	}

	run, err := s.store.FindPipelineRunByPendingInputId(ctx, req.UserId, req.PendingInputId)
	if err != nil {
		s.logger.Error(ctx, "failed to find pipeline run by pending input ID", "error", err, "pendingInputId", req.PendingInputId)
		return nil, status.Error(codes.Internal, "failed to find pipeline run")
	}
	if run != nil {
		updateData := map[string]interface{}{
			"status": int32(pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_CANCELLED),
		}
		if err := s.store.UpdatePipelineRun(ctx, req.UserId, run.Id, updateData); err != nil {
			s.logger.Error(ctx, "failed to cancel pipeline run", "error", err, "runId", run.Id)
			return nil, status.Error(codes.Internal, "failed to cancel pipeline run")
		}
	}

	input.Status = pipeline.PendingInput_STATUS_COMPLETED
	if err := s.store.UpdatePendingInput(ctx, req.UserId, input); err != nil {
		s.logger.Error(ctx, "failed to update pending input after cancel", "error", err)
		return nil, status.Error(codes.Internal, "failed to update pending input")
	}

	s.enqueueCancelledNotification(ctx, req.UserId)
	return &emptypb.Empty{}, nil
}

func (s *Service) CancelPipelineRun(ctx context.Context, req *pbsvc.CancelPipelineRunRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and run_id are required")
	}

	run, err := s.store.GetPipelineRun(ctx, req.UserId, req.RunId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get pipeline run")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "pipeline run not found")
	}
	if run.Status != pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PENDING &&
		run.Status != pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING {
		return nil, status.Error(codes.FailedPrecondition, "pipeline run is not in a cancellable state")
	}

	updateData := map[string]interface{}{
		"status": int32(pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_CANCELLED),
	}
	if err := s.store.UpdatePipelineRun(ctx, req.UserId, run.Id, updateData); err != nil {
		s.logger.Error(ctx, "failed to cancel pipeline run", "error", err, "runId", run.Id)
		return nil, status.Error(codes.Internal, "failed to cancel pipeline run")
	}

	// Best-effort: also complete any linked pending input so the UI cleans up
	if pendingID := run.GetPendingInputId(); pendingID != "" {
		input, fetchErr := s.store.GetPendingInput(ctx, req.UserId, pendingID)
		if fetchErr == nil && input != nil && input.Status == pipeline.PendingInput_STATUS_WAITING {
			input.Status = pipeline.PendingInput_STATUS_COMPLETED
			if err := s.store.UpdatePendingInput(ctx, req.UserId, input); err != nil {
				s.logger.Warn(ctx, "best-effort pending input cleanup failed", "error", err, "pendingId", pendingID)
			}
		}
	}

	s.enqueueCancelledNotification(ctx, req.UserId)
	return &emptypb.Empty{}, nil
}

// enqueueCancelledNotification sends a PIPELINE_CANCELLED notification to the user.
// Best-effort — a publish failure is logged but does not fail the cancel operation.
func (s *Service) enqueueCancelledNotification(ctx context.Context, userID string) {
	req := &pbnotification.NotificationRequest{
		UserId: userID,
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_CANCELLED,
		Title:  "Pipeline Cancelled",
		Body:   "Your activity pipeline was cancelled.",
	}
	if err := notificationpub.Enqueue(ctx, s.publisher, req); err != nil {
		s.logger.Warn(ctx, "Failed to enqueue pipeline cancelled notification", "error", err, "user_id", userID)
	}
}

func (s *Service) RepostActivity(ctx context.Context, req *pbsvc.RepostActivityRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}

	// Validate mode
	switch req.Mode {
	case "full-pipeline":
		// No additional fields required
	case "missed-destination", "retry-destination":
		if req.Destination == "" {
			return nil, status.Error(codes.InvalidArgument, "destination is required for mode: "+req.Mode)
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "mode must be one of: full-pipeline, missed-destination, retry-destination")
	}

	// Look up the most recent pipeline run for this activity (Rule E35)
	run, err := s.store.FindPipelineRunByActivityId(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to find pipeline run by activity", "error", err, "activityId", req.ActivityId)
		return nil, status.Error(codes.Internal, "failed to look up pipeline run")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "no pipeline run found for activity")
	}

	// Rule E22 (Reset-on-Repost): always use clean, unmutated original payload
	if run.OriginalPayloadUri == "" {
		return nil, status.Error(codes.FailedPrecondition, "pipeline run has no original payload URI; activity is not repostable")
	}

	payloadBytes, err := s.blobStore.Get(ctx, run.OriginalPayloadUri)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch original payload from GCS", "error", err, "uri", run.OriginalPayloadUri)
		return nil, status.Error(codes.Internal, "failed to fetch original payload")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		s.logger.Error(ctx, "failed to parse original payload", "error", err)
		return nil, status.Error(codes.Internal, "failed to parse original payload")
	}

	// Inject repost metadata
	payload["isRepost"] = true
	payload["repostMode"] = req.Mode
	payload["activityId"] = req.ActivityId
	if req.Destination != "" {
		payload["repostDestination"] = req.Destination
	}

	updatedPayloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to serialize repost payload")
	}

	// Publish to topic-raw-activity so the activity re-enters the full pipeline
	ce := cloudevents.NewEvent()
	ce.SetID(fmt.Sprintf("%d", time.Now().UnixNano()))
	ce.SetSource("com.fitglue.repost_handler")
	ce.SetType("com.fitglue.cloud_event.repost")
	ce.SetData(cloudevents.ApplicationJSON, updatedPayloadBytes)

	if _, err := s.publisher.PublishCloudEvent(ctx, shared.TopicRawActivity, ce); err != nil {
		s.logger.Error(ctx, "failed to publish repost event", "error", err)
		return nil, status.Error(codes.Internal, "failed to publish repost event")
	}

	s.logger.Info(ctx, "Repost published", "activityId", req.ActivityId, "mode", req.Mode, "topic", shared.TopicRawActivity)
	return &emptypb.Empty{}, nil
}

func (s *Service) GetPipelineRun(ctx context.Context, req *pbsvc.GetPipelineRunRequest) (*pipeline.PipelineRun, error) {
	if req.UserId == "" || req.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	run, err := s.store.GetPipelineRun(ctx, req.UserId, req.RunId)
	if err != nil {
		s.logger.Error(ctx, "failed to get pipeline run", "error", err)
		return nil, status.Error(codes.Internal, "failed to read run")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "run not found")
	}

	return run, nil
}

func (s *Service) ListPipelineRuns(ctx context.Context, req *pbsvc.ListPipelineRunsRequest) (*pbsvc.ListPipelineRunsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	var since, until *time.Time
	if req.Since != nil {
		t := req.Since.AsTime()
		since = &t
	}
	if req.Until != nil {
		t := req.Until.AsTime()
		until = &t
	}
	runs, nextToken, err := s.store.ListPipelineRuns(ctx, req.UserId, req.PipelineId, req.Limit, req.PageToken, since, until)
	if err != nil {
		s.logger.Error(ctx, "failed to list pipeline runs", "error", err)
		return nil, status.Error(codes.Internal, "failed to list runs")
	}

	return &pbsvc.ListPipelineRunsResponse{
		Runs:          runs,
		NextPageToken: nextToken,
	}, nil
}

func (s *Service) AdminListPipelineRuns(ctx context.Context, req *pbsvc.AdminListPipelineRunsRequest) (*pbsvc.AdminListPipelineRunsResponse, error) {
	limit := req.GetLimit()
	runs, err := s.store.AdminListPipelineRuns(ctx, req.GetStatus(), req.GetSource(), req.GetUserId(), limit)
	if err != nil {
		s.logger.Error(ctx, "failed to admin list pipeline runs", "error", err)
		return nil, status.Error(codes.Internal, "failed to list pipeline runs")
	}
	return &pbsvc.AdminListPipelineRunsResponse{Runs: runs}, nil
}

func (s *Service) ListSourceActivities(ctx context.Context, req *pbsvc.ListSourceActivitiesRequest) (*pbsvc.ListSourceActivitiesResponse, error) {
	if req.UserId == "" || req.Source == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and source are required")
	}
	if s.userSvc == nil {
		return nil, status.Error(codes.Unimplemented, "user service not available")
	}

	provider, ok := sourceplugins.ForSource(req.Source)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "source %q does not support historical sync", req.Source)
	}

	integResp, err := s.userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   req.UserId,
		Provider: req.Source,
	})
	if err != nil {
		s.logger.Error(ctx, "failed to get integration", "error", err, "source", req.Source)
		return nil, status.Error(codes.Internal, "failed to retrieve integration")
	}

	rawActivities, nextPageToken, err := provider.ListActivities(ctx, integResp.Integrations, req.PageToken)
	if err != nil {
		s.logger.Error(ctx, "failed to list source activities", "error", err, "source", req.Source)
		return nil, status.Error(codes.Internal, "failed to list activities from source")
	}

	items := make([]*pbsvc.SourceActivityItem, 0, len(rawActivities))
	for _, a := range rawActivities {
		var existingRun *pipeline.PipelineRun
		if req.PipelineId != "" {
			existingRun, err = s.store.FindPipelineRunBySourceActivityID(ctx, req.UserId, req.PipelineId, a.ID)
		} else {
			existingRun, err = s.store.FindAnyPipelineRunBySourceActivityID(ctx, req.UserId, a.ID)
		}
		if err != nil {
			s.logger.Error(ctx, "dedup check failed", "error", err, "sourceActivityId", a.ID)
		}
		items = append(items, &pbsvc.SourceActivityItem{
			SourceActivityId: a.ID,
			Title:            a.Title,
			Type:             a.Type,
			StartTime:        timestamppb.New(a.StartTime),
			AlreadySynced:    existingRun != nil,
		})
	}

	return &pbsvc.ListSourceActivitiesResponse{
		Activities:    items,
		NextPageToken: nextPageToken,
	}, nil
}

func (s *Service) BackfillActivities(ctx context.Context, req *pbsvc.BackfillActivitiesRequest) (*pbsvc.BackfillActivitiesResponse, error) {
	if req.UserId == "" || req.Source == "" || len(req.SourceActivityIds) == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id, source, and source_activity_ids are required")
	}
	if s.userSvc == nil {
		return nil, status.Error(codes.Unimplemented, "user service not available")
	}

	provider, ok := sourceplugins.ForSource(req.Source)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "source %q does not support historical sync", req.Source)
	}

	integResp, err := s.userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   req.UserId,
		Provider: req.Source,
	})
	if err != nil {
		s.logger.Error(ctx, "failed to get integration", "error", err, "source", req.Source)
		return nil, status.Error(codes.Internal, "failed to retrieve integration")
	}

	var queued int32
	for _, sourceActivityID := range req.SourceActivityIds {
		payload, err := provider.FetchActivity(ctx, integResp.Integrations, req.UserId, sourceActivityID)
		if err != nil {
			s.logger.Error(ctx, "failed to fetch activity for backfill", "error", err, "sourceActivityId", sourceActivityID)
			continue
		}

		// Pre-target the pipeline so the splitter routes directly without re-matching.
		// When pipeline_id is empty (connection-scoped backfill), let the splitter route
		// to all matching pipelines for this user/source combination.
		if req.PipelineId != "" {
			payload.PipelineId = &req.PipelineId
		}

		ce, err := infrapubsub.NewCloudEvent("com.fitglue.backfill", "com.fitglue.cloud_event.backfill", payload)
		if err != nil {
			s.logger.Error(ctx, "failed to build backfill cloud event", "error", err)
			continue
		}
		ce.SetID(fmt.Sprintf("%d-%s", time.Now().UnixNano(), sourceActivityID))

		if _, err := s.publisher.PublishCloudEvent(ctx, shared.TopicRawActivity, ce); err != nil {
			s.logger.Error(ctx, "failed to publish backfill event", "error", err, "sourceActivityId", sourceActivityID)
			continue
		}

		queued++
	}

	s.logger.Info(ctx, "Backfill queued", "pipelineId", req.PipelineId, "source", req.Source, "queued", queued, "requested", len(req.SourceActivityIds), "connection_scoped", req.PipelineId == "")
	return &pbsvc.BackfillActivitiesResponse{QueuedCount: queued}, nil
}
