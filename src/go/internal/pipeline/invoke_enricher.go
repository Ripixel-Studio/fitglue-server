// nolint:proto-json
package pipeline

import (
	"context"
	"errors"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher"
	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EnricherInvoker runs a single registered enricher out-of-band against an activity,
// returning the resulting proposed enricher-run layer (or nil when the enricher produced
// no change worth proposing). Implemented by enricher.SingleInvoker in production; an
// interface here so the pipeline service can be unit-tested without the provider graph.
type EnricherInvoker interface {
	InvokeSingle(ctx context.Context, providerName string, activity *pbactivity.StandardizedActivity, userRec *user.Record) (*pbactivity.EnricherRunLayer, error)
}

// enricherUserSource fetches the user record an enricher needs when invoked out-of-band.
type enricherUserSource interface {
	GetUser(ctx context.Context, id string) (*user.Record, error)
}

// SetEnricherInvocation wires the dependencies required by the out-of-band single-enricher
// endpoints (InvokeEnricher / AcceptProposedEnricherRun / DismissProposedEnricherRun). It is
// kept separate from NewService so the many existing call sites — and unit tests that don't
// exercise enricher invocation — need no change; when it is not called those endpoints
// return Unimplemented.
func (s *Service) SetEnricherInvocation(userDB enricherUserSource, invoker EnricherInvoker) {
	s.userDB = userDB
	s.invoker = invoker
}

// loadRecordForActivity locates an activity's most recent pipeline run, resolves the layered
// ActivityRecord it points at, and returns the record with the GCS bucket/object where it
// lives (so the caller can write it back). Mirrors the lookups RefreshActivitySource does.
func (s *Service) loadRecordForActivity(ctx context.Context, userID, activityID string) (rec *pbactivity.ActivityRecord, bucket, object string, err error) {
	run, err := s.store.FindPipelineRunByActivityId(ctx, userID, activityID)
	if err != nil {
		s.logger.Error(ctx, "failed to find pipeline run by activity", "error", err, "activityId", activityID)
		return nil, "", "", status.Error(codes.Internal, "failed to look up activity")
	}
	if run == nil {
		return nil, "", "", status.Error(codes.NotFound, "no pipeline run found for activity")
	}
	if run.ActivityRecordUri == "" {
		return nil, "", "", status.Error(codes.FailedPrecondition, "activity has no layered record; enrichers are not re-runnable")
	}
	b, o, ok := activitydomain.ParseGCSURI(run.ActivityRecordUri)
	if !ok {
		return nil, "", "", status.Error(codes.Internal, "invalid activity record uri")
	}
	rec, err = s.loadActivityRecord(ctx, run.ActivityRecordUri)
	if err != nil {
		s.logger.Error(ctx, "failed to load activity record", "error", err, "uri", run.ActivityRecordUri)
		return nil, "", "", status.Error(codes.Internal, "failed to load activity record")
	}
	return rec, b, o, nil
}

// persistRecord marshals and writes the layered record back to its GCS object.
func (s *Service) persistRecord(ctx context.Context, bucket, object string, rec *pbactivity.ActivityRecord) error {
	data, err := protojson.Marshal(rec)
	if err != nil {
		s.logger.Error(ctx, "failed to marshal activity record", "error", err)
		return status.Error(codes.Internal, "failed to persist activity record")
	}
	if err := s.blobStore.Write(ctx, bucket, object, data); err != nil {
		s.logger.Error(ctx, "failed to write activity record", "error", err, "bucket", bucket, "object", object)
		return status.Error(codes.Internal, "failed to persist activity record")
	}
	return nil
}

// InvokeEnricher re-runs one enricher out-of-band against a persisted activity and records
// the result as a proposed enricher-run layer (editable-activities spec, DECISION 2a). The
// proposal never touches derived_activity, the applied enricher_layers, or the user-edit
// overlay — so it cannot clobber the user's edits. See the RPC doc on PipelineService.
func (s *Service) InvokeEnricher(ctx context.Context, req *pbsvc.InvokeEnricherRequest) (*pbsvc.InvokeEnricherResponse, error) {
	if req.UserId == "" || req.ActivityId == "" || req.ProviderName == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id, activity_id and provider_name are required")
	}
	if s.invoker == nil || s.userDB == nil {
		return nil, status.Error(codes.Unimplemented, "enricher invocation is not configured")
	}

	rec, bucket, object, err := s.loadRecordForActivity(ctx, req.UserId, req.ActivityId)
	if err != nil {
		return nil, err
	}
	if rec.GetDerivedActivity() == nil {
		return nil, status.Error(codes.FailedPrecondition, "activity has no derived data to enrich")
	}

	userRec, err := s.userDB.GetUser(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to load user for enricher invocation", "error", err, "userId", req.UserId)
		return nil, status.Error(codes.Internal, "failed to load user")
	}
	if userRec == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// Run the enricher against the derived activity (source + already-applied enrichers).
	layer, err := s.invoker.InvokeSingle(ctx, req.ProviderName, rec.GetDerivedActivity(), userRec)
	if err != nil {
		switch {
		case errors.Is(err, enricher.ErrProviderNotFound):
			return nil, status.Errorf(codes.NotFound, "enricher %q not found", req.ProviderName)
		case errors.Is(err, enricher.ErrNotIndividuallyInvokable):
			return nil, status.Errorf(codes.FailedPrecondition, "enricher %q cannot be individually invoked", req.ProviderName)
		case errors.Is(err, enricher.ErrRequiresInput):
			return nil, status.Errorf(codes.FailedPrecondition, "enricher %q requires user input, which is unavailable out-of-band", req.ProviderName)
		default:
			s.logger.Error(ctx, "enricher invocation failed", "error", err, "provider", req.ProviderName, "activityId", req.ActivityId)
			return nil, status.Errorf(codes.Internal, "enricher %q failed", req.ProviderName)
		}
	}

	// No layerable change and no typed enrichment: nothing to propose. Return the current
	// resolved activity unchanged so the caller can report "no change".
	if layer == nil {
		return &pbsvc.InvokeEnricherResponse{
			Preview:         activitydomain.EffectiveActivity(rec),
			ProposedCreated: false,
		}, nil
	}

	now := time.Now()
	rec.ProposedEnricherLayers = append(rec.ProposedEnricherLayers, layer)
	rec.UpdatedAt = timestamppb.New(now)
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = activitydomain.ActivityRecordSchemaVersion
	}
	if err := s.persistRecord(ctx, bucket, object, rec); err != nil {
		return nil, err
	}

	s.logger.Info(ctx, "Recorded proposed enricher run", "activityId", req.ActivityId, "provider", req.ProviderName, "executionId", layer.GetExecutionId())

	// Preview: derived + this proposal's contribution, with the user overlay applied on top
	// (the overlay still wins). Non-persistent.
	preview := proto.Clone(rec.GetDerivedActivity()).(*pbactivity.StandardizedActivity)
	activitydomain.ApplyEnricherContribution(preview, layer.GetContribution())
	activitydomain.ApplyUserEditOverlay(preview, rec.GetUserEditOverlay())

	return &pbsvc.InvokeEnricherResponse{
		Proposed:        layer,
		Preview:         preview,
		ProposedCreated: true,
	}, nil
}

