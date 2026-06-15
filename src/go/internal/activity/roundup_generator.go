package activity

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/pkg/notificationpub"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
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

	// Rich media — photo mosaic + GPS route wall, drawn from the period's entries.
	roundup.Photos, roundup.Routes = collectRoundupMedia(entries, profile.ShowPhotoGallery)

	// Highlight stats — single best across the period
	for _, e := range entries {
		if e.DurationSeconds > roundup.LongestActivityDurationSeconds {
			roundup.LongestActivityDurationSeconds = e.DurationSeconds
		}
		if e.AvgHeartRate != nil && *e.AvgHeartRate > roundup.HighestAvgBpm {
			roundup.HighestAvgBpm = *e.AvgHeartRate
			roundup.HighestAvgBpmActivityTitle = e.Title
		}
		if e.CaloriesKcal != nil && e.DurationSeconds > 0 {
			cph := float64(*e.CaloriesKcal) / e.DurationSeconds * 3600.0
			if cph > roundup.HighestCaloriesPerHourKcal {
				roundup.HighestCaloriesPerHourKcal = cph
			}
		}
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
	// Consistency calendar — build per-day effort entries
	type dayKey struct{ y, m, d int }
	type dayData struct {
		effortLevel   int32
		activityCount int32
	}
	dayMap := make(map[dayKey]*dayData)
	for _, e := range entries {
		if e.StartTime == nil {
			continue
		}
		t := e.StartTime.AsTime().UTC()
		k := dayKey{t.Year(), int(t.Month()), t.Day()}
		if _, ok := dayMap[k]; !ok {
			dayMap[k] = &dayData{}
		}
		dayMap[k].activityCount++
		if lvl := deriveEffortLevel(e); lvl > dayMap[k].effortLevel {
			dayMap[k].effortLevel = lvl
		}
	}
	type sortedDay struct {
		key  dayKey
		data *dayData
	}
	var sortedDays []sortedDay
	for k, d := range dayMap {
		sortedDays = append(sortedDays, sortedDay{k, d})
	}
	sort.Slice(sortedDays, func(i, j int) bool {
		a, b := sortedDays[i].key, sortedDays[j].key
		if a.y != b.y {
			return a.y < b.y
		}
		if a.m != b.m {
			return a.m < b.m
		}
		return a.d < b.d
	})
	for _, sd := range sortedDays {
		roundup.DayEntries = append(roundup.DayEntries, &pbactivity.RoundupDayEntry{
			Date:          fmt.Sprintf("%04d-%02d-%02d", sd.key.y, sd.key.m, sd.key.d),
			EffortLevel:   sd.data.effortLevel,
			ActivityCount: sd.data.activityCount,
		})
	}

	// Callout activities — biggest session, longest streak, peak PR day
	var biggestEntry *pbactivity.ShowcaseProfileEntry
	for _, e := range entries {
		if biggestEntry == nil || e.DurationSeconds > biggestEntry.DurationSeconds {
			biggestEntry = e
		}
	}
	if biggestEntry != nil && biggestEntry.DurationSeconds > 0 {
		h := int(biggestEntry.DurationSeconds) / 3600
		m := (int(biggestEntry.DurationSeconds) % 3600) / 60
		statVal := fmt.Sprintf("%d:%02d", h, m)
		statUnit := "H:MM"
		if biggestEntry.DistanceMeters > 500 {
			statUnit = fmt.Sprintf("H:MM · %.0f KM", biggestEntry.DistanceMeters/1000)
		}
		dateStr := ""
		if biggestEntry.StartTime != nil {
			dateStr = biggestEntry.StartTime.AsTime().UTC().Format("2 Jan")
		}
		roundup.CalloutActivities = append(roundup.CalloutActivities, &pbactivity.ShowcaseCalloutActivity{
			Kind:         "BIGGEST SESSION",
			Title:        biggestEntry.Title,
			StatValue:    statVal,
			StatUnit:     statUnit,
			Date:         dateStr,
			ActivityType: biggestEntry.ActivityType,
		})
	}

	// Longest streak from day entries
	if len(sortedDays) > 0 {
		longestStreak, curStreak := 0, 0
		var bestStart, bestEnd, curStart time.Time
		var prevDay time.Time
		for _, sd := range sortedDays {
			day := time.Date(sd.key.y, time.Month(sd.key.m), sd.key.d, 0, 0, 0, 0, time.UTC)
			if !prevDay.IsZero() && day.Sub(prevDay) == 24*time.Hour {
				curStreak++
			} else {
				curStreak = 1
				curStart = day
			}
			if curStreak > longestStreak {
				longestStreak = curStreak
				bestStart = curStart
				bestEnd = day
			}
			prevDay = day
		}
		if longestStreak >= 3 {
			sub := ""
			if !bestStart.IsZero() {
				sub = bestStart.Format("2 Jan") + " — " + bestEnd.Format("2 Jan") + " · no zeros"
			}
			roundup.CalloutActivities = append(roundup.CalloutActivities, &pbactivity.ShowcaseCalloutActivity{
				Kind:      "LONGEST STREAK",
				Title:     fmt.Sprintf("%d Days Unbroken", longestStreak),
				StatValue: fmt.Sprintf("%d", longestStreak),
				StatUnit:  "CONSECUTIVE DAYS",
				Sub:       sub,
				Date:      "STREAK",
			})
		}
	}

	// Peak PR day
	if len(roundup.PrsAchieved) > 0 {
		prsByDate := make(map[string]int)
		for _, pr := range roundup.PrsAchieved {
			if pr.AchievedAt != nil {
				d := pr.AchievedAt.AsTime().UTC().Format("2006-01-02")
				prsByDate[d]++
			}
		}
		peakDate, peakCount := "", 0
		for d, count := range prsByDate {
			if count > peakCount {
				peakCount = count
				peakDate = d
			}
		}
		dateLabel := ""
		if peakDate != "" {
			if parsed, err2 := time.Parse("2006-01-02", peakDate); err2 == nil {
				dateLabel = parsed.Format("2 Jan")
			}
		}
		statVal := fmt.Sprintf("%d", len(roundup.PrsAchieved))
		statUnit := "RECORDS THIS PERIOD"
		sub := ""
		if peakCount >= 2 {
			statVal = fmt.Sprintf("%d", peakCount)
			statUnit = "RECORDS IN ONE SESSION"
			sub = fmt.Sprintf("%d total PRs this period", len(roundup.PrsAchieved))
		} else {
			dateLabel = ""
		}
		roundup.CalloutActivities = append(roundup.CalloutActivities, &pbactivity.ShowcaseCalloutActivity{
			Kind:      "PERSONAL RECORDS",
			Title:     "Records Broken",
			StatValue: statVal,
			StatUnit:  statUnit,
			Sub:       sub,
			Date:      dateLabel,
		})
	}

	roundup.AiSummary = generateRoundupAISummary(ctx, s.logger, roundup)

	if err := s.store.SetRoundup(ctx, roundup); err != nil {
		return nil, fmt.Errorf("save roundup: %w", err)
	}

	s.sendRoundupNotification(ctx, userID, roundup)

	return roundup, nil
}

func (s *Service) sendRoundupNotification(ctx context.Context, userID string, roundup *pbactivity.ShowcaseRoundup) {
	if s.publisher == nil {
		return
	}
	period := "Weekly"
	switch roundup.PeriodType {
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH:
		period = "Monthly"
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR:
		period = "Yearly"
	}
	req := &pbnotification.NotificationRequest{
		UserId: userID,
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_SHOWCASE_ROUNDUP,
		Title:  fmt.Sprintf("%s Showcase Roundup Ready", period),
		Body:   fmt.Sprintf("%d activities · view your %s summary", roundup.TotalActivities, strings.ToLower(period)),
		Data:   map[string]string{"slug": roundup.Slug, "period": roundup.PeriodKey},
	}
	if err := notificationpub.Enqueue(ctx, s.publisher, req); err != nil {
		s.logger.Warn(ctx, "Failed to enqueue roundup notification", "error", err, "user_id", userID)
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
