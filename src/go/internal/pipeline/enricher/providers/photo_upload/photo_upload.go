package photo_upload

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

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

func init() {
	providers.Register(&Provider{})
}

func (p *Provider) SetService(s *bootstrap.Service) {
	p.service = s
}

func (p *Provider) Name() string { return "photo-upload" }

func (p *Provider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PHOTO_UPLOAD
}

// Enrich pauses the pipeline to request activity photo uploads from the user.
// If doNotRetry is set (user dismissed), the enricher skips gracefully.
func (p *Provider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, userRec *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	if doNotRetry {
		logger.Debug("photo-upload: skipping — do-not-retry flag set")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"photo_upload_status": "skipped"},
		}, nil
	}

	if p.service == nil {
		return nil, fmt.Errorf("service not initialized")
	}

	stableID := pendinginput.GenerateID(activity.Source.String(), activity.ExternalId, p.Name())

	pending, err := p.service.DB.GetPendingInput(ctx, userRec.UserId, stableID)
	if err == nil && pending != nil {
		if pending.Status == pbpipeline.PendingInput_STATUS_COMPLETED {
			return p.applyCompletedInput(pending)
		}
		if pending.Status == pbpipeline.PendingInput_STATUS_WAITING {
			logger.Debug("photo-upload: still waiting for photos")
			return nil, buildWaitError(stableID, p.Name())
		}
	}

	logger.Debug("photo-upload: requesting photo upload via pending input", "activity_id", stableID)
	return nil, buildWaitError(stableID, p.Name())
}

// EnrichResume is called when the user has submitted photo URLs via the pending input.
func (p *Provider) EnrichResume(_ context.Context, _ *pbactivity.StandardizedActivity, _ *user.Record, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	return p.applyCompletedInput(pendingInput)
}

func (p *Provider) applyCompletedInput(pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	rawPhotos := pendingInput.InputData["photos"]
	if rawPhotos == "" {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"photo_upload_status": "no_photos"},
		}, nil
	}

	// Parse the JSON array of GCS public URLs submitted by the client.
	var photoURLs []string
	if err := json.Unmarshal([]byte(rawPhotos), &photoURLs); err != nil {
		return nil, fmt.Errorf("photo-upload: failed to parse photo URLs: %w", err)
	}

	if len(photoURLs) == 0 {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"photo_upload_status": "no_photos"},
		}, nil
	}

	return &providers.EnrichmentResult{
		Metadata: map[string]string{
			"photo_urls":          strings.Join(photoURLs, ","),
			"photo_upload_status": "applied",
			"photo_count":         fmt.Sprintf("%d", len(photoURLs)),
		},
	}, nil
}

func buildWaitError(stableID, providerName string) *user_input.WaitForInputError {
	return &user_input.WaitForInputError{
		ActivityID:         stableID,
		RequiredFields:     []string{"photos"},
		EnricherProviderID: providerName,
		Metadata: map[string]string{
			"display.field_labels": `{"photos":"Activity Photos"}`,
			"display.field_types":  `{"photos":"photo_upload"}`,
			"display.summary":      "Add photos from this activity (optional)",
			"display.title":        "Upload Activity Photos",
			"display.max_photos":   "10",
		},
	}
}
