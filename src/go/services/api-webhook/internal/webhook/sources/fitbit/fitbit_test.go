package fitbit_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/fitbit"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// mockUserServiceClient implements userpb.UserServiceClient
type mockUserServiceClient struct {
	userpb.UserServiceClient
	getIntegrationResp *userpb.GetIntegrationResponse
	getIntegrationErr  error
}

func (m *mockUserServiceClient) GetIntegration(ctx context.Context, in *userpb.GetIntegrationRequest, opts ...grpc.CallOption) (*userpb.GetIntegrationResponse, error) {
	if m.getIntegrationErr != nil {
		return nil, m.getIntegrationErr
	}
	return m.getIntegrationResp, nil
}

func TestVerifySubscription(t *testing.T) {
	provider, err := fitbit.NewProvider("secret-code", "any-secret")
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?verify=secret-code", nil)
		rec := httptest.NewRecorder()

		provider.VerifySubscription(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?verify=wrong-code", nil)
		rec := httptest.NewRecorder()

		provider.VerifySubscription(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func computeFitbitSignature(body []byte, clientSecret string) string {
	mac := hmac.New(sha1.New, []byte(clientSecret+"&"))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestParseEvent(t *testing.T) {
	t.Run("valid activity collection with HMAC", func(t *testing.T) {
		clientSecret := "any-secret"
		provider, err := fitbit.NewProvider("secret-code", clientSecret)
		assert.NoError(t, err)

		payload := `[` +
			`{"collectionType":"activities","date":"2023-10-25","ownerId":"fitbitUser1","ownerType":"user","subscriptionId":"fitglue-activities"},` +
			`{"collectionType":"sleep","date":"2023-10-25","ownerId":"fitbitUser1","ownerType":"user","subscriptionId":"fitglue-sleep"}` +
			`]`

		body := []byte(payload)
		sig := computeFitbitSignature(body, clientSecret)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))
		req.Header.Set("X-Fitbit-Signature", sig)

		events, err := provider.ParseEvent(req)

		assert.NoError(t, err)
		assert.Len(t, events, 1) // Only activities should be parsed
		assert.Equal(t, "fitbit", events[0].Provider)
		assert.Equal(t, "fitbitUser1", events[0].ProviderUID)
		assert.Equal(t, "2023-10-25", events[0].ActivityID) // Fitbit uses date
		assert.Equal(t, "update", events[0].Event)
	})

	t.Run("valid HMAC signature", func(t *testing.T) {
		clientSecret := "test-secret"
		provider, err := fitbit.NewProvider("secret-code", clientSecret)
		assert.NoError(t, err)

		payload := `[{"collectionType":"activities","date":"2023-10-25","ownerId":"fitbitUser1","ownerType":"user","subscriptionId":"fitglue-activities"}]`
		body := []byte(payload)
		sig := computeFitbitSignature(body, clientSecret)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))
		req.Header.Set("X-Fitbit-Signature", sig)

		events, err := provider.ParseEvent(req)

		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	t.Run("invalid HMAC signature", func(t *testing.T) {
		provider, err := fitbit.NewProvider("secret-code", "test-secret")
		assert.NoError(t, err)

		payload := `[{"collectionType":"activities","date":"2023-10-25","ownerId":"fitbitUser1","ownerType":"user","subscriptionId":"fitglue-activities"}]`

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(payload))
		req.Header.Set("X-Fitbit-Signature", "invalid-signature")

		events, err := provider.ParseEvent(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid X-Fitbit-Signature")
		assert.Nil(t, events)
	})

	t.Run("missing HMAC signature header", func(t *testing.T) {
		provider, err := fitbit.NewProvider("secret-code", "test-secret")
		assert.NoError(t, err)

		payload := `[{"collectionType":"activities","date":"2023-10-25","ownerId":"fitbitUser1","ownerType":"user","subscriptionId":"fitglue-activities"}]`

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(payload))

		events, err := provider.ParseEvent(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing X-Fitbit-Signature header")
		assert.Nil(t, events)
	})
}

func TestFetchActivity(t *testing.T) {
	provider, err := fitbit.NewProvider("secret", "any-secret")
	if err != nil {
		t.Fatalf("unexpected error creating fitbit provider: %v", err)
	}

	t.Run("missing integration returns error", func(t *testing.T) {
		userSvc := &mockUserServiceClient{
			getIntegrationErr: nil,
			getIntegrationResp: &userpb.GetIntegrationResponse{
				Integrations: &user.UserIntegrations{},
			},
		}

		evt := &webhook.WebhookEvent{
			Provider:   "fitbit",
			ActivityID: "2023-10-25",
		}

		payload, err := provider.FetchActivity(context.Background(), userSvc, "user1", evt)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fitbit integration not found or access token missing")
		assert.Nil(t, payload)
	})
}
