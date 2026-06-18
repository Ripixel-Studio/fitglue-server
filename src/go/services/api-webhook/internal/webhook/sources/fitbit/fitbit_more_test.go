package fitbit_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domainuser "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/fitbit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_EmptyClientSecret(t *testing.T) {
	_, err := fitbit.NewProvider("verify", "", nil)
	assert.ErrorContains(t, err, "FITBIT_OAUTH_CLIENT_SECRET must be set")
}

func TestProvider_ID(t *testing.T) {
	p, err := fitbit.NewProvider("verify", "secret", nil)
	require.NoError(t, err)
	assert.Equal(t, "fitbit", p.ID())
}

func TestParseEvent_InvalidJSONWithValidSignature(t *testing.T) {
	clientSecret := "sig-secret"
	provider, err := fitbit.NewProvider("verify", clientSecret, nil)
	require.NoError(t, err)

	body := []byte(`{not json`)
	sig := computeFitbitSignature(body, clientSecret)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))
	req.Header.Set("X-Fitbit-Signature", sig)

	events, err := provider.ParseEvent(req)
	assert.ErrorContains(t, err, "invalid json")
	assert.Nil(t, events)
}

func TestParseEvent_NoActivitiesCollectionReturnsEmpty(t *testing.T) {
	// Valid signed payload but only non-activity collections -> no events, no error.
	clientSecret := "sig-secret"
	provider, err := fitbit.NewProvider("verify", clientSecret, nil)
	require.NoError(t, err)

	body := []byte(`[{"collectionType":"sleep","date":"2023-10-25","ownerId":"u1","ownerType":"user","subscriptionId":"s"}]`)
	sig := computeFitbitSignature(body, clientSecret)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))
	req.Header.Set("X-Fitbit-Signature", sig)

	events, err := provider.ParseEvent(req)
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestFetchActivity_MissingDate(t *testing.T) {
	provider, err := fitbit.NewProvider("verify", "secret", nil)
	require.NoError(t, err)

	evt := &webhook.WebhookEvent{Provider: "fitbit", ActivityID: ""}
	payload, err := provider.FetchActivity(context.Background(), nil, "user1", evt)

	assert.ErrorContains(t, err, "missing date for fitbit activity fetch")
	assert.Nil(t, payload)
}

func TestFetchActivity_TokenError(t *testing.T) {
	// Provide a date so it proceeds to the token fetch, which fails because
	// the backing DB returns an error (no fitbit integration / lookup failure).
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return nil, errors.New("db down")
		},
	}
	provider, err := fitbit.NewProvider("verify", "secret", db)
	require.NoError(t, err)

	evt := &webhook.WebhookEvent{Provider: "fitbit", ActivityID: "2023-10-25"}
	payload, err := provider.FetchActivity(context.Background(), nil, "user1", evt)

	assert.ErrorContains(t, err, "failed to get fitbit token")
	assert.Nil(t, payload)
}
