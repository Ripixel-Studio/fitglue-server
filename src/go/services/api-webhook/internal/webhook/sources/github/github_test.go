package github_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/github"
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

func TestProvider_ID(t *testing.T) {
	p := github.NewProvider("")
	assert.Equal(t, "github", p.ID())
}

func TestProvider_VerifySubscription(t *testing.T) {
	p := github.NewProvider("")
	req := httptest.NewRequest(http.MethodGet, "/webhook/github", nil)
	w := httptest.NewRecorder()

	p.VerifySubscription(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProvider_ParseEvent(t *testing.T) {
	p := github.NewProvider("")

	t.Run("ignore non-push event", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewBufferString(`{}`))
		req.Header.Set("X-GitHub-Event", "pull_request")

		events, err := p.ParseEvent(req)

		assert.NoError(t, err)
		assert.Nil(t, events)
	})

	t.Run("valid push payload", func(t *testing.T) {
		bodyStr := `{
			"ref": "refs/heads/main",
			"repository": {
				"owner": {
					"login": "octocat"
				}
			},
			"commits": [
				{
					"id": "abc1234",
					"committer": {
						"name": "Octocat",
						"email": "octocat@github.com"
					}
				}
			],
			"head_commit": {
				"id": "abc1234"
			}
		}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewBufferString(bodyStr))
		req.Header.Set("X-GitHub-Event", "push")

		events, err := p.ParseEvent(req)

		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "github", events[0].Provider)
		assert.Equal(t, "octocat", events[0].ProviderUID)
		assert.Equal(t, "abc1234", events[0].ActivityID)
		assert.Equal(t, "push", events[0].Event)
	})

	t.Run("ignore all bot commits", func(t *testing.T) {
		bodyStr := `{
			"ref": "refs/heads/main",
			"repository": {
				"owner": {
					"login": "octocat"
				}
			},
			"commits": [
				{
					"id": "def5678",
					"committer": {
						"name": "FitGlue Bot",
						"email": "bot@fitglue.com"
					}
				}
			],
			"head_commit": {
				"id": "def5678"
			}
		}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewBufferString(bodyStr))
		req.Header.Set("X-GitHub-Event", "push")

		events, err := p.ParseEvent(req)

		assert.NoError(t, err)
		assert.Nil(t, events)
	})

	t.Run("fallback to commits array if no head_commit", func(t *testing.T) {
		bodyStr := `{
			"ref": "refs/heads/main",
			"repository": {
				"owner": {
					"login": "octocat"
				}
			},
			"commits": [
				{
					"id": "123def",
					"committer": {
						"name": "User",
						"email": "user@test.com"
					}
				}
			]
		}`
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewBufferString(bodyStr))
		req.Header.Set("X-GitHub-Event", "push")

		events, err := p.ParseEvent(req)

		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "123def", events[0].ActivityID)
	})

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewBufferString(`{invalid`))
		req.Header.Set("X-GitHub-Event", "push")

		_, err := p.ParseEvent(req)
		assert.ErrorContains(t, err, "invalid json")
	})
}

func TestFetchActivity(t *testing.T) {
	provider := github.NewProvider("")

	t.Run("valid payload", func(t *testing.T) {
		userSvc := &mockUserServiceClient{}

		evt := &webhook.WebhookEvent{
			Provider:   "github",
			ActivityID: "commit123",
			RawPayload: []byte(`{"ref": "refs/heads/main"}`),
		}

		payload, err := provider.FetchActivity(context.Background(), userSvc, "user1", evt)

		assert.NoError(t, err)
		assert.NotNil(t, payload)
		assert.Equal(t, activitypb.ActivitySource_SOURCE_GITHUB, payload.Source)
		assert.Equal(t, "user1", payload.UserId)
		assert.Equal(t, string(evt.RawPayload), payload.OriginalPayloadJson)
	})

	t.Run("missing raw payload", func(t *testing.T) {
		userSvc := &mockUserServiceClient{}

		evt := &webhook.WebhookEvent{
			Provider:   "github",
			ActivityID: "no-payload",
		}

		payload, err := provider.FetchActivity(context.Background(), userSvc, "user2", evt)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing raw payload for github activity push")
		assert.Nil(t, payload)
	})
}
