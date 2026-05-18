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
	"github.com/fitglue/server/src/go/pkg/types/formatters"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Service implements the pbsvc.PipelineServiceServer interface.
type Service struct {
	pbsvc.UnimplementedPipelineServiceServer
	store     PipelineStore
	publisher Publisher
	blobStore BlobStore
	logger    infra.Logger
}

func NewService(store PipelineStore, publisher Publisher, blobStore BlobStore, logger infra.Logger) *Service {
	return &Service{
		store:     store,
		publisher: publisher,
		blobStore: blobStore,
		logger:    logger,
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

	if input.OriginalPayloadUri == "" || input.LinkedActivityId == "" {
		return nil, status.Error(codes.Internal, "pending input missing payload URI or linked activity ID")
	}

	// Fetch payload from GCS
	payloadBytes, err := s.blobStore.Get(ctx, input.OriginalPayloadUri)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch original payload from GCS", "error", err, "uri", input.OriginalPayloadUri)
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

	runs, nextToken, err := s.store.ListPipelineRuns(ctx, req.UserId, req.PipelineId, req.Limit, req.PageToken)
	if err != nil {
		s.logger.Error(ctx, "failed to list pipeline runs", "error", err)
		return nil, status.Error(codes.Internal, "failed to list runs")
	}

	return &pbsvc.ListPipelineRunsResponse{
		Runs:          runs,
		NextPageToken: nextToken,
	}, nil
}
