package type_mapper

import (
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"log/slog"
	"testing"

	"github.com/fitglue/server/src/go/pkg/domain/activity"
)

func TestTypeMapperProvider_Enrich(t *testing.T) {
	provider := NewTypeMapperProvider()
	ctx := context.Background()

	// The type mapper works by matching title substrings to target activity types.
	// Config key is "type_rules" with format: {"title substring": "TargetActivityType"}
	tests := []struct {
		name           string
		activityName   string
		activityType   pbactivity.ActivityType
		typeRules      string // JSON object: {"title substring": "TargetActivityType"}
		expectedType   pbactivity.ActivityType
		expectMetadata bool
	}{
		{
			name:           "Maps activity with 'morning' in title to Yoga",
			activityName:   "Morning Stretch Session",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
			typeRules:      `{"morning": "Yoga"}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_YOGA,
			expectMetadata: true,
		},
		{
			name:           "Maps activity with 'treadmill' in title to VirtualRun",
			activityName:   "Treadmill Run",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			typeRules:      `{"treadmill": "VirtualRun"}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN,
			expectMetadata: true,
		},
		{
			name:           "Case-insensitive matching",
			activityName:   "ZWIFT Ride",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
			typeRules:      `{"zwift": "VirtualRide"}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE,
			expectMetadata: true,
		},
		{
			name:           "No matching substring keeps original",
			activityName:   "Weight Training Session",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
			typeRules:      `{"treadmill": "VirtualRun"}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, // no mapping — orchestrator preserves original
			expectMetadata: false,
		},
		{
			name:           "Empty rules does nothing",
			activityName:   "Morning Run",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			typeRules:      "",
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, // no mapping — orchestrator preserves original
			expectMetadata: false,
		},
		{
			name:           "Invalid JSON does nothing",
			activityName:   "Morning Run",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			typeRules:      `{invalid}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, // no mapping — orchestrator preserves original
			expectMetadata: false,
		},
		{
			name:           "Multiple rules - only one matches",
			activityName:   "Outdoor Treadmill Session",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			typeRules:      `{"zwift": "VirtualRide", "treadmill": "VirtualRun"}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN,
			expectMetadata: true,
		},
		{
			// Regression: when several substrings match, the first rule in config order
			// must win. Previously rules were read from a map, whose randomized iteration
			// order made the outcome non-deterministic.
			name:           "Multiple matching rules - first in config order wins",
			activityName:   "Pull focused Hyrox PT",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT,
			typeRules:      `{"pull": "WeightTraining", "Hyrox": "Crossfit"}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
			expectMetadata: true,
		},
		{
			// Same rules, reversed order: the other rule should now win, proving order
			// is honoured rather than incidental.
			name:           "Multiple matching rules - reversed config order",
			activityName:   "Pull focused Hyrox PT",
			activityType:   pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT,
			typeRules:      `{"Hyrox": "Crossfit", "pull": "WeightTraining"}`,
			expectedType:   pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT,
			expectMetadata: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := &pbactivity.StandardizedActivity{
				Name: tt.activityName,
				Type: tt.activityType,
			}
			config := map[string]string{}
			if tt.typeRules != "" {
				config["type_rules"] = tt.typeRules
			}

			res, err := provider.Enrich(ctx, slog.Default(), act, nil, config, false)
			if err != nil {
				t.Fatalf("Enrich failed: %v", err)
			}

			// Type is returned via EnrichmentResult.ActivityType, not by mutating act.Type
			if act.Type != tt.activityType {
				t.Errorf("act.Type should not be mutated: expected %v, got %v", tt.activityType, act.Type)
			}
			if res.ActivityType != tt.expectedType {
				t.Errorf("expected result ActivityType %v, got %v", tt.expectedType, res.ActivityType)
			}

			if tt.expectMetadata {
				expectedStravaName := activity.GetStravaActivityType(tt.expectedType)
				if res.Metadata["new_type"] != expectedStravaName {
					t.Errorf("Metadata new_type expected %s, got %s", expectedStravaName, res.Metadata["new_type"])
				}
				if res.Metadata["matched_pattern"] == "" {
					t.Error("Expected matched_pattern in metadata")
				}
			}
		})
	}
}
