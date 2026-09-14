package pipeline

import (
	"context"
	"time"

	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/sourceplugins"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RefreshActivitySource re-pulls an activity from its origin provider and refreshes the
// immutable source layer of the persisted, layered ActivityRecord.
//
// Scope (editable-activities spec, Phase 2 "re-pull from source"): this replaces the parsed
// source snapshot, stores a fresh raw payload, and re-stamps the ingest / prune timestamps.
// It deliberately does NOT re-run enrichers — re-runs are a separate, user-triggered action
// and are never automatic. Because the enrichers are not re-applied, the derived activity is
// rebased onto the freshly-pulled source (the honest fold when zero enricher runs apply to
// the new source); the append-only enricher_layers are kept as history/provenance, and the
// mutable user-edit overlay is preserved and still wins on read. The resolved activity
// (fresh source + user overlay) is returned so the caller can render it immediately.
func (s *Service) RefreshActivitySource(ctx context.Context, req *pbsvc.RefreshActivitySourceRequest) (*pbactivity.StandardizedActivity, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}
	if s.userSvc == nil {
		return nil, status.Error(codes.Unimplemented, "user service not available")
	}

	// Locate the activity's most recent pipeline run and the layered record it points at.
	run, err := s.store.FindPipelineRunByActivityId(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to find pipeline run by activity", "error", err, "activityId", req.ActivityId)
		return nil, status.Error(codes.Internal, "failed to look up activity")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "no pipeline run found for activity")
	}
	if run.ActivityRecordUri == "" {
		// Predates layered storage — there is no source layer to refresh.
		return nil, status.Error(codes.FailedPrecondition, "activity has no layered record; not re-pullable")
	}

	rec, err := s.loadActivityRecord(ctx, run.ActivityRecordUri)
	if err != nil {
		s.logger.Error(ctx, "failed to load activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, status.Error(codes.Internal, "failed to load activity record")
	}

	source := rec.GetSource().String()
	provider, ok := sourceplugins.ForSource(source)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "source %q does not support re-pull", source)
	}

	externalID := rec.GetExternalId()
	if externalID == "" {
		return nil, status.Error(codes.FailedPrecondition, "activity has no source id to re-pull")
	}

	integResp, err := s.userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   req.UserId,
		Provider: source,
	})
	if err != nil {
		s.logger.Error(ctx, "failed to get integration for re-pull", "error", err, "source", source)
		return nil, status.Errorf(codes.FailedPrecondition, "%s is not connected; reconnect to re-pull", source)
	}

	payload, err := provider.FetchActivity(ctx, integResp.Integrations, req.UserId, externalID)
	if err != nil {
		s.logger.Error(ctx, "failed to re-pull activity from source", "error", err, "source", source, "externalId", externalID)
		return nil, status.Error(codes.Internal, "failed to re-pull activity from source")
	}
	if payload.GetStandardizedActivity() == nil {
		s.logger.Error(ctx, "re-pull returned no parsed activity", "source", source, "externalId", externalID)
		return nil, status.Error(codes.Internal, "source returned no parsed activity")
	}

	// The record and its raw payloads share the artifacts bucket; reuse it so we need no
	// extra config wiring and the payloads/ lifecycle rule applies to the re-pulled payload.
	bucket, recordObject, ok := activitydomain.ParseGCSURI(run.ActivityRecordUri)
	if !ok {
		return nil, status.Error(codes.Internal, "invalid activity record uri")
	}

	now := time.Now()

	// Store the fresh raw payload under payloads/ (30-day lifecycle, re-pullable thereafter).
	var rawPayloadURI string
	if raw := payload.GetOriginalPayloadJson(); raw != "" {
		payloadObject := activitydomain.RepulledSourcePayloadObjectPath(req.UserId, req.ActivityId, now)
		if err := s.blobStore.Write(ctx, bucket, payloadObject, []byte(raw)); err != nil {
			s.logger.Error(ctx, "failed to store re-pulled raw payload", "error", err)
			return nil, status.Error(codes.Internal, "failed to store re-pulled payload")
		}
		rawPayloadURI = "gs://" + bucket + "/" + payloadObject
	}

	// Refresh the (sanctioned mutation of the) immutable source layer.
	parsed := proto.Clone(payload.GetStandardizedActivity()).(*pbactivity.StandardizedActivity)
	nowPb := timestamppb.New(now)
	rec.SourceLayer = &pbactivity.ActivitySourceLayer{
		Parsed:              parsed,
		RawPayloadUri:       rawPayloadURI,
		IngestedAt:          nowPb,
		RawPayloadExpiresAt: timestamppb.New(now.Add(activitydomain.RawPayloadRetention)),
	}

	// Rebase the derived activity onto the fresh source. Enrichers are not re-applied here
	// (no auto re-runs), so the merged typed enrichments are cleared to keep the resolved
	// view internally consistent; enricher_layers remain as append-only history and
	// re-appear when the user re-runs enrichers. The user-edit overlay is left untouched.
	rec.DerivedActivity = proto.Clone(parsed).(*pbactivity.StandardizedActivity)
	rec.Enrichments = nil
	rec.UpdatedAt = nowPb
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = activitydomain.ActivityRecordSchemaVersion
	}

	data, err := protojson.Marshal(rec)
	if err != nil {
		s.logger.Error(ctx, "failed to marshal refreshed activity record", "error", err)
		return nil, status.Error(codes.Internal, "failed to persist refreshed record")
	}
	if err := s.blobStore.Write(ctx, bucket, recordObject, data); err != nil {
		s.logger.Error(ctx, "failed to write refreshed activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, status.Error(codes.Internal, "failed to persist refreshed record")
	}

	s.logger.Info(ctx, "Re-pulled activity source", "activityId", req.ActivityId, "source", source, "externalId", externalID)

	// Resolved view: fresh source with the user-edit overlay applied on top.
	resolved := activitydomain.EffectiveActivity(rec)
	return resolved, nil
}

// loadActivityRecord fetches and unmarshals the layered ActivityRecord at uri using the
// pipeline service's URI-addressed BlobStore (activitydomain.LoadActivityRecord expects a
// bucket/object store, which this service does not use).
func (s *Service) loadActivityRecord(ctx context.Context, uri string) (*pbactivity.ActivityRecord, error) {
	raw, err := s.blobStore.Get(ctx, uri)
	if err != nil {
		return nil, err
	}
	var rec pbactivity.ActivityRecord
	if err := protojson.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}
