package pubsub

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCloudEvent_NonProto(t *testing.T) {
	type payload struct {
		UserID string `json:"user_id"`
		Score  int    `json:"score"`
	}

	data := payload{UserID: "u123", Score: 42}
	evt, err := NewCloudEvent("fitglue/test", "com.fitglue.test.created", data)

	require.NoError(t, err)
	assert.Equal(t, "1.0", evt.SpecVersion())
	assert.Equal(t, "com.fitglue.test.created", evt.Type())
	assert.Equal(t, "fitglue/test", evt.Source())

	// Data must be JSON-encodable to the original struct
	var decoded payload
	require.NoError(t, json.Unmarshal(evt.Data(), &decoded))
	assert.Equal(t, data, decoded)
}

func TestNewCloudEvent_MapPayload(t *testing.T) {
	data := map[string]interface{}{"key": "value", "num": float64(1)}
	evt, err := NewCloudEvent("fitglue/test", "com.fitglue.map.event", data)

	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(evt.Data(), &decoded))
	assert.Equal(t, "value", decoded["key"])
}

func TestNewCloudEvent_EmptySource(t *testing.T) {
	evt, err := NewCloudEvent("", "com.fitglue.test", map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "", evt.Source())
}

func TestNewCloudEvent_SourceAndTypeSet(t *testing.T) {
	evt, err := NewCloudEvent("fitglue/other", "com.fitglue.other.event", nil)
	require.NoError(t, err)
	assert.Equal(t, "fitglue/other", evt.Source())
	assert.Equal(t, "com.fitglue.other.event", evt.Type())
}
