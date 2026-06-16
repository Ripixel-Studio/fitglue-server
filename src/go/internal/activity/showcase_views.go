package activity

import (
	"context"

	"github.com/fitglue/server/src/go/pkg/domain/showcaseview"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// maxShowcasesForViewStats bounds the per-showcase fan-out when aggregating a
// user's view metrics. Showcase counts per user are small in practice.
const maxShowcasesForViewStats = 200

// RecordShowcaseView records a public view for a pre-resolved target_key. Bot
// filtering and visitor-hash derivation happen at the api-public gateway; this
// RPC only performs the de-duplicated counting.
func (s *Service) RecordShowcaseView(ctx context.Context, req *pbsvc.RecordShowcaseViewRequest) (*emptypb.Empty, error) {
	if req.TargetKey == "" {
		return nil, status.Error(codes.InvalidArgument, "target_key is required")
	}

	if err := s.store.RecordShowcaseView(ctx, req.TargetKey, req.VisitorHash); err != nil {
		s.logger.Error(ctx, "failed to record showcase view", "error", err, "target_key", req.TargetKey)
		return nil, status.Error(codes.Internal, "failed to record showcase view")
	}

	return &emptypb.Empty{}, nil
}

// GetShowcaseViewStats returns metrics for a single owned showcase surface.
func (s *Service) GetShowcaseViewStats(ctx context.Context, req *pbsvc.GetShowcaseViewStatsRequest) (*pbactivity.ShowcaseViewStats, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	key, err := s.resolveOwnedTargetKey(ctx, req.UserId, req.Target, req.TargetId)
	if err != nil {
		return nil, err
	}

	stats, err := s.store.GetShowcaseViewStats(ctx, key)
	if err != nil {
		s.logger.Error(ctx, "failed to read showcase view stats", "error", err, "target_key", key)
		return nil, status.Error(codes.Internal, "failed to read showcase view stats")
	}
	return stats, nil
}

// ListShowcaseViewStats returns per-showcase and profile metrics plus aggregate
// totals for everything a user owns.
func (s *Service) ListShowcaseViewStats(ctx context.Context, req *pbsvc.ListShowcaseViewStatsRequest) (*pbsvc.ListShowcaseViewStatsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	resp := &pbsvc.ListShowcaseViewStatsResponse{}

	// Profile-page metrics (keyed by the owner's current slug).
	if profile, err := s.store.GetShowcasePreferences(ctx, req.UserId); err == nil && profile != nil && profile.Slug != "" {
		if stats, err := s.store.GetShowcaseViewStats(ctx, showcaseview.ProfileKey(profile.Slug)); err == nil {
			resp.Profile = stats
			resp.TotalViews += stats.Views
			resp.TotalVisitors += stats.Visitors
		}
	}

	// Per-showcased-activity metrics.
	showcases, _, err := s.store.ListShowcasedActivitiesByUser(ctx, req.UserId, maxShowcasesForViewStats, 0)
	if err != nil {
		s.logger.Error(ctx, "failed to list showcases for view stats", "error", err)
		return nil, status.Error(codes.Internal, "failed to list showcase view stats")
	}

	for _, sc := range showcases {
		stats, err := s.store.GetShowcaseViewStats(ctx, showcaseview.ActivityKey(sc.ShowcaseId))
		if err != nil {
			s.logger.Warn(ctx, "skipping showcase view stats", "error", err, "showcase_id", sc.ShowcaseId)
			continue
		}
		resp.Showcases = append(resp.Showcases, stats)
		resp.TotalViews += stats.Views
		resp.TotalVisitors += stats.Visitors
	}

	return resp, nil
}

// resolveOwnedTargetKey verifies the caller owns the requested surface and
// returns its view-metrics target key. Profile resolution intentionally uses
// the caller's own slug, so a user can only ever read their own profile stats.
func (s *Service) resolveOwnedTargetKey(ctx context.Context, userID string, target pbactivity.ShowcaseViewTarget, targetID string) (string, error) {
	switch target {
	case pbactivity.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_ACTIVITY:
		if targetID == "" {
			return "", status.Error(codes.InvalidArgument, "target_id is required for activity stats")
		}
		_, ownerUserID, err := s.store.GetPublicShowcase(ctx, targetID)
		if err != nil {
			s.logger.Error(ctx, "failed to resolve showcase owner", "error", err, "showcase_id", targetID)
			return "", status.Error(codes.Internal, "failed to resolve showcase")
		}
		if ownerUserID == "" {
			return "", status.Error(codes.NotFound, "showcase not found")
		}
		if ownerUserID != userID {
			return "", status.Error(codes.PermissionDenied, "not the owner of this showcase")
		}
		return showcaseview.ActivityKey(targetID), nil

	case pbactivity.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_PROFILE:
		profile, err := s.store.GetShowcasePreferences(ctx, userID)
		if err != nil {
			s.logger.Error(ctx, "failed to load profile for view stats", "error", err)
			return "", status.Error(codes.Internal, "failed to load profile")
		}
		if profile == nil || profile.Slug == "" {
			return "", status.Error(codes.NotFound, "no published profile")
		}
		return showcaseview.ProfileKey(profile.Slug), nil

	default:
		return "", status.Error(codes.InvalidArgument, "unsupported view target")
	}
}
