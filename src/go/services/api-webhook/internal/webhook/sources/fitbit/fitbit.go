// nolint:proto-json
package fitbit

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/infrastructure/oauth"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
)

type Provider struct {
	verifyCode   string
	clientSecret string
	db           shared.Database
}

func NewProvider(verifyCode, clientSecret string, db shared.Database) (*Provider, error) {
	if clientSecret == "" {
		return nil, fmt.Errorf("fitbit: FITBIT_OAUTH_CLIENT_SECRET must be set")
	}
	return &Provider{verifyCode: verifyCode, clientSecret: clientSecret, db: db}, nil
}

func (p *Provider) ID() string {
	return "fitbit"
}

// VerifySubscription handles Fitbit's subscriber verification
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	verifyParam := r.URL.Query().Get("verify")

	if verifyParam == p.verifyCode {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.Error(w, "Forbidden", http.StatusNotFound)
}

type fitbitWebhookPayload []struct {
	CollectionType string `json:"collectionType"`
	Date           string `json:"date"`
	OwnerId        string `json:"ownerId"`
	OwnerType      string `json:"ownerType"`
	SubscriptionId string `json:"subscriptionId"`
}

// ParseEvent extracts events from a Fitbit webhook
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Verify X-Fitbit-Signature HMAC-SHA1
	sig := r.Header.Get("X-Fitbit-Signature")
	if sig == "" {
		return nil, fmt.Errorf("missing X-Fitbit-Signature header")
	}

	mac := hmac.New(sha1.New, []byte(p.clientSecret+"&"))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, fmt.Errorf("invalid X-Fitbit-Signature")
	}

	var payload fitbitWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	var events []*webhook.WebhookEvent
	for _, evt := range payload {
		// We only care about activities
		if evt.CollectionType != "activities" {
			continue
		}

		events = append(events, &webhook.WebhookEvent{
			Provider:    p.ID(),
			ProviderUID: evt.OwnerId,
			ActivityID:  evt.Date, // Fitbit uses date as ID for generic syncs sometimes, or fetches all on that date
			Event:       "update",
			RawPayload:  body, // Sending entire array body for now
		})
	}

	return events, nil
}

func (p *Provider) FetchActivity(ctx context.Context, _ userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	// Fitbit activity id in the webhook is actually the date "YYYY-MM-DD"
	date := evt.ActivityID
	if date == "" {
		return nil, fmt.Errorf("missing date for fitbit activity fetch")
	}

	// 1. Get a valid (auto-refreshed) token via Firestore
	token, err := oauth.NewFirestoreTokenSourceFromDB(p.db, internalUserID, "fitbit").Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get fitbit token: %w", err)
	}

	// 2. Fetch activity list for the date from Fitbit API
	url := fmt.Sprintf("https://api.fitbit.com/1/user/-/activities/date/%s.json", date)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fitbit activity list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fitbit api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 3. Construct Payload
	// In the real system, this raw JSON contains a list of activities that need to be parsed
	// and individually converted into `StandardizedActivity` objects, likely fetching TCX for each.
	// For Phase 3.4 webhook orchestration, we pass the raw daily summary JSON forward in the payload.
	payload := &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_FITBIT,
		UserId:              internalUserID,
		OriginalPayloadJson: string(rawBody),
		ActivityId:          &evt.ActivityID,
	}

	return payload, nil
}
