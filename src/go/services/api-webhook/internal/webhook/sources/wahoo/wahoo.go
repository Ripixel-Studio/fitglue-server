// nolint:proto-json
package wahoo

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

// Provider implements webhook.SourceProvider for Wahoo
type Provider struct{}

// NewProvider creates a new Wahoo SourceProvider
func NewProvider() *Provider {
	return &Provider{}
}

// ID returns the provider identifier
func (p *Provider) ID() string {
	return "wahoo"
}

// VerifySubscription handles Wahoo webhook verification
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	// TODO(SEC-03): needs per-user webhook secret storage to verify Wahoo HMAC signatures.
	// Until then, early rejection relies on ResolveUserByIntegration in the processor
	// failing for any ProviderUID that maps to no known FitGlue user.
	w.WriteHeader(http.StatusOK)
}

type wahooWebhookEvent struct {
	EventType    string `json:"event_type"`
	WebhookToken string `json:"webhook_token"`
	User         struct {
		ID int64 `json:"id"`
	} `json:"user"`
	WorkoutSummary *struct {
		ID int64 `json:"id"`
	} `json:"workout_summary"`
}

// ParseEvent extracts events from a Wahoo webhook
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload wahooWebhookEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	if payload.EventType != "workout_summary" {
		// Ignore non-workout events
		return nil, nil
	}

	if payload.WorkoutSummary == nil || payload.WorkoutSummary.ID == 0 {
		return nil, fmt.Errorf("missing workout summary ID")
	}

	evt := &webhook.WebhookEvent{
		Provider:    p.ID(),
		ProviderUID: fmt.Sprintf("%d", payload.User.ID),
		ActivityID:  fmt.Sprintf("%d", payload.WorkoutSummary.ID),
		Event:       payload.EventType,
		RawPayload:  body,
	}

	return []*webhook.WebhookEvent{evt}, nil
}

func (p *Provider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	workoutID := evt.ActivityID
	if workoutID == "" {
		return nil, fmt.Errorf("missing workout id for wahoo activity fetch")
	}

	// 1. Fetch Wahoo tokens for user
	integResp, err := userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   internalUserID,
		Provider: p.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get integration for user: %w", err)
	}

	wahooInteg := integResp.Integrations.Wahoo
	if wahooInteg == nil || wahooInteg.AccessToken == "" {
		return nil, fmt.Errorf("wahoo integration not found or access token missing")
	}

	// 2. Fetch activity from Wahoo API
	url := fmt.Sprintf("https://api.wahoofitness.com/v1/workouts/%s", workoutID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+wahooInteg.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch wahoo activity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wahoo api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 3. Construct Payload
	payload := &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_WAHOO,
		UserId:              internalUserID,
		OriginalPayloadJson: string(rawBody),
		ActivityId:          &evt.ActivityID,
	}

	return payload, nil
}
