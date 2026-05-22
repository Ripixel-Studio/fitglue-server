package power_summary

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// PowerSummary calculates and appends power statistics (watts) to the activity description.
type PowerSummary struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewPowerSummary())
}

func NewPowerSummary() *PowerSummary {
	return &PowerSummary{}
}

func (p *PowerSummary) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *PowerSummary) Name() string {
	return "power-summary"
}

func (p *PowerSummary) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_POWER_SUMMARY
}

func (p *PowerSummary) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("power_summary: starting", "activity_name", activity.Name)

	showCurve := inputs["show_curve"] == "true"

	var powers []int32
	var totalDurationSeconds float64

	for _, session := range activity.Sessions {
		totalDurationSeconds += session.TotalElapsedTime
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Power > 0 {
					powers = append(powers, record.Power)
				}
			}
		}
	}

	if len(powers) == 0 {
		logger.Info("No power data found for power summary enricher")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "No power data found",
			Metadata: map[string]string{
				"power_summary_status": "skipped",
				"status_detail":        "No power data found",
			},
		}, nil
	}

	var sumPower int64
	maxPower := powers[0]
	for _, pw := range powers {
		sumPower += int64(pw)
		if pw > maxPower {
			maxPower = pw
		}
	}

	avgPower := float64(sumPower) / float64(len(powers))
	np := calculateNormalizedPower(powers)
	kj := int32(avgPower * totalDurationSeconds / 1000)

	logger.Info("Power summary calculated",
		"avg_power", avgPower,
		"max_power", maxPower,
		"normalized_power", np,
		"kilojoules", kj,
		"sample_count", len(powers),
	)

	var sb strings.Builder

	if showCurve && len(powers) >= 5 {
		sb.WriteString("⚡ Power:\n")
		sb.WriteString(fmt.Sprintf("• %.0fW avg\n", avgPower))
		sb.WriteString(fmt.Sprintf("• %dW max\n", maxPower))

		peak5s := calculatePeakPower(powers, 5)
		peak1m := calculatePeakPower(powers, 60)
		peak5m := calculatePeakPower(powers, 300)
		peak20m := calculatePeakPower(powers, 1200)

		if peak5s > 0 {
			sb.WriteString(fmt.Sprintf("• Peak 5s: %dW\n", peak5s))
		}
		if peak1m > 0 {
			sb.WriteString(fmt.Sprintf("• Peak 1m: %dW\n", peak1m))
		}
		if peak5m > 0 {
			sb.WriteString(fmt.Sprintf("• Peak 5m: %dW\n", peak5m))
		}
		if peak20m > 0 {
			ftp := float64(peak20m) * 0.95
			sb.WriteString(fmt.Sprintf("• Peak 20m: %dW\n", peak20m))
			sb.WriteString(fmt.Sprintf("• Est. FTP: %.0fW", ftp))
		}
	} else {
		sb.WriteString(fmt.Sprintf("⚡ Power: %.0fW avg • %dW max", avgPower, maxPower))
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"power_summary_status": "success",
			"power_avg":            fmt.Sprintf("%.0f", avgPower),
			"power_max":            fmt.Sprintf("%d", maxPower),
			"power_np":             fmt.Sprintf("%d", np),
			"power_kj":             fmt.Sprintf("%d", kj),
			"power_sample_count":   fmt.Sprintf("%d", len(powers)),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			Power: &pbactivity.PowerSummary{
				AvgWatts:        int32(avgPower),
				MaxWatts:        maxPower,
				NormalizedPower: np,
				Kilojoules:      kj,
			},
		},
	}, nil
}

// calculateNormalizedPower uses the 30-second rolling average raised to the 4th power method.
func calculateNormalizedPower(powers []int32) int32 {
	const window = 30
	if len(powers) < window {
		return 0
	}

	var windowSum int64
	for i := 0; i < window; i++ {
		windowSum += int64(powers[i])
	}

	var sumFourthPower float64
	count := 0

	avg := float64(windowSum) / float64(window)
	sumFourthPower += math.Pow(avg, 4)
	count++

	for i := window; i < len(powers); i++ {
		windowSum += int64(powers[i]) - int64(powers[i-window])
		avg = float64(windowSum) / float64(window)
		sumFourthPower += math.Pow(avg, 4)
		count++
	}

	if count == 0 {
		return 0
	}
	return int32(math.Round(math.Pow(sumFourthPower/float64(count), 0.25)))
}

// calculatePeakPower finds the highest average power over a rolling window of durationSec seconds.
func calculatePeakPower(powers []int32, durationSec int) int32 {
	if len(powers) < durationSec {
		return 0
	}

	var windowSum int64
	for i := 0; i < durationSec; i++ {
		windowSum += int64(powers[i])
	}
	maxAvg := windowSum

	for i := durationSec; i < len(powers); i++ {
		windowSum += int64(powers[i]) - int64(powers[i-durationSec])
		if windowSum > maxAvg {
			maxAvg = windowSum
		}
	}

	return int32(maxAvg / int64(durationSec))
}
