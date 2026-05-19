package training_load

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

type TrainingLoad struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewTrainingLoad())
}

func NewTrainingLoad() *TrainingLoad {
	return &TrainingLoad{}
}

func (p *TrainingLoad) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *TrainingLoad) Name() string {
	return "training-load"
}

func (p *TrainingLoad) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_TRAINING_LOAD
}

func (p *TrainingLoad) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("training_load: starting", "activity_name", activity.Name)
	// Config defaults
	maxHR := 190.0
	restHR := 60.0
	gender := "male"

	if v, ok := inputs["max_hr"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			maxHR = f
		}
	}
	if v, ok := inputs["rest_hr"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			restHR = f
		}
	}
	if v, ok := inputs["gender"]; ok {
		gender = v
	}

	b := 1.92
	if gender == "female" {
		b = 1.67
	}

	hrRange := maxHR - restHR
	if hrRange <= 0 {
		logger.Warn("Invalid HR range (max_hr <= rest_hr)", "max_hr", maxHR, "rest_hr", restHR)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"training_load_status": "skipped",
				"status_detail":        "Invalid HR range",
			},
		}, nil
	}

	var totalTRIMP float64
	var lastTime *time.Time

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate <= 0 {
					continue
				}

				currentTime := record.Timestamp.AsTime()
				if lastTime == nil {
					lastTime = &currentTime
					continue
				}

				deltaMinutes := currentTime.Sub(*lastTime).Minutes()
				// Limit delta to 10 minutes to avoid spikes from gaps in recording
				if deltaMinutes > 10 {
					deltaMinutes = 0
				}

				if deltaMinutes > 0 {
					hrr := (float64(record.HeartRate) - restHR) / hrRange
					if hrr < 0 {
						hrr = 0
					}
					if hrr > 1 {
						hrr = 1
					}

					contribution := deltaMinutes * hrr * 0.64 * math.Exp(b*hrr)
					totalTRIMP += contribution
				}

				lastTime = &currentTime
			}
		}
	}

	if totalTRIMP == 0 {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"training_load_status": "skipped",
				"status_detail":        "No TRIMP calculated (insufficient HR data)",
			},
		}, nil
	}

	zone := getTrainingLoadZone(totalTRIMP)
	summaryText := fmt.Sprintf("💪 Training Load: %.0f (%s)", totalTRIMP, zone)

	logger.Info("Training Load calculated", "trimp", totalTRIMP, "zone", zone)

	return &providers.EnrichmentResult{
		Description: summaryText,
		Metadata: map[string]string{
			"training_load_status": "success",
			"trimp":                fmt.Sprintf("%.0f", totalTRIMP),
			"trimp_zone":           zone,
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			TrainingLoad: &pbactivity.TrainingLoadSummary{
				Trimp:  int32(totalTRIMP),
				Bucket: zone,
			},
		},
	}, nil
}

func getTrainingLoadZone(trimp float64) string {
	switch {
	case trimp < 30:
		return "Recovery"
	case trimp < 60:
		return "Easy"
	case trimp < 90:
		return "Moderate"
	case trimp < 150:
		return "Hard"
	default:
		return "Very Hard"
	}
}
