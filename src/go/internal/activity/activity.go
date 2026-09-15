// nolint:proto-json
package activity

import (
	"context"
	"encoding/json"

	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
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
			UserId:     req.UserId,
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

// GetResolvedActivity returns the resolved activity together with per-field provenance:
// the deterministic fold of the layered ActivityRecord (source → enricher runs → user
// overlay) plus, for each layerable field, which layer/run set the winning value. This is
// the read the web editing surface renders. Runs written before layered storage have no
// record and thus no provenance — the resolved activity is still returned (reconstructed
// via the same fallback GetActivity uses), with an empty provenance slice, so the read
// contract stays uniform.
func (s *Service) GetResolvedActivity(ctx context.Context, req *pbsvc.GetResolvedActivityRequest) (*pbactivity.ResolvedActivity, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}

	// ActivityId maps to the PipelineRun ID (Rule E37, Activity Storage Consolidation).
	run, err := s.store.GetPipelineRun(ctx, req.UserId, req.ActivityId)
	if err != nil {
		s.logger.Error(ctx, "failed to get pipeline run for resolved activity", "error", err)
		return nil, status.Error(codes.Internal, "failed to read activity metadata")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "activity not found")
	}

	// Prefer the durable, layered ActivityRecord so we can emit first-class provenance.
	if run.ActivityRecordUri != "" {
		rec, err := activitydomain.LoadActivityRecord(ctx, run.ActivityRecordUri, s.blobStore)
		if err != nil {
			s.logger.Warn(ctx, "failed to read layered activity record, resolving without provenance", "error", err, "uri", run.ActivityRecordUri)
		} else if resolved := activitydomain.Resolve(rec); resolved != nil && resolved.Activity != nil {
			return resolved, nil
		}
	}

	// Legacy / no-record fallback: reuse GetActivity (which reconstructs the activity from
	// the enriched-event blob or run metadata) and wrap it with empty provenance.
	act, err := s.GetActivity(ctx, &pbsvc.GetActivityRequest{UserId: req.UserId, ActivityId: req.ActivityId})
	if err != nil {
		return nil, err
	}
	return &pbactivity.ResolvedActivity{Activity: act}, nil
}

// ListResolvedActivities is GetResolvedActivity over a page of the user's activities: each
// entry carries the resolved activity and its per-field provenance. Activities with a
// layered record are resolved (provenance included); pre-layered runs (or records that
// fail to load) fall back to a lightweight activity built from the run metadata with no
// provenance, mirroring ListActivities. Note this loads one record blob per activity in
// the page, so the page size (req.Limit) bounds the fan-out.
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
		if run.ActivityRecordUri != "" {
			rec, err := activitydomain.LoadActivityRecord(ctx, run.ActivityRecordUri, s.blobStore)
			if err != nil {
				s.logger.Warn(ctx, "failed to read layered activity record for list, using run summary", "error", err, "uri", run.ActivityRecordUri, "run_id", run.Id)
			} else if resolved := activitydomain.Resolve(rec); resolved != nil && resolved.Activity != nil {
				activities = append(activities, resolved)
				continue
			}
		}

		sourceEnum := pbactivity.ActivitySource_SOURCE_UNSPECIFIED
		if val, ok := pbactivity.ActivitySource_value[run.Source]; ok {
			sourceEnum = pbactivity.ActivitySource(val)
		}
		activities = append(activities, &pbactivity.ResolvedActivity{
			Activity: &pbactivity.StandardizedActivity{
				Id:                run.Id,
				PipelineRunStatus: run.Status.String(),
				Source:            sourceEnum,
				ExternalId:        run.ActivityId,
				UserId:            req.UserId,
				StartTime:         run.StartTime,
				Name:              run.Title,
				Type:              run.Type,
			},
		})
	}

	return &pbsvc.ListResolvedActivitiesResponse{
		Activities:    activities,
		NextPageToken: nextToken,
	}, nil
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
