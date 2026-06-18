package wahoo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/wahoo"
	"github.com/stretchr/testify/assert"
)

func TestFetchActivity_MissingWorkoutID(t *testing.T) {
	provider := wahoo.NewProvider()
	evt := &webhook.WebhookEvent{Provider: "wahoo", ActivityID: ""}

	payload, err := provider.FetchActivity(context.Background(), &mockUserServiceClient{}, "user1", evt)

	assert.ErrorContains(t, err, "missing workout id for wahoo activity fetch")
	assert.Nil(t, payload)
}

func TestFetchActivity_GetIntegrationError(t *testing.T) {
	provider := wahoo.NewProvider()
	userSvc := &mockUserServiceClient{getIntegrationErr: errors.New("rpc down")}
	evt := &webhook.WebhookEvent{Provider: "wahoo", ActivityID: "67890"}

	payload, err := provider.FetchActivity(context.Background(), userSvc, "user1", evt)

	assert.ErrorContains(t, err, "failed to get integration for user")
	assert.Nil(t, payload)
}
