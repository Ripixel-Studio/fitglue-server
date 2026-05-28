// nolint:proto-json
package strava_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/strava"
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
	provider := strava.NewProvider("secret-token")

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?hub.mode=subscribe&hub.verify_token=secret-token&hub.challenge=12345", nil)
		rec := httptest.NewRecorder()

		provider.VerifySubscription(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "{\"hub.challenge\":\"12345\"}\n", rec.Body.String())
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?hub.mode=subscribe&hub.verify_token=wrong-token&hub.challenge=12345", nil)
		rec := httptest.NewRecorder()

		provider.VerifySubscription(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestParseEvent(t *testing.T) {
	provider := strava.NewProvider("secret-token")

	t.Run("valid activity create", func(t *testing.T) {
		payload := map[string]interface{}{
			"object_type": "activity",
			"object_id":   123456,
			"aspect_type": "create",
			"owner_id":    98765,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))

		events, err := provider.ParseEvent(req)

		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "strava", events[0].Provider)
		assert.Equal(t, "98765", events[0].ProviderUID)
		assert.Equal(t, "123456", events[0].ActivityID)
		assert.Equal(t, "create", events[0].Event)
	})

	t.Run("ignore non-activity", func(t *testing.T) {
		payload := map[string]interface{}{
			"object_type": "athlete",
			"object_id":   123456,
			"aspect_type": "create",
			"owner_id":    98765,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))

		events, err := provider.ParseEvent(req)

		assert.NoError(t, err)
		assert.Empty(t, events)
	})
}

func TestFetchActivity(t *testing.T) {
	provider := strava.NewProvider("secret")

	// Mock Strava API
	stravaSvr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/activities/act123", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("include_all_efforts"))
		assert.Equal(t, "Bearer strava-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 123456, "name": "Morning Run", "type": "Run"}`))
	}))
	defer stravaSvr.Close()

	// Need to override the hardcoded Strava URL in strava.go for testing,
	// but since it's hardcoded to `https://www.strava.com/...` we'll just test the error paths
	// for now OR we inject the base URL if needed.
	// Since we haven't injected the URL in Provider, let's at least test the failure modes where HTTP request fails
	// or tokens are missing.

	t.Run("missing integration returns error", func(t *testing.T) {
		userSvc := &mockUserServiceClient{
			getIntegrationErr: nil,
			getIntegrationResp: &userpb.GetIntegrationResponse{
				Integrations: &user.UserIntegrations{},
			},
		}

		evt := &webhook.WebhookEvent{
			Provider:   "strava",
			ActivityID: "act123",
		}

		payload, err := provider.FetchActivity(context.Background(), userSvc, "user1", evt)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "strava integration not found or access token missing")
		assert.Nil(t, payload)
	})
}
