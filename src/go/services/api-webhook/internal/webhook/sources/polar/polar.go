// nolint:proto-json
package polar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
)

// Provider implements webhook.SourceProvider for Polar
type Provider struct{}

// NewProvider creates a new Polar SourceProvider
func NewProvider() *Provider {
	return &Provider{}
}

// ID returns the provider identifier
func (p *Provider) ID() string {
	return "polar"
}

// VerifySubscription handles Polar webhook verification
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	// TODO(SEC-03): needs per-user webhook secret storage to verify Polar webhook signatures.
	// Until then, early rejection relies on ResolveUserByIntegration in the processor
	// failing for any ProviderUID that maps to no known FitGlue user.
	w.WriteHeader(http.StatusOK)
}

type polarWebhookNotification struct {
	Event     string `json:"event"`
	UserID    int64  `json:"user_id"`
	EntityID  string `json:"entity_id"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
}

// ParseEvent extracts events from a Polar webhook
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload polarWebhookNotification
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	if payload.Event != "EXERCISE" {
		// Ignore non-exercise events
		return nil, nil
	}

	if payload.EntityID == "" {
		return nil, fmt.Errorf("missing entity_id")
	}

	evt := &webhook.WebhookEvent{
		Provider:    p.ID(),
		ProviderUID: fmt.Sprintf("%d", payload.UserID),
		ActivityID:  payload.EntityID,
		Event:       payload.Event,
		RawPayload:  body,
	}

	return []*webhook.WebhookEvent{evt}, nil
}

func (p *Provider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	// 1. Fetch Polar tokens for user
	integResp, err := userSvc.GetIntegration(ctx, &userpb.GetIntegrationRequest{
		UserId:   internalUserID,
		Provider: p.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get integration for user: %w", err)
	}

	polarInteg := integResp.Integrations.Polar
	if polarInteg == nil || polarInteg.AccessToken == "" || polarInteg.PolarUserId == "" {
		return nil, fmt.Errorf("polar integration not found or missing tokens")
	}

	accessToken := polarInteg.AccessToken
	polarUserID := polarInteg.PolarUserId

	client := &http.Client{}

	// Step 1: Start Transaction
	startTxURL := fmt.Sprintf("https://www.polaraccesslink.com/v3/users/%s/exercise-transactions", polarUserID)
	reqTx, err := http.NewRequestWithContext(ctx, http.MethodPost, startTxURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction request: %w", err)
	}
	reqTx.Header.Set("Authorization", "Bearer "+accessToken)
	reqTx.Header.Set("Accept", "application/json")

	respTx, err := client.Do(reqTx)
	if err != nil {
		return nil, fmt.Errorf("failed to start polar transaction: %w", err)
	}
	defer respTx.Body.Close()

	if respTx.StatusCode == http.StatusNoContent {
		// 204 means no new exercises
		return nil, fmt.Errorf("no new exercises in polar transaction")
	}

	if respTx.StatusCode != http.StatusCreated && respTx.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respTx.Body)
		return nil, fmt.Errorf("failed to start polar transaction, status=%d body=%s", respTx.StatusCode, string(body))
	}

	var txData struct {
		TransactionID int64  `json:"transaction-id"`
		ResourceURI   string `json:"resource-uri"`
	}
	if err := json.NewDecoder(respTx.Body).Decode(&txData); err != nil {
		return nil, fmt.Errorf("failed to decode transaction response: %w", err)
	}

	txID := txData.TransactionID

	// Step 4: Always commit the transaction when we're done (even on error below)
	defer func() {
		commitURL := fmt.Sprintf("https://www.polaraccesslink.com/v3/users/%s/exercise-transactions/%d", polarUserID, txID)
		reqCommit, err := http.NewRequestWithContext(context.Background(), http.MethodPut, commitURL, nil)
		if err == nil {
			reqCommit.Header.Set("Authorization", "Bearer "+accessToken)
			client.Do(reqCommit)
		}
	}()

	// Step 2: List Exercises in Transaction
	listURL := fmt.Sprintf("https://www.polaraccesslink.com/v3/users/%s/exercise-transactions/%d", polarUserID, txID)
	reqList, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %w", err)
	}
	reqList.Header.Set("Authorization", "Bearer "+accessToken)
	reqList.Header.Set("Accept", "application/json")

	respList, err := client.Do(reqList)
	if err != nil {
		return nil, fmt.Errorf("failed to list exercises: %w", err)
	}
	defer respList.Body.Close()

	if respList.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respList.Body)
		return nil, fmt.Errorf("failed to list polar exercises, status=%d body=%s", respList.StatusCode, string(body))
	}

	var listData struct {
		Exercises []string `json:"exercises"`
	}
	if err := json.NewDecoder(respList.Body).Decode(&listData); err != nil {
		return nil, fmt.Errorf("failed to decode list response: %w", err)
	}

	if len(listData.Exercises) == 0 {
		return nil, fmt.Errorf("polar transaction opened but contained no exercises")
	}

	// Step 3: Fetch the specific exercise from the list (or just the first one)
	// Polar returns full URLs. We'll simply grab the first one, though Ideally we should match the evt.ActivityID
	// The URL ends with the exercise ID, e.g., .../exercises/12345
	var exerciseURL string
	for _, exURL := range listData.Exercises {
		// Just take the first one or match entity ID
		// Polar's entity_id from webhook is usually the exercise ID at the end of the URL
		if evt.ActivityID != "" {
			// Basic suffix check
			if len(exURL) >= len(evt.ActivityID) && exURL[len(exURL)-len(evt.ActivityID):] == evt.ActivityID {
				exerciseURL = exURL
				break
			}
		}
	}

	if exerciseURL == "" && len(listData.Exercises) > 0 {
		// Fallback to first if no exact match (Polar can be weird with IDs)
		exerciseURL = listData.Exercises[0]
	}

	if exerciseURL == "" {
		return nil, fmt.Errorf("no exercise url found to fetch")
	}

	// SEC-09: Validate that the exercise URL resolves to the expected Polar host before
	// issuing the request, preventing SSRF via a tampered API response.
	parsed, err := url.Parse(exerciseURL)
	if err != nil || !strings.HasSuffix(parsed.Hostname(), "polaraccesslink.com") {
		return nil, fmt.Errorf("polar: exercise URL has unexpected host: %s", exerciseURL)
	}

	reqEx, err := http.NewRequestWithContext(ctx, http.MethodGet, exerciseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create exercise request: %w", err)
	}
	reqEx.Header.Set("Authorization", "Bearer "+accessToken)
	reqEx.Header.Set("Accept", "application/json")

	respEx, err := client.Do(reqEx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch polar exercise: %w", err)
	}
	defer respEx.Body.Close()

	if respEx.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respEx.Body)
		return nil, fmt.Errorf("failed to fetch polar exercise, status=%d body=%s", respEx.StatusCode, string(body))
	}

	rawBody, err := io.ReadAll(respEx.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read exercise body: %w", err)
	}

	payload := &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_POLAR,
		UserId:              internalUserID,
		OriginalPayloadJson: string(rawBody),
		ActivityId:          &evt.ActivityID,
	}

	return payload, nil
}
