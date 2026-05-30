package activity

import (
	"context"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) GetPublicRoundup(ctx context.Context, req *pbsvc.GetPublicRoundupRequest) (*pbactivity.ShowcaseRoundup, error) {
	if req.Slug == "" || req.PeriodKey == "" {
		return nil, status.Error(codes.InvalidArgument, "slug and period_key are required")
	}
	roundup, err := s.store.GetRoundup(ctx, req.Slug, req.PeriodKey)
	if err != nil {
		s.logger.Error(ctx, "failed to get roundup", "error", err)
		return nil, status.Error(codes.Internal, "failed to read roundup")
	}
	if roundup == nil {
		return nil, status.Error(codes.NotFound, "roundup not found")
	}
	return roundup, nil
}

func (s *Service) GetRecentPublicRoundups(ctx context.Context, req *pbsvc.GetRecentPublicRoundupsRequest) (*pbsvc.GetRecentPublicRoundupsResponse, error) {
	if req.Slug == "" {
		return nil, status.Error(codes.InvalidArgument, "slug is required")
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 3
	}
	roundups, err := s.store.ListRecentRoundups(ctx, req.Slug, limit)
	if err != nil {
		s.logger.Error(ctx, "failed to list recent roundups", "error", err)
		return nil, status.Error(codes.Internal, "failed to list roundups")
	}
	return &pbsvc.GetRecentPublicRoundupsResponse{Roundups: roundups}, nil
}

func (s *Service) UpdateRoundupSettings(ctx context.Context, req *pbsvc.UpdateRoundupSettingsRequest) (*pbactivity.ShowcaseProfile, error) {
	if req.UserId == "" || req.Settings == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and settings are required")
	}
	profile, err := s.ensureShowcaseProfile(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to ensure showcase profile", "error", err)
		return nil, status.Error(codes.Internal, "failed to load profile")
	}
	profile.RoundupSettings = req.Settings
	updated, err := s.store.UpdateShowcasePreferences(ctx, req.UserId, profile)
	if err != nil {
		s.logger.Error(ctx, "failed to update roundup settings", "error", err)
		return nil, status.Error(codes.Internal, "failed to save settings")
	}
	return updated, nil
}
