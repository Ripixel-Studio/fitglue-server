// nolint:proto-json
package enricher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"google.golang.org/protobuf/encoding/protojson"

	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	activityPkg "github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/framework"

	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"

	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	// Register providers
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/activity_filter"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/ai_activity_type"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/ai_banner"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/ai_companion"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/auto_increment"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/branding"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/cadence_summary"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/calories_burned"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/condition_matcher"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/distance_milestones"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/effort_score"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/elevation_summary"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/fit_file_heart_rate"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/fitbit_heart_rate"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/goal_tracker"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/heart_rate_summary"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/heart_rate_zones"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/hybrid_race_tagger"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/ical_title"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/intervals"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/location_naming"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/logic_gate"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/manual_workout_entry"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/mock"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/muscle_heatmap"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/muscle_heatmap_image"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/pace_summary"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/parkrun"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/personal_records"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/photo_upload"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/power_summary"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/recovery_advisor"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/route_thumbnail"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/running_dynamics"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/source_link"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/speed_summary"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/spotify_tracks"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/streak_tracker"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/training_load"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/type_mapper"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/virtual_gps"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/weather"
	_ "github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/workout_summary"
)

var (
	svc     *bootstrap.Service
	svcOnce sync.Once
	svcErr  error
)

func init() {
	// CloudEvent handler for EventArc triggers (raw-activity topic)
	functions.CloudEvent("EnrichActivity", EnrichActivity)

	// HTTP handler for push subscriptions (lag topic) - properly returns HTTP 500 on error
	functions.HTTP("EnrichActivityHTTP", EnrichActivityHTTP)
}

func initService(ctx context.Context) (*bootstrap.Service, error) {
	if svc != nil {
		return svc, nil
	}
	svcOnce.Do(func() {
		svc, svcErr = bootstrap.NewService(ctx)
	})
	return svc, svcErr
}

// EnrichActivity is the entry point for EventArc triggers
func EnrichActivity(ctx context.Context, e cloudevents.Event) error {
	svc, err := initService(ctx)
	if err != nil {
		return fmt.Errorf("service init failed: %v", err)
	}
	return framework.WrapCloudEvent("enricher", svc, enrichHandler)(ctx, e)
}

