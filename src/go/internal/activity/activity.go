// nolint:proto-json
package activity

import (
	"context"
	"encoding/json"

	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Service) GetActivity(ctx context.Context, req *pbsvc.GetActivityRequest) (*pbactivity.StandardizedActivity, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}

	// Because of Activity Storage Consolidation (Rule E37), we look up the PipelineRun record.
	// ActivityId in the request maps to the PipelineRun ID.
	run, err := s.store.GetPipelineRun(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to get pipeline run for activity", "error", err)
		return nil, status.Error(codes.Internal, "failed to read activity metadata")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "activity not found")
	}

	// Prefer the durable, layered ActivityRecord when present (replaces the pass-through
	// model). The effective activity is the derived (source + enrichers) activity with the
	// user-edit overlay applied. Fall back to the legacy enriched-event blob below for runs
	// created before layered storage, or if the record can't be read.
	if run.ActivityRecordUri != "" {
		rec, err := activitydomain.LoadActivityRecord(ctx, run.ActivityRecordUri, s.blobStore)
		if err != nil {
			s.logger.Warn(ctx, "failed to read layered activity record, falling back to enriched-event blob", "error", err, "uri", run.ActivityRecordUri)
		} else if eff := activitydomain.EffectiveActivity(rec); eff != nil {
			return eff, nil
		}
	}

	return s.legacyActivityFromRun(ctx, req.UserId, run)
}

// legacyActivityFromRun reconstructs the effective activity for a run that has no usable
// layered ActivityRecord: from the pre-layered enriched-event / original-payload blob, or
// from the run metadata when no blob exists at all. It is the fallback shared by
// GetActivity and the resolved-read path (which attaches empty provenance to the result,
// since a legacy run carries no per-field attribution).
func (s *Service) legacyActivityFromRun(ctx context.Context, userID string, run *pbpipeline.PipelineRun) (*pbactivity.StandardizedActivity, error) {
	// Resolve the GCS URI where the actual giant payload is stored
	uri := run.EnrichedEventUri
	if uri == "" && run.OriginalPayloadUri != "" {
		uri = run.OriginalPayloadUri
	}

	if uri == "" {
		// Try to build a lightweight StandardizedActivity from the run metadata if no blob exists
		sourceEnum := pbactivity.ActivitySource_SOURCE_UNSPECIFIED
		if val, ok := pbactivity.ActivitySource_value[run.Source]; ok {
			sourceEnum = pbactivity.ActivitySource(val)
		}

		return &pbactivity.StandardizedActivity{
			Source:     sourceEnum,
			ExternalId: run.ActivityId,
			UserId:     userID,
			StartTime:  run.StartTime,
			Name:       run.Title,
			Type:       run.Type,
		}, nil
	}

	// Fetch from BlobStore
	data, err := s.blobStore.Get(ctx, "", uri) // using full URI inside Get() convention
	if err != nil {
		s.logger.Error(ctx, "failed to read activity payload from GCS", "error", err, "uri", uri)
		return nil, status.Error(codes.Internal, "failed to read activity data")
	}

	// Payload could be ActivityPayload or EnrichedActivityEvent.
	// Check if it's an Enriched event first.
	var enriched pbevents.EnrichedActivityEvent
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(data, &enriched); err == nil && enriched.ActivityData != nil {
		return enriched.ActivityData, nil
	}

	// Otherwise, it might just be the raw ActivityPayload
	var raw pbevents.ActivityPayload
	if err := unmarshalOpts.Unmarshal(data, &raw); err == nil && raw.StandardizedActivity != nil {
		return raw.StandardizedActivity, nil
	}

	// Or it might be serialized as raw JSON if the above proto unmarshaling failed
	// (some legacy data doesn't use protojson formatting correctly).
	var legacyData struct {
		StandardizedActivity json.RawMessage `json:"standardizedActivity"`
	}
	if err := json.Unmarshal(data, &legacyData); err == nil && len(legacyData.StandardizedActivity) > 0 {
		var stdAct pbactivity.StandardizedActivity
		if err := unmarshalOpts.Unmarshal(legacyData.StandardizedActivity, &stdAct); err == nil {
			return &stdAct, nil
		}
	}

	return nil, status.Error(codes.Internal, "failed to parse activity data from blob")
}

// GetResolvedActivity returns the resolved activity (the derived source+enricher fold with
// the user-edit overlay applied) together with per-field provenance attributing each
// resolved field to the layer/run that set it. When a run predates layered storage (or its
// record can't be read), it resolves the effective activity from the legacy blob and
// returns empty provenance rather than failing — the field attribution simply isn't known.
func (s *Service) GetResolvedActivity(ctx context.Context, req *pbsvc.GetResolvedActivityRequest) (*pbactivity.ResolvedActivity, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}

	// ActivityId maps to the PipelineRun ID (Activity Storage Consolidation, Rule E37).
	run, err := s.store.GetPipelineRun(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to get pipeline run for resolved activity", "error", err)
		return nil, status.Error(codes.Internal, "failed to read activity metadata")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "activity not found")
	}

	return s.resolveFromRun(ctx, req.UserId, run)
}

