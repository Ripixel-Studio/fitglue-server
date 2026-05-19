package heart_rate_summary

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

type HeartRateSummary struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewHeartRateSummary())
}

func NewHeartRateSummary() *HeartRateSummary {
	return &HeartRateSummary{}
}

func (p *HeartRateSummary) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *HeartRateSummary) Name() string {
	return "heart-rate-summary"
}

func (p *HeartRateSummary) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_HEART_RATE_SUMMARY
}

func (p *HeartRateSummary) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("heart_rate_summary: starting",
		"activity_name", activity.Name,
		"session_count", len(activity.Sessions),
	)

	// Parse config
	showDrift := inputs["show_drift"] == "true"

	// Collect all heart rate values from the activity
	var heartRates []int32

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate > 0 {
					heartRates = append(heartRates, record.HeartRate)
				}
			}
		}
	}

	if len(heartRates) == 0 {
		logger.Debug("heart_rate_summary: skipping - no heart rate data found")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"hr_summary_status": "skipped",
				"status_detail":     "No heart rate data found",
			},
		}, nil
	}

	// Calculate min, avg, max
	minHR, maxHR := heartRates[0], heartRates[0]
	var sumHR int64 = 0

	for _, hr := range heartRates {
		if hr < minHR {
			minHR = hr
		}
		if hr > maxHR {
			maxHR = hr
		}
		sumHR += int64(hr)
	}

	avgHR := float64(sumHR) / float64(len(heartRates))
	avgHRInt := int32(avgHR)

	// Write back to the first session so downstream enrichers and uploaders see typed HR values.
	if len(activity.Sessions) > 0 {
		s := activity.Sessions[0]
		if s.AvgHeartRate == nil {
			s.AvgHeartRate = &avgHRInt
		}
		if s.MaxHeartRate == nil {
			s.MaxHeartRate = &maxHR
		}
		if s.MinHeartRate == nil {
			s.MinHeartRate = &minHR
		}
	}

	logger.Info("Heart rate summary calculated",
		"min_hr", minHR,
		"avg_hr", avgHR,
		"max_hr", maxHR,
		"sample_count", len(heartRates),
	)

	// Build output based on config
	var sb strings.Builder

	// Calculate drift early so we can include in multi-line format if applicable
	var drift float64
	var hasDrift bool
	if showDrift && len(heartRates) >= 20 {
		// Compare first 20% to last 20%
		sampleSize := len(heartRates) / 5
		if sampleSize < 5 {
			sampleSize = 5
		}

		var firstSum, lastSum int64
		for i := 0; i < sampleSize && i < len(heartRates); i++ {
			firstSum += int64(heartRates[i])
		}
		for i := len(heartRates) - sampleSize; i < len(heartRates); i++ {
			lastSum += int64(heartRates[i])
		}

		firstAvg := float64(firstSum) / float64(sampleSize)
		lastAvg := float64(lastSum) / float64(sampleSize)
		drift = lastAvg - firstAvg
		hasDrift = drift > 5 || drift < -5
	}

	if showDrift {
		// Multi-line bullet format
		sb.WriteString("❤️ Heart Rate:\n")
		sb.WriteString(fmt.Sprintf("• %d bpm min\n", minHR))
		sb.WriteString(fmt.Sprintf("• %.0f bpm avg\n", avgHR))
		sb.WriteString(fmt.Sprintf("• %d bpm max\n", maxHR))

		if hasDrift {
			if drift > 5 {
				sb.WriteString(fmt.Sprintf("• Drift: +%.0f bpm (check hydration)", drift))
			} else if drift < -5 {
				sb.WriteString(fmt.Sprintf("• Drift: %.0f bpm (good warm-up)", drift))
			}
		}
	} else {
		// Simple single-line format
		sb.WriteString(fmt.Sprintf("❤️ Heart Rate: %d bpm min • %.0f bpm avg • %d bpm max", minHR, avgHR, maxHR))
	}

	var driftBpm int32
	driftWarning := false
	if showDrift && hasDrift {
		driftBpm = int32(drift)
		driftWarning = drift > 5
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"hr_summary_status": "success",
			"hr_min":            fmt.Sprintf("%d", minHR),
			"hr_avg":            fmt.Sprintf("%.0f", avgHR),
			"hr_max":            fmt.Sprintf("%d", maxHR),
			"hr_sample_count":   fmt.Sprintf("%d", len(heartRates)),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			HeartRate: &pbactivity.HeartRateSummary{
				MinBpm:       minHR,
				AvgBpm:       int32(avgHR),
				MaxBpm:       maxHR,
				DriftBpm:     driftBpm,
				DriftWarning: driftWarning,
			},
		},
	}, nil
}
