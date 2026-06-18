package polar_test

import (
	"context"
	"errors"
	"testing"

	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/polar"
	"github.com/stretchr/testify/assert"
)

func TestFetchActivity_GetIntegrationError(t *testing.T) {
	provider := polar.NewProvider()
	userSvc := &mockUserServiceClient{
		getIntegrationErr: errors.New("rpc down"),
	}
	evt := &webhook.WebhookEvent{Provider: "polar", ActivityID: "ex123"}

	payload, err := provider.FetchActivity(context.Background(), userSvc, "user1", evt)

	assert.ErrorContains(t, err, "failed to get integration for user")
	assert.Nil(t, payload)
}

func TestFetchActivity_NilIntegrationsResponse(t *testing.T) {
	// GetIntegration succeeds but Polar sub-integration is absent.
	provider := polar.NewProvider()
	userSvc := &mockUserServiceClient{
		getIntegrationResp: &userpb.GetIntegrationResponse{
			Integrations: &user.UserIntegrations{},
		},
	}
	evt := &webhook.WebhookEvent{Provider: "polar", ActivityID: "ex123"}

	payload, err := provider.FetchActivity(context.Background(), userSvc, "user1", evt)

	assert.ErrorContains(t, err, "polar integration not found or missing tokens")
	assert.Nil(t, payload)
}
