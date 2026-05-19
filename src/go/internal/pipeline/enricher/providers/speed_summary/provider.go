package speed_summary

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

// SpeedSummary calculates and appends speed statistics (km/h) to the activity description.
// Shows avg/max speed from GPS or sensor data.
type SpeedSummary struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewSpeedSummary())
}

func NewSpeedSummary() *SpeedSummary {
	return &SpeedSummary{}
}

func (p *SpeedSummary) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *SpeedSummary) Name() string {
	return "speed-summary"
}

func (p *SpeedSummary) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_SPEED_SUMMARY
}

func (p *SpeedSummary) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("speed_summary: starting", "activity_name", activity.Name)

	// Parse config
	showAnalysis := inputs["show_analysis"] == "true"

	// Collect all speed values from the activity (m/s)
	var speeds []float64

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Speed > 0 {
					speeds = append(speeds, record.Speed)
				}
			}
		}
	}

	if len(speeds) == 0 {
		logger.Info("No speed data found for speed summary enricher")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"speed_summary_status": "skipped",
				"status_detail":        "No speed data found",
			},
		}, nil
	}

	// Calculate avg and max speed
	var sumSpeed float64
	var maxSpeed float64 = speeds[0]

	for _, speed := range speeds {
		sumSpeed += speed
		if speed > maxSpeed {
			maxSpeed = speed
		}
	}

	avgSpeed := sumSpeed / float64(len(speeds))

	// Convert m/s to km/h (* 3.6)
	avgSpeedKmh := avgSpeed * 3.6
	maxSpeedKmh := maxSpeed * 3.6

	logger.Info("Speed summary calculated",
		"avg_speed_kmh", avgSpeedKmh,
		"max_speed_kmh", maxSpeedKmh,
		"sample_count", len(speeds),
	)

	// Build output based on config
	var sb strings.Builder

	if showAnalysis && len(speeds) >= 10 {
		// Multi-line bullet format with consistency analysis
		sb.WriteString("🚀 Speed:\n")
		sb.WriteString(fmt.Sprintf("• %.1f km/h avg\n", avgSpeedKmh))
		sb.WriteString(fmt.Sprintf("• %.1f km/h max\n", maxSpeedKmh))

		// Calculate coefficient of variation (CV) as consistency metric
		consistency := calculateSpeedConsistency(speeds, avgSpeed)
		var consistencyLabel string
		if consistency >= 85 {
			consistencyLabel = "very consistent"
		} else if consistency >= 70 {
			consistencyLabel = "consistent"
		} else if consistency >= 50 {
			consistencyLabel = "moderate variance"
		} else {
			consistencyLabel = "high variance"
		}
		sb.WriteString(fmt.Sprintf("• Consistency: %.0f%% (%s)", consistency, consistencyLabel))
	} else {
		// Simple single-line format
		sb.WriteString(fmt.Sprintf("🚀 Speed: %.1f km/h avg • %.1f km/h max", avgSpeedKmh, maxSpeedKmh))
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"speed_summary_status": "success",
			"speed_avg_kmh":        fmt.Sprintf("%.1f", avgSpeedKmh),
			"speed_max_kmh":        fmt.Sprintf("%.1f", maxSpeedKmh),
			"speed_sample_count":   fmt.Sprintf("%d", len(speeds)),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			Speed: &pbactivity.SpeedSummary{
				AvgSpeedKmh: avgSpeedKmh,
				MaxSpeedKmh: maxSpeedKmh,
			},
		},
	}, nil
}

// calculateSpeedConsistency returns a 0-100 score based on coefficient of variation
// Lower CV = higher consistency
func calculateSpeedConsistency(speeds []float64, mean float64) float64 {
	if len(speeds) < 2 || mean == 0 {
		return 0
	}

	// Calculate standard deviation
	var sumSquaredDiff float64
	for _, speed := range speeds {
		diff := speed - mean
		sumSquaredDiff += diff * diff
	}
	stdDev := math.Sqrt(sumSquaredDiff / float64(len(speeds)))

	// Coefficient of variation (as percentage)
	cv := (stdDev / mean) * 100

	// Convert to consistency score (inverse, capped at 100)
	// CV of 0 = 100% consistency, CV of 50+ = 0% consistency
	consistency := 100 - (cv * 2)
	if consistency < 0 {
		consistency = 0
	}
	if consistency > 100 {
		consistency = 100
	}

	return consistency
}
