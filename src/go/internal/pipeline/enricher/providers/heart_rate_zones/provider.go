package heart_rate_zones

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// HeartRateZone represents a training zone with its boundaries and display properties
type HeartRateZone struct {
	Name   string
	MinPct float64 // Minimum percentage of max HR
	MaxPct float64 // Maximum percentage of max HR
	Emoji  string  // Colored emoji for this zone
}

// StandardZones defines the 6-zone heart rate training model (Zone 0-5)
var StandardZones = []HeartRateZone{
	{Name: "Zone 0 (Rest)", MinPct: 0.00, MaxPct: 0.50, Emoji: "🟪"},
	{Name: "Zone 1 (Recovery)", MinPct: 0.50, MaxPct: 0.60, Emoji: "🟦"},
	{Name: "Zone 2 (Endurance)", MinPct: 0.60, MaxPct: 0.70, Emoji: "🟩"},
	{Name: "Zone 3 (Tempo)", MinPct: 0.70, MaxPct: 0.80, Emoji: "🟨"},
	{Name: "Zone 4 (Threshold)", MinPct: 0.80, MaxPct: 0.90, Emoji: "🟧"},
	{Name: "Zone 5 (VO2 Max)", MinPct: 0.90, MaxPct: 1.00, Emoji: "🟥"},
}

type HeartRateZonesProvider struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewHeartRateZonesProvider())
}

func NewHeartRateZonesProvider() *HeartRateZonesProvider {
	return &HeartRateZonesProvider{}
}

func (p *HeartRateZonesProvider) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *HeartRateZonesProvider) Name() string {
	return "heart-rate-zones"
}

func (p *HeartRateZonesProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_HEART_RATE_ZONES
}

func (p *HeartRateZonesProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("heart_rate_zones: starting",
		"activity_name", activity.Name,
		"session_count", len(activity.Sessions),
	)

	// Parse config options
	maxHR := 190.0
	if v, ok := inputs["max_hr"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			maxHR = f
		}
	}

	style := "emoji" // Default style
	if v, ok := inputs["style"]; ok {
		style = v
	}

	barLength := 5
	if v, ok := inputs["bar_length"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			if n < 3 {
				barLength = 3
			} else if n > 10 {
				barLength = 10
			} else {
				barLength = n
			}
		}
	}

	// Collect time spent in each zone
	zoneDurations := make([]time.Duration, len(StandardZones))
	var lastTime *time.Time
	var totalDuration time.Duration

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate <= 0 {
					continue
				}

				currentTime := record.Timestamp.AsTime()
				if lastTime != nil {
					delta := currentTime.Sub(*lastTime)
					// Cap delta at 10 minutes to avoid spikes from recording gaps
					if delta > 10*time.Minute {
						delta = 0
					}

					if delta > 0 {
						// Determine which zone this HR falls into
						hrPct := float64(record.HeartRate) / maxHR
						zoneIdx := getZoneIndex(hrPct)
						if zoneIdx < len(zoneDurations) {
							zoneDurations[zoneIdx] += delta
							totalDuration += delta
						}
					}
				}
				lastTime = &currentTime
			}
		}
	}

	if totalDuration == 0 {
		logger.Debug("heart_rate_zones: skipping - no heart rate data found")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"hr_zones_status": "skipped",
				"status_detail":   "No heart rate data found",
			},
		}, nil
	}

	// Generate output based on style
	var sb strings.Builder
	sb.WriteString("❤️ Heart Rate Zones:\n")

	for i, zone := range StandardZones {
		duration := zoneDurations[i]
		minutes := int(duration.Minutes())

		sb.WriteString(formatZoneRow(zone, duration, minutes, totalDuration, barLength, style))
	}

	logger.Info("Heart rate zones calculated",
		"total_duration", totalDuration,
		"max_hr", maxHR,
	)

	totalMinutes := int32(totalDuration.Minutes())
	zoneBuckets := make([]*pbactivity.HeartRateZoneBucket, len(StandardZones))
	for i, zone := range StandardZones {
		mins := int32(zoneDurations[i].Minutes())
		pct := 0.0
		if totalMinutes > 0 {
			pct = float64(mins) / float64(totalMinutes)
		}
		zoneBuckets[i] = &pbactivity.HeartRateZoneBucket{
			ZoneIndex:  int32(i),
			Name:       zone.Name,
			Minutes:    mins,
			Percentage: pct,
		}
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"hr_zones_status": "success",
			"max_hr":          fmt.Sprintf("%.0f", maxHR),
			"total_duration":  fmt.Sprintf("%.0f", totalDuration.Minutes()),
			"zone0_minutes":   fmt.Sprintf("%d", int(zoneDurations[0].Minutes())),
			"zone1_minutes":   fmt.Sprintf("%d", int(zoneDurations[1].Minutes())),
			"zone2_minutes":   fmt.Sprintf("%d", int(zoneDurations[2].Minutes())),
			"zone3_minutes":   fmt.Sprintf("%d", int(zoneDurations[3].Minutes())),
			"zone4_minutes":   fmt.Sprintf("%d", int(zoneDurations[4].Minutes())),
			"zone5_minutes":   fmt.Sprintf("%d", int(zoneDurations[5].Minutes())),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			HeartRateZones: &pbactivity.HeartRateZonesSummary{
				Zones:        zoneBuckets,
				TotalMinutes: totalMinutes,
			},
		},
	}, nil
}

// getZoneIndex returns the zone index (0-5) for a given HR percentage
func getZoneIndex(hrPct float64) int {
	for i, zone := range StandardZones {
		if hrPct >= zone.MinPct && hrPct < zone.MaxPct {
			return i
		}
	}
	// Handle edge case: exactly at max HR (100%)
	if hrPct >= 1.0 {
		return 5 // Zone 5
	}
	// Should not reach here since Zone 0 covers 0-50%
	return 0
}

// formatZoneRow formats a single zone row based on style
func formatZoneRow(zone HeartRateZone, duration time.Duration, minutes int, totalDuration time.Duration, barLength int, style string) string {
	switch style {
	case "percentage":
		pct := 0.0
		if totalDuration > 0 {
			pct = float64(duration) / float64(totalDuration) * 100
		}
		return fmt.Sprintf("%s: %.0f%% (%d min)\n", zone.Name, pct, minutes)

	case "text":
		level := "None"
		if minutes > 0 {
			pct := float64(duration) / float64(totalDuration) * 100
			if pct >= 40 {
				level = "Primary"
			} else if pct >= 20 {
				level = "High"
			} else if pct >= 10 {
				level = "Moderate"
			} else {
				level = "Low"
			}
		}
		return fmt.Sprintf("%s: %s (%d min)\n", zone.Name, level, minutes)

	default: // emoji bars
		rating := 0
		if totalDuration > 0 {
			rating = int((float64(duration) / float64(totalDuration)) * float64(barLength) * 2)
		}
		if rating > barLength {
			rating = barLength
		}
		if rating == 0 && minutes > 0 {
			rating = 1
		}

		bar := ""
		for i := 0; i < barLength; i++ {
			if i < rating {
				bar += zone.Emoji
			} else {
				bar += "⬜"
			}
		}
		return fmt.Sprintf("%s: %s %d min\n", zone.Name, bar, minutes)
	}
}