// AcceptProposedEnricherRun applies a proposed enricher-run layer: it folds the proposal's
// contribution into the derived activity, moves it into the append-only enricher_layers as
// real history, merges its typed enrichments, and removes it from proposed_enricher_layers.
// The user-edit overlay is left untouched and still wins on read.
func (s *Service) AcceptProposedEnricherRun(ctx context.Context, req *pbsvc.AcceptProposedEnricherRunRequest) (*pbactivity.StandardizedActivity, error) {
	if req.UserId == "" || req.ActivityId == "" || req.ExecutionId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id, activity_id and execution_id are required")
	}

	rec, bucket, object, err := s.loadRecordForActivity(ctx, req.UserId, req.ActivityId)
	if err != nil {
		return nil, err
	}

	layer, remaining := extractProposedLayer(rec.GetProposedEnricherLayers(), req.ExecutionId)
	if layer == nil {
		return nil, status.Error(codes.NotFound, "proposed enricher run not found")
	}

	// Fold the proposal into the derived activity and merge its typed enrichments.
	if rec.DerivedActivity == nil {
		return nil, status.Error(codes.FailedPrecondition, "activity has no derived data")
	}
	activitydomain.ApplyEnricherContribution(rec.DerivedActivity, layer.GetContribution())
	if layer.GetEnrichments() != nil {
		if rec.Enrichments == nil {
			rec.Enrichments = &pbactivity.ActivityEnrichments{}
		}
		proto.Merge(rec.Enrichments, layer.GetEnrichments())
	}

	// Promote the proposal to real, append-only history and drop it from the proposed set.
	rec.EnricherLayers = append(rec.EnricherLayers, layer)
	rec.ProposedEnricherLayers = remaining
	rec.UpdatedAt = timestamppb.New(time.Now())
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = activitydomain.ActivityRecordSchemaVersion
	}

	if err := s.persistRecord(ctx, bucket, object, rec); err != nil {
		return nil, err
	}

	s.logger.Info(ctx, "Accepted proposed enricher run", "activityId", req.ActivityId, "provider", layer.GetProviderName(), "executionId", req.ExecutionId)

	// Resolved view: derived (now including the accepted enricher) with the user overlay
	// applied on top. The overlay was never modified, so its edits still win.
	return activitydomain.EffectiveActivity(rec), nil
}

// DismissProposedEnricherRun drops a proposed enricher-run layer without applying it.
func (s *Service) DismissProposedEnricherRun(ctx context.Context, req *pbsvc.DismissProposedEnricherRunRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ActivityId == "" || req.ExecutionId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id, activity_id and execution_id are required")
	}

	rec, bucket, object, err := s.loadRecordForActivity(ctx, req.UserId, req.ActivityId)
	if err != nil {
		return nil, err
	}

	layer, remaining := extractProposedLayer(rec.GetProposedEnricherLayers(), req.ExecutionId)
	if layer == nil {
		return nil, status.Error(codes.NotFound, "proposed enricher run not found")
	}

	rec.ProposedEnricherLayers = remaining
	rec.UpdatedAt = timestamppb.New(time.Now())
	if err := s.persistRecord(ctx, bucket, object, rec); err != nil {
		return nil, err
	}

	s.logger.Info(ctx, "Dismissed proposed enricher run", "activityId", req.ActivityId, "executionId", req.ExecutionId)
	return &emptypb.Empty{}, nil
}

// extractProposedLayer returns the proposed layer with the given execution id and the
// remaining layers with it removed. Returns (nil, layers) when not found.
func extractProposedLayer(layers []*pbactivity.EnricherRunLayer, executionID string) (*pbactivity.EnricherRunLayer, []*pbactivity.EnricherRunLayer) {
	for i, l := range layers {
		if l.GetExecutionId() == executionID {
			remaining := make([]*pbactivity.EnricherRunLayer, 0, len(layers)-1)
			remaining = append(remaining, layers[:i]...)
			remaining = append(remaining, layers[i+1:]...)
			return l, remaining
		}
	}
	return nil, layers
}
