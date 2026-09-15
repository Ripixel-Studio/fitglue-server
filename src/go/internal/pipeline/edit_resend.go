package pipeline

import (
	"context"
	"fmt"
	"time"

	shared "github.com/fitglue/server/src/go/pkg"
	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// resendRepostMode is the repost_mode stamped onto a re-sent enriched event. It is not
// "full-pipeline" (which mints a fresh run and re-runs enrichers) and not a per-destination
// targeted mode, so the destination executor treats it as an explicit user re-push: it
// bypasses the redelivery idempotency guard and updates every already-succeeded destination
// in place rather than skipping or duplicating it.
const resendRepostMode = "resend"

// UpdateActivity writes the user-edit overlay of a persisted, layered ActivityRecord.
//
// Scope (editable-activities spec, Phase 1 "edit"): only the editable overlay fields named in
// update_mask (name / description / type / tags) are set; the immutable source layer and the
// append-only enricher layers are never touched, so provenance and history stay honest. The
// overlay always wins on read, so a later enricher run can never silently clobber a user edit.
// The resolved (derived + overlay) activity is returned so the caller can render the edit
// immediately — the same value a subsequent re-send would push.
func (s *Service) UpdateActivity(ctx context.Context, req *pbsvc.UpdateActivityRequest) (*pbactivity.StandardizedActivity, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}
	if len(req.UpdateMask) == 0 {
		return nil, status.Error(codes.InvalidArgument, "update_mask must name at least one editable field")
	}

	run, err := s.store.FindPipelineRunByActivityId(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to find pipeline run by activity", "error", err, "activityId", req.ActivityId)
		return nil, status.Error(codes.Internal, "failed to look up activity")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "no pipeline run found for activity")
	}
	if run.ActivityRecordUri == "" {
		// Predates layered storage — there is no overlay to write.
		return nil, status.Error(codes.FailedPrecondition, "activity has no layered record; not editable")
	}

	rec, err := s.loadActivityRecord(ctx, run.ActivityRecordUri)
	if err != nil {
		s.logger.Error(ctx, "failed to load activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, status.Error(codes.Internal, "failed to load activity record")
	}

	overlay := rec.UserEditOverlay
	if overlay == nil {
		overlay = &pbactivity.ActivityUserEditOverlay{}
	}

	// Apply only the masked fields. A field named in the mask is written (an absent value
	// clears it to empty); a field not named is left as-is on the overlay. This is the only
	// way to express "clear all tags" unambiguously, since tags is a repeated field.
	for _, field := range req.UpdateMask {
		switch field {
		case activitydomain.FieldName:
			overlay.Name = proto.String(req.Name)
		case activitydomain.FieldDescription:
			overlay.Description = proto.String(req.Description)
		case activitydomain.FieldType:
			overlay.Type = req.Type.Enum()
		case activitydomain.FieldTags:
			overlay.Tags = append([]string(nil), req.Tags...)
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unknown update_mask field %q (editable: name, description, type, tags)", field)
		}
	}

	now := time.Now()
	nowPb := timestamppb.New(now)
	overlay.EditedAt = nowPb
	rec.UserEditOverlay = overlay
	rec.UpdatedAt = nowPb
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = activitydomain.ActivityRecordSchemaVersion
	}

	bucket, object, ok := activitydomain.ParseGCSURI(run.ActivityRecordUri)
	if !ok {
		return nil, status.Error(codes.Internal, "invalid activity record uri")
	}
	data, err := protojson.Marshal(rec)
	if err != nil {
		s.logger.Error(ctx, "failed to marshal edited activity record", "error", err)
		return nil, status.Error(codes.Internal, "failed to persist edit")
	}
	if err := s.blobStore.Write(ctx, bucket, object, data); err != nil {
		s.logger.Error(ctx, "failed to write edited activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, status.Error(codes.Internal, "failed to persist edit")
	}

	s.logger.Info(ctx, "Wrote user-edit overlay", "activityId", req.ActivityId, "fields", req.UpdateMask)

	return activitydomain.EffectiveActivity(rec), nil
}

