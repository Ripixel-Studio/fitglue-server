package webhook_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/internal/infra"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func ptr[T any](v T) *T {
	return &v
}

// mockProvider implements webhook.SourceProvider for testing
type mockProvider struct {
	id            string
	verifyCalled  bool
	parseCalled   bool
	fetchCalled   bool
	parseEvents   []*webhook.WebhookEvent
	parseError    error
	fetchActivity *pbevents.ActivityPayload
	fetchError    error
}

func (m *mockProvider) ID() string {
	return m.id
}

func (m *mockProvider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	m.verifyCalled = true
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("verified"))
}

func (m *mockProvider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	m.parseCalled = true
	if m.parseError != nil {
		return nil, m.parseError
	}
	return m.parseEvents, nil
}

func (m *mockProvider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	m.fetchCalled = true
	if m.fetchError != nil {
		return nil, m.fetchError
	}
	return m.fetchActivity, nil
}

// mockUserServiceClient implements userpb.UserServiceClient
type mockUserServiceClient struct {
	userpb.UserServiceClient
	resolveResp *userpb.ResolveUserByIntegrationResponse
	resolveErr  error
}

func (m *mockUserServiceClient) ResolveUserByIntegration(ctx context.Context, in *userpb.ResolveUserByIntegrationRequest, opts ...grpc.CallOption) (*userpb.ResolveUserByIntegrationResponse, error) {
	if m.resolveErr != nil {
		return nil, m.resolveErr
	}
	return m.resolveResp, nil
}

// mockPublisher implements webhook.Publisher
type mockPublisher struct {
	publishedEvents []cloudevents.Event
	publishErr      error
}

func (m *mockPublisher) PublishCloudEvent(ctx context.Context, topic string, event cloudevents.Event) (string, error) {
	if m.publishErr != nil {
		return "", m.publishErr
	}
	m.publishedEvents = append(m.publishedEvents, event)
	return "msg-id", nil
}

func TestProcessor_HandleVerification(t *testing.T) {
	logger := infra.NewLogger()
	processor := webhook.NewProcessor(logger, nil, nil, nil)
	mock := &mockProvider{id: "testprovider"}
	processor.Register(mock)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/webhook/testprovider", nil)
		w := httptest.NewRecorder()

		processor.HandleVerification(w, req, "testprovider")

		assert.True(t, mock.verifyCalled)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "verified", w.Body.String())
	})

	t.Run("unknown provider", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/webhook/unknown", nil)
		w := httptest.NewRecorder()

		processor.HandleVerification(w, req, "unknown")

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Unknown provider")
	})
}

func TestProcessor_HandleEvent(t *testing.T) {
	userClient := &mockUserServiceClient{}
	publisher := &mockPublisher{}
	logger := infra.NewLogger()
	processor := webhook.NewProcessor(logger, userClient, publisher, nil)

	mock := &mockProvider{
		id: "testprovider",
		parseEvents: []*webhook.WebhookEvent{
			{
				Provider:    "testprovider",
				ProviderUID: "provider-uid-123",
				ActivityID:  "act456",
				Event:       "create",
			},
		},
		fetchActivity: &pbevents.ActivityPayload{
			ActivityId: ptr("act456"),
		},
	}
	processor.Register(mock)

	t.Run("success processing events", func(t *testing.T) {
		mock.parseCalled = false
		mock.fetchCalled = false
		publisher.publishedEvents = nil

		userClient.resolveResp = &userpb.ResolveUserByIntegrationResponse{
			Profile: &pbuser.UserProfile{
				UserId: "internal-user-abc",
			},
		}
		userClient.resolveErr = nil
		mock.fetchError = nil

		req := httptest.NewRequest(http.MethodPost, "/webhook/testprovider", bytes.NewBufferString("{}"))
		w := httptest.NewRecorder()

		processor.HandleEvent(w, req, "testprovider")
		time.Sleep(100 * time.Millisecond) // allow async processEvents goroutine to complete

		assert.True(t, mock.parseCalled)
		assert.True(t, mock.fetchCalled)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, publisher.publishedEvents, 1)

		ce := publisher.publishedEvents[0]
		assert.Equal(t, "com.fitglue.activity.created", ce.Type())
		assert.Equal(t, "/integrations/testprovider/webhook", ce.Source())
	})

	t.Run("unknown provider", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook/unknown", bytes.NewBufferString("{}"))
		w := httptest.NewRecorder()

		processor.HandleEvent(w, req, "unknown")

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("parsing error returns bad request", func(t *testing.T) {
		errMock := &mockProvider{
			id:         "errorprovider",
			parseError: errors.New("signature mismatch"),
		}
		processor.Register(errMock)

		req := httptest.NewRequest(http.MethodPost, "/webhook/errorprovider", bytes.NewBufferString("{}"))
		w := httptest.NewRecorder()

		processor.HandleEvent(w, req, "errorprovider")

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "signature mismatch")
	})

	t.Run("user resolution failure skips event", func(t *testing.T) {
		mock.parseCalled = false
		mock.fetchCalled = false
		publisher.publishedEvents = nil

		userClient.resolveErr = errors.New("user not found")

		req := httptest.NewRequest(http.MethodPost, "/webhook/testprovider", bytes.NewBufferString("{}"))
		w := httptest.NewRecorder()

		processor.HandleEvent(w, req, "testprovider")

		assert.True(t, mock.parseCalled)
		assert.False(t, mock.fetchCalled)      // Should not reach fetch
		assert.Equal(t, http.StatusOK, w.Code) // Still returns 200 to acknowledge receipt
		assert.Empty(t, publisher.publishedEvents)
	})

	t.Run("activity fetch failure skips event", func(t *testing.T) {
		mock.parseCalled = false
		mock.fetchCalled = false
		publisher.publishedEvents = nil

		userClient.resolveResp = &userpb.ResolveUserByIntegrationResponse{
			Profile: &pbuser.UserProfile{
				UserId: "internal-user-abc",
			},
		}
		userClient.resolveErr = nil
		mock.fetchError = errors.New("fetch failed")

		req := httptest.NewRequest(http.MethodPost, "/webhook/testprovider", bytes.NewBufferString("{}"))
		w := httptest.NewRecorder()

		processor.HandleEvent(w, req, "testprovider")
		time.Sleep(100 * time.Millisecond) // allow async processEvents goroutine to complete

		assert.True(t, mock.parseCalled)
		assert.True(t, mock.fetchCalled)
		assert.Equal(t, http.StatusOK, w.Code)     // Still returns 200 OK
		assert.Empty(t, publisher.publishedEvents) // Nothing published
	})
}
