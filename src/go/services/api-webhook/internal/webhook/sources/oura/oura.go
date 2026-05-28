// nolint:proto-json
package oura

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

// Provider implements webhook.SourceProvider for Oura
type Provider struct{}

// NewProvider creates a new Oura SourceProvider
func NewProvider() *Provider {
	return &Provider{}
}

// ID returns the provider identifier
func (p *Provider) ID() string {
	return "oura"
}

// VerifySubscription handles Oura webhook verification
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	// Oura might pass an x-oura-signature or verification_token.
	// For standard API webhook endpoints, just acknowledging 200 OK
	// TODO(SEC-03): needs per-user webhook secret storage to verify X-Oura-Signature HMAC.
	// Until then, early rejection relies on ResolveUserByIntegration in the processor
	// failing for any ProviderUID that maps to no known FitGlue user.
	w.WriteHeader(http.StatusOK)
}

type ouraPayload struct {
	EventType string `json:"event_type"`
	DataType  string `json:"data_type"`
	ObjectID  string `json:"object_id"`
	UserID    string `json:"user_id"`
	Timestamp string `json:"timestamp"`
}

// ParseEvent extracts events from an Oura webhook
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload ouraPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	if payload.DataType != "workout" {
		// Ignore non-workout events
		return nil, nil
	}

	if payload.EventType == "delete" || payload.EventType == "update" {
		// Ignore deletes and updates for now
		return nil, nil
	}

	if payload.ObjectID == "" || payload.UserID == "" {
		return nil, fmt.Errorf("missing object_id or user_id")
	}

	evt := &webhook.WebhookEvent{
		Provider:    p.ID(),
		ProviderUID: payload.UserID,
		ActivityID:  payload.ObjectID,
		Event:       payload.EventType,
		RawPayload:  body,
	}

	return []*webhook.WebhookEvent{evt}, nil
}

func (p *Provider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	workoutID := evt.ActivityID
	if workoutID == "" {
		return nil, fmt.Errorf("missing workout id for oura activity fetch")
	}

	// 1. Fetch Oura tokens for user
	integResp, err := userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   internalUserID,
		Provider: p.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get integration for user: %w", err)
	}

	ouraInteg := integResp.Integrations.Oura
	if ouraInteg == nil || ouraInteg.AccessToken == "" {
		return nil, fmt.Errorf("oura integration not found or access token missing")
	}

	// 2. Fetch activity from Oura API
	url := fmt.Sprintf("https://api.ouraring.com/v2/usercollection/workout/%s", workoutID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+ouraInteg.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch oura activity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("oura api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 3. Construct Payload
	payload := &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_OURA,
		UserId:              internalUserID,
		OriginalPayloadJson: string(rawBody),
		ActivityId:          &evt.ActivityID,
	}

	return payload, nil
}
