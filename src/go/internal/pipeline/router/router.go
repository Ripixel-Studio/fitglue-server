package router

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/internal/pipeline"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/domain/activity"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
)

type Router struct {
	store      pipeline.PipelineStore
	publisher  pipeline.Publisher
	blobStore  pipeline.BlobStore
	bucketName string
	logger     infra.Logger
}

func NewRouter(store pipeline.PipelineStore, publisher pipeline.Publisher, blobStore pipeline.BlobStore, bucketName string, logger infra.Logger) *Router {
	return &Router{
		store:      store,
		publisher:  publisher,
		blobStore:  blobStore,
		bucketName: bucketName,
		logger:     logger,
	}
}

// RouteActivity is the entry point
func (r *Router) RouteActivity(ctx context.Context, e cloudevents.Event) error {
	rawData := e.Data()

	// Sanitize payload to handle legacy objects in string fields
	rawData = activity.SanitizeActivityPayloadJSON(rawData)

	var eventPayload pbevents.EnrichedActivityEvent
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(rawData, &eventPayload); err != nil {
		return fmt.Errorf("protojson unmarshal: %w", err)
	}

	r.logger.Info(ctx, "Starting routing", "source", eventPayload.Source.String(), "pipeline", eventPayload.PipelineId)

	pipelineExecID := ""
	if eventPayload.PipelineExecutionId != nil {
		pipelineExecID = *eventPayload.PipelineExecutionId
	}
	if pipelineExecID == "" {
		pipelineExecID = uuid.NewString()
		r.logger.Warn(ctx, "PipelineExecutionId missing from enriched event, generated fallback", "fallback_id", pipelineExecID)
	}

	destinations := eventPayload.Destinations
	r.logger.Info(ctx, "Resolved destinations from payload", "dests", destinations)

	routedCount := 0
	for _, dest := range destinations {
		destName := dest.String()

		// Publish to consolidated destination upload topic
		topic := shared.TopicDestinationUpload

		routeEvent, err := infrapubsub.NewCloudEvent(
			infrapubsub.GetCloudEventSource(pbevents.CloudEventSource_CLOUD_EVENT_SOURCE_ROUTER),
			fmt.Sprintf("com.fitglue.job.%s", destName),
			rawData,
		)
		if err != nil {
			r.logger.Error(ctx, "Failed to create routing event", "error", err)
			continue
		}

		routeEvent.SetExtension("pipeline_execution_id", pipelineExecID)

		resID, err := r.publisher.PublishCloudEvent(ctx, topic, routeEvent)
		if err != nil {
			r.logger.Error(ctx, "Failed to publish to queue", "dest", destName, "topic", topic, "error", err)
		} else {
			r.logger.Info(ctx, "Routed to destination", "dest", destName, "topic", topic, "message_id", resID)
			routedCount++
		}
	}

	r.logger.Info(ctx, "Routing complete", "routed_count", routedCount)

	if eventPayload.UserId != "" && pipelineExecID != "" {
		// Write a proto Timestamp directly so the Firestore client stores a native
		// timestamp. protojson.Format yields a quote-wrapped JSON string ("...Z") that
		// Firestore persists verbatim, which then fails to decode back into the
		// PipelineRun proto (invalid google.protobuf.Timestamp value) and wedges the run.
		updateData := map[string]interface{}{
			"updated_at": timestamppb.Now(),
		}

		if eventPayload.ActivityDataUri != "" {
			updateData["enriched_event_uri"] = eventPayload.ActivityDataUri
			r.logger.Debug(ctx, "Reusing activity_data_uri for enriched_event_uri", "uri", eventPayload.ActivityDataUri)
		} else {
			if r.bucketName != "" {
				gcsPath := fmt.Sprintf("enriched_events/%s/%s.json", eventPayload.UserId, pipelineExecID)
				jsonBytes, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&eventPayload)
				if err != nil {
					r.logger.Warn(ctx, "Failed to marshal enriched event for GCS", "error", err)
				} else if err := r.blobStore.Write(ctx, r.bucketName, gcsPath, jsonBytes); err != nil {
					r.logger.Warn(ctx, "Failed to upload enriched event to GCS", "error", err)
				} else {
					updateData["enriched_event_uri"] = fmt.Sprintf("gs://%s/%s", r.bucketName, gcsPath)
					r.logger.Debug(ctx, "Uploaded enriched event to GCS", "uri", updateData["enriched_event_uri"])
				}
			}
		}

		if err := r.store.UpdatePipelineRun(ctx, eventPayload.UserId, pipelineExecID, updateData); err != nil {
			r.logger.Error(ctx, "Failed to update pipeline run with enriched event URI", "error", err, "pipeline_run_id", pipelineExecID)
		}
	}

	return nil
}
