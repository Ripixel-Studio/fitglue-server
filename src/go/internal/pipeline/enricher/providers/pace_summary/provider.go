package pace_summary

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PaceSummary calculates and appends pace statistics (min/km) to the activity description.
// Uses speed (m/s) data from records, converts to pace, and shows avg/best pace.
// Enhanced features: splits, negative split detection, fatigue analysis.
type PaceSummary struct {
	Service *bootstrap.Service
}

// Split represents a single km/mile split
type Split struct {
	Distance  float64                // in meters
	Duration  time.Duration          // time for this split
	Pace      float64                // min/km
	StartTime *timestamppb.Timestamp // original lap start time for time markers
}

func init() {
	providers.Register(NewPaceSummary())
}

func NewPaceSummary() *PaceSummary {
	return &PaceSummary{}
}

func (p *PaceSummary) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *PaceSummary) Name() string {
	return "pace-summary"
}

func (p *PaceSummary) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PACE_SUMMARY
}

func (p *PaceSummary) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("pace_summary: starting", "activity_name", activity.Name)

	// Parse config options
	showSplits := inputs["show_splits"] == "true"
	showNegativeSplit := inputs["negative_split_alert"] == "true"
	showFatigue := inputs["show_fatigue"] == "true"

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
		logger.Info("No speed data found for pace summary enricher")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"pace_summary_status": "skipped",
				"status_detail":       "No speed data found",
			},
		}, nil
	}

	// Calculate avg and best (fastest) pace
	var sumSpeed float64
	var maxSpeed float64 = speeds[0]

	for _, speed := range speeds {
		sumSpeed += speed
		if speed > maxSpeed {
			maxSpeed = speed
		}
	}

	avgSpeed := sumSpeed / float64(len(speeds))
	avgPace := 1000.0 / avgSpeed / 60.0 // minutes per km
	bestPace := 1000.0 / maxSpeed / 60.0

	logger.Info("Pace summary calculated",
		"avg_pace_min_km", avgPace,
		"best_pace_min_km", bestPace,
		"sample_count", len(speeds),
	)

	avgPaceStr := formatPace(avgPace)
	bestPaceStr := formatPace(bestPace)

	// Build the summary text
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚡ Pace: %s/km avg • %s/km best", avgPaceStr, bestPaceStr))

	// Calculate splits if requested
	var splits []Split
	if showSplits || showNegativeSplit || showFatigue {
		splits = calculateSplitsFromLaps(activity)
	}

	// Show splits
	if showSplits && len(splits) > 0 {
		sb.WriteString("\n📊 Splits:")
		fastestIdx, slowestIdx := 0, 0
		for i, split := range splits {
			if split.Pace < splits[fastestIdx].Pace {
				fastestIdx = i
			}
			if split.Pace > splits[slowestIdx].Pace {
				slowestIdx = i
			}
		}
		for i, split := range splits {
			marker := ""
			if i == fastestIdx {
				marker = " 🏆"
			} else if i == slowestIdx {
				marker = " 🐢"
			}
			sb.WriteString(fmt.Sprintf("\n• Km %d: %s%s", i+1, formatPace(split.Pace), marker))
		}
	}

	// Negative split detection
	if showNegativeSplit && len(splits) >= 2 {
		midpoint := len(splits) / 2
		var firstHalfPace, secondHalfPace float64
		for i := 0; i < midpoint; i++ {
			firstHalfPace += splits[i].Pace
		}
		for i := midpoint; i < len(splits); i++ {
			secondHalfPace += splits[i].Pace
		}
		firstHalfPace /= float64(midpoint)
		secondHalfPace /= float64(len(splits) - midpoint)

		if secondHalfPace < firstHalfPace {
			diff := firstHalfPace - secondHalfPace
			diffSeconds := int(diff * 60)
			sb.WriteString(fmt.Sprintf("\n🔥 Negative Split! Second half %ds/km faster", diffSeconds))
		}
	}

	// Fatigue analysis
	if showFatigue && len(splits) >= 4 {
		quarter := len(splits) / 4
		var firstQuarterPace, lastQuarterPace float64
		for i := 0; i < quarter; i++ {
			firstQuarterPace += splits[i].Pace
		}
		for i := len(splits) - quarter; i < len(splits); i++ {
			lastQuarterPace += splits[i].Pace
		}
		firstQuarterPace /= float64(quarter)
		lastQuarterPace /= float64(quarter)

		if lastQuarterPace > firstQuarterPace {
			fatiguePercent := ((lastQuarterPace - firstQuarterPace) / firstQuarterPace) * 100
			if fatiguePercent > 1 { // Only show if significant
				sb.WriteString(fmt.Sprintf("\n😓 Fatigue: %.0f%% pace drop in final quarter", fatiguePercent))
			}
		} else {
			// Strong finish!
			sb.WriteString("\n💪 Strong Finish: Final quarter faster than start")
		}
	}

	metadata := map[string]string{
		"pace_summary_status": "success",
		"pace_avg":            avgPaceStr,
		"pace_best":           bestPaceStr,
		"pace_sample_count":   fmt.Sprintf("%d", len(speeds)),
	}

	// Add split data to metadata
	if len(splits) > 0 {
		metadata["splits_count"] = fmt.Sprintf("%d", len(splits))
	}

	// Generate time markers for split boundaries
	var timeMarkers []*pbactivity.TimeMarker
	if showSplits && len(splits) > 0 {
		timeMarkers = generateSplitTimeMarkers(splits)
		metadata["time_markers"] = fmt.Sprintf("%d", len(timeMarkers))
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		TimeMarkers: timeMarkers,
		Metadata:    metadata,
	}, nil
}

