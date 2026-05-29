package activity

import (
	"context"
	"fmt"

	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/domain/fit_parser"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) ParseFitFile(ctx context.Context, req *pbsvc.ParseFitFileRequest) (*pbactivity.StandardizedActivity, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(req.FitFileContent) == 0 {
		return nil, status.Error(codes.InvalidArgument, "fit_file_content is required")
	}

	// Parse FIT file
	activity, err := fit_parser.ParseFitFile(req.FitFileContent)
	if err != nil {
		s.logger.Error(ctx, "failed to parse FIT file", "error", err)
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("failed to parse FIT file: %v", err))
	}

	// Generate unique external ID
	externalId := fmt.Sprintf("upload_%s", uuid.NewString())

	// Apply user overrides
	activity.UserId = req.UserId
	activity.ExternalId = externalId
	if req.Title != "" {
		activity.Name = req.Title
	} else if activity.StartTime != nil {
		// No title provided at upload time — reset to the auto-generated fallback so that
		// the iCal title enricher (which detects auto-generated names by value-comparison)
		// is allowed to override it. Any SportProfileName from the FIT file is not treated
		// as an explicit user title when the upload form was left blank.
		activity.Name = fit_parser.GenerateActivityName(activity.Type, activity.StartTime.AsTime())
	}
	if req.Description != "" {
		activity.Description = req.Description
	}

	// Create activity payload
	execID := uuid.NewString()
	payload := &pbevents.ActivityPayload{
		Source:               pbactivity.ActivitySource_SOURCE_FILE_UPLOAD,
		UserId:               req.UserId,
		Timestamp:            timestamppb.Now(),
		StandardizedActivity: activity,
		IsResume:             false,
		PipelineExecutionId:  &execID,
	}

	if req.PipelineId != "" {
		payload.PipelineId = &req.PipelineId
	}

	// Create CloudEvent for publishing
	event, err := infrapubsub.NewCloudEvent("/fit-parser", "com.fitglue.activity.raw", payload)
	if err != nil {
		s.logger.Error(ctx, "failed to create cloud event", "error", err)
		return nil, status.Error(codes.Internal, "failed to create event")
	}

	// Publish to raw-activity topic
	_, err = s.publisher.PublishCloudEvent(ctx, shared.TopicRawActivity, event)
	if err != nil {
		s.logger.Error(ctx, "failed to publish raw activity event", "error", err)
		return nil, status.Error(codes.Internal, "failed to queue activity")
	}

	return activity, nil
}
