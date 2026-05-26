package hevy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	hevy "github.com/fitglue/server/src/go/pkg/api/hevy"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
)

// mapToHevyWorkout converts an ActivityPayload to Hevy's workout format
func mapToHevyWorkout(ctx context.Context, payload *pbevents.ActivityPayload, resolver *TemplateResolver, logger *slog.Logger, isPrivate bool) (*hevy.PostWorkoutsRequestBody, error) {
	startTime := time.Now()
	if payload.Timestamp != nil {
		startTime = payload.Timestamp.AsTime()
	}

	endTime := startTime
	var totalDuration float64 = 0
	if payload.StandardizedActivity != nil {
		for _, session := range payload.StandardizedActivity.Sessions {
			totalDuration += session.TotalElapsedTime
		}
	}
	if totalDuration > 0 {
		endTime = startTime.Add(time.Duration(totalDuration) * time.Second)
	} else {
		endTime = startTime.Add(30 * time.Minute)
	}

	startTimeStr := startTime.Format(time.RFC3339)
	endTimeStr := endTime.Format(time.RFC3339)
	// isPrivate is passed in from the pipeline config (hevy_is_private metadata key)
	exercises := []hevy.PostWorkoutsRequestExercise{}

	activityName := payload.Metadata["activity_name"]
	description := payload.Metadata["description"]

	activityTypeVal, ok := pbactivity.ActivityType_value[payload.Metadata["activity_type"]]
	activityType := pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED
	if ok {
		activityType = pbactivity.ActivityType(activityTypeVal)
	}

	workout := &hevy.PostWorkoutsRequestBody{
		Workout: &struct {
			Description *string                             `json:"description"`
			EndTime     *string                             `json:"end_time,omitempty"`
			Exercises   *[]hevy.PostWorkoutsRequestExercise `json:"exercises,omitempty"`
			IsPrivate   *bool                               `json:"is_private,omitempty"`
			StartTime   *string                             `json:"start_time,omitempty"`
			Title       *string                             `json:"title,omitempty"`
		}{
			Title:       &activityName,
			Description: &description,
			StartTime:   &startTimeStr,
			EndTime:     &endTimeStr,
			IsPrivate:   &isPrivate,
			Exercises:   &exercises,
		},
	}

	if payload.StandardizedActivity == nil || len(payload.StandardizedActivity.Sessions) == 0 {
		logger.Warn("No activity data available for mapping")
		exercise, err := mapCardioActivityToExercise(ctx, activityName, description, activityType, totalDuration, resolver)
		if err != nil {
			return nil, fmt.Errorf("failed to map cardio activity: %w", err)
		}
		workout.Workout.Exercises = &[]hevy.PostWorkoutsRequestExercise{exercise}
		return workout, nil
	}

	for _, session := range payload.StandardizedActivity.Sessions {
		if len(session.StrengthSets) > 0 {
			exercises, err := mapStrengthSetsToExercises(ctx, session.StrengthSets, resolver, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to map strength sets: %w", err)
			}
			workout.Workout.Exercises = appendExercises(workout.Workout.Exercises, exercises...)
		}

		hasLapExerciseNames := false
		for _, lap := range session.Laps {
			if lap.ExerciseName != "" {
				hasLapExerciseNames = true
				break
			}
		}

		if hasLapExerciseNames {
			for _, lap := range session.Laps {
				if lap.IsTelemetryContainerOnly {
					logger.Debug("Skipping telemetry-only lap for Hevy upload", "exercise_name", lap.ExerciseName)
					continue
				}

				// Skip if a StrengthSet already exists for this exact time and exercise.
				// This prevents double-uploading laps when hybrid_race_tagger
				// or another enricher has already represented it as a StrengthSet.
				hasMatchingSet := false
				for _, set := range session.StrengthSets {
					if set.StartTime != nil && lap.StartTime != nil && set.StartTime.Seconds == lap.StartTime.Seconds && set.ExerciseName == lap.ExerciseName {
						hasMatchingSet = true
						break
					}
				}
				if hasMatchingSet {
					logger.Debug("Skipping lap mapping because a matching StrengthSet exists", "exercise_name", lap.ExerciseName)
					continue
				}

				lapExercise, err := mapLapToExercise(ctx, lap, activityType, resolver)
				if err != nil {
					logger.Warn("Failed to map lap to exercise", "error", err, "exercise_name", lap.ExerciseName)
					continue
				}
				workout.Workout.Exercises = appendExercises(workout.Workout.Exercises, lapExercise)
			}
		} else if len(session.StrengthSets) == 0 && (session.TotalDistance > 0 || session.TotalElapsedTime > 0) {
			cardioExercise, err := mapCardioSessionToExercise(ctx, activityType, session, resolver)
			if err != nil {
				return nil, fmt.Errorf("failed to map cardio session: %w", err)
			}
			workout.Workout.Exercises = appendExercises(workout.Workout.Exercises, cardioExercise)
		}
	}

	if workout.Workout.Exercises == nil || len(*workout.Workout.Exercises) == 0 {
		exercise, err := mapCardioActivityToExercise(ctx, activityName, description, activityType, totalDuration, resolver)
		if err != nil {
			return nil, fmt.Errorf("failed to map fallback activity: %w", err)
		}
		workout.Workout.Exercises = &[]hevy.PostWorkoutsRequestExercise{exercise}
	}

	return workout, nil
}

