package hevy

import (
	"testing"

	hevy "github.com/fitglue/server/src/go/pkg/api/hevy"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapSetType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"warmup", "warmup"},
		{"dropset", "dropset"},
		{"failure", "failure"},
		{"normal", "normal"},
		{"", "normal"},
		{"unknown", "normal"},
		{"WARMUP", "normal"}, // case sensitive
		{"drop_set", "normal"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := mapSetType(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetCardioExerciseName(t *testing.T) {
	tests := []struct {
		activityType pbactivity.ActivityType
		expected     string
	}{
		{pbactivity.ActivityType_ACTIVITY_TYPE_RUN, "Running (Outdoor)"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WALK, "Walking"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, "Cycling (Outdoor)"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SWIM, "Swimming"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING, "Weightlifting"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, "Other Cardio"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_HIKE, "Hiking"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_YOGA, "Yoga"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_PILATES, "Pilates"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT, "CrossFit"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING, "HIIT"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN, "Trail Running"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN, "Running (Treadmill)"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE, "Cycling (Stationary)"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_ROWING, "Rowing"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SNOWBOARD, "Snowboarding"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_TENNIS, "Tennis"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SOCCER, "Football (Soccer)"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING, "Rock Climbing"},
	}

	for _, tc := range tests {
		t.Run(tc.activityType.String(), func(t *testing.T) {
			result := getCardioExerciseName(tc.activityType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestAppendExercises(t *testing.T) {
	t.Run("NilExistingSlice", func(t *testing.T) {
		e1 := hevy.PostWorkoutsRequestExercise{}
		result := appendExercises(nil, e1)
		require.NotNil(t, result)
		assert.Len(t, *result, 1)
	})

	t.Run("EmptyExistingSlice", func(t *testing.T) {
		existing := []hevy.PostWorkoutsRequestExercise{}
		e1 := hevy.PostWorkoutsRequestExercise{}
		e2 := hevy.PostWorkoutsRequestExercise{}
		result := appendExercises(&existing, e1, e2)
		require.NotNil(t, result)
		assert.Len(t, *result, 2)
	})

	t.Run("ExistingWithItems", func(t *testing.T) {
		id1 := "existing"
		id2 := "appended"
		existing := []hevy.PostWorkoutsRequestExercise{{ExerciseTemplateId: &id1}}
		e := hevy.PostWorkoutsRequestExercise{ExerciseTemplateId: &id2}
		result := appendExercises(&existing, e)
		require.NotNil(t, result)
		assert.Len(t, *result, 2)
		assert.Equal(t, "existing", *(*result)[0].ExerciseTemplateId)
		assert.Equal(t, "appended", *(*result)[1].ExerciseTemplateId)
	})

	t.Run("AppendMultiple", func(t *testing.T) {
		existing := []hevy.PostWorkoutsRequestExercise{}
		items := []hevy.PostWorkoutsRequestExercise{{}, {}, {}}
		result := appendExercises(&existing, items...)
		assert.Len(t, *result, 3)
	})
}

func TestConvertStrengthSetExact(t *testing.T) {
	// weight_duration exercises: Hevy accepts weight + duration only.
	// Weighted stations (Sled Push/Pull, Farmers Carry, Sandbag Lunges, Wall Balls) use this type
	// so the variable factor (weight) is recorded. Preset distances are fixed and recorded in descriptions.
	t.Run("SledPush_WeightDuration_ExcludesDistanceAndReps", func(t *testing.T) {
		set := &pbactivity.StrengthSet{
			ExerciseName:    "Sled Push",
			SetType:         "normal",
			WeightKg:        152,
			DistanceMeters:  50,
			DurationSeconds: 90,
			Reps:            0,
		}
		result := convertStrengthSetExact(set, "weight_duration")
		require.NotNil(t, result.Type)
		assert.Equal(t, hevy.PostWorkoutsRequestSetType("normal"), *result.Type)
		// weight + duration must be present
		require.NotNil(t, result.WeightKg)
		assert.InDelta(t, 152.0, float64(*result.WeightKg), 0.01)
		require.NotNil(t, result.DurationSeconds)
		assert.Equal(t, 90, *result.DurationSeconds)
		// distance and reps must be absent for weight_duration type
		assert.Nil(t, result.DistanceMeters, "distance must not be sent for weight_duration exercises")
		assert.Nil(t, result.Reps, "reps must not be sent for weight_duration exercises")
	})

	t.Run("FarmersCarry_WeightDuration_ExcludesDistanceAndReps", func(t *testing.T) {
		set := &pbactivity.StrengthSet{
			ExerciseName:    "Farmers Carry",
			SetType:         "normal",
			WeightKg:        48,
			DistanceMeters:  200,
			DurationSeconds: 120,
		}
		result := convertStrengthSetExact(set, "weight_duration")
		require.NotNil(t, result.WeightKg)
		assert.InDelta(t, 48.0, float64(*result.WeightKg), 0.01)
		require.NotNil(t, result.DurationSeconds)
		assert.Equal(t, 120, *result.DurationSeconds)
		assert.Nil(t, result.DistanceMeters)
		assert.Nil(t, result.Reps)
	})

	t.Run("SkiErg_DistanceDuration_ExcludesWeightAndReps", func(t *testing.T) {
		set := &pbactivity.StrengthSet{
			ExerciseName:    "SkiErg",
			SetType:         "normal",
			WeightKg:        0,
			DistanceMeters:  1000,
			DurationSeconds: 300,
		}
		result := convertStrengthSetExact(set, "distance_duration")
		require.NotNil(t, result.DistanceMeters)
		assert.Equal(t, 1000, *result.DistanceMeters)
		require.NotNil(t, result.DurationSeconds)
		assert.Equal(t, 300, *result.DurationSeconds)
		assert.Nil(t, result.WeightKg)
		assert.Nil(t, result.Reps)
	})

	// weight_duration exercises: Hevy accepts weight + duration only.
	// Reps and distance must NOT be sent even when populated on the StrengthSet.
	t.Run("WallBalls_WeightDuration_ExcludesRepsAndDistance", func(t *testing.T) {
		set := &pbactivity.StrengthSet{
			ExerciseName:    "Wall Balls",
			SetType:         "normal",
			WeightKg:        9,
			Reps:            100,
			DistanceMeters:  0,
			DurationSeconds: 240,
		}
		result := convertStrengthSetExact(set, "weight_duration")
		// weight + duration must be present
		require.NotNil(t, result.WeightKg)
		assert.InDelta(t, 9.0, float64(*result.WeightKg), 0.01)
		require.NotNil(t, result.DurationSeconds)
		assert.Equal(t, 240, *result.DurationSeconds)
		// reps and distance must be absent for weight_duration type
		assert.Nil(t, result.Reps, "reps must not be sent for weight_duration exercises")
		assert.Nil(t, result.DistanceMeters, "distance must not be sent for weight_duration exercises")
	})

	// weight_reps (default): accepts weight + reps, duration carried through.
	t.Run("GenericStrength_WeightReps", func(t *testing.T) {
		set := &pbactivity.StrengthSet{
			ExerciseName:    "Deadlift",
			SetType:         "warmup",
			WeightKg:        80.5,
			Reps:            10,
			DistanceMeters:  0,
			DurationSeconds: 0,
		}
		result := convertStrengthSetExact(set, "weight_reps")
		require.NotNil(t, result.Type)
		assert.Equal(t, hevy.PostWorkoutsRequestSetType("warmup"), *result.Type)
		require.NotNil(t, result.WeightKg)
		assert.InDelta(t, 80.5, float64(*result.WeightKg), 0.01)
		require.NotNil(t, result.Reps)
		assert.Equal(t, 10, *result.Reps)
		assert.Nil(t, result.DistanceMeters)
		assert.Nil(t, result.DurationSeconds)
	})

	t.Run("ZeroFields_WeightReps", func(t *testing.T) {
		set := &pbactivity.StrengthSet{
			SetType:         "normal",
			WeightKg:        0,
			Reps:            0,
			DistanceMeters:  0,
			DurationSeconds: 0,
		}
		result := convertStrengthSetExact(set, "weight_reps")
		require.NotNil(t, result.Type)
		assert.Equal(t, hevy.PostWorkoutsRequestSetType("normal"), *result.Type)
		// Zero values should NOT be set (only set when > 0)
		assert.Nil(t, result.WeightKg)
		assert.Nil(t, result.Reps)
		assert.Nil(t, result.DistanceMeters)
		assert.Nil(t, result.DurationSeconds)
	})

	t.Run("DropsetType", func(t *testing.T) {
		set := &pbactivity.StrengthSet{SetType: "dropset"}
		result := convertStrengthSetExact(set, "weight_reps")
		require.NotNil(t, result.Type)
		assert.Equal(t, hevy.PostWorkoutsRequestSetType("dropset"), *result.Type)
	})

	t.Run("UnknownSetType", func(t *testing.T) {
		set := &pbactivity.StrengthSet{SetType: "custom"}
		result := convertStrengthSetExact(set, "weight_reps")
		require.NotNil(t, result.Type)
		assert.Equal(t, hevy.PostWorkoutsRequestSetType("normal"), *result.Type)
	})
}