// EnrichActivityHTTP is the HTTP handler for push subscriptions (lag topic).
// This handler properly returns HTTP 500 on errors, allowing Pub/Sub to NACK and retry.
func EnrichActivityHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	svc, err := initService(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("service init failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Parse CloudEvent from request
	// Try CloudEvents format first (structured or binary)
	event, err := cehttp.NewEventFromHTTPRequest(r)
	if err != nil {
		// Fall back to Pub/Sub push message format
		event, err = parseCloudEventFromPubSubPush(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to parse event: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Call the existing CloudEvent handler
	handlerErr := framework.WrapCloudEvent("enricher", svc, enrichHandler)(ctx, *event)

	if handlerErr != nil {
		// Return HTTP 500 to trigger Pub/Sub NACK and retry
		http.Error(w, handlerErr.Error(), http.StatusInternalServerError)
		return
	}

	// Success - Pub/Sub will ACK
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// parseCloudEventFromPubSubPush parses a CloudEvent from a Pub/Sub push message.
// Pub/Sub push sends messages in a wrapper format with message.data containing the actual event.
func parseCloudEventFromPubSubPush(r *http.Request) (*cloudevents.Event, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	// Pub/Sub push format: {"message": {"data": "base64...", "messageId": "...", ...}, "subscription": "..."}
	var pushMsg struct {
		Message struct {
			Data        []byte            `json:"data"`
			Attributes  map[string]string `json:"attributes"`
			MessageID   string            `json:"messageId"`
			PublishTime string            `json:"publishTime"`
		} `json:"message"`
		Subscription string `json:"subscription"`
	}

	if err := json.Unmarshal(body, &pushMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal push message: %w", err)
	}

	if len(pushMsg.Message.Data) == 0 {
		return nil, fmt.Errorf("no data in push message")
	}

	// The data might be a CloudEvent JSON or just the payload
	// Try to parse as CloudEvent first
	var event cloudevents.Event
	if err := json.Unmarshal(pushMsg.Message.Data, &event); err == nil && event.Type() != "" {
		return &event, nil
	}

	// If not a CloudEvent, create one from the raw data
	event = cloudevents.NewEvent()
	event.SetID(pushMsg.Message.MessageID)
	event.SetSource(infrapubsub.GetCloudEventSource(pbevents.CloudEventSource_CLOUD_EVENT_SOURCE_ENRICHER))
	event.SetType(infrapubsub.GetCloudEventType(pbevents.CloudEventType_CLOUD_EVENT_TYPE_ENRICHMENT_LAG))
	event.SetData(cloudevents.ApplicationJSON, pushMsg.Message.Data)

	// Copy attributes as extensions
	for k, v := range pushMsg.Message.Attributes {
		event.SetExtension(k, v)
	}

	return &event, nil
}

// enrichHandler contains the business logic
func enrichHandler(ctx context.Context, e cloudevents.Event, fwCtx *framework.FrameworkContext) (interface{}, error) {
	// Extract payload and attributes
	// We assume strict CloudEvent input (legacy Pub/Sub messages are no longer supported)
	rawData := e.Data()
	isLagRetry := false
	if val, ok := e.Extensions()["origin"].(string); ok && val == "lag-queue" {
		isLagRetry = true
	}

	var rawEvent pbevents.ActivityPayload
	// Use protojson to unmarshal, which supports both camelCase (canonical) and snake_case field names
	unmarshalOpts := protojson.UnmarshalOptions{
		DiscardUnknown: true, // Be resilient to future schema changes
	}
	if err := unmarshalOpts.Unmarshal(rawData, &rawEvent); err != nil {
		return nil, fmt.Errorf("protojson unmarshal: %v", err)
	}

	if rawEvent.UserId == "" {
		return nil, fmt.Errorf("missing userId in payload")
	}

	fwCtx.Logger.Info("Starting enrichment", "timestamp", rawEvent.Timestamp, "source", rawEvent.Source)

	// Extract pipeline_execution_id from payload or use current execution ID
	pipelineExecID := rawEvent.PipelineExecutionId
	if pipelineExecID == nil || *pipelineExecID == "" {
		pipelineExecID = &fwCtx.ExecutionID
	}

	// Initialize Orchestrator
	bucketName := fwCtx.Service.Config.GCSArtifactBucket
	if bucketName == "" {
		bucketName = "fitglue-server-dev-artifacts" // Fallback for local development
	}

	orchestrator := NewOrchestrator(fwCtx.Service.DB, fwCtx.Service.Store, bucketName, fwCtx.Service.Notifications)

	// Register Providers from registry
	for _, provider := range providers.GetAll() {
		// Set service if the provider supports it
		if sp, ok := provider.(interface{ SetService(*bootstrap.Service) }); ok {
			sp.SetService(fwCtx.Service)
		}
		orchestrator.Register(provider)
	}

	// Calculate lag exhaustion (Force mode / Do Not Retry)
	doNotRetry := false
	// For Pub/Sub events, e.Time() is the publish time.
	// We want to force if the message is older than our max backoff (20 mins + buffer)
	// Note: For unwrapped events, e.Time() is the original event time, which is what we want.
	if !e.Time().IsZero() {
		lagDuration := time.Since(e.Time())
		if lagDuration > 15*time.Minute {
			fwCtx.Logger.Warn("Activity lag exhausted, forcing partial enrichment", "age", lagDuration)
			doNotRetry = true
		}
	}

	// Process
	processResult, err := orchestrator.Process(ctx, fwCtx.Logger, &rawEvent, fwCtx.ExecutionID, *pipelineExecID, doNotRetry)

	if err != nil {
		// Check if the error is retryable (e.g. data lag)
		if ok := isRetryable(err); ok {

			if isLagRetry {
				fwCtx.Logger.Warn("Lag Retry failed (will retry with backoff)", "error", err)
				// Return error to trigger Pub/Sub retry with backoff (keep status for execution tracking)
				fwCtx.Logger.Info("Returning error to trigger retry", "status", "STATUS_LAGGED_RETRY")
				return map[string]interface{}{
					"status": "STATUS_LAGGED_RETRY",
					"error":  err.Error(),
				}, fmt.Errorf("lagged retry failed (status=STATUS_LAGGED_RETRY): %w", err)
			} else {
				// Preserve the original error before it gets shadowed
				originalErr := err
				fwCtx.Logger.Info("Activity data lagging, offloading to lag queue", "error", originalErr)

				// Publish to Lag Topic with "origin=lag-queue" to break infinite loop on next consumption
				// Create CloudEvent
				lagEvent, err := infrapubsub.NewCloudEvent("/enricher", "com.fitglue.enrichment.lag", rawData)
				if err != nil {
					fwCtx.Logger.Error("Failed to create lag event", "error", err)
					return nil, err
				}
				lagEvent.SetExtension("origin", "lag-queue")

				_, pubErr := fwCtx.Service.Pub.PublishCloudEvent(ctx, shared.TopicEnrichmentLag, lagEvent)
				if pubErr != nil {
					fwCtx.Logger.Error("Failed to publish to lag topic", "error", pubErr)
					return nil, pubErr // Fail execution to trigger retry of this offload attempt
				}

				return map[string]interface{}{
					"status": "LAGGED_RETRY",
					"reason": originalErr.Error(),
				}, nil // ACK original message since we've successfully moved it to the delay queue
			}
		}

		fwCtx.Logger.Error("Orchestrator failed", "error", err)
		return nil, err
	}

	if len(processResult.Events) == 0 {
		fwCtx.Logger.Info("No pipelines matched, skipping enrichment")
		return map[string]interface{}{
			"status":              "SKIPPED",
			"reason":              "No enriched event created - possibly halted by a provider",
			"published_events":    []interface{}{},
			"provider_executions": processResult.ProviderExecutions,
		}, nil
	}

	// Publish Results to Router
	var publishedCount int

	// Track published events for rich output
	type PublishedEvent struct {
		ActivityID         string   `json:"activity_id"`
		PipelineID         string   `json:"pipeline_id"`
		Destinations       []string `json:"destinations"`
		AppliedEnrichments []string `json:"applied_enrichments"`
		FitFileURI         string   `json:"fit_file_uri,omitempty"`
		PubSubMessageID    string   `json:"pubsub_message_id"`
	}
	publishedEvents := []PublishedEvent{}

	for _, event := range processResult.Events {
		// Propagate pipeline execution ID
		event.PipelineExecutionId = pipelineExecID

		// Always offload activity data to GCS for consistent behavior
		// This ensures all destinations (especially Showcase) have access to the data
		eventToPublish := event
		bucketName := fwCtx.Service.Config.GCSArtifactBucket
		if bucketName != "" {
			preparedEvent, uploadedSize, err := activityPkg.PrepareForPublish(ctx, event, fwCtx.Service.Store, bucketName)
			if err != nil {
				fwCtx.Logger.Warn("Failed to offload activity data to GCS, publishing inline", "error", err)
			} else if uploadedSize > 0 {
				eventToPublish = preparedEvent
				fwCtx.Logger.Info("Offloaded activity data to GCS",
					"uri", preparedEvent.ActivityDataUri,
					"size_bytes", uploadedSize)
			}
		}

		resultEvent, err := infrapubsub.NewCloudEvent("/enricher", "com.fitglue.activity.enriched", eventToPublish)
		if err != nil {
			fwCtx.Logger.Error("Failed to create result event", "error", err)
			continue
		}

		// Add as CloudEvent extension for framework to extract
		resultEvent.SetExtension("pipeline_execution_id", *pipelineExecID)

		msgID, err := fwCtx.Service.Pub.PublishCloudEvent(ctx, shared.TopicEnrichedActivity, resultEvent)
		if err != nil {
			fwCtx.Logger.Error("Failed to publish result", "error", err, "pipeline_id", event.PipelineId)
		} else {
			publishedCount++
			fwCtx.Logger.Info("Published enriched event",
				"activity_id", event.ActivityId,
				"pipeline_id", event.PipelineId,
				"destinations", event.Destinations,
				"message_id", msgID)

			publishedEvents = append(publishedEvents, PublishedEvent{
				ActivityID:         event.ActivityId,
				PipelineID:         event.PipelineId,
				Destinations:       destinationsToStrings(event.Destinations),
				AppliedEnrichments: event.AppliedEnrichments,
				FitFileURI:         event.FitFileUri,
				PubSubMessageID:    msgID,
			})
		}
	}

	fwCtx.Logger.Info("Enrichment complete", "published_count", publishedCount)

	finalStatus := "SUCCESS"
	if processResult.Status == pbpipeline.ExecutionStatus_STATUS_WAITING {
		finalStatus = "WAITING"
	}

	return map[string]interface{}{
		"status":              finalStatus,
		"published_count":     publishedCount,
		"total_events":        len(processResult.Events),
		"published_events":    publishedEvents,
		"provider_executions": processResult.ProviderExecutions,
	}, nil
}

func isRetryable(err error) bool {
	_, ok := err.(*providers.RetryableError)
	return ok
}

func destinationsToStrings(dests []pbplugin.DestinationType) []string {
	strs := make([]string, len(dests))
	for i, d := range dests {
		strs[i] = d.String()
	}
	return strs
}
