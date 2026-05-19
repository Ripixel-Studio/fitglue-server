package calories_burned

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"strconv"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// MET values for different activity types (Metabolic Equivalent of Task)
// Higher MET = more intense activity
var activityMETs = map[pbactivity.ActivityType]float64{
	pbactivity.ActivityType_ACTIVITY_TYPE_RUN:                              9.8,
	pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN:                        10.5,
	pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN:                      8.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_WALK:                             3.5,
	pbactivity.ActivityType_ACTIVITY_TYPE_HIKE:                             6.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_RIDE:                             7.5,
	pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE:               8.5,
	pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE:                      8.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE:                     6.8,
	pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE:                       4.5,
	pbactivity.ActivityType_ACTIVITY_TYPE_SWIM:                             8.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING:                  5.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT:                         10.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_YOGA:                             3.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_PILATES:                          3.5,
	pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING: 11.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_ROWING:                           7.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_ELLIPTICAL:                       5.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_STAIR_STEPPER:                    9.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_SOCCER:                           7.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_TENNIS:                           7.3,
	pbactivity.ActivityType_ACTIVITY_TYPE_BADMINTON:                        5.5,
	pbactivity.ActivityType_ACTIVITY_TYPE_NORDIC_SKI:                       9.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_ALPINE_SKI:                       5.3,
	pbactivity.ActivityType_ACTIVITY_TYPE_SNOWBOARD:                        5.3,
	pbactivity.ActivityType_ACTIVITY_TYPE_KAYAKING:                         5.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_SURFING:                          3.0,
	pbactivity.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING:                    8.0,
}

// Fun food equivalents for calorie display
type FoodEquivalent struct {
	Name     string
	Calories float64
	Emoji    string
}

var foodEquivalents = []FoodEquivalent{
	{Name: "slice of pizza", Calories: 285, Emoji: "🍕"},
	{Name: "donut", Calories: 250, Emoji: "🍩"},
	{Name: "banana", Calories: 105, Emoji: "🍌"},
	{Name: "beer", Calories: 150, Emoji: "🍺"},
	{Name: "chocolate bar", Calories: 230, Emoji: "🍫"},
	{Name: "cookie", Calories: 80, Emoji: "🍪"},
	{Name: "apple", Calories: 95, Emoji: "🍎"},
	{Name: "glass of wine", Calories: 125, Emoji: "🍷"},
	{Name: "burger", Calories: 540, Emoji: "🍔"},
	{Name: "ice cream scoop", Calories: 140, Emoji: "🍨"},
}

type CaloriesBurned struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewCaloriesBurned())
}

func NewCaloriesBurned() *CaloriesBurned {
	return &CaloriesBurned{}
}

func (p *CaloriesBurned) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *CaloriesBurned) Name() string {
	return "calories-burned"
}

func (p *CaloriesBurned) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_CALORIES_BURNED
}

func (p *CaloriesBurned) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("calories_burned: starting", "activity_name", activity.Name)

	// Parse config
	funMode := inputs["fun_mode"] == "true"

	// Get user weight (default 70kg if not provided)
	userWeight := 70.0
	if weightStr, ok := inputs["user_weight"]; ok && weightStr != "" {
		if w, err := strconv.ParseFloat(weightStr, 64); err == nil && w > 0 {
			userWeight = w
		}
	}

	// Calculate total duration in hours
	var totalSeconds float64
	for _, session := range activity.Sessions {
		totalSeconds += session.TotalElapsedTime
	}
	durationHours := totalSeconds / 3600.0

	if durationHours <= 0 {
		logger.Debug("calories_burned: skipping - no duration data")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "No duration data",
			Metadata: map[string]string{
				"calories_status": "skipped",
				"status_detail":   "No duration data",
			},
		}, nil
	}

	// Get MET value for activity type
	met := getMET(activity.Type)

	// Calculate calories: MET × weight(kg) × duration(hours)
	calories := met * userWeight * durationHours

	// Build output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔥 Calories: %.0f kcal", calories))

	if funMode && calories > 50 {
		equiv := getFoodEquivalent(calories)
		sb.WriteString(fmt.Sprintf(" ≈ %.1f %s %s", calories/equiv.Calories, equiv.Name, equiv.Emoji))
	}

	logger.Info("Calories calculated",
		"calories", calories,
		"met", met,
		"weight_kg", userWeight,
		"duration_hours", durationHours,
	)

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"calories_status": "success",
			"calories":        fmt.Sprintf("%.0f", calories),
			"met_value":       fmt.Sprintf("%.1f", met),
			"duration_hours":  fmt.Sprintf("%.2f", durationHours),
			"weight_kg":       fmt.Sprintf("%.0f", userWeight),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			Calories: &pbactivity.CaloriesSummary{
				Kcal: int32(calories),
			},
		},
	}, nil
}

// getMET returns the MET value for an activity type, with a default fallback
func getMET(actType pbactivity.ActivityType) float64 {
	if met, ok := activityMETs[actType]; ok {
		return met
	}
	// Default MET for unknown activities (moderate intensity)
	return 5.0
}

// getFoodEquivalent picks a fun food to compare calories to
func getFoodEquivalent(calories float64) FoodEquivalent {
	// Pick a food that gives a reasonable ratio (not too high, not too low)
	for _, food := range foodEquivalents {
		ratio := calories / food.Calories
		if ratio >= 0.5 && ratio <= 10 {
			return food
		}
	}
	// Default to pizza
	return foodEquivalents[0]
}
