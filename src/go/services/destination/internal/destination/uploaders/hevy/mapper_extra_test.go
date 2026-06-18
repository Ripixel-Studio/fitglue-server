package hevy

import (
	"context"
	"testing"
	"time"

	hevy "github.com/fitglue/server/src/go/pkg/api/hevy"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// resolverWithTemplates builds a resolver whose cache is pre-populated for the
// given exercise names, so ResolveTemplate never makes a network call.
func resolverWithTemplates(names ...string) *TemplateResolver {
	r := NewTemplateResolver("api-key", testLogger())
	r.fetched = true
	for i, name := range names {
		id := "tmpl-" + name
		typ := "weight_reps"
		_ = i
		r.cache[normalizeExerciseName(name)] = &hevy.ExerciseTemplate{
			Id:    strPtr(id),
			Title: strPtr(name),
			Type:  &typ,
		}
	}
	return r
}

func TestConvertStrengthSetExact_RemainingTypes(t *testing.T) {
	t.Run("weight_distance", func(t *testing.T) {
		set := &pbactivity.StrengthSet{WeightKg: 20, DistanceMeters: 100, Reps: 5}
		got := convertStrengthSetExact(set, "weight_distance")
		require.NotNil(t, got.WeightKg)
		require.NotNil(t, got.DistanceMeters)
		assert.Nil(t, got.Reps)
		assert.Nil(t, got.DurationSeconds)
	})

	t.Run("reps_only", func(t *testing.T) {
		set := &pbactivity.StrengthSet{Reps: 12, WeightKg: 50}
		got := convertStrengthSetExact(set, "reps_only")
		require.NotNil(t, got.Reps)
		assert.Equal(t, 12, *got.Reps)
		assert.Nil(t, got.WeightKg)
	})

	t.Run("duration", func(t *testing.T) {
		set := &pbactivity.StrengthSet{DurationSeconds: 60, WeightKg: 50}
		got := convertStrengthSetExact(set, "duration")
		require.NotNil(t, got.DurationSeconds)
		assert.Equal(t, 60, *got.DurationSeconds)
		assert.Nil(t, got.WeightKg)
	})

	t.Run("default sends everything", func(t *testing.T) {
		set := &pbactivity.StrengthSet{WeightKg: 50, Reps: 8, DurationSeconds: 30, DistanceMeters: 10}
		got := convertStrengthSetExact(set, "totally_unknown")
		assert.NotNil(t, got.WeightKg)
		assert.NotNil(t, got.Reps)
		assert.NotNil(t, got.DurationSeconds)
		assert.NotNil(t, got.DistanceMeters)
	})
}

func TestMapStrengthSetsToExercises_GroupsConsecutive(t *testing.T) {
	r := resolverWithTemplates("Deadlift", "Squat")
	sets := []*pbactivity.StrengthSet{
		{ExerciseName: "Deadlift", WeightKg: 100, Reps: 5},
		{ExerciseName: "Deadlift", WeightKg: 100, Reps: 5},
		{ExerciseName: "Squat", WeightKg: 80, Reps: 8},
	}
	exercises, err := mapStrengthSetsToExercises(context.Background(), sets, r, testLogger())
	require.NoError(t, err)
	// Two consecutive Deadlifts group into one exercise with two sets, Squat is its own.
	require.Len(t, exercises, 2)
	assert.Equal(t, "tmpl-Deadlift", *exercises[0].ExerciseTemplateId)
	assert.Len(t, *exercises[0].Sets, 2)
	assert.Equal(t, "tmpl-Squat", *exercises[1].ExerciseTemplateId)
	assert.Len(t, *exercises[1].Sets, 1)
}

func TestMapStrengthSetsToExercises_EmptyNameBecomesUnknown(t *testing.T) {
	r := resolverWithTemplates("Unknown Exercise")
	sets := []*pbactivity.StrengthSet{{WeightKg: 50, Reps: 5}}
	exercises, err := mapStrengthSetsToExercises(context.Background(), sets, r, testLogger())
	require.NoError(t, err)
	require.Len(t, exercises, 1)
	assert.Equal(t, "tmpl-Unknown Exercise", *exercises[0].ExerciseTemplateId)
}

func TestMapCardioSessionToExercise(t *testing.T) {
	r := resolverWithTemplates("Running (Outdoor)")
	session := &pbactivity.Session{TotalDistance: 5000, TotalElapsedTime: 1500}
	ex, err := mapCardioSessionToExercise(context.Background(), pbactivity.ActivityType_ACTIVITY_TYPE_RUN, session, r)
	require.NoError(t, err)
	require.Len(t, *ex.Sets, 1)
	assert.Equal(t, 5000, *(*ex.Sets)[0].DistanceMeters)
	assert.Equal(t, 1500, *(*ex.Sets)[0].DurationSeconds)
}

func TestMapLapToExercise(t *testing.T) {
	t.Run("named lap", func(t *testing.T) {
		r := resolverWithTemplates("Burpees")
		lap := &pbactivity.Lap{ExerciseName: "Burpees", TotalDistance: 0, TotalElapsedTime: 60}
		ex, err := mapLapToExercise(context.Background(), lap, pbactivity.ActivityType_ACTIVITY_TYPE_RUN, r)
		require.NoError(t, err)
		assert.Equal(t, "tmpl-Burpees", *ex.ExerciseTemplateId)
	})

	t.Run("unnamed lap falls back to cardio name", func(t *testing.T) {
		r := resolverWithTemplates("Running (Outdoor)")
		lap := &pbactivity.Lap{TotalDistance: 1000, TotalElapsedTime: 300}
		ex, err := mapLapToExercise(context.Background(), lap, pbactivity.ActivityType_ACTIVITY_TYPE_RUN, r)
		require.NoError(t, err)
		assert.Equal(t, "tmpl-Running (Outdoor)", *ex.ExerciseTemplateId)
	})
}

func TestMapCardioActivityToExercise_DefaultsDuration(t *testing.T) {
	r := resolverWithTemplates("Other Cardio")
	ex, err := mapCardioActivityToExercise(context.Background(), "My Run", "great run", pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, 0, r)
	require.NoError(t, err)
	require.Len(t, *ex.Sets, 1)
	// zero duration defaults to 1800
	assert.Equal(t, 1800, *(*ex.Sets)[0].DurationSeconds)
	require.NotNil(t, ex.Notes)
	assert.Equal(t, "great run", *ex.Notes)
}

func TestMapToHevyWorkout_NoActivityData(t *testing.T) {
	r := resolverWithTemplates("Other Cardio")
	payload := &pbevents.ActivityPayload{
		Timestamp: timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)),
		Metadata: map[string]string{
			"activity_name": "Mystery",
			"description":   "no data",
			"activity_type": "ACTIVITY_TYPE_UNSPECIFIED",
		},
	}
	workout, err := mapToHevyWorkout(context.Background(), payload, r, testLogger(), false)
	require.NoError(t, err)
	require.NotNil(t, workout.Workout)
	assert.Equal(t, "Mystery", *workout.Workout.Title)
	require.NotNil(t, workout.Workout.Exercises)
	assert.Len(t, *workout.Workout.Exercises, 1)
}

