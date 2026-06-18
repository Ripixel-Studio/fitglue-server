package hevy

import (
	"testing"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapToStandardizedActivity_FlatWorkout(t *testing.T) {
	// No "workout" wrapper key — the flat-unmarshal branch.
	raw := []byte(`{
		"id": "flat-1",
		"title": "Leg Day",
		"description": "heavy",
		"start_time": "2025-12-29T08:00:00.000Z",
		"end_time": "2025-12-29T08:30:00.000Z",
		"exercises": [
			{
				"title": "Squat",
				"superset_id": 2,
				"notes": "deep",
				"sets": [
					{"reps": 5, "weight_kg": 100, "distance_meters": 0, "duration_seconds": 0, "type": "normal"}
				]
			}
		]
	}`)

	act, err := mapToStandardizedActivity(raw, "u1", activitypb.ActivitySource_SOURCE_HEVY)
	require.NoError(t, err)
	assert.Equal(t, "flat-1", act.ExternalId)
	assert.Equal(t, "Leg Day", act.Name)
	assert.Equal(t, "heavy", act.Description)
	require.Len(t, act.Sessions, 1)
	assert.Equal(t, float64(1800), act.Sessions[0].TotalElapsedTime)
	require.Len(t, act.Sessions[0].StrengthSets, 1)
	set := act.Sessions[0].StrengthSets[0]
	assert.Equal(t, "Squat", set.ExerciseName)
	assert.Equal(t, "deep", set.Notes)
	assert.Equal(t, "2", set.SupersetId)
	assert.Equal(t, "normal", set.SetType)
	assert.Equal(t, int32(5), set.Reps)
}

func TestMapToStandardizedActivity_NoEndTimeFallbackToSetDurations(t *testing.T) {
	// No end_time -> TotalElapsedTime computed from set durations.
	raw := []byte(`{
		"workout": {
			"id": "2",
			"title": "Quick",
			"start_time": "2025-12-29T08:00:00.000Z",
			"exercises": [
				{"title": "Curl", "sets": [
					{"reps": 8, "duration_seconds": 45},
					{"reps": 8, "duration_seconds": 45}
				]}
			]
		}
	}`)

	act, err := mapToStandardizedActivity(raw, "u1", activitypb.ActivitySource_SOURCE_HEVY)
	require.NoError(t, err)
	require.Len(t, act.Sessions, 1)
	assert.Equal(t, float64(90), act.Sessions[0].TotalElapsedTime)
}

func TestMapToStandardizedActivity_NoSetsDefaultsToOneMinute(t *testing.T) {
	// No exercises/sets and no end_time -> default 60s elapsed.
	raw := []byte(`{"workout": {"id": "3", "title": "Empty"}}`)

	act, err := mapToStandardizedActivity(raw, "u1", activitypb.ActivitySource_SOURCE_HEVY)
	require.NoError(t, err)
	require.Len(t, act.Sessions, 1)
	assert.Equal(t, float64(60), act.Sessions[0].TotalElapsedTime)
}

func TestMapToStandardizedActivity_SetsWithoutDurationFallbackPerSet(t *testing.T) {
	// Sets without durations -> 60s fallback per set.
	raw := []byte(`{
		"workout": {
			"id": "4",
			"exercises": [
				{"title": "Push", "sets": [{"reps": 10}, {"reps": 10}, {"reps": 10}]}
			]
		}
	}`)

	act, err := mapToStandardizedActivity(raw, "u1", activitypb.ActivitySource_SOURCE_HEVY)
	require.NoError(t, err)
	assert.Equal(t, float64(180), act.Sessions[0].TotalElapsedTime)
}

func TestMapToStandardizedActivity_InvalidJSON(t *testing.T) {
	_, err := mapToStandardizedActivity([]byte(`{not json`), "u1", activitypb.ActivitySource_SOURCE_HEVY)
	assert.ErrorContains(t, err, "failed to unmarshal raw json")
}
