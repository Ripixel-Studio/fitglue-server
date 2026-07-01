package activity

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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

// roundupsPageSize mirrors the profile activity feed's page size, so both
// "load more" affordances on the showcase profile page paginate consistently.
const roundupsPageSize = 12

func (s *Service) GetRecentPublicRoundups(ctx context.Context, req *pbsvc.GetRecentPublicRoundupsRequest) (*pbsvc.GetRecentPublicRoundupsResponse, error) {
	if req.Slug == "" {
		return nil, status.Error(codes.InvalidArgument, "slug is required")
	}

	all, err := s.store.ListAllRoundups(ctx, req.Slug)
	if err != nil {
		s.logger.Error(ctx, "failed to list roundups", "error", err)
		return nil, status.Error(codes.Internal, "failed to list roundups")
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	start := int(page-1) * roundupsPageSize
	if start < 0 {
		start = 0
	}

	var pageRoundups []*pbactivity.ShowcaseRoundup
	if start < len(all) {
		end := start + roundupsPageSize
		if end > len(all) {
			end = len(all)
		}
		pageRoundups = all[start:end]
	}

	total := int32(len(all))
	totalPages := (total + int32(roundupsPageSize) - 1) / int32(roundupsPageSize)
	if totalPages < 1 {
		totalPages = 1
	}

	return &pbsvc.GetRecentPublicRoundupsResponse{
		Roundups:    pageRoundups,
		TotalPages:  totalPages,
		CurrentPage: page,
	}, nil
}

func (s *Service) RecomputeRoundup(ctx context.Context, req *pbsvc.RecomputeRoundupRequest) (*pbactivity.ShowcaseRoundup, error) {
	if req.UserId == "" || req.PeriodKey == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and period_key are required")
	}
	periodType, periodStart, periodEnd, err := parsePeriodKey(req.PeriodKey)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid period_key: %v", err))
	}
	roundup, err := s.generateRoundup(ctx, req.UserId, periodType, periodStart, periodEnd)
	if err != nil {
		s.logger.Error(ctx, "failed to recompute roundup", "error", err)
		return nil, status.Error(codes.Internal, "failed to recompute roundup")
	}
	if roundup == nil {
		return nil, status.Error(codes.NotFound, "no activities found for this period or roundups not enabled")
	}
	return roundup, nil
}

func parsePeriodKey(key string) (pbactivity.RoundupPeriodType, time.Time, time.Time, error) {
	parts := strings.Split(key, "-")
	switch parts[0] {
	case "week":
		// week-WW-YYYY
		if len(parts) != 3 {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("expected week-WW-YYYY")
		}
		week, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("invalid week: %w", err)
		}
		year, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("invalid year: %w", err)
		}
		// ISO week 1 Monday: Jan 4 is always in week 1
		jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
		wd := int(jan4.Weekday())
		if wd == 0 {
			wd = 7
		}
		weekOneMonday := jan4.AddDate(0, 0, -(wd - 1))
		start := weekOneMonday.AddDate(0, 0, (week-1)*7)
		end := start.AddDate(0, 0, 7)
		return pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, start, end, nil
	case "month":
		// month-MM-YYYY
		if len(parts) != 3 {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("expected month-MM-YYYY")
		}
		month, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("invalid month: %w", err)
		}
		year, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("invalid year: %w", err)
		}
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		return pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH, start, end, nil
	case "year":
		// year-YYYY
		if len(parts) != 2 {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("expected year-YYYY")
		}
		year, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, time.Time{}, time.Time{}, fmt.Errorf("invalid year: %w", err)
		}
		start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
		return pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR, start, end, nil
	default:
		return 0, time.Time{}, time.Time{}, fmt.Errorf("unknown prefix %q", parts[0])
	}
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
