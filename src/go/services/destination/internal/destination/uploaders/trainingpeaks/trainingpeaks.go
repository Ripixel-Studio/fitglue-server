// nolint:proto-json
package trainingpeaks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/description"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	httputil "github.com/fitglue/server/src/go/pkg/infrastructure/http"
	"github.com/fitglue/server/src/go/pkg/infrastructure/oauth"
	"github.com/fitglue/server/src/go/pkg/loopprevention"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Uploader implements destination.Destination for TrainingPeaks
type Uploader struct {
	svc *bootstrap.Service
}

// New returns a new TrainingPeaks Uploader initialized with dependencies.
func New(svc *bootstrap.Service) *Uploader {
	return &Uploader{
		svc: svc,
	}
}

// Name returns the identifier for this uploader
func (u *Uploader) Name() string {
	return "trainingpeaks"
}

// Create uploads a new activity to TrainingPeaks.
func (u *Uploader) Create(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record) (string, error) {
	if userRec.Integrations == nil || userRec.Integrations.Trainingpeaks == nil || !userRec.Integrations.Trainingpeaks.Enabled {
		return "", fmt.Errorf("user has no TrainingPeaks integration configured")
	}

	athleteID := userRec.Integrations.Trainingpeaks.AthleteId
	tokenSource := oauth.NewFirestoreTokenSource(u.svc, payload.UserId, "trainingpeaks")
	httpClient := oauth.NewClientWithUsageTracking(tokenSource, u.svc, payload.UserId, "trainingpeaks", infra.NewLogger())
	logger := slog.Default()

	workout := buildTrainingPeaksWorkout(payload)

	workoutID, err := u.createTrainingPeaksWorkout(ctx, httpClient, athleteID, workout, logger)
	if err != nil {
		return "", fmt.Errorf("failed to create TrainingPeaks workout: %w", err)
	}

	if workoutID != "" {
		uploadRecord := &pbactivity.UploadedActivityRecord{
			Id:            loopprevention.BuildUploadedActivityID(pbplugin.DestinationType_DESTINATION_TRAININGPEAKS, workoutID),
			UserId:        payload.UserId,
			Source:        payload.Source,
			ExternalId:    payload.StandardizedActivity.GetExternalId(),
			StartTime:     payload.Timestamp,
			Destination:   pbplugin.DestinationType_DESTINATION_TRAININGPEAKS,
			DestinationId: workoutID,
			UploadedAt:    timestamppb.Now(),
		}
		_ = u.svc.DB.SetUploadedActivity(ctx, payload.UserId, uploadRecord)
	}

	return workoutID, nil
}

// Update modifies an existing TrainingPeaks activity.
func (u *Uploader) Update(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record, pipelineRun *pbpipeline.PipelineRun) error {
	if userRec.Integrations == nil || userRec.Integrations.Trainingpeaks == nil || !userRec.Integrations.Trainingpeaks.Enabled {
		return fmt.Errorf("user has no TrainingPeaks integration configured")
	}

	athleteID := userRec.Integrations.Trainingpeaks.AthleteId
	logger := slog.Default()

	var workoutIDStr string
	if pipelineRun != nil {
		for _, dest := range pipelineRun.Destinations {
			if dest.Destination == pbplugin.DestinationType_DESTINATION_TRAININGPEAKS && dest.ExternalId != nil && *dest.ExternalId != "" {
				workoutIDStr = *dest.ExternalId
				break
			}
		}
	}
	if workoutIDStr == "" {
		return fmt.Errorf("no TrainingPeaks destination found in pipeline run")
	}

	existingDescription := pipelineRun.Description
	payloadDesc := payload.Metadata["description"]
	mergedDescription := existingDescription
	if payloadDesc != "" {
		sectionHeader := ""
		for key, val := range payload.Metadata {
			if strings.HasPrefix(key, "section_header_") {
				sectionHeader = val
				break
			}
		}

		if sectionHeader != "" && description.HasSection(mergedDescription, sectionHeader) {
			newSectionContent := description.ExtractSection(payloadDesc, sectionHeader)
			if newSectionContent != "" {
				mergedDescription = description.ReplaceSection(mergedDescription, sectionHeader, newSectionContent)
			}
		} else if mergedDescription != "" {
			mergedDescription += "\n\n" + payloadDesc
		} else {
			mergedDescription = payloadDesc
		}
	}

	updatePayload := &TrainingPeaksWorkout{}
	hasChanges := false

	if mergedDescription != existingDescription {
		updatePayload.Description = mergedDescription
		hasChanges = true
	}

	if !hasChanges {
		return nil
	}

	tokenSource := oauth.NewFirestoreTokenSource(u.svc, payload.UserId, "trainingpeaks")
	httpClient := oauth.NewClientWithUsageTracking(tokenSource, u.svc, payload.UserId, "trainingpeaks", infra.NewLogger())

	if err := u.updateTrainingPeaksWorkout(ctx, httpClient, athleteID, workoutIDStr, updatePayload, logger); err != nil {
		return fmt.Errorf("failed to update TrainingPeaks workout: %w", err)
	}

	return nil
}

