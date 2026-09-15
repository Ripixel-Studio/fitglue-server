package activity

import (
	"context"
	"fmt"
	"strings"
	"time"

	shared "github.com/fitglue/server/src/go/pkg"
	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UpdateActivity writes the user-edit overlay for the named fields of a persisted, layered
// activity and returns the freshly resolved activity + per-field provenance — the exact
// value ReSendActivity would push. It is the edit half of the correction loop (spec Phase 1):
// the overlay is the highest-precedence layer of the resolution stack, so an edit here always
// wins over the source and every enricher run, and is never silently clobbered by a later run.
//
// The record is loaded from, mutated, and written back to the same stable GCS object the
// pipeline wrote on completion, so the immutable source layer and append-only enricher layers
// are untouched — only the overlay changes.
func (s *Service) UpdateActivity(ctx context.Context, req *pbsvc.UpdateActivityRequest) (*pbactivity.ResolvedActivity, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}
	if len(req.UpdateFields) == 0 {
		return nil, status.Error(codes.InvalidArgument, "update_fields must name at least one field")
	}

	rec, run, err := s.loadEditableRecord(ctx, req.UserId, req.ActivityId)
	if err != nil {
		return nil, err
	}

	if err := activitydomain.ApplyOverlayEdit(rec, req.Overlay, req.UpdateFields, time.Now()); err != nil {
		// ApplyOverlayEdit only fails on a bad request (empty mask / unknown field), and
		// leaves the record untouched in that case.
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := activitydomain.WriteActivityRecord(ctx, run.ActivityRecordUri, s.blobStore, rec); err != nil {
		s.logger.Error(ctx, "failed to write updated activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, status.Error(codes.Internal, "failed to persist activity edit")
	}

	s.logger.Info(ctx, "Applied user-edit overlay", "activity_id", req.ActivityId, "fields", req.UpdateFields)
	return activitydomain.Resolve(rec), nil
}

// ReSendActivity re-resolves a persisted activity and pushes it to its destinations,
// reflecting any user edits without re-running the enricher pipeline. It is the push half of
// the correction loop (spec Phase 1): re-send = re-resolve + push. Because destinations always
// receive the *resolved* activity, this pushes exactly what UpdateActivity's response showed.
//
// It reuses the live enrichment→destination path: it builds an EnrichedActivityEvent carrying
// the resolved activity and publishes it to topic-enriched-activity, where the router fans it
// out per destination and the destination service uploads. The event reuses the original
// pipeline run id and is flagged so the destination executor updates the already-synced
// destinations in place (rather than skipping them as a redelivery or creating duplicates),
// and creates any destination that never succeeded.
func (s *Service) ReSendActivity(ctx context.Context, req *pbsvc.ReSendActivityRequest) (*pbsvc.ReSendActivityResponse, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}
	if s.publisher == nil {
		return nil, status.Error(codes.Internal, "publisher not configured")
	}

	rec, run, err := s.loadEditableRecord(ctx, req.UserId, req.ActivityId)
	if err != nil {
		return nil, err
	}

	resolved := activitydomain.EffectiveActivity(rec)
	if resolved == nil {
		return nil, status.Error(codes.FailedPrecondition, "activity has no resolvable data to re-send")
	}

	dests := selectResendDestinations(run, req.Destinations)
	if len(dests) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "activity has no destinations to re-send to")
	}

	event := s.buildResendEvent(rec, run, resolved, dests)

	// Offload the (potentially large) activity data to GCS just like the enricher does, so the
	// Pub/Sub message stays small and the router/executor resolve the full data back from GCS.
	toPublish := event
	if s.bucketName != "" {
		prepared, _, prepErr := activitydomain.PrepareForPublish(ctx, event, s.blobStore, s.bucketName)
		if prepErr != nil {
			s.logger.Warn(ctx, "failed to offload re-send activity data to GCS, publishing inline", "error", prepErr)
		} else {
			toPublish = prepared
		}
	}

	ce, err := infrapubsub.NewCloudEvent(
		infrapubsub.GetCloudEventSource(pbevents.CloudEventSource_CLOUD_EVENT_SOURCE_ENRICHER),
		"com.fitglue.activity.enriched",
		toPublish,
	)
	if err != nil {
		s.logger.Error(ctx, "failed to build re-send cloud event", "error", err)
		return nil, status.Error(codes.Internal, "failed to build re-send event")
	}
	ce.SetExtension("pipeline_execution_id", run.Id)

	if _, err := s.publisher.PublishCloudEvent(ctx, shared.TopicEnrichedActivity, ce); err != nil {
		s.logger.Error(ctx, "failed to publish re-send event", "error", err)
		return nil, status.Error(codes.Internal, "failed to publish re-send event")
	}

	s.logger.Info(ctx, "Re-sent activity", "activity_id", req.ActivityId, "pipeline_run_id", run.Id, "destinations", destinationNames(dests))
	return &pbsvc.ReSendActivityResponse{Destinations: dests}, nil
}

