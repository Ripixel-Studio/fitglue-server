// nolint:proto-json
package parkrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
)

// Provider implements webhook.SourceProvider for Parkrun
type Provider struct{}

// NewProvider creates a new Parkrun SourceProvider
func NewProvider() *Provider {
	return &Provider{}
}

// ID returns the provider identifier
func (p *Provider) ID() string {
	return "parkrun"
}

// VerifySubscription handles Parkrun webhook verification
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	// TODO(SEC-03): needs per-user webhook secret storage to authenticate Parkrun scraper payloads.
	// Until then, early rejection relies on ResolveUserByIntegration in the processor
	// failing for any ProviderUID that maps to no known FitGlue user.
	w.WriteHeader(http.StatusOK)
}

type parkrunPayload struct {
	AthleteID string `json:"athleteId"`
	RunNumber string `json:"runNumber"`
	Event     string `json:"event"`
}

// ParseEvent extracts events from a Parkrun webhook
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload parkrunPayload
	// Since Parkrun is a Cloud Scheduler scraper, it might push individual results
	// or an array. We'll assume a single object for now.
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	if payload.AthleteID == "" || payload.RunNumber == "" {
		return nil, fmt.Errorf("missing athleteId or runNumber")
	}

	evt := &webhook.WebhookEvent{
		Provider:    p.ID(),
		ProviderUID: payload.AthleteID,
		ActivityID:  payload.RunNumber,
		Event:       payload.Event,
		RawPayload:  body,
	}

	return []*webhook.WebhookEvent{evt}, nil
}

func (p *Provider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	if len(evt.RawPayload) == 0 {
		return nil, fmt.Errorf("missing raw payload for parkrun results")
	}

	payload := &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_PARKRUN_RESULTS,
		UserId:              internalUserID,
		OriginalPayloadJson: string(evt.RawPayload),
		ActivityId:          &evt.ActivityID,
	}

	return payload, nil
}
