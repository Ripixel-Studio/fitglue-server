package fitbit_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/fitbit"
	"github.com/stretchr/testify/assert"
)

func TestVerifySubscription(t *testing.T) {
	provider, err := fitbit.NewProvider("secret-code", "any-secret", nil)
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
		provider, err := fitbit.NewProvider("secret-code", clientSecret, nil)
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
		provider, err := fitbit.NewProvider("secret-code", clientSecret, nil)
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
		provider, err := fitbit.NewProvider("secret-code", "test-secret", nil)
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
		provider, err := fitbit.NewProvider("secret-code", "test-secret", nil)
		assert.NoError(t, err)

		payload := `[{"collectionType":"activities","date":"2023-10-25","ownerId":"fitbitUser1","ownerType":"user","subscriptionId":"fitglue-activities"}]`

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(payload))

		events, err := provider.ParseEvent(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing X-Fitbit-Signature header")
		assert.Nil(t, events)
	})
}

