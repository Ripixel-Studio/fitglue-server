package temperature_summary

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	"github.com/fitglue/server/src/go/pkg/domain/user"
)

type TemperatureSummary struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewTemperatureSummary())
}

func NewTemperatureSummary() *TemperatureSummary {
	return &TemperatureSummary{}
}

func (p *TemperatureSummary) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *TemperatureSummary) Name() string {
	return "temperature-summary"
}

func (p *TemperatureSummary) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_TEMPERATURE_SUMMARY
}

func (p *TemperatureSummary) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("temperature_summary: starting",
		"activity_name", activity.Name,
		"session_count", len(activity.Sessions),
	)

	var temps []int32

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Temperature != nil {
					temps = append(temps, record.GetTemperature())
				}
			}
		}
	}

	if len(temps) == 0 {
		logger.Debug("temperature_summary: skipping - no temperature data found")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"temp_summary_status": "skipped",
				"status_detail":       "No temperature data found",
			},
		}, nil
	}

	minT, maxT := temps[0], temps[0]
	var sumT int64

	for _, t := range temps {
		if t < minT {
			minT = t
		}
		if t > maxT {
			maxT = t
		}
		sumT += int64(t)
	}

	avgT := float64(sumT) / float64(len(temps))
	avgTInt := int32(avgT)

	logger.Info("Temperature summary calculated",
		"min_c", minT,
		"avg_c", avgT,
		"max_c", maxT,
		"sample_count", len(temps),
	)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌡️ Temperature: %d°C min • %.0f°C avg • %d°C max", minT, avgT, maxT))

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"temp_summary_status": "success",
			"temp_min":            fmt.Sprintf("%d", minT),
			"temp_avg":            fmt.Sprintf("%.0f", avgT),
			"temp_max":            fmt.Sprintf("%d", maxT),
			"temp_sample_count":   fmt.Sprintf("%d", len(temps)),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			Temperature: &pbactivity.TemperatureSummary{
				MinC: minT,
				AvgC: avgTInt,
				MaxC: maxT,
			},
		},
	}, nil
}