// lapElapsedSeconds returns the elapsed time for a lap in seconds.
// For structured workouts (e.g. Garmin), TotalElapsedTime may be 0.
// In that case we fall back to computing the duration from timestamps:
// either (nextLapStartTime - thisLapStartTime) when a successor lap exists,
// or (sessionEndTime - thisLapStartTime) for the final lap.
func lapElapsedSeconds(lap *pbactivity.Lap, nextLapStartTime *timestamppb.Timestamp, sessionEndTime *timestamppb.Timestamp) float64 {
	if lap.TotalElapsedTime > 0 {
		return lap.TotalElapsedTime
	}
	if lap.StartTime == nil {
		return 0
	}
	if nextLapStartTime != nil {
		return nextLapStartTime.AsTime().Sub(lap.StartTime.AsTime()).Seconds()
	}
	if sessionEndTime != nil {
		return sessionEndTime.AsTime().Sub(lap.StartTime.AsTime()).Seconds()
	}
	return 0
}

// calculateSplitsFromLaps attempts to derive km splits from lap data
func calculateSplitsFromLaps(activity *pbactivity.StandardizedActivity) []Split {
	var splits []Split

	for _, session := range activity.Sessions {
		laps := session.Laps

		// Compute a session end time for the final lap's fallback
		var sessionEndTime *timestamppb.Timestamp
		if session.StartTime != nil && session.TotalElapsedTime > 0 {
			end := session.StartTime.AsTime().Add(time.Duration(session.TotalElapsedTime * float64(time.Second)))
			sessionEndTime = timestamppb.New(end)
		}

		for i, lap := range laps {
			var nextStart *timestamppb.Timestamp
			if i+1 < len(laps) && laps[i+1].StartTime != nil {
				nextStart = laps[i+1].StartTime
			}

			elapsedSec := lapElapsedSeconds(lap, nextStart, sessionEndTime)

			// Each lap with distance >= 900m is roughly a km split
			if lap.TotalDistance >= 900 && lap.TotalDistance <= 1100 {
				if elapsedSec <= 0 {
					continue
				}
				duration := time.Duration(elapsedSec * float64(time.Second))
				pace := (elapsedSec / lap.TotalDistance) * 1000 / 60 // min/km
				splits = append(splits, Split{
					Distance:  lap.TotalDistance,
					Duration:  duration,
					Pace:      pace,
					StartTime: lap.StartTime,
				})
			} else if lap.TotalDistance > 1100 {
				// Longer lap - estimate number of km splits within
				numKm := int(math.Floor(lap.TotalDistance / 1000))
				if numKm > 0 && elapsedSec > 0 {
					avgPace := (elapsedSec / lap.TotalDistance) * 1000 / 60
					lapDuration := elapsedSec / float64(numKm)
					for k := 0; k < numKm; k++ {
						var splitStart *timestamppb.Timestamp
						if lap.StartTime != nil {
							offset := time.Duration(float64(k) * lapDuration * float64(time.Second))
							splitStart = timestamppb.New(lap.StartTime.AsTime().Add(offset))
						}
						splits = append(splits, Split{
							Distance:  1000,
							Duration:  time.Duration(lapDuration) * time.Second,
							Pace:      avgPace,
							StartTime: splitStart,
						})
					}
				}
			}
		}
	}

	return splits
}

// generateSplitTimeMarkers creates TimeMarker entries for each km split boundary.
func generateSplitTimeMarkers(splits []Split) []*pbactivity.TimeMarker {
	var markers []*pbactivity.TimeMarker
	for i, split := range splits {
		if split.StartTime == nil {
			continue
		}
		markers = append(markers, &pbactivity.TimeMarker{
			Timestamp:  split.StartTime,
			Label:      fmt.Sprintf("Km %d", i+1),
			MarkerType: "split",
		})
	}
	return markers
}

// formatPace converts pace in minutes (float) to MM:SS format
func formatPace(paceMinutes float64) string {
	minutes := int(paceMinutes)
	seconds := int((paceMinutes - float64(minutes)) * 60)
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
