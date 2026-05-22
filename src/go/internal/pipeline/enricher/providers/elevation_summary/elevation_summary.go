package elevation_summary

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

// Profile bar characters for elevation visualization (from low to high)
var profileBars = []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

type ElevationSummary struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewElevationSummary())
}

func NewElevationSummary() *ElevationSummary {
	return &ElevationSummary{}
}

func (p *ElevationSummary) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *ElevationSummary) Name() string {
	return "elevation-summary"
}

func (p *ElevationSummary) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_ELEVATION_SUMMARY
}

func (p *ElevationSummary) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("elevation_summary: starting", "activity_name", activity.Name)

	// Parse config
	showProfile := inputs["style"] == "profile"

	var gain float64
	var loss float64
	var maxAltitude float64
	var minAltitude float64 = -1
	var previousAltitude float64
	var hasPrevious bool
	var recordCount int
	var altitudes []float64 // For profile generation

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Altitude > 0 {
					if record.Altitude > maxAltitude {
						maxAltitude = record.Altitude
					}
					if minAltitude < 0 || record.Altitude < minAltitude {
						minAltitude = record.Altitude
					}

					if hasPrevious {
						diff := record.Altitude - previousAltitude
						if diff > 0 {
							gain += diff
						} else if diff < 0 {
							loss += math.Abs(diff)
						}
					}

					previousAltitude = record.Altitude
					hasPrevious = true
					recordCount++

					if showProfile {
						altitudes = append(altitudes, record.Altitude)
					}
				}
			}
		}
	}

	if recordCount == 0 {
		logger.Info("No elevation data found for elevation summary enricher")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "No altitude data found",
			Metadata: map[string]string{
				"elevation_summary_status": "skipped",
				"status_detail":            "No altitude data found",
			},
		}, nil
	}

	logger.Info("Elevation summary calculated",
		"gain", gain,
		"loss", loss,
		"max_altitude", maxAltitude,
		"record_count", recordCount,
	)

	// Build the summary text
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⛰️ Elevation: +%.0fm gain • -%.0fm loss • %.0fm max", gain, loss, maxAltitude))

	// Generate elevation profile if requested
	if showProfile && len(altitudes) >= 10 {
		profile := generateElevationProfile(altitudes, minAltitude, maxAltitude, 20)
		sb.WriteString(fmt.Sprintf("\n📈 %s", profile))
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"elevation_summary_status": "success",
			"elevation_gain":           fmt.Sprintf("%.2f", gain),
			"elevation_loss":           fmt.Sprintf("%.2f", loss),
			"elevation_max":            fmt.Sprintf("%.2f", maxAltitude),
			"elevation_record_count":   fmt.Sprintf("%d", recordCount),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			Elevation: &pbactivity.ElevationSummary{
				TotalGainM:   gain,
				TotalLossM:   loss,
				MaxAltitudeM: maxAltitude,
			},
		},
	}, nil
}

// generateElevationProfile creates an ASCII art profile of the elevation
func generateElevationProfile(altitudes []float64, minAlt, maxAlt float64, numBuckets int) string {
	if len(altitudes) == 0 || maxAlt <= minAlt {
		return ""
	}

	altRange := maxAlt - minAlt
	if altRange == 0 {
		altRange = 1 // Avoid division by zero for flat courses
	}

	// Sample altitudes into buckets
	buckets := make([]float64, numBuckets)
	pointsPerBucket := len(altitudes) / numBuckets
	if pointsPerBucket < 1 {
		pointsPerBucket = 1
	}

	for i := 0; i < numBuckets; i++ {
		startIdx := i * pointsPerBucket
		endIdx := startIdx + pointsPerBucket
		if endIdx > len(altitudes) {
			endIdx = len(altitudes)
		}
		if startIdx >= len(altitudes) {
			startIdx = len(altitudes) - 1
		}

		// Average altitude in this bucket
		var sum float64
		count := 0
		for j := startIdx; j < endIdx; j++ {
			sum += altitudes[j]
			count++
		}
		if count > 0 {
			buckets[i] = sum / float64(count)
		}
	}

	// Convert to bar characters
	var profile strings.Builder
	for _, alt := range buckets {
		normalized := (alt - minAlt) / altRange
		barIdx := int(normalized * float64(len(profileBars)-1))
		if barIdx < 0 {
			barIdx = 0
		}
		if barIdx >= len(profileBars) {
			barIdx = len(profileBars) - 1
		}
		profile.WriteString(profileBars[barIdx])
	}

	return profile.String()
}
