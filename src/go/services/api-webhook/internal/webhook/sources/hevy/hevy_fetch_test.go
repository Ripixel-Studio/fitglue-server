package hevy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/hevy"
	"github.com/stretchr/testify/assert"
)

func TestFetchActivity_MissingWorkoutID(t *testing.T) {
	provider := hevy.NewProvider()
	evt := &webhook.WebhookEvent{Provider: "hevy", ActivityID: ""}

	payload, err := provider.FetchActivity(context.Background(), &mockUserServiceClient{}, "user1", evt)

	assert.ErrorContains(t, err, "missing workout id for hevy activity fetch")
	assert.Nil(t, payload)
}

func TestFetchActivity_GetIntegrationError(t *testing.T) {
	provider := hevy.NewProvider()
	userSvc := &mockUserServiceClient{getIntegrationErr: errors.New("rpc down")}
	evt := &webhook.WebhookEvent{Provider: "hevy", ActivityID: "workout123"}

	payload, err := provider.FetchActivity(context.Background(), userSvc, "user1", evt)

	assert.ErrorContains(t, err, "failed to get integration for user")
	assert.Nil(t, payload)
}
