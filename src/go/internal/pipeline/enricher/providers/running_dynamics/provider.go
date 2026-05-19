package running_dynamics

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

type RunningDynamics struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewRunningDynamics())
}

func NewRunningDynamics() *RunningDynamics {
	return &RunningDynamics{}
}

func (p *RunningDynamics) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *RunningDynamics) Name() string {
	return "running-dynamics"
}

func (p *RunningDynamics) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_RUNNING_DYNAMICS
}

func (p *RunningDynamics) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("running_dynamics: starting",
		"activity_name", activity.Name,
	)

	var gcts []int32
	var vos []int32
	var vrs []int32
	var sls []float64

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.GroundContactTime != nil && *record.GroundContactTime > 0 {
					gcts = append(gcts, *record.GroundContactTime)
				}
				if record.VerticalOscillation != nil && *record.VerticalOscillation > 0 {
					vos = append(vos, *record.VerticalOscillation)
				}
				if record.VerticalRatio != nil && *record.VerticalRatio > 0 {
					vrs = append(vrs, *record.VerticalRatio)
				}
				if record.StepLength != nil && *record.StepLength > 0 {
					sls = append(sls, *record.StepLength)
				}
			}
		}
	}

	if len(gcts) == 0 && len(vos) == 0 && len(sls) == 0 {
		logger.Debug("running_dynamics: skipping - no running dynamics data found")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "No running dynamics data found",
			Metadata: map[string]string{
				"running_dynamics_status": "skipped",
			},
		}, nil
	}

	// Calculate averages
	var summaryParts []string

	if len(gcts) > 0 {
		var sum int64
		for _, v := range gcts {
			sum += int64(v)
		}
		avg := float64(sum) / float64(len(gcts))
		summaryParts = append(summaryParts, fmt.Sprintf("⏱️ GCT: %.0f ms", avg))
	}

	if len(sls) > 0 {
		var sum float64
		for _, v := range sls {
			sum += v
		}
		avg := sum / float64(len(sls))
		summaryParts = append(summaryParts, fmt.Sprintf("📏 Stride: %.2f m", avg))
	}

	if len(vos) > 0 {
		var sum int64
		for _, v := range vos {
			sum += int64(v)
		}
		avg := float64(sum) / float64(len(vos))
		summaryParts = append(summaryParts, fmt.Sprintf("↕️ Vert: %.1f cm", avg/10.0)) // mm to cm
	}

	if len(summaryParts) == 0 {
		return &providers.EnrichmentResult{Skipped: true, SkipReason: "No summary parts generated"}, nil
	}

	// Build summary line
	summaryText := "🏃 Running Dynamics: "
	for i, part := range summaryParts {
		if i > 0 {
			summaryText += " • "
		}
		summaryText += part
	}

	return &providers.EnrichmentResult{
		Description: summaryText,
		Metadata: map[string]string{
			"running_dynamics_status": "success",
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			RunningDynamics: &pbactivity.RunningDynamicsSummary{
				AvgGroundContactMs:       avgGCT(gcts),
				AvgVerticalOscillationCm: avgVO(vos),
				AvgStepLengthM:           avgSL(sls),
			},
		},
	}, nil
}

func avgGCT(v []int32) float64 {
	if len(v) == 0 {
		return 0
	}
	var s int64
	for _, x := range v {
		s += int64(x)
	}
	return float64(s) / float64(len(v))
}

func avgVO(v []int32) float64 {
	if len(v) == 0 {
		return 0
	}
	var s int64
	for _, x := range v {
		s += int64(x)
	}
	return float64(s) / float64(len(v)) / 10.0 // mm → cm
}

func avgSL(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
