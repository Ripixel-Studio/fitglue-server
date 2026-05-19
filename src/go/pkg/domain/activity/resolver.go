package activity

import (
	"context"
	"fmt"
	"regexp"

	"google.golang.org/protobuf/encoding/protojson"

	shared "github.com/fitglue/server/src/go/pkg"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
)

// GCS URI pattern: gs://bucket/path
var gcsURIPattern = regexp.MustCompile(`^gs://([^/]+)/(.+)$`)

// ParseGCSURI extracts bucket and object path from a GCS URI.
// Returns bucket, object, and bool indicating if the URI was valid.
func ParseGCSURI(uri string) (bucket, object string, ok bool) {
	matches := gcsURIPattern.FindStringSubmatch(uri)
	if len(matches) != 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}

// ResolveActivityData fetches activity_data from GCS if activity_data_uri is set,
// otherwise returns the inline activity_data. This enables passing large activity
// data through GCS rather than Pub/Sub messages.
//
// The GCS blob contains the full EnrichedActivityEvent, so we unmarshal it and
// return just the activity_data field.
//
// Returns the StandardizedActivity (either inline or fetched from GCS).
// If URI is set but fetch fails, returns an error.
func ResolveActivityData(ctx context.Context, event *pbevents.EnrichedActivityEvent, store shared.BlobStore) (*pbactivity.StandardizedActivity, error) {
	// If no URI, return inline data (may be nil)
	if event.ActivityDataUri == "" {
		return event.ActivityData, nil
	}

	// Parse GCS URI
	bucket, object, ok := ParseGCSURI(event.ActivityDataUri)
	if !ok {
		return nil, fmt.Errorf("invalid activity_data_uri: %s", event.ActivityDataUri)
	}

	// Fetch from GCS
	data, err := store.Get(ctx, bucket, object)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch enriched event from GCS: %w", err)
	}

	// The GCS blob contains the full EnrichedActivityEvent
	var fullEvent pbevents.EnrichedActivityEvent
	if err := protojson.Unmarshal(data, &fullEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal enriched event: %w", err)
	}

	return fullEvent.ActivityData, nil
}

// ResolveEnrichedEvent populates the ActivityData field by fetching from GCS if needed.
// This modifies the event in place for convenience.
func ResolveEnrichedEvent(ctx context.Context, event *pbevents.EnrichedActivityEvent, store shared.BlobStore) error {
	if event.ActivityDataUri == "" || event.ActivityData != nil {
		// No URI or already has inline data
		return nil
	}

	activityData, err := ResolveActivityData(ctx, event, store)
	if err != nil {
		return err
	}

	event.ActivityData = activityData
	return nil
}

// FetchFullEnrichedEvent fetches the complete EnrichedActivityEvent from GCS.
// This is useful when you need ALL fields from the original event, not just activity_data.
// For example, the repost-handler uses this to get the full event for replay.
func FetchFullEnrichedEvent(ctx context.Context, gcsUri string, store shared.BlobStore) (*pbevents.EnrichedActivityEvent, error) {
	bucket, object, ok := ParseGCSURI(gcsUri)
	if !ok {
		return nil, fmt.Errorf("invalid GCS URI: %s", gcsUri)
	}

	data, err := store.Get(ctx, bucket, object)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch enriched event from GCS: %w", err)
	}

	var event pbevents.EnrichedActivityEvent
	if err := protojson.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal enriched event: %w", err)
	}

	return &event, nil
}

// ActivityDataThreshold is the size threshold (in bytes) above which activity data
// should be offloaded to GCS instead of being included inline in Pub/Sub messages.
// Pub/Sub limit is 10MB, but we use 5MB as a safety margin.
const ActivityDataThreshold = 5 * 1024 * 1024 // 5MB

// ShouldOffloadActivityData returns true if the activity data should be stored in GCS
// rather than included inline in the Pub/Sub message.
func ShouldOffloadActivityData(activityData *pbactivity.StandardizedActivity) bool {
	if activityData == nil {
		return false
	}

	// Quick estimate: marshal to JSON and check size
	jsonBytes, err := protojson.Marshal(activityData)
	if err != nil {
		// If we can't marshal, better to offload to be safe
		return true
	}

	return len(jsonBytes) > ActivityDataThreshold
}

// PrepareForPublish uploads the FULL EnrichedActivityEvent to GCS and returns a copy
// with activity_data_uri set and activity_data cleared. The original event is not modified.
//
// The GCS blob contains the FULL event (including activity_data), so it can be used
// for both Pub/Sub efficiency (consumers fetch full event) and repost functionality
// (enriched_event_uri points to the same blob).
//
// We always offload to GCS to ensure consistent behavior across all activity types
// and destinations. The minor storage/latency cost is worth the simplified logic.
//
// Returns the (possibly modified) event and the size uploaded (0 if not uploaded).
func PrepareForPublish(ctx context.Context, event *pbevents.EnrichedActivityEvent, store shared.BlobStore, bucketName string) (*pbevents.EnrichedActivityEvent, int, error) {
	if event.ActivityData == nil {
		// No data to offload
		return event, 0, nil
	}

	// Generate GCS path
	pipelineExecID := ""
	if event.PipelineExecutionId != nil {
		pipelineExecID = *event.PipelineExecutionId
	}
	if pipelineExecID == "" {
		pipelineExecID = event.ActivityId // Fallback
	}

	gcsPath := fmt.Sprintf("enriched_events/%s/%s.json", event.UserId, pipelineExecID)

	// Marshal the FULL event (including activity_data) for storage
	jsonBytes, err := protojson.Marshal(event)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal enriched event: %w", err)
	}

	// Upload to GCS
	if err := store.Write(ctx, bucketName, gcsPath, jsonBytes); err != nil {
		return nil, 0, fmt.Errorf("failed to upload enriched event to GCS: %w", err)
	}

	gcsUri := fmt.Sprintf("gs://%s/%s", bucketName, gcsPath)

	// Create a slim copy with URI set and data cleared for Pub/Sub
	result := &pbevents.EnrichedActivityEvent{
		ActivityId:          event.ActivityId,
		UserId:              event.UserId,
		PipelineId:          event.PipelineId,
		FitFileUri:          event.FitFileUri,
		Name:                event.Name,
		Description:         event.Description,
		ActivityType:        event.ActivityType,
		StartTime:           event.StartTime,
		Source:              event.Source,
		ActivityData:        nil, // Cleared - full event in GCS
		AppliedEnrichments:  event.AppliedEnrichments,
		EnrichmentMetadata:  event.EnrichmentMetadata,
		Enrichments:         event.Enrichments,
		Destinations:        event.Destinations,
		Tags:                event.Tags,
		PipelineExecutionId: event.PipelineExecutionId,
		ActivityDataUri:     gcsUri, // Points to full event in GCS
	}

	return result, len(jsonBytes), nil
}
