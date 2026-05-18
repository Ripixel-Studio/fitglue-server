package splitter

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/internal/pipeline"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/domain/activity"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	"github.com/fitglue/server/src/go/pkg/types/formatters"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"github.com/google/uuid"
)

type Splitter struct {
	store     pipeline.PipelineStore
	publisher pipeline.Publisher
	logger    infra.Logger
}

func NewSplitter(store pipeline.PipelineStore, publisher pipeline.Publisher, logger infra.Logger) *Splitter {
	return &Splitter{
		store:     store,
		publisher: publisher,
		logger:    logger,
	}
}

// SplitByPipeline receives raw activities and fans out to per-pipeline messages
func (s *Splitter) SplitByPipeline(ctx context.Context, e cloudevents.Event) error {
	// Parse ActivityPayload
	rawData := e.Data()

	// Sanitize payload to handle legacy objects in string fields
	rawData = activity.SanitizeActivityPayloadJSON(rawData)

	var payload pbevents.ActivityPayload
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(rawData, &payload); err != nil {
		return fmt.Errorf("protojson unmarshal: %w", err)
	}

	// If pipelineId is already set, pass through unchanged (resume/targeted repost)
	if payload.PipelineId != nil && *payload.PipelineId != "" {
		s.logger.Info(ctx, "PipelineId already set, passing through", "pipelineId", *payload.PipelineId)
		return s.passThrough(ctx, &payload)
	}

	// Resolve matching pipelines for this source
	pipelines, err := s.resolvePipelinesForSource(ctx, payload.UserId, payload.Source)
	if err != nil {
		return fmt.Errorf("resolve pipelines: %w", err)
	}

	if len(pipelines) == 0 {
		s.logger.Info(ctx, "No pipelines configured for source", "source", payload.Source.String())
		return nil
	}

	// Fan out: publish one message per pipeline
	basePipelineExecId := ""
	if payload.PipelineExecutionId != nil {
		basePipelineExecId = *payload.PipelineExecutionId
	}
	if basePipelineExecId == "" {
		basePipelineExecId = uuid.NewString()
		s.logger.Warn(ctx, "PipelineExecutionId missing from payload, generated fallback", "fallback_id", basePipelineExecId)
	}

	s.logger.Info(ctx, "Fanning out to pipelines", "count", len(pipelines), "source", payload.Source.String())

	publishedCount := 0
	for _, p := range pipelines {
		pipelineId := p.Id
		pipelineExecId := fmt.Sprintf("%s-%s", basePipelineExecId, pipelineId)

		// Clone the payload for this pipeline
		clonedPayload := proto.Clone(&payload).(*pbevents.ActivityPayload)
		clonedPayload.PipelineId = &pipelineId
		clonedPayload.PipelineExecutionId = &pipelineExecId

		// Publish to pipeline-activity topic
		if err := s.publishToPipelineActivity(ctx, clonedPayload); err != nil {
			s.logger.Error(ctx, "Failed to publish pipeline message", "pipelineId", pipelineId, "error", err)
			continue
		}

		publishedCount++
		s.logger.Info(ctx, "Published pipeline message", "pipelineId", pipelineId, "pipelineExecId", pipelineExecId)
	}

	return nil
}

// passThrough publishes the payload directly to pipeline-activity topic without modification
func (s *Splitter) passThrough(ctx context.Context, payload *pbevents.ActivityPayload) error {
	if err := s.publishToPipelineActivity(ctx, payload); err != nil {
		return fmt.Errorf("pass-through publish: %w", err)
	}
	return nil
}

// resolvePipelinesForSource finds all pipelines matching the given source
func (s *Splitter) resolvePipelinesForSource(ctx context.Context, userId string, source pbactivity.ActivitySource) ([]*pbpipeline.PipelineConfig, error) {
	userPipelines, err := s.store.ListPipelines(ctx, userId)
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}

	var matching []*pbpipeline.PipelineConfig

	for _, p := range userPipelines {
		// Skip disabled pipelines
		if p.Disabled {
			s.logger.Info(ctx, "Skipping disabled pipeline", "id", p.Id, "name", p.Name)
			continue
		}

		// Match by source - normalize the stored source string to an enum for comparison,
		// since Firestore stores "file_upload" but the proto enum name is "SOURCE_FILE_UPLOAD"
		parsedSource := formatters.ParseActivitySource(p.Source)
		if parsedSource == source && parsedSource != pbactivity.ActivitySource_SOURCE_UNSPECIFIED {
			matching = append(matching, p)
		}
	}

	return matching, nil
}

// publishToPipelineActivity publishes an ActivityPayload to the pipeline-activity topic
func (s *Splitter) publishToPipelineActivity(ctx context.Context, payload *pbevents.ActivityPayload) error {
	// Serialize to JSON
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Create CloudEvent
	ce, err := infrapubsub.NewCloudEvent(
		infrapubsub.GetCloudEventSource(pbevents.CloudEventSource_CLOUD_EVENT_SOURCE_PIPELINE_SPLITTER),
		"com.fitglue.activity.pipeline",
		data,
	)
	if err != nil {
		return fmt.Errorf("create cloud event: %w", err)
	}

	// Set extensions for tracing
	if payload.PipelineExecutionId != nil {
		ce.SetExtension("pipeline_execution_id", *payload.PipelineExecutionId)
	}

	// Publish
	_, err = s.publisher.PublishCloudEvent(ctx, shared.TopicPipelineActivity, ce)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	return nil
}