func mapStrengthSetsToExercises(ctx context.Context, sets []*pbactivity.StrengthSet, resolver *TemplateResolver, logger *slog.Logger) ([]hevy.PostWorkoutsRequestExercise, error) {
	type exerciseGroup struct {
		name string
		sets []*pbactivity.StrengthSet
	}
	var groups []*exerciseGroup

	for _, set := range sets {
		name := set.ExerciseName
		if name == "" {
			name = "Unknown Exercise"
		}

		if len(groups) > 0 && groups[len(groups)-1].name == name {
			groups[len(groups)-1].sets = append(groups[len(groups)-1].sets, set)
		} else {
			groups = append(groups, &exerciseGroup{
				name: name,
				sets: []*pbactivity.StrengthSet{set},
			})
		}
	}

	exercises := []hevy.PostWorkoutsRequestExercise{}
	for _, group := range groups {
		tmpl, err := resolver.ResolveTemplate(ctx, group.name)
		if err != nil {
			logger.Error("Failed to resolve template", "exerciseName", group.name, "error", err)
			return nil, fmt.Errorf("failed to resolve template for %q: %w", group.name, err)
		}

		templateType := "weight_reps"
		if tmpl.Type != nil {
			templateType = *tmpl.Type
		}

		setsCopy := make([]hevy.PostWorkoutsRequestSet, len(group.sets))
		for i, set := range group.sets {
			setsCopy[i] = convertStrengthSetExact(set, templateType)
		}

		exercise := hevy.PostWorkoutsRequestExercise{
			ExerciseTemplateId: tmpl.Id,
			Sets:               &setsCopy,
		}
		exercises = append(exercises, exercise)
	}

	return exercises, nil
}

// convertStrengthSetExact converts a StrengthSet to a Hevy set, populating exactly
// the fields expected by Hevy for the given template type. This guarantees Hevy
// processes the correct data without discarding values.
func convertStrengthSetExact(set *pbactivity.StrengthSet, templateType string) hevy.PostWorkoutsRequestSet {
	setType := hevy.PostWorkoutsRequestSetType(mapSetType(set.SetType))
	hevySet := hevy.PostWorkoutsRequestSet{
		Type: &setType,
	}

	switch templateType {
	case "distance_duration":
		if set.DistanceMeters > 0 {
			distance := int(set.DistanceMeters)
			hevySet.DistanceMeters = &distance
		}
		if set.DurationSeconds > 0 {
			duration := int(set.DurationSeconds)
			hevySet.DurationSeconds = &duration
		}
	case "weight_duration":
		if set.WeightKg > 0 {
			weight := float32(set.WeightKg)
			hevySet.WeightKg = &weight
		}
		if set.DurationSeconds > 0 {
			duration := int(set.DurationSeconds)
			hevySet.DurationSeconds = &duration
		}
	case "weight_distance":
		if set.WeightKg > 0 {
			weight := float32(set.WeightKg)
			hevySet.WeightKg = &weight
		}
		if set.DistanceMeters > 0 {
			distance := int(set.DistanceMeters)
			hevySet.DistanceMeters = &distance
		}
	case "weight_reps":
		if set.WeightKg > 0 {
			weight := float32(set.WeightKg)
			hevySet.WeightKg = &weight
		}
		if set.Reps > 0 {
			reps := int(set.Reps)
			hevySet.Reps = &reps
		}
	case "reps_only":
		if set.Reps > 0 {
			reps := int(set.Reps)
			hevySet.Reps = &reps
		}
	case "duration":
		if set.DurationSeconds > 0 {
			duration := int(set.DurationSeconds)
			hevySet.DurationSeconds = &duration
		}
	default:
		// Attempt to send everything and let Hevy decide
		if set.WeightKg > 0 {
			weight := float32(set.WeightKg)
			hevySet.WeightKg = &weight
		}
		if set.Reps > 0 {
			reps := int(set.Reps)
			hevySet.Reps = &reps
		}
		if set.DurationSeconds > 0 {
			duration := int(set.DurationSeconds)
			hevySet.DurationSeconds = &duration
		}
		if set.DistanceMeters > 0 {
			distance := int(set.DistanceMeters)
			hevySet.DistanceMeters = &distance
		}
	}

	return hevySet
}