type TrainingPeaksWorkout struct {
	Title            string  `json:"title,omitempty"`
	Description      string  `json:"description,omitempty"`
	WorkoutType      string  `json:"workoutType,omitempty"`
	StartDate        string  `json:"startDate,omitempty"`
	TotalTimePlanned float64 `json:"totalTimePlanned,omitempty"`
	DistancePlanned  float64 `json:"distancePlanned,omitempty"`
	HeartRateAvg     *int    `json:"heartRateAvg,omitempty"`
	HeartRateMax     *int    `json:"heartRateMax,omitempty"`
}

func buildTrainingPeaksWorkout(payload *pbevents.ActivityPayload) *TrainingPeaksWorkout {
	activityName := payload.Metadata["activity_name"]
	description := payload.Metadata["description"]

	activityTypeVal, ok := pbactivity.ActivityType_value[payload.Metadata["activity_type"]]
	activityType := pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED
	if ok {
		activityType = pbactivity.ActivityType(activityTypeVal)
	}

	workout := &TrainingPeaksWorkout{
		Title:       activityName,
		Description: description,
		WorkoutType: mapToTrainingPeaksType(activityType),
	}

	if payload.Timestamp != nil {
		workout.StartDate = payload.Timestamp.AsTime().Format(time.RFC3339)
	}

	if payload.StandardizedActivity != nil && len(payload.StandardizedActivity.Sessions) > 0 {
		var totalDuration float64
		var totalDistance float64
		var hrSum, hrCount int
		var hrMax int

		for _, session := range payload.StandardizedActivity.Sessions {
			totalDuration += session.TotalElapsedTime
			totalDistance += session.TotalDistance

			for _, lap := range session.Laps {
				for _, record := range lap.Records {
					if record.HeartRate > 0 {
						hrSum += int(record.HeartRate)
						hrCount++
						if int(record.HeartRate) > hrMax {
							hrMax = int(record.HeartRate)
						}
					}
				}
			}
		}

		if totalDuration > 0 {
			workout.TotalTimePlanned = totalDuration
		}
		if totalDistance > 0 {
			workout.DistancePlanned = totalDistance
		}
		if hrCount > 0 {
			avgHR := hrSum / hrCount
			workout.HeartRateAvg = &avgHR
			workout.HeartRateMax = &hrMax
		}
	}

	return workout
}

func mapToTrainingPeaksType(activityType pbactivity.ActivityType) string {
	switch activityType {
	case pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN:
		return "Run"
	case pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_EMOUNTAIN_BIKE_RIDE:
		return "Bike"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SWIM:
		return "Swim"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
		pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT,
		pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING:
		return "Strength"
	default:
		return "Other"
	}
}

func (u *Uploader) createTrainingPeaksWorkout(ctx context.Context, httpClient *http.Client, athleteID string, workout *TrainingPeaksWorkout, logger *slog.Logger) (string, error) {
	bodyJSON, err := json.Marshal(workout)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workout: %w", err)
	}

	url := fmt.Sprintf("https://api.trainingpeaks.com/v1/athlete/%s/workouts", athleteID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("TrainingPeaks API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", httputil.WrapResponseError(resp, "TrainingPeaks API error")
	}

	var respBody struct {
		ID        interface{} `json:"Id"`
		WorkoutId interface{} `json:"workoutId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", fmt.Errorf("failed to decode TrainingPeaks response: %w", err)
	}

	workoutID := ""
	if respBody.ID != nil {
		workoutID = fmt.Sprintf("%v", respBody.ID)
	} else if respBody.WorkoutId != nil {
		workoutID = fmt.Sprintf("%v", respBody.WorkoutId)
	}

	if workoutID == "" {
		return "", fmt.Errorf("no workout ID in TrainingPeaks response")
	}

	return workoutID, nil
}

func (u *Uploader) updateTrainingPeaksWorkout(ctx context.Context, httpClient *http.Client, athleteID, workoutID string, workout *TrainingPeaksWorkout, logger *slog.Logger) error {
	bodyJSON, err := json.Marshal(workout)
	if err != nil {
		return fmt.Errorf("failed to marshal workout update: %w", err)
	}

	url := fmt.Sprintf("https://api.trainingpeaks.com/v1/athlete/%s/workouts/%s", athleteID, workoutID)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create PUT request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("TrainingPeaks API PUT request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return httputil.WrapResponseError(resp, "TrainingPeaks PUT error")
	}

	return nil
}