// ListResolvedActivities is the paged list counterpart of GetResolvedActivity: it resolves
// every run in the page to a ResolvedActivity (activity + provenance). A run that fails to
// resolve is skipped rather than failing the whole page.
func (s *Service) ListResolvedActivities(ctx context.Context, req *pbsvc.ListResolvedActivitiesRequest) (*pbsvc.ListResolvedActivitiesResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	runs, nextToken, err := s.store.ListPipelineRuns(ctx, req.UserId, req.Limit, req.PageToken)
	if err != nil {
		s.logger.Error(ctx, "failed to list resolved activities", "error", err)
		return nil, status.Error(codes.Internal, "failed to list activities")
	}

	activities := make([]*pbactivity.ResolvedActivity, 0, len(runs))
	for _, run := range runs {
		resolved, err := s.resolveFromRun(ctx, req.UserId, run)
		if err != nil {
			s.logger.Warn(ctx, "skipping activity that failed to resolve", "error", err, "run_id", run.GetId())
			continue
		}
		activities = append(activities, resolved)
	}

	return &pbsvc.ListResolvedActivitiesResponse{
		Activities:    activities,
		NextPageToken: nextToken,
	}, nil
}

// resolveFromRun resolves a single run to a ResolvedActivity. It prefers the layered
// ActivityRecord (full per-field provenance); for runs written before layered storage —
// or when the record can't be read — it falls back to the legacy effective activity with
// empty provenance.
func (s *Service) resolveFromRun(ctx context.Context, userID string, run *pbpipeline.PipelineRun) (*pbactivity.ResolvedActivity, error) {
	if run.GetActivityRecordUri() != "" {
		rec, err := activitydomain.LoadActivityRecord(ctx, run.GetActivityRecordUri(), s.blobStore)
		if err != nil {
			s.logger.Warn(ctx, "failed to read layered activity record, resolving from legacy blob without provenance", "error", err, "uri", run.GetActivityRecordUri())
		} else if resolved := activitydomain.Resolve(rec); resolved.GetActivity() != nil {
			return resolved, nil
		}
	}

	act, err := s.legacyActivityFromRun(ctx, userID, run)
	if err != nil {
		return nil, err
	}
	return &pbactivity.ResolvedActivity{Activity: act}, nil
}

func (s *Service) DeleteActivity(ctx context.Context, req *pbsvc.DeleteActivityRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}

	run, err := s.store.GetPipelineRun(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to check pipeline run", "error", err)
		return nil, status.Error(codes.Internal, "failed to check activity")
	}

	if run != nil {
		// Clean up GCS blobs if they exist
		if run.EnrichedEventUri != "" {
			_ = s.blobStore.Delete(ctx, "", run.EnrichedEventUri)
		}
		if run.OriginalPayloadUri != "" && run.OriginalPayloadUri != run.EnrichedEventUri {
			_ = s.blobStore.Delete(ctx, "", run.OriginalPayloadUri)
		}
		if run.ActivityRecordUri != "" {
			_ = s.blobStore.Delete(ctx, "", run.ActivityRecordUri)
		}

		// Delete the Firestore record
		if err := s.store.DeletePipelineRun(ctx, req.UserId, req.ActivityId); err != nil {
			s.logger.Error(ctx, "failed to delete activity record", "error", err)
			return nil, status.Error(codes.Internal, "failed to delete activity")
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) ListActivities(ctx context.Context, req *pbsvc.ListActivitiesRequest) (*pbsvc.ListActivitiesResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	runs, nextToken, err := s.store.ListPipelineRuns(ctx, req.UserId, req.Limit, req.PageToken)
	if err != nil {
		s.logger.Error(ctx, "failed to list activities", "error", err)
		return nil, status.Error(codes.Internal, "failed to list activities")
	}

	var activities []*pbactivity.StandardizedActivity
	for _, run := range runs {
		sourceEnum := pbactivity.ActivitySource_SOURCE_UNSPECIFIED
		if val, ok := pbactivity.ActivitySource_value[run.Source]; ok {
			sourceEnum = pbactivity.ActivitySource(val)
		}

		activities = append(activities, &pbactivity.StandardizedActivity{
			Id:                run.Id,
			PipelineRunStatus: run.Status.String(),
			Source:            sourceEnum,
			ExternalId:        run.ActivityId,
			UserId:            req.UserId,
			StartTime:         run.StartTime,
			Name:              run.Title,
			Type:              run.Type,
		})
	}

	return &pbsvc.ListActivitiesResponse{
		Activities:    activities,
		NextPageToken: nextToken,
	}, nil
}
