// nolint:proto-json
package sourceplugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	hevyapi "github.com/fitglue/server/src/go/pkg/api/hevy"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func init() {
	register("SOURCE_HEVY", &hevyProvider{})
}

type hevyProvider struct{}

func (p *hevyProvider) newClient(apiKey string) (*hevyapi.ClientWithResponses, error) {
	return hevyapi.NewClientWithResponses(
		"https://api.hevyapp.com",
		hevyapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("api-key", apiKey)
			return nil
		}),
	)
}

func (p *hevyProvider) ListActivities(ctx context.Context, integ *userpb.UserIntegrations, pageToken string) ([]*SourceActivity, string, error) {
	if integ.Hevy == nil || integ.Hevy.ApiKey == "" {
		return nil, "", fmt.Errorf("hevy integration not configured")
	}

	client, err := p.newClient(integ.Hevy.ApiKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create hevy client: %w", err)
	}

	page := 1
	if pageToken != "" {
		if n, err := strconv.Atoi(pageToken); err == nil && n > 0 {
			page = n
		}
	}
	pageSize := 10

	resp, err := client.GetV1WorkoutsWithResponse(ctx, &hevyapi.GetV1WorkoutsParams{
		Page:     &page,
		PageSize: &pageSize,
	})
	if err != nil {
		return nil, "", fmt.Errorf("hevy api error: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, "", fmt.Errorf("hevy api error: status=%d body=%s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		return nil, "", nil
	}

	var activities []*SourceActivity
	if resp.JSON200.Workouts != nil {
		for _, w := range *resp.JSON200.Workouts {
			a := &SourceActivity{Type: "WeightTraining"}
			if w.Id != nil {
				a.ID = *w.Id
			}
			if w.Title != nil {
				a.Title = *w.Title
			}
			if w.StartTime != nil {
				if t, err := time.Parse(time.RFC3339, *w.StartTime); err == nil {
					a.StartTime = t
				}
			}
			activities = append(activities, a)
		}
	}

	var nextPageToken string
	if resp.JSON200.Page != nil && resp.JSON200.PageCount != nil && *resp.JSON200.Page < *resp.JSON200.PageCount {
		nextPageToken = strconv.Itoa(*resp.JSON200.Page + 1)
	}

	return activities, nextPageToken, nil
}

func (p *hevyProvider) FetchActivity(ctx context.Context, integ *userpb.UserIntegrations, userID string, sourceActivityID string) (*pbevents.ActivityPayload, error) {
	if integ.Hevy == nil || integ.Hevy.ApiKey == "" {
		return nil, fmt.Errorf("hevy integration not configured")
	}

	client, err := p.newClient(integ.Hevy.ApiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create hevy client: %w", err)
	}

	workoutUUID, err := uuid.Parse(sourceActivityID)
	if err != nil {
		return nil, fmt.Errorf("invalid hevy workout id %q: %w", sourceActivityID, err)
	}

	resp, err := client.GetV1WorkoutsWorkoutIdWithResponse(ctx, workoutUUID, &hevyapi.GetV1WorkoutsWorkoutIdParams{})
	if err != nil {
		return nil, fmt.Errorf("hevy api error: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("hevy api error: status=%d body=%s", resp.StatusCode(), string(resp.Body))
	}

	stdActivity, err := hevyMapToStandardizedActivity(resp.Body, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hevy workout: %w", err)
	}

	return &pbevents.ActivityPayload{
		Source:               activitypb.ActivitySource_SOURCE_HEVY,
		UserId:               userID,
		OriginalPayloadJson:  string(resp.Body),
		ActivityId:           &sourceActivityID,
		StandardizedActivity: stdActivity,
	}, nil
}

// HevyMapToStandardizedActivity maps a Hevy workout JSON body (bare or {"workout": …})
// onto a StandardizedActivity with strength sets. Exported for cmd/showcase-reboost.
func HevyMapToStandardizedActivity(rawJSON []byte, userID string) (*activitypb.StandardizedActivity, error) {
	return hevyMapToStandardizedActivity(rawJSON, userID)
}

func hevyMapToStandardizedActivity(rawJSON []byte, userID string) (*activitypb.StandardizedActivity, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	var workout hevyapi.Workout
	if workoutObj, ok := raw["workout"]; ok {
		b, err := json.Marshal(workoutObj)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &workout); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(rawJSON, &workout); err != nil {
			return nil, err
		}
	}

	act := &activitypb.StandardizedActivity{
		Source: activitypb.ActivitySource_SOURCE_HEVY,
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
		if t, err := time.Parse(time.RFC3339, *workout.StartTime); err == nil {
			startTime = t
		}
	}
	if startTime.IsZero() {
		startTime = time.Now()
	}
	act.StartTime = timestamppb.New(startTime)

	session := &activitypb.Session{StartTime: act.StartTime}
	if workout.EndTime != nil {
		if endTime, err := time.Parse(time.RFC3339, *workout.EndTime); err == nil {
			session.TotalElapsedTime = float64(endTime.Sub(startTime).Seconds())
		}
	}

	if workout.Exercises != nil {
		for _, ex := range *workout.Exercises {
			exName := ""
			if ex.Title != nil {
				exName = *ex.Title
			}
			supersetID := ""
			if ex.SupersetId != nil {
				supersetID = fmt.Sprintf("%d", int(*ex.SupersetId))
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
						SupersetId:   supersetID,
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

	if session.TotalElapsedTime <= 0 {
		session.TotalElapsedTime = 60
	}
	act.Sessions = []*activitypb.Session{session}

	return act, nil
}
