// nolint:proto-json
package hevy

import (
	"encoding/json"
	"fmt"
	"time"

	hevyapi "github.com/fitglue/server/src/go/pkg/api/hevy"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func mapToStandardizedActivity(rawJSON []byte, userID string, source activitypb.ActivitySource) (*activitypb.StandardizedActivity, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw json: %w", err)
	}

	var workout hevyapi.Workout
	if workoutObj, ok := raw["workout"]; ok {
		b, err := json.Marshal(workoutObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal inner workout: %w", err)
		}
		if err := json.Unmarshal(b, &workout); err != nil {
			return nil, fmt.Errorf("failed to unmarshal inner workout: %w", err)
		}
	} else {
		if err := json.Unmarshal(rawJSON, &workout); err != nil {
			return nil, fmt.Errorf("failed to unmarshal flat workout: %w", err)
		}
	}

	act := &activitypb.StandardizedActivity{
		Source: source,
		UserId: userID,
		Type:   activitypb.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
	}

	if workout.Id != nil {
		act.ExternalId = *workout.Id
	}
	if workout.Title != nil {
		act.Name = *workout.Title
	}
	if workout.Description != nil {
		act.Description = *workout.Description
	}

	var startTime time.Time
	if workout.StartTime != nil {
		t, err := time.Parse(time.RFC3339, *workout.StartTime)
		if err == nil {
			startTime = t
		}
	}
	// Fallback to current time if no start time is parsed
	if startTime.IsZero() {
		startTime = time.Now()
	}

	act.StartTime = timestamppb.New(startTime)

	session := &activitypb.Session{
		StartTime: act.StartTime,
	}

	if workout.EndTime != nil && !startTime.IsZero() {
		endTime, err := time.Parse(time.RFC3339, *workout.EndTime)
		if err == nil {
			session.TotalElapsedTime = float64(endTime.Sub(startTime).Seconds())
		}
	}

	if workout.Exercises != nil {
		for _, ex := range *workout.Exercises {
			exName := ""
			if ex.Title != nil {
				exName = *ex.Title
			}

			supersetId := ""
			if ex.SupersetId != nil {
				supersetId = fmt.Sprintf("%d", int(*ex.SupersetId))
			}
			notes := ""
			if ex.Notes != nil {
				notes = *ex.Notes
			}

			if ex.Sets != nil {
				for _, s := range *ex.Sets {
					set := &activitypb.StrengthSet{
						ExerciseName: exName,
						Notes:        notes,
						SupersetId:   supersetId,
					}
					if s.Reps != nil {
						set.Reps = int32(*s.Reps)
					}
					if s.WeightKg != nil {
						set.WeightKg = float64(*s.WeightKg)
					}
					if s.DistanceMeters != nil {
						set.DistanceMeters = float64(*s.DistanceMeters)
					}
					if s.DurationSeconds != nil {
						set.DurationSeconds = int32(*s.DurationSeconds)
					}
					if s.Type != nil {
						set.SetType = *s.Type
					}
					session.StrengthSets = append(session.StrengthSets, set)
				}
			}
		}
	}

	// Fallback for logical duration if missing
	if session.TotalElapsedTime == 0 && len(session.StrengthSets) > 0 {
		var total float64 = 0
		for _, s := range session.StrengthSets {
			if s.DurationSeconds > 0 {
				total += float64(s.DurationSeconds)
			} else {
				total += 60 // fallback per set
			}
		}
		session.TotalElapsedTime = total
	}

	// If it's still 0, orchestrator requires >0. Thus default to 1 min.
	if session.TotalElapsedTime <= 0 {
		session.TotalElapsedTime = 60
	}

	act.Sessions = []*activitypb.Session{session}

	return act, nil
}