func mapSetType(setType string) string {
	switch setType {
	case "warmup":
		return "warmup"
	case "dropset":
		return "dropset"
	case "failure":
		return "failure"
	default:
		return "normal"
	}
}

func mapCardioSessionToExercise(ctx context.Context, activityType pbactivity.ActivityType, session *pbactivity.Session, resolver *TemplateResolver) (hevy.PostWorkoutsRequestExercise, error) {
	exerciseName := getCardioExerciseName(activityType)
	tmpl, err := resolver.ResolveTemplate(ctx, exerciseName)
	if err != nil {
		return hevy.PostWorkoutsRequestExercise{}, fmt.Errorf("failed to resolve cardio template: %w", err)
	}

	distance := int(session.TotalDistance)
	duration := int(session.TotalElapsedTime)
	setType := hevy.PostWorkoutsRequestSetType("normal")

	sets := []hevy.PostWorkoutsRequestSet{{
		Type:            &setType,
		DistanceMeters:  &distance,
		DurationSeconds: &duration,
	}}

	return hevy.PostWorkoutsRequestExercise{
		ExerciseTemplateId: tmpl.Id,
		Sets:               &sets,
	}, nil
}

func mapLapToExercise(ctx context.Context, lap *pbactivity.Lap, fallbackActivityType pbactivity.ActivityType, resolver *TemplateResolver) (hevy.PostWorkoutsRequestExercise, error) {
	exerciseName := lap.ExerciseName
	if exerciseName == "" {
		exerciseName = getCardioExerciseName(fallbackActivityType)
	}

	tmpl, err := resolver.ResolveTemplate(ctx, exerciseName)
	if err != nil {
		return hevy.PostWorkoutsRequestExercise{}, fmt.Errorf("failed to resolve lap template for %q: %w", exerciseName, err)
	}

	distance := int(lap.TotalDistance)
	duration := int(lap.TotalElapsedTime)
	setType := hevy.PostWorkoutsRequestSetType("normal")

	sets := []hevy.PostWorkoutsRequestSet{{
		Type:            &setType,
		DistanceMeters:  &distance,
		DurationSeconds: &duration,
	}}

	return hevy.PostWorkoutsRequestExercise{
		ExerciseTemplateId: tmpl.Id,
		Sets:               &sets,
	}, nil
}

func mapCardioActivityToExercise(ctx context.Context, activityName, description string, activityType pbactivity.ActivityType, durationSeconds float64, resolver *TemplateResolver) (hevy.PostWorkoutsRequestExercise, error) {
	exerciseName := getCardioExerciseName(activityType)
	tmpl, err := resolver.ResolveTemplate(ctx, exerciseName)
	if err != nil {
		return hevy.PostWorkoutsRequestExercise{}, fmt.Errorf("failed to resolve cardio template: %w", err)
	}

	duration := int(durationSeconds)
	if duration == 0 {
		duration = 1800
	}

	setType := hevy.PostWorkoutsRequestSetType("normal")
	sets := []hevy.PostWorkoutsRequestSet{{
		Type:            &setType,
		DurationSeconds: &duration,
	}}

	exercise := hevy.PostWorkoutsRequestExercise{
		ExerciseTemplateId: tmpl.Id,
		Sets:               &sets,
	}
	if description != "" {
		exercise.Notes = &description
	}

	return exercise, nil
}

