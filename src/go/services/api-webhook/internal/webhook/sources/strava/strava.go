// nolint:proto-json
package strava

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

type Provider struct {
	verifyToken string
}

func NewProvider(verifyToken string) *Provider {
	if verifyToken == "" {
		panic("STRAVA_WEBHOOK_VERIFY_TOKEN must be set")
	}
	return &Provider{verifyToken: verifyToken}
}

func (p *Provider) ID() string {
	return "strava"
}

// VerifySubscription handles Strava's `hub.challenge` loop
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == p.verifyToken {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"hub.challenge": challenge})
		return
	}

	http.Error(w, "Forbidden", http.StatusForbidden)
}

type stravaWebhookPayload struct {
	ObjectType string                 `json:"object_type"`
	ObjectID   int64                  `json:"object_id"`
	AspectType string                 `json:"aspect_type"`
	OwnerID    int64                  `json:"owner_id"`
	Updates    map[string]interface{} `json:"updates"`
}

// ParseEvent extracts events from a Strava webhook
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload stravaWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	// We only process activity events for now.
	if payload.ObjectType != "activity" {
		// Return empty list so processor ignores it but returns 200 OK
		return nil, nil
	}

	evt := &webhook.WebhookEvent{
		Provider:    p.ID(),
		ProviderUID: fmt.Sprintf("%d", payload.OwnerID),
		ActivityID:  fmt.Sprintf("%d", payload.ObjectID),
		Event:       payload.AspectType, // "create", "update", "delete"
		RawPayload:  body,
	}

	return []*webhook.WebhookEvent{evt}, nil
}

func (p *Provider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	// 1. Fetch Strava tokens for user
	integResp, err := userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   internalUserID,
		Provider: p.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get integration for user: %w", err)
	}

	stravaInteg := integResp.Integrations.Strava
	if stravaInteg == nil || stravaInteg.AccessToken == "" {
		return nil, fmt.Errorf("strava integration not found or access token missing")
	}

	// 2. Fetch activity from Strava API
	url := fmt.Sprintf("https://www.strava.com/api/v3/activities/%s?include_all_efforts=true", evt.ActivityID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+stravaInteg.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch strava activity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("strava api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 3. Map to StandardizedActivity
	// For now we just create the skeleton with the raw json payload,
	// Actual parsing of Strava types -> StandardizedActivity should be fully implemented here
	// or in a separate standardization package.
	// Since phase 3.4 just wants the payload to get to pubsub, we will attach the Raw Payload json
	payload := &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_STRAVA,
		UserId:              internalUserID,
		OriginalPayloadJson: string(rawBody),
		ActivityId:          &evt.ActivityID,
	}

	return payload, nil
}