func TestMapToHevyWorkout_StrengthSession(t *testing.T) {
	r := resolverWithTemplates("Deadlift")
	payload := &pbevents.ActivityPayload{
		Timestamp: timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)),
		Metadata: map[string]string{
			"activity_name": "Lift Day",
			"activity_type": "ACTIVITY_TYPE_WEIGHT_TRAINING",
		},
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{
					TotalElapsedTime: 3600,
					StrengthSets: []*pbactivity.StrengthSet{
						{ExerciseName: "Deadlift", WeightKg: 100, Reps: 5},
					},
				},
			},
		},
	}
	workout, err := mapToHevyWorkout(context.Background(), payload, r, testLogger(), true)
	require.NoError(t, err)
	require.NotNil(t, workout.Workout.IsPrivate)
	assert.True(t, *workout.Workout.IsPrivate)
	require.Len(t, *workout.Workout.Exercises, 1)
	assert.Equal(t, "tmpl-Deadlift", *(*workout.Workout.Exercises)[0].ExerciseTemplateId)
	// End time should be start + 3600s
	require.NotNil(t, workout.Workout.EndTime)
}

func TestMapToHevyWorkout_CardioSession(t *testing.T) {
	r := resolverWithTemplates("Running (Outdoor)")
	payload := &pbevents.ActivityPayload{
		Timestamp: timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)),
		Metadata: map[string]string{
			"activity_name": "Morning Run",
			"activity_type": "ACTIVITY_TYPE_RUN",
		},
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{TotalElapsedTime: 1800, TotalDistance: 5000},
			},
		},
	}
	workout, err := mapToHevyWorkout(context.Background(), payload, r, testLogger(), false)
	require.NoError(t, err)
	require.Len(t, *workout.Workout.Exercises, 1)
	assert.Equal(t, "tmpl-Running (Outdoor)", *(*workout.Workout.Exercises)[0].ExerciseTemplateId)
}

func TestMapToHevyWorkout_LapsWithExerciseNames(t *testing.T) {
	r := resolverWithTemplates("SkiErg", "Sled Push")
	startTs := timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	payload := &pbevents.ActivityPayload{
		Timestamp: startTs,
		Metadata: map[string]string{
			"activity_name": "Hyrox",
			"activity_type": "ACTIVITY_TYPE_RUN",
		},
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{
					TotalElapsedTime: 3600,
					Laps: []*pbactivity.Lap{
						{ExerciseName: "SkiErg", TotalDistance: 1000, TotalElapsedTime: 300, StartTime: startTs},
						{ExerciseName: "Telemetry", IsTelemetryContainerOnly: true},
						{ExerciseName: "Sled Push", TotalDistance: 50, TotalElapsedTime: 90},
					},
				},
			},
		},
	}
	workout, err := mapToHevyWorkout(context.Background(), payload, r, testLogger(), false)
	require.NoError(t, err)
	// Telemetry-only lap is skipped; SkiErg + Sled Push mapped.
	require.Len(t, *workout.Workout.Exercises, 2)
}

func TestMapToHevyWorkout_LapSkippedWhenMatchingStrengthSet(t *testing.T) {
	r := resolverWithTemplates("Sled Push")
	startTs := timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	payload := &pbevents.ActivityPayload{
		Timestamp: startTs,
		Metadata: map[string]string{
			"activity_name": "Hyrox",
			"activity_type": "ACTIVITY_TYPE_RUN",
		},
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{
					TotalElapsedTime: 3600,
					StrengthSets: []*pbactivity.StrengthSet{
						{ExerciseName: "Sled Push", StartTime: startTs, WeightKg: 100},
					},
					Laps: []*pbactivity.Lap{
						{ExerciseName: "Sled Push", StartTime: startTs, TotalDistance: 50},
					},
				},
			},
		},
	}
	workout, err := mapToHevyWorkout(context.Background(), payload, r, testLogger(), false)
	require.NoError(t, err)
	// The lap matches the existing StrengthSet (same time + name) so only the
	// strength exercise is produced.
	require.Len(t, *workout.Workout.Exercises, 1)
}