// loadEditableRecord resolves the pipeline run for an activity and loads its layered
// ActivityRecord, returning gRPC status errors the RPC handlers can surface directly. An
// activity with no layered record (legacy, pre-layered pass-through run) is not editable /
// re-sendable and yields FailedPrecondition.
func (s *Service) loadEditableRecord(ctx context.Context, userID, activityID string) (*pbactivity.ActivityRecord, *pbpipeline.PipelineRun, error) {
	run, err := s.store.GetPipelineRun(ctx, userID, activityID)
	if err != nil {
		s.logger.Error(ctx, "failed to get pipeline run for activity", "error", err)
		return nil, nil, status.Error(codes.Internal, "failed to read activity metadata")
	}
	if run == nil {
		return nil, nil, status.Error(codes.NotFound, "activity not found")
	}
	if run.ActivityRecordUri == "" {
		return nil, nil, status.Error(codes.FailedPrecondition, "activity predates layered storage and cannot be edited or re-sent")
	}
	rec, err := activitydomain.LoadActivityRecord(ctx, run.ActivityRecordUri, s.blobStore)
	if err != nil {
		s.logger.Error(ctx, "failed to load activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, nil, status.Error(codes.Internal, "failed to read activity record")
	}
	if rec == nil {
		return nil, nil, status.Error(codes.NotFound, "activity record not found")
	}
	return rec, run, nil
}

// buildResendEvent assembles the EnrichedActivityEvent for a re-send from the layered record,
// the pipeline run and the resolved activity. It mirrors the shape the enricher publishes so
// the router and destination service treat it identically, with two deliberate additions:
//   - use_update_method=true, so already-synced destinations are updated (not skipped as a
//     redelivery), reflecting the user's edits in place;
//   - pipeline_resumed=true, so a destination that already succeeded is refreshed with the
//     resolved data while a destination that never succeeded still falls through to Create.
//
// It reuses the original pipeline run id (PipelineExecutionId) so the executor can find the
// existing destination-side activity ids for its Update calls.
func (s *Service) buildResendEvent(rec *pbactivity.ActivityRecord, run *pbpipeline.PipelineRun, resolved *pbactivity.StandardizedActivity, dests []pbplugin.DestinationType) *pbevents.EnrichedActivityEvent {
	runID := run.Id
	metadata := map[string]string{
		"use_update_method": "true",
		"pipeline_resumed":  "true",
		"activity_resend":   "true",
	}
	// Same-source-destination signalling (mirrors the orchestrator): when the activity's
	// source platform is also a destination (e.g. Strava→Strava), tell the uploader to
	// overwrite title/description rather than section-merge, so the resolved value is sent
	// verbatim.
	sourceName := strings.ToLower(strings.TrimPrefix(rec.Source.String(), "SOURCE_"))
	for _, d := range dests {
		destName := strings.ToLower(strings.TrimPrefix(d.String(), "DESTINATION_"))
		if sourceName == destName {
			metadata["same_source_destination_"+destName] = "true"
		}
	}

	var startTime = resolved.GetStartTime()
	if startTime == nil && len(resolved.GetSessions()) > 0 {
		startTime = resolved.GetSessions()[0].GetStartTime()
	}

	event := &pbevents.EnrichedActivityEvent{
		UserId:              rec.UserId,
		Source:              rec.Source,
		ActivityId:          rec.ActivityId,
		ActivityData:        resolved,
		ActivityType:        resolved.GetType(),
		Name:                resolved.GetName(),
		Description:         resolved.GetDescription(),
		Tags:                resolved.GetTags(),
		Enrichments:         rec.Enrichments,
		Destinations:        dests,
		PipelineId:          run.PipelineId,
		PipelineExecutionId: &runID,
		StartTime:           startTime,
		EnrichmentMetadata:  metadata,
	}

	// The FIT file is written at a stable per-activity path and is not subject to the
	// raw-payload prune, so reconstruct the same URI the enricher set (uploaders that need
	// the FIT on Update read it from here).
	if s.bucketName != "" {
		event.FitFileUri = fmt.Sprintf("gs://%s/activities/%s/%s.fit", s.bucketName, rec.UserId, rec.ActivityId)
	}
	return event
}

// selectResendDestinations returns the destinations to re-send to. With no override it is
// every destination the activity was sent to (i.e. that has a recorded outcome on the run),
// de-duplicated and in outcome order. An override narrows this to the intersection with those
// destinations — a destination the activity was never sent to is ignored (re-send only
// re-pushes existing sends; adding a brand-new destination is a repost/missed-destination
// concern, out of scope here).
func selectResendDestinations(run *pbpipeline.PipelineRun, override []pbplugin.DestinationType) []pbplugin.DestinationType {
	sent := make(map[pbplugin.DestinationType]bool)
	var order []pbplugin.DestinationType
	for _, o := range run.GetDestinations() {
		d := o.GetDestination()
		if d == pbplugin.DestinationType_DESTINATION_UNSPECIFIED || sent[d] {
			continue
		}
		sent[d] = true
		order = append(order, d)
	}

	if len(override) == 0 {
		return order
	}

	var out []pbplugin.DestinationType
	seen := make(map[pbplugin.DestinationType]bool)
	for _, d := range override {
		if sent[d] && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func destinationNames(dests []pbplugin.DestinationType) []string {
	names := make([]string, 0, len(dests))
	for _, d := range dests {
		names = append(names, d.String())
	}
	return names
}
