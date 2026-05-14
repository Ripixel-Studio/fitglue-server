// nolint:proto-json
package intervals

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	intervalsclient "github.com/fitglue/server/src/go/pkg/integrations/intervals"
	fit_parser "github.com/fitglue/server/src/go/pkg/domain/fit_parser"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
)

// Provider implements webhook.SourceProvider for Intervals.icu
type Provider struct{}

// NewProvider creates a new Intervals SourceProvider
func NewProvider() *Provider {
	return &Provider{}
}

// ID returns the provider identifier
func (p *Provider) ID() string {
	return "intervals"
}

// VerifySubscription handles Intervals.icu webhook verification.
// Intervals does not use a challenge-response handshake — users register
// the URL directly in their account settings.
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// intervalsWebhookPayload is the shape Intervals.icu POSTs to webhook URLs.
type intervalsWebhookPayload struct {
	ID   int64  `json:"id"`  // activity ID
	UID  string `json:"uid"` // athlete ID, e.g. "i12345"
	Type string `json:"type"`
	Name string `json:"name"`
}

// ParseEvent validates and extracts event data from an Intervals.icu webhook POST.
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload intervalsWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	if payload.UID == "" || payload.ID == 0 {
		// Not a usable activity event — return empty so processor ACKs quietly.
		return nil, nil
	}

	activityID := strconv.FormatInt(payload.ID, 10)

	evt := &webhook.WebhookEvent{
		Provider:    p.ID(),
		ProviderUID: payload.UID, // athlete_id stored in integrations.intervals.athlete_id
		ActivityID:  activityID,
		Event:       "create",
		RawPayload:  body,
	}

	return []*webhook.WebhookEvent{evt}, nil
}

// FetchActivity retrieves the full activity from the Intervals.icu API, downloads
// the original FIT file, and parses it into a StandardizedActivity.
func (p *Provider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	// 1. Fetch the user's Intervals credentials.
	integResp, err := userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   internalUserID,
		Provider: p.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get integration for user: %w", err)
	}

	integ := integResp.Integrations.Intervals
	if integ == nil || integ.ApiKey == "" || integ.AthleteId == "" {
		return nil, fmt.Errorf("intervals integration not configured or credentials missing")
	}

	// 2. Parse the activity ID.
	activityID, err := strconv.ParseInt(evt.ActivityID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid activity id %q: %w", evt.ActivityID, err)
	}

	// 3. Download the FIT file for this activity.
	client := intervalsclient.NewClient(integ.ApiKey, integ.AthleteId)
	fitData, err := client.DownloadFITFile(ctx, activityID)
	if err != nil {
		return nil, fmt.Errorf("failed to download FIT file for activity %d: %w", activityID, err)
	}

	// 4. Parse the FIT file into a StandardizedActivity.
	stdActivity, err := fit_parser.ParseFitFile(fitData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse FIT file for activity %d: %w", activityID, err)
	}

	stdActivity.Source = activitypb.ActivitySource_SOURCE_INTERVALS
	stdActivity.UserId = internalUserID
	stdActivity.ExternalId = evt.ActivityID

	return &pbevents.ActivityPayload{
		Source:               activitypb.ActivitySource_SOURCE_INTERVALS,
		UserId:               internalUserID,
		OriginalPayloadJson:  string(evt.RawPayload),
		ActivityId:           &evt.ActivityID,
		StandardizedActivity: stdActivity,
	}, nil
}
