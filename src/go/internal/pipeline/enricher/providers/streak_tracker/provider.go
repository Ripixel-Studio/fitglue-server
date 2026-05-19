package streak_tracker

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// StreakTracker tracks consecutive activity days/weeks.
type StreakTracker struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewStreakTracker())
}

func NewStreakTracker() *StreakTracker {
	return &StreakTracker{}
}

func (p *StreakTracker) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *StreakTracker) Name() string {
	return "streak-tracker"
}

func (p *StreakTracker) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_STREAK_TRACKER
}

func (p *StreakTracker) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("streak_tracker: starting", "activity_name", activity.Name)

	// Parse config
	activityTypes := inputs["activity_types"]
	if activityTypes == "" {
		activityTypes = "any"
	}

	// Get activity date (use start time if available)
	activityDate := time.Now().Format("2006-01-02")
	if activity.StartTime != nil {
		activityDate = activity.StartTime.AsTime().Format("2006-01-02")
	}

	// Fetch streak data from booster_data
	var currentStreak int
	var longestStreak int
	var lastActivityDate string
	boosterId := fmt.Sprintf("streak_tracker_%s", activityTypes)

	var lastExternalId string
	var cachedDescription string
	var cachedMetadata map[string]string

	if p.Service != nil && p.Service.DB != nil {
		data, err := p.Service.DB.GetBoosterData(ctx, user.UserId, boosterId)
		if err != nil {
			logger.Warn("Failed to fetch streak data", "error", err)
		} else if data != nil {
			currentStreak = int(providers.ToFloat64(data["current_streak"]))
			longestStreak = int(providers.ToFloat64(data["longest_streak"]))
			if val, ok := data["last_activity_date"].(string); ok {
				lastActivityDate = val
			}
			if v, ok := data["last_external_id"].(string); ok {
				lastExternalId = v
			}
			if v, ok := data["last_result_description"].(string); ok {
				cachedDescription = v
			}
			if v, ok := data["last_result_metadata"].(map[string]interface{}); ok {
				cachedMetadata = make(map[string]string)
				for k, val := range v {
					if s, ok := val.(string); ok {
						cachedMetadata[k] = s
					}
				}
			}
		}
	}

	// Same-source dedup: if this activity was already processed, return cached result
	externalId := inputs["external_id"]
	if externalId != "" && lastExternalId == externalId && cachedDescription != "" {
		logger.Info("streak_tracker: returning cached result for same-source activity",
			"external_id", externalId)
		if cachedMetadata == nil {
			cachedMetadata = map[string]string{}
		}
		cachedMetadata["dedup"] = "true"
		return &providers.EnrichmentResult{
			Description: cachedDescription,
			Metadata:    cachedMetadata,
		}, nil
	}

	// Determine streak continuation
	streakBroken := false
	isNewDay := lastActivityDate != activityDate

	if isNewDay && lastActivityDate != "" {
		actDate, _ := time.Parse("2006-01-02", activityDate)
		lastDate, _ := time.Parse("2006-01-02", lastActivityDate)

		// If this activity is from a date BEFORE or SAME as the last recorded date,
		// it's out-of-order or a race condition duplicate — don't modify the streak
		if !actDate.After(lastDate) {
			isNewDay = false
			logger.Info("streak_tracker: skipping out-of-order or duplicate activity",
				"activity_date", activityDate,
				"last_activity_date", lastActivityDate,
			)
		} else {
			// Check if streak continues: last activity must be exactly the day before this activity
			expectedPrev := actDate.AddDate(0, 0, -1).Format("2006-01-02")
			if lastActivityDate != expectedPrev {
				// Streak broken - reset
				streakBroken = true
				currentStreak = 0
			}
		}
	}

	// Increment streak only if this is a new day
	if isNewDay {
		currentStreak++
	}

	// Update longest streak
	if currentStreak > longestStreak {
		longestStreak = currentStreak
	}

	// Build output
	activityLabel := getActivityLabel(activityTypes)
	var sb strings.Builder
	sb.WriteString("🔥 Streak Tracker:\n")

	if streakBroken {
		sb.WriteString(fmt.Sprintf("• Day 1 of your %s streak - starting fresh!", activityLabel))
	} else if currentStreak == 1 {
		sb.WriteString(fmt.Sprintf("• %s streak started - let's go!", capitalise(activityLabel)))
	} else {
		emoji := getStreakEmoji(currentStreak)
		sb.WriteString(fmt.Sprintf("• %s %d-day %s streak!", emoji, currentStreak, activityLabel))

		if currentStreak == longestStreak && currentStreak > 1 && isNewDay {
			sb.WriteString("\n• 🏆 New personal best streak!")
		} else if longestStreak > currentStreak {
			sb.WriteString(fmt.Sprintf("\n• Best: %d days", longestStreak))
		}
	}

	logger.Info("Streak tracker processed",
		"activity_types", activityTypes,
		"current_streak", currentStreak,
		"longest_streak", longestStreak,
		"streak_broken", streakBroken,
	)

	resultMetadata := map[string]string{
		"streak_current":       fmt.Sprintf("%d", currentStreak),
		"streak_longest":       fmt.Sprintf("%d", longestStreak),
		"activity_type_filter": activityTypes,
	}

	// Persist updated streak + cached result for same-source dedup
	if p.Service != nil && p.Service.DB != nil && isNewDay {
		metadataMap := make(map[string]interface{})
		for k, v := range resultMetadata {
			metadataMap[k] = v
		}
		updateData := map[string]interface{}{
			"current_streak":          currentStreak,
			"longest_streak":          longestStreak,
			"last_activity_date":      activityDate,
			"last_external_id":        externalId,
			"last_result_description": sb.String(),
			"last_result_metadata":    metadataMap,
		}
		if err := p.Service.DB.SetBoosterData(ctx, user.UserId, boosterId, updateData); err != nil {
			logger.Warn("Failed to save streak data", "error", err)
		}

		// Roll up current/longest streak onto UserProfile for the stats dashboard.
		if err := p.Service.DB.UpdateUser(ctx, user.UserId, map[string]interface{}{
			"current_streak_days": currentStreak,
			"longest_streak_days": longestStreak,
		}); err != nil {
			logger.Warn("Failed to update user streak fields", "error", err)
		}
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata:    resultMetadata,
		Enrichments: &pbactivity.ActivityEnrichments{
			Streak: &pbactivity.StreakSummary{
				CurrentDays: int32(currentStreak),
				LongestDays: int32(longestStreak),
			},
		},
	}, nil
}

func getStreakEmoji(streak int) string {
	switch {
	case streak >= 30:
		return "🏆🔥"
	case streak >= 14:
		return "💪🔥"
	case streak >= 7:
		return "🔥🔥"
	default:
		return "🔥"
	}
}

func getActivityLabel(filter string) string {
	switch filter {
	case "running":
		return "running"
	case "cycling":
		return "cycling"
	case "swimming":
		return "swimming"
	case "strength":
		return "strength training"
	default:
		return "activity"
	}
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