func getCardioExerciseName(activityType pbactivity.ActivityType) string {
	switch activityType {
	// Running variants
	case pbactivity.ActivityType_ACTIVITY_TYPE_RUN:
		return "Running (Outdoor)"
	case pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN:
		return "Trail Running"
	case pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN:
		return "Running (Treadmill)"

	// Walking / hiking
	case pbactivity.ActivityType_ACTIVITY_TYPE_WALK:
		return "Walking"
	case pbactivity.ActivityType_ACTIVITY_TYPE_HIKE:
		return "Hiking"

	// Cycling variants
	case pbactivity.ActivityType_ACTIVITY_TYPE_RIDE:
		return "Cycling (Outdoor)"
	case pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE:
		return "Cycling (Outdoor)"
	case pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE:
		return "Mountain Biking"
	case pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE:
		return "Cycling (Stationary)"
	case pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE:
		return "Cycling (Outdoor)"
	case pbactivity.ActivityType_ACTIVITY_TYPE_EMOUNTAIN_BIKE_RIDE:
		return "Mountain Biking"

	// Swimming / water sports
	case pbactivity.ActivityType_ACTIVITY_TYPE_SWIM:
		return "Swimming"
	case pbactivity.ActivityType_ACTIVITY_TYPE_ROWING:
		return "Rowing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_ROW:
		return "Rowing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_KAYAKING:
		return "Kayaking"
	case pbactivity.ActivityType_ACTIVITY_TYPE_CANOEING:
		return "Canoeing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_STAND_UP_PADDLING:
		return "Stand Up Paddle Boarding"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SURFING:
		return "Surfing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WINDSURF:
		return "Windsurfing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_KITESURF:
		return "Kitesurfing"

	// Strength / gym
	case pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING:
		return "Weightlifting"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT:
		return "Weightlifting"
	case pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT:
		return "CrossFit"
	case pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING:
		return "HIIT"
	case pbactivity.ActivityType_ACTIVITY_TYPE_ELLIPTICAL:
		return "Elliptical"
	case pbactivity.ActivityType_ACTIVITY_TYPE_STAIR_STEPPER:
		return "Stair Stepper"

	// Mind-body / flexibility
	case pbactivity.ActivityType_ACTIVITY_TYPE_YOGA:
		return "Yoga"
	case pbactivity.ActivityType_ACTIVITY_TYPE_PILATES:
		return "Pilates"

	// Winter sports
	case pbactivity.ActivityType_ACTIVITY_TYPE_ALPINE_SKI:
		return "Alpine Skiing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_BACKCOUNTRY_SKI:
		return "Skiing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_NORDIC_SKI:
		return "Cross Country Skiing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SNOWBOARD:
		return "Snowboarding"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SNOWSHOE:
		return "Snowshoeing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_ICE_SKATE:
		return "Ice Skating"
	case pbactivity.ActivityType_ACTIVITY_TYPE_ROLLER_SKI:
		return "Roller Skiing"
	case pbactivity.ActivityType_ACTIVITY_TYPE_INLINE_SKATE:
		return "Inline Skating"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SKATEBOARD:
		return "Skateboarding"

	// Racket sports
	case pbactivity.ActivityType_ACTIVITY_TYPE_TENNIS:
		return "Tennis"
	case pbactivity.ActivityType_ACTIVITY_TYPE_BADMINTON:
		return "Badminton"
	case pbactivity.ActivityType_ACTIVITY_TYPE_RACQUETBALL:
		return "Racquetball"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SQUASH:
		return "Squash"
	case pbactivity.ActivityType_ACTIVITY_TYPE_PICKLEBALL:
		return "Pickleball"
	case pbactivity.ActivityType_ACTIVITY_TYPE_TABLE_TENNIS:
		return "Table Tennis"

	// Team / field sports
	case pbactivity.ActivityType_ACTIVITY_TYPE_SOCCER:
		return "Football (Soccer)"
	case pbactivity.ActivityType_ACTIVITY_TYPE_GOLF:
		return "Golf"

	// Climbing
	case pbactivity.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING:
		return "Rock Climbing"

	// Handcycle / wheelchair / velomobile
	case pbactivity.ActivityType_ACTIVITY_TYPE_HANDCYCLE:
		return "Cycling (Stationary)"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WHEELCHAIR:
		return "Other Cardio"
	case pbactivity.ActivityType_ACTIVITY_TYPE_VELOMOBILE:
		return "Cycling (Outdoor)"

	// Sailing
	case pbactivity.ActivityType_ACTIVITY_TYPE_SAIL:
		return "Other Cardio"

	default:
		return "Other Cardio"
	}
}

func appendExercises(existing *[]hevy.PostWorkoutsRequestExercise, items ...hevy.PostWorkoutsRequestExercise) *[]hevy.PostWorkoutsRequestExercise {
	if existing == nil {
		return &items
	}
	result := append(*existing, items...)
	return &result
}
