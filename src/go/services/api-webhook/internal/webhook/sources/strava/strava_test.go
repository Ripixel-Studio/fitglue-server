// nolint:proto-json
package strava_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/strava"
	"github.com/stretchr/testify/assert"
)

func TestVerifySubscription(t *testing.T) {
	provider := strava.NewProvider("secret-token", nil)

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
	provider := strava.NewProvider("secret-token", nil)

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

