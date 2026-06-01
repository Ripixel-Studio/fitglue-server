package pubsub

import (
	"testing"

	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"github.com/stretchr/testify/assert"
)

func TestGetCloudEventType(t *testing.T) {
	tests := []struct {
		name     string
		input    pbevents.CloudEventType
		expected string
	}{
		{
			name:     "activity created returns correct URN",
			input:    pbevents.CloudEventType_CLOUD_EVENT_TYPE_ACTIVITY_CREATED,
			expected: "com.fitglue.activity.created",
		},
		{
			name:     "unspecified returns unknown",
			input:    pbevents.CloudEventType_CLOUD_EVENT_TYPE_UNSPECIFIED,
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetCloudEventType(tt.input))
		})
	}
}

func TestGetCloudEventSource(t *testing.T) {
	// Unspecified should return unknown since it has no ce_source annotation
	result := GetCloudEventSource(pbevents.CloudEventSource_CLOUD_EVENT_SOURCE_UNSPECIFIED)
	assert.Equal(t, "unknown", result)
}

func TestGetDestinationTopic(t *testing.T) {
	// Unspecified destination should return empty string
	result := GetDestinationTopic(pbplugin.DestinationType_DESTINATION_UNSPECIFIED)
	assert.Equal(t, "", result)
}