// ResendActivity re-resolves a persisted, layered activity and re-pushes the resolved result
// to the destinations it was originally routed to.
//
// Scope (editable-activities spec, Phase 1 "re-send"): re-send is re-resolve + push, so the
// destinations receive exactly the resolved activity (derived fold + user-edit overlay) the
// read API renders. Enrichers are deliberately NOT re-run here — a re-run is a separate,
// user-triggered Phase 2 action. The stored enriched event is reused as the template (it
// carries the destination list and per-destination config), its content fields are replaced
// with the resolved activity, and it is re-published to topic-enriched-activity. The original
// pipeline run is reused (its execution id is preserved) and the event is marked as a re-send
// so the destination executor updates already-succeeded destinations in place instead of
// skipping them as a Pub/Sub redelivery — no duplicates.
func (s *Service) ResendActivity(ctx context.Context, req *pbsvc.ResendActivityRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}

	run, err := s.store.FindPipelineRunByActivityId(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to find pipeline run by activity", "error", err, "activityId", req.ActivityId)
		return nil, status.Error(codes.Internal, "failed to look up activity")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "no pipeline run found for activity")
	}
	if run.ActivityRecordUri == "" {
		return nil, status.Error(codes.FailedPrecondition, "activity has no layered record; not re-sendable")
	}
	if run.EnrichedEventUri == "" {
		return nil, status.Error(codes.FailedPrecondition, "activity has no enriched event to re-send")
	}

	rec, err := s.loadActivityRecord(ctx, run.ActivityRecordUri)
	if err != nil {
		s.logger.Error(ctx, "failed to load activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, status.Error(codes.Internal, "failed to load activity record")
	}
	resolved := activitydomain.EffectiveActivity(rec)
	if resolved == nil {
		return nil, status.Error(codes.FailedPrecondition, "activity has no resolved data to re-send")
	}

	// Load the stored enriched event as the destination template: it carries the destination
	// list, the per-destination config in EnrichmentMetadata, the FIT file URI and the original
	// pipeline execution id.
	eventBytes, err := s.blobStore.Get(ctx, run.EnrichedEventUri)
	if err != nil {
		s.logger.Error(ctx, "failed to load enriched event for re-send", "error", err, "uri", run.EnrichedEventUri)
		return nil, status.Error(codes.Internal, "failed to load enriched event")
	}
	var event pbevents.EnrichedActivityEvent
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(eventBytes, &event); err != nil {
		s.logger.Error(ctx, "failed to unmarshal enriched event for re-send", "error", err)
		return nil, status.Error(codes.Internal, "failed to parse enriched event")
	}
	if len(event.Destinations) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "activity has no destinations to re-send to")
	}

	// Replace the content fields with the resolved activity. Enrichers are not re-run, so the
	// current resolved state is exactly what is sent. Inline the data and clear any prior
	// activity_data_uri so we re-offload the resolved payload below rather than pointing at a
	// stale blob.
	event.ActivityData = resolved
	event.ActivityDataUri = ""
	event.Name = resolved.Name
	event.Description = resolved.Description
	event.ActivityType = resolved.Type
	event.Tags = resolved.Tags

	// Mark as an explicit user re-send so the destination executor bypasses the redelivery
	// idempotency guard and updates already-succeeded destinations in place (no duplicates).
	if event.EnrichmentMetadata == nil {
		event.EnrichmentMetadata = map[string]string{}
	}
	event.EnrichmentMetadata["is_repost"] = "true"
	event.EnrichmentMetadata["repost_mode"] = resendRepostMode

	pipelineExecID := event.GetPipelineExecutionId()
	if pipelineExecID == "" {
		// Fall back to the run id so the destination executor can still find prior outcomes.
		pipelineExecID = run.Id
		event.PipelineExecutionId = &pipelineExecID
	}

	// The enriched event blob and the record share the artifacts bucket. Offload the resolved
	// event under a fresh object (nanosecond-suffixed) so we neither clobber the original
	// enriched event nor exceed the Pub/Sub message-size limit; publish a slim event that
	// points at it. Mirrors the enricher's offload-then-publish path.
	bucket, _, ok := activitydomain.ParseGCSURI(run.EnrichedEventUri)
	if !ok {
		return nil, status.Error(codes.Internal, "invalid enriched event uri")
	}
	now := time.Now()
	object := fmt.Sprintf("enriched_events/%s/%s-resend-%d.json", req.UserId, pipelineExecID, now.UnixNano())
	fullBytes, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(&event)
	if err != nil {
		s.logger.Error(ctx, "failed to marshal re-send event", "error", err)
		return nil, status.Error(codes.Internal, "failed to build re-send event")
	}
	if err := s.blobStore.Write(ctx, bucket, object, fullBytes); err != nil {
		s.logger.Error(ctx, "failed to store re-send event", "error", err)
		return nil, status.Error(codes.Internal, "failed to store re-send event")
	}
	uri := "gs://" + bucket + "/" + object

	slim := proto.Clone(&event).(*pbevents.EnrichedActivityEvent)
	slim.ActivityData = nil
	slim.ActivityDataUri = uri

	// Publish to topic-enriched-activity as an enriched event; the router fans it out to each
	// destination (topic-destination-upload) exactly as it does for a freshly enriched activity.
	ce, err := infrapubsub.NewCloudEvent("/resend", "com.fitglue.activity.enriched", slim)
	if err != nil {
		s.logger.Error(ctx, "failed to create re-send cloud event", "error", err)
		return nil, status.Error(codes.Internal, "failed to build re-send event")
	}
	ce.SetExtension("pipeline_execution_id", pipelineExecID)

	if _, err := s.publisher.PublishCloudEvent(ctx, shared.TopicEnrichedActivity, ce); err != nil {
		s.logger.Error(ctx, "failed to publish re-send event", "error", err)
		return nil, status.Error(codes.Internal, "failed to publish re-send event")
	}

	s.logger.Info(ctx, "Re-sent resolved activity", "activityId", req.ActivityId, "destinations", event.Destinations, "topic", shared.TopicEnrichedActivity)
	return &emptypb.Empty{}, nil
}
