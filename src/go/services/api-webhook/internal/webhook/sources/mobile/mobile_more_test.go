package mobile_test

import (
	"context"
	"testing"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/mobile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchActivity_UnknownSourceDefaultsToUnspecified(t *testing.T) {
	provider := mobile.NewProvider()
	evt := &webhook.WebhookEvent{
		Provider:   "mobile",
		ActivityID: "x1",
		RawPayload: []byte(`{"source": "SOMETHING_ELSE"}`),
	}

	payload, err := provider.FetchActivity(context.Background(), nil, "user1", evt)
	require.NoError(t, err)
	assert.Equal(t, activitypb.ActivitySource_SOURCE_UNSPECIFIED, payload.Source)
	assert.Equal(t, "user1", payload.UserId)
}

func TestFetchActivity_InvalidRawPayload(t *testing.T) {
	provider := mobile.NewProvider()
	evt := &webhook.WebhookEvent{
		Provider:   "mobile",
		ActivityID: "x1",
		RawPayload: []byte(`{not json`),
	}

	payload, err := provider.FetchActivity(context.Background(), nil, "user1", evt)
	assert.ErrorContains(t, err, "invalid mobile sync payload")
	assert.Nil(t, payload)
}
