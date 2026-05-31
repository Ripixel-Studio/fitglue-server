package activity

import (
	"context"
	"fmt"
	"strings"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) generateRoundup(ctx context.Context, userID string, periodType pbactivity.RoundupPeriodType, periodStart, periodEnd time.Time) (*pbactivity.ShowcaseRoundup, error) {
	profile, err := s.store.GetShowcasePreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	if profile == nil || profile.Slug == "" {
		return nil, nil
	}
	settings := profile.RoundupSettings
	if settings == nil {
		return nil, nil
	}
	switch periodType {
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK:
		if !settings.EnabledWeekly {
			return nil, nil
		}
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH:
		if !settings.EnabledMonthly {
			return nil, nil
		}
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR:
		if !settings.EnabledYearly {
			return nil, nil
		}
	}
	entries, err := s.store.ListShowcaseEntriesInRange(ctx, userID, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	roundup := &pbactivity.ShowcaseRoundup{
		Slug:                   profile.Slug,
		UserId:                 userID,
		PeriodType:             periodType,
		PeriodKey:              periodKey(periodType, periodStart),
		PeriodStart:            timestamppb.New(periodStart),
		PeriodEnd:              timestamppb.New(periodEnd),
		GeneratedAt:            timestamppb.Now(),
		OwnerDisplayName:       profile.DisplayName,
		OwnerProfilePictureUrl: profile.ProfilePictureUrl,
		OwnerProfileSlug:       profile.Slug,
	}
	roundup.RoundupId = profile.Slug + "-" + roundup.PeriodKey
	typeBreakdowns := make(map[pbactivity.ActivityType]*pbactivity.RoundupActivityTypeBreakdown)
	sourcesSeen := make(map[pbactivity.ActivitySource]struct{})
	zoneMinutes := make([]int32, 6)
	for _, e := range entries {
		roundup.TotalActivities++
		roundup.TotalDurationSeconds += e.DurationSeconds
		roundup.TotalDistanceMeters += e.DistanceMeters
		if e.CaloriesKcal != nil {
			roundup.TotalCaloriesKcal += *e.CaloriesKcal
		}
		for i, mins := range e.HrZoneMinutes {
			if i < 6 {
				zoneMinutes[i] += mins
			}
		}
		bd, ok := typeBreakdowns[e.ActivityType]
		if !ok {
			bd = &pbactivity.RoundupActivityTypeBreakdown{ActivityType: e.ActivityType}
			typeBreakdowns[e.ActivityType] = bd
		}
		bd.ActivityCount++
		bd.TotalDurationSeconds += e.DurationSeconds
		bd.TotalDistanceMeters += e.DistanceMeters
		bd.TotalSets += e.TotalSets
		bd.TotalReps += e.TotalReps
		bd.TotalWeightKg += e.TotalWeightKg
		sourcesSeen[e.Source] = struct{}{}
	}
	roundup.HrZoneMinutes = zoneMinutes
	for _, bd := range typeBreakdowns {
		roundup.ActivityTypeBreakdowns = append(roundup.ActivityTypeBreakdowns, bd)
	}
	for src := range sourcesSeen {
		roundup.Sources = append(roundup.Sources, src)
	}
	allPRs, err := s.store.ListUserPersonalRecords(ctx, userID)
	if err == nil {
		for _, pr := range allPRs {
			if pr.AchievedAt == nil {
				continue
			}
			t := pr.AchievedAt.AsTime()
			if !t.Before(periodStart) && t.Before(periodEnd) {
				roundup.PrsAchieved = append(roundup.PrsAchieved, pr)
			}
		}
	}
	if err := s.store.SetRoundup(ctx, roundup); err != nil {
		return nil, fmt.Errorf("save roundup: %w", err)
	}

	s.sendRoundupNotification(ctx, userID, roundup)

	return roundup, nil
}

func (s *Service) sendRoundupNotification(ctx context.Context, userID string, roundup *pbactivity.ShowcaseRoundup) {
	if s.notifications == nil {
		return
	}
	tokens, notifyRoundup, err := s.store.GetUserNotificationData(ctx, userID)
	if err != nil || !notifyRoundup || len(tokens) == 0 {
		return
	}
	period := "Weekly"
	switch roundup.PeriodType {
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH:
		period = "Monthly"
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR:
		period = "Yearly"
	}
	title := fmt.Sprintf("%s Showcase Roundup Ready", period)
	body := fmt.Sprintf("%d activities · view your %s summary", roundup.TotalActivities, strings.ToLower(period))
	data := map[string]string{
		"type":    "SHOWCASE_ROUNDUP",
		"user_id": userID,
		"slug":    roundup.Slug,
		"period":  roundup.PeriodKey,
	}
	if err := s.notifications.SendPushNotification(ctx, userID, title, body, tokens, data); err != nil {
		s.logger.Warn(ctx, "Failed to send roundup notification", "error", err, "user_id", userID)
	}
}

func periodKey(t pbactivity.RoundupPeriodType, start time.Time) string {
	switch t {
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK:
		year, week := start.ISOWeek()
		return fmt.Sprintf("week-%02d-%d", week, year)
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH:
		return fmt.Sprintf("month-%02d-%d", int(start.Month()), start.Year())
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR:
		return fmt.Sprintf("year-%d", start.Year())
	default:
		return "unknown"
	}
}

// RoundupPeriodBounds computes [start, end) for the period that just completed.
func RoundupPeriodBounds(t pbactivity.RoundupPeriodType, now time.Time) (start, end time.Time) {
	now = now.UTC()
	switch t {
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK:
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		currentMonday := time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, time.UTC)
		end = currentMonday
		start = currentMonday.AddDate(0, 0, -7)
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH:
		firstOfCurrentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = firstOfCurrentMonth
		start = firstOfCurrentMonth.AddDate(0, -1, 0)
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR:
		end = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		start = time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return
}
