package hdrop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"

	pendinginput "github.com/fitglue/server/src/go/pkg/pending_input"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

type Provider struct {
	service *bootstrap.Service
}

var _ providers.SupportsNonBlocking = (*Provider)(nil)

func init() {
	providers.Register(&Provider{})
}

func (p *Provider) SetService(s *bootstrap.Service) {
	p.service = s
}

func (p *Provider) Name() string { return "hdrop" }

func (p *Provider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_HDROP
}

func (p *Provider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, userRec *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	if doNotRetry {
		logger.Debug("hdrop: skipping — do-not-retry flag set")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"hdrop_status": "skipped"},
		}, nil
	}

	if p.service == nil {
		return nil, fmt.Errorf("hdrop: service not initialized")
	}

	stableID := pendinginput.GenerateID(activity.Source.String(), activity.ExternalId, p.Name())

	pending, err := p.service.DB.GetPendingInput(ctx, userRec.UserId, stableID)
	if err == nil && pending != nil {
		if pending.Status == pbpipeline.PendingInput_STATUS_COMPLETED {
			return processHDropData(pending.InputData["hdrop_json"])
		}
		if pending.Status == pbpipeline.PendingInput_STATUS_WAITING {
			logger.Debug("hdrop: still waiting for JSON upload")
			return nil, buildWaitError(stableID, p.Name())
		}
	}

	logger.Debug("hdrop: requesting JSON upload via pending input", "activity_id", stableID)
	return nil, buildWaitError(stableID, p.Name())
}

func (p *Provider) EnrichResume(_ context.Context, _ *pbactivity.StandardizedActivity, _ *user.Record, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	return processHDropData(pendingInput.InputData["hdrop_json"])
}

// hDropJSON mirrors the structure of an hDrop export file.
type hDropJSON struct {
	Metadata struct {
		TotalSweatLoss      float64 `json:"totalSweatLoss"`
		SweatRate           float64 `json:"sweatRate"`
		TotalSodium         float64 `json:"totalSodium"`
		TotalPotassium      float64 `json:"totalPotassium"`
		SodiumConcentration string  `json:"sodiumConcentration"`
		AveragehDropScore   float64 `json:"averagehDropScore"`
		MinhDropScore       float64 `json:"minhDropScore"`
		BodyLocation        string  `json:"bodyLocation"`
		MinTemperature      float64 `json:"minTemperature"`
		MaxTemperature      float64 `json:"maxTemperature"`
	} `json:"metadata"`
	TimeseriesData []struct {
		TimeMinutes         float64 `json:"timeMinutes"`
		SweatRate           float64 `json:"sweatRate"`
		FluidLoss           float64 `json:"fluidLoss"`
		Temperature         float64 `json:"temperature"`
		SodiumConcentration float64 `json:"sodiumConcentration"`
	} `json:"timeseriesData"`
}

func processHDropData(raw string) (*providers.EnrichmentResult, error) {
	if raw == "" {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"hdrop_status": "no_data"},
		}, nil
	}

	var data hDropJSON
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("hdrop: failed to parse JSON: %w", err)
	}

	m := data.Metadata

	// Build timeseries proto points
	pts := make([]*pbactivity.HDropTimeseriesPoint, 0, len(data.TimeseriesData))
	for _, t := range data.TimeseriesData {
		pts = append(pts, &pbactivity.HDropTimeseriesPoint{
			TimeMinutes:         t.TimeMinutes,
			SweatRate:           t.SweatRate,
			FluidLossCumulative: t.FluidLoss,
			SodiumConcentration: t.SodiumConcentration,
			Temperature:         t.Temperature,
		})
	}

	summary := &pbactivity.HDropSummary{
		TotalFluidLossL:           m.TotalSweatLoss,
		SweatRateLPerHr:           m.SweatRate,
		TotalSodiumMg:             m.TotalSodium,
		TotalPotassiumMg:          m.TotalPotassium,
		SodiumConcentrationMgPerL: parseSodiumConcentration(m.SodiumConcentration),
		AvgHdropScore:             m.AveragehDropScore,
		MinHdropScore:             m.MinhDropScore,
		BodyLocation:              m.BodyLocation,
		MinTemperatureC:           m.MinTemperature,
		MaxTemperatureC:           m.MaxTemperature,
		Timeseries:                pts,
	}

	desc := buildDescription(m.TotalSweatLoss, m.SweatRate, m.TotalSodium, m.TotalPotassium, parseSodiumConcentration(m.SodiumConcentration), m.AveragehDropScore, m.MinhDropScore, m.MinTemperature, m.MaxTemperature)

	return &providers.EnrichmentResult{
		Description:   desc,
		SectionHeader: "💧 hDrop Sweat Analysis:",
		Metadata:      map[string]string{"hdrop_status": "applied"},
		Enrichments: &pbactivity.ActivityEnrichments{
			Hdrop: summary,
		},
	}, nil
}

func buildDescription(fluidL, sweatRate, sodiumMg, potassiumMg, sodiumConc, avgScore, minScore, minTemp, maxTemp float64) string {
	return fmt.Sprintf(
		"💧 hDrop Sweat Analysis:\n• Fluid loss: %.2fL (%.2f L/hr)\n• Sodium lost: %s mg (%s mg/L concentration)\n• Potassium lost: %s mg\n• hDrop score: %.0f/100 (min: %.0f)\n• Skin temp: %.1f°C → %.1f°C",
		fluidL,
		sweatRate,
		formatRounded(sodiumMg),
		formatRounded(sodiumConc),
		formatRounded(potassiumMg),
		avgScore,
		minScore,
		minTemp,
		maxTemp,
	)
}

func formatRounded(v float64) string {
	return fmt.Sprintf("%.0f", math.Round(v))
}

// parseSodiumConcentration handles both numeric and string forms in the export.
func parseSodiumConcentration(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

func buildWaitError(stableID, providerName string) *user_input.WaitForInputError {
	return &user_input.WaitForInputError{
		ActivityID:         stableID,
		RequiredFields:     []string{"hdrop_json"},
		EnricherProviderID: providerName,
		Metadata: map[string]string{
			"display.title":        "Upload hDrop Sweat Data",
			"display.summary":      "Paste your hDrop session JSON export to get hydration insights",
			"display.field_labels": `{"hdrop_json":"hDrop JSON Export"}`,
			"display.field_types":  `{"hdrop_json":"textarea"}`,
		},
	}
}
