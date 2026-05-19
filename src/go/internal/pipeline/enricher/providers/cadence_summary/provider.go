package cadence_summary

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"math"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// CadenceSummary calculates and appends cadence statistics to the activity description.
// Shows avg/max cadence in rpm (revolutions/steps per minute).
type CadenceSummary struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewCadenceSummary())
}

func NewCadenceSummary() *CadenceSummary {
	return &CadenceSummary{}
}

func (p *CadenceSummary) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *CadenceSummary) Name() string {
	return "cadence-summary"
}

func (p *CadenceSummary) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_CADENCE_SUMMARY
}

func (p *CadenceSummary) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("cadence_summary: starting", "activity_name", activity.Name)

	// Parse config
	showCorrelation := inputs["show_correlation"] == "true"

	// Collect cadence and speed values from the activity (for correlation)
	var cadences []int32
	var speeds []float64 // m/s values paired with cadence

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Cadence > 0 {
					cadences = append(cadences, record.Cadence)
					if record.Speed > 0 {
						speeds = append(speeds, record.Speed)
					} else {
						speeds = append(speeds, 0)
					}
				}
			}
		}
	}

	if len(cadences) == 0 {
		logger.Info("No cadence data found for cadence summary enricher")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "No cadence data found",
			Metadata: map[string]string{
				"cadence_summary_status": "skipped",
				"status_detail":          "No cadence data found",
			},
		}, nil
	}

	// Calculate avg and max cadence
	var sumCadence int64
	var maxCadence int32 = cadences[0]

	for _, cadence := range cadences {
		sumCadence += int64(cadence)
		if cadence > maxCadence {
			maxCadence = cadence
		}
	}

	avgCadence := float64(sumCadence) / float64(len(cadences))

	// Determine unit based on activity type (spm for running, rpm for cycling)
	unit := "rpm"
	if isRunningActivity(activity.Type) {
		unit = "spm"
	}

	logger.Info("Cadence summary calculated",
		"avg_cadence", avgCadence,
		"max_cadence", maxCadence,
		"sample_count", len(cadences),
	)

	// Build output based on config
	var sb strings.Builder

	if showCorrelation {
		// Multi-line bullet format with correlation
		sb.WriteString("🦶 Cadence:\n")
		sb.WriteString(fmt.Sprintf("• %.0f %s avg\n", avgCadence, unit))
		sb.WriteString(fmt.Sprintf("• %d %s max\n", maxCadence, unit))

		// Calculate Pearson correlation between cadence and speed
		corr := calculatePaceCorrelation(cadences, speeds)
		if !math.IsNaN(corr) && len(speeds) >= 10 {
			var interpretation string
			if corr > 0.3 {
				interpretation = "faster pace = higher cadence"
			} else if corr < -0.3 {
				interpretation = "slower pace = higher cadence"
			} else {
				interpretation = "no strong correlation"
			}
			sb.WriteString(fmt.Sprintf("• Pace Correlation: %+.2f (%s)", corr, interpretation))
		}
	} else {
		// Simple single-line format
		sb.WriteString(fmt.Sprintf("🦶 Cadence: %.0f %s avg • %d %s max", avgCadence, unit, maxCadence, unit))
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"cadence_summary_status": "success",
			"cadence_avg":            fmt.Sprintf("%.0f", avgCadence),
			"cadence_max":            fmt.Sprintf("%d", maxCadence),
			"cadence_sample_count":   fmt.Sprintf("%d", len(cadences)),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			Cadence: &pbactivity.CadenceSummary{
				AvgRpm: int32(avgCadence),
				MaxRpm: maxCadence,
			},
		},
	}, nil
}

// calculatePaceCorrelation computes Pearson correlation coefficient between cadence and speed
func calculatePaceCorrelation(cadences []int32, speeds []float64) float64 {
	n := len(cadences)
	if n != len(speeds) || n < 2 {
		return math.NaN()
	}

	// Filter to only include records with both cadence and speed > 0
	var validCadences []float64
	var validSpeeds []float64
	for i := 0; i < n; i++ {
		if cadences[i] > 0 && speeds[i] > 0 {
			validCadences = append(validCadences, float64(cadences[i]))
			validSpeeds = append(validSpeeds, float64(speeds[i]))
		}
	}

	if len(validCadences) < 10 {
		return math.NaN()
	}

	// Calculate means
	var sumC, sumS float64
	for i := range validCadences {
		sumC += validCadences[i]
		sumS += validSpeeds[i]
	}
	meanC := sumC / float64(len(validCadences))
	meanS := sumS / float64(len(validSpeeds))

	// Calculate Pearson correlation
	var numSum, denomC, denomS float64
	for i := range validCadences {
		diffC := validCadences[i] - meanC
		diffS := validSpeeds[i] - meanS
		numSum += diffC * diffS
		denomC += diffC * diffC
		denomS += diffS * diffS
	}

	if denomC == 0 || denomS == 0 {
		return math.NaN()
	}

	return numSum / (math.Sqrt(denomC) * math.Sqrt(denomS))
}

// isRunningActivity returns true if the activity type is a running/walking activity
func isRunningActivity(activityType pbactivity.ActivityType) bool {
	switch activityType {
	case pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_WALK,
		pbactivity.ActivityType_ACTIVITY_TYPE_HIKE:
		return true
	default:
		return false
	}
}
