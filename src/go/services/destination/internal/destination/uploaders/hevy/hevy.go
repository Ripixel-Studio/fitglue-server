// nolint:proto-json
package hevy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	hevyapi "github.com/fitglue/server/src/go/pkg/api/hevy"
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

// Uploader implements destination.Destination for Hevy
type Uploader struct {
	svc *bootstrap.Service
}

// New returns a new Hevy Uploader initialized with dependencies.
func New(svc *bootstrap.Service) *Uploader {
	return &Uploader{
		svc: svc,
	}
}

// Name returns the identifier for this uploader
func (u *Uploader) Name() string {
	return "hevy"
}

// Create uploads a new activity to Hevy.
func (u *Uploader) Create(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record) (string, error) {
	if userRec.Integrations == nil || userRec.Integrations.Hevy == nil || userRec.Integrations.Hevy.ApiKey == "" {
		return "", fmt.Errorf("user has no Hevy API key configured")
	}
	apiKey := userRec.Integrations.Hevy.ApiKey
	logger := slog.Default()

	isPrivate := false
	if val, ok := payload.Metadata["hevy_is_private"]; ok && val == "true" {
		isPrivate = true
	}

	resolver := NewTemplateResolver(apiKey, logger)
	workout, err := mapToHevyWorkout(ctx, payload, resolver, logger, isPrivate)
	if err != nil {
		return "", fmt.Errorf("failed to map activity to Hevy format: %w", err)
	}

	// Pre-store a pending bounceback record BEFORE calling Hevy so that the
	// IsBounceback check succeeds even if Hevy fires the webhook before we receive
	// the workout ID back. Keyed by startTime because the real Hevy ID is unknown yet.
	if payload.Timestamp != nil {
		pendingId := fmt.Sprintf("pending:%d", payload.Timestamp.AsTime().Unix())
		preRecord := &pbactivity.UploadedActivityRecord{
			Id:            loopprevention.BuildUploadedActivityID(pbplugin.DestinationType_DESTINATION_HEVY, pendingId),
			UserId:        payload.UserId,
			Source:        payload.Source,
			ExternalId:    payload.StandardizedActivity.GetExternalId(),
			StartTime:     payload.Timestamp,
			Destination:   pbplugin.DestinationType_DESTINATION_HEVY,
			DestinationId: pendingId,
			UploadedAt:    timestamppb.Now(),
		}
		if err := u.svc.DB.SetUploadedActivity(ctx, payload.UserId, preRecord); err != nil {
			logger.WarnContext(ctx, "Failed to pre-store Hevy pending bounceback record; webhook may re-trigger", "error", err)
		}
	}

	workoutID, err := u.createHevyWorkout(ctx, apiKey, workout, logger)
	if err != nil {
		return "", fmt.Errorf("failed to create Hevy workout: %w", err)
	}

	if workoutID == "" {
		// Hevy returned success but no workout ID — log so we can diagnose bounceback failures.
		logger.WarnContext(ctx, "Hevy Create returned empty workoutID; bounceback record not stored")
	} else {
		uploadRecord := &pbactivity.UploadedActivityRecord{
			Id:            loopprevention.BuildUploadedActivityID(pbplugin.DestinationType_DESTINATION_HEVY, workoutID),
			UserId:        payload.UserId,
			Source:        payload.Source,
			ExternalId:    payload.StandardizedActivity.GetExternalId(),
			StartTime:     payload.Timestamp,
			Destination:   pbplugin.DestinationType_DESTINATION_HEVY,
			DestinationId: workoutID,
			UploadedAt:    timestamppb.Now(),
		}
		if err := u.svc.DB.SetUploadedActivity(ctx, payload.UserId, uploadRecord); err != nil {
			logger.WarnContext(ctx, "Failed to store Hevy bounceback record; webhook may re-trigger", "workout_id", workoutID, "error", err)
		}
	}

	return workoutID, nil
}

// Update modifies an existing Hevy activity.
func (u *Uploader) Update(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record, pipelineRun *pbpipeline.PipelineRun) error {
	if userRec.Integrations == nil || userRec.Integrations.Hevy == nil || userRec.Integrations.Hevy.ApiKey == "" {
		return fmt.Errorf("user has no Hevy API key configured")
	}
	apiKey := userRec.Integrations.Hevy.ApiKey
	logger := slog.Default()

	isSameSource := false
	if val, ok := payload.Metadata["same_source_destination_hevy"]; ok && val == "true" {
		isSameSource = true
	}

	var workoutID string
	if pipelineRun != nil {
		for _, dest := range pipelineRun.Destinations {
			if dest.Destination == pbplugin.DestinationType_DESTINATION_HEVY && dest.ExternalId != nil && *dest.ExternalId != "" {
				workoutID = *dest.ExternalId
				break
			}
		}
	}

	if workoutID == "" && isSameSource {
		workoutID = payload.StandardizedActivity.GetExternalId()
	}

	if workoutID == "" {
		return fmt.Errorf("activity_not_found")
	}

	client := oauth.NewClientWithErrorLogging(logger, "hevy", 30*time.Second)

	getURL := fmt.Sprintf("https://api.hevyapp.com/v1/workouts/%s", workoutID)
	getReq, err := http.NewRequestWithContext(ctx, "GET", getURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}
	getReq.Header.Set("api-key", apiKey)

	getResp, err := client.Do(getReq)
	if err != nil {
		return fmt.Errorf("failed to GET existing workout: %w", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		var errorBody bytes.Buffer
		errorBody.ReadFrom(getResp.Body)
		return fmt.Errorf("GET workout failed: status %d, body: %s", getResp.StatusCode, errorBody.String())
	}

	var existingWorkout hevyapi.Workout
	var getRespBytes bytes.Buffer
	getRespBytes.ReadFrom(getResp.Body)
	getRespData := getRespBytes.Bytes()

	var rawGetResp map[string]interface{}
	if err := json.Unmarshal(getRespData, &rawGetResp); err == nil {
		if workoutObj, ok := rawGetResp["workout"]; ok {
			workoutBytes, marshalErr := json.Marshal(workoutObj)
			if marshalErr == nil {
				if decodeErr := json.Unmarshal(workoutBytes, &existingWorkout); decodeErr != nil {
					return fmt.Errorf("failed to decode unwrapped workout: %w", decodeErr)
				}
			}
		} else {
			if err := json.Unmarshal(getRespData, &existingWorkout); err != nil {
				return fmt.Errorf("failed to decode existing workout: %w", err)
			}
		}
	} else {
		return fmt.Errorf("failed to decode existing workout response: %w", err)
	}

	existingTitle := ""
	if existingWorkout.Title != nil {
		existingTitle = *existingWorkout.Title
	}
	existingDesc := ""
	if existingWorkout.Description != nil {
		existingDesc = *existingWorkout.Description
	}

	payloadName := payload.Metadata["activity_name"]
	payloadDesc := payload.Metadata["description"]

	var mergedDescription string
	var mergedTitle string
	if isSameSource {
		mergedDescription = payloadDesc
		mergedTitle = payloadName
	} else {
		mergedDescription = existingDesc
		mergedTitle = existingTitle
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
	}

	updatedFields := map[string]interface{}{}
	if mergedTitle != existingTitle {
		updatedFields["title"] = "updated"
	}
	if mergedDescription != existingDesc {
		updatedFields["description"] = "updated"
	}

	if len(updatedFields) == 0 {
		return nil
	}

	var putExercises []hevyapi.PostWorkoutsRequestExercise
	if existingWorkout.Exercises != nil {
		for _, ex := range *existingWorkout.Exercises {
			var putSets []hevyapi.PostWorkoutsRequestSet
			if ex.Sets != nil {
				for _, s := range *ex.Sets {
					putSet := hevyapi.PostWorkoutsRequestSet{
						CustomMetric: s.CustomMetric,
						WeightKg:     s.WeightKg,
					}
					if s.Rpe != nil {
						rpe := hevyapi.PostWorkoutsRequestSetRpe(*s.Rpe)
						putSet.Rpe = &rpe
					}
					if s.Type != nil {
						setType := hevyapi.PostWorkoutsRequestSetType(*s.Type)
						putSet.Type = &setType
					}
					if s.DistanceMeters != nil {
						v := int(*s.DistanceMeters)
						putSet.DistanceMeters = &v
					}
					if s.DurationSeconds != nil {
						v := int(*s.DurationSeconds)
						putSet.DurationSeconds = &v
					}
					if s.Reps != nil {
						v := int(*s.Reps)
						putSet.Reps = &v
					}
					putSets = append(putSets, putSet)
				}
			}

			putEx := hevyapi.PostWorkoutsRequestExercise{
				ExerciseTemplateId: ex.ExerciseTemplateId,
			}
			if ex.Notes != nil && *ex.Notes != "" {
				putEx.Notes = ex.Notes
			}
			if len(putSets) > 0 {
				putEx.Sets = &putSets
			}
			if ex.SupersetId != nil {
				v := int(*ex.SupersetId)
				putEx.SupersetId = &v
			}
			putExercises = append(putExercises, putEx)
		}
	}

	isPrivate := false
	if val, ok := payload.Metadata["hevy_is_private"]; ok && val == "true" {
		isPrivate = true
	}
	putPayload := hevyapi.PostWorkoutsRequestBody{
		Workout: &struct {
			Description *string                                `json:"description"`
			EndTime     *string                                `json:"end_time,omitempty"`
			Exercises   *[]hevyapi.PostWorkoutsRequestExercise `json:"exercises,omitempty"`
			IsPrivate   *bool                                  `json:"is_private,omitempty"`
			StartTime   *string                                `json:"start_time,omitempty"`
			Title       *string                                `json:"title,omitempty"`
		}{
			Title:       &mergedTitle,
			Description: &mergedDescription,
			StartTime:   existingWorkout.StartTime,
			EndTime:     existingWorkout.EndTime,
			IsPrivate:   &isPrivate,
		},
	}
	if len(putExercises) > 0 {
		putPayload.Workout.Exercises = &putExercises
	}

	bodyJSON, err := json.Marshal(putPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	// Store the bounceback record BEFORE calling Hevy so the record exists by the
	// time Hevy fires the webhook back. The workoutID is already known at this point
	// (from pipelineRun or isSameSource), so we can safely pre-write it.
	uploadRecord := &pbactivity.UploadedActivityRecord{
		Id:            loopprevention.BuildUploadedActivityID(pbplugin.DestinationType_DESTINATION_HEVY, workoutID),
		UserId:        payload.UserId,
		Source:        payload.Source,
		ExternalId:    payload.StandardizedActivity.GetExternalId(),
		StartTime:     payload.Timestamp,
		Destination:   pbplugin.DestinationType_DESTINATION_HEVY,
		DestinationId: workoutID,
		UploadedAt:    timestamppb.Now(),
	}
	if err := u.svc.DB.SetUploadedActivity(ctx, payload.UserId, uploadRecord); err != nil {
		logger.WarnContext(ctx, "Failed to pre-store Hevy bounceback record; webhook may re-trigger", "workout_id", workoutID, "error", err)
	}

	putURL := fmt.Sprintf("https://api.hevyapp.com/v1/workouts/%s", workoutID)
	putReq, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create PUT request: %w", err)
	}
	putReq.Header.Set("api-key", apiKey)
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := client.Do(putReq)
	if err != nil {
		return fmt.Errorf("failed to PUT workout: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode >= 400 {
		err := httputil.WrapResponseError(putResp, "Hevy PUT failed")
		return err
	}

	return nil
}

func (u *Uploader) createHevyWorkout(ctx context.Context, apiKey string, workout *hevyapi.PostWorkoutsRequestBody, logger *slog.Logger) (string, error) {
	bodyJSON, err := json.Marshal(workout)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workout: %w", err)
	}

	client := oauth.NewClientWithErrorLogging(logger, "hevy", 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.hevyapp.com/v1/workouts", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Hevy API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", httputil.WrapResponseError(resp, "Hevy API error")
	}

	var bodyBuf bytes.Buffer
	bodyBuf.ReadFrom(resp.Body)
	respBytes := bodyBuf.Bytes()

	var rawResp map[string]interface{}
	if err := json.Unmarshal(respBytes, &rawResp); err != nil {
		return "", fmt.Errorf("failed to decode Hevy response: %w", err)
	}

	if workoutData, ok := rawResp["workout"]; ok {
		// Wrapped format: {"workout": {"id": "..."}}
		if obj, ok := workoutData.(map[string]interface{}); ok {
			if id, ok := obj["id"]; ok {
				if idStr, ok := id.(string); ok && idStr != "" {
					return idStr, nil
				}
			}
		}
		// Array variant: {"workout": [{"id": "..."}, ...]}
		if arr, ok := workoutData.([]interface{}); ok && len(arr) > 0 {
			if first, ok := arr[0].(map[string]interface{}); ok {
				if id, ok := first["id"]; ok {
					if idStr, ok := id.(string); ok && idStr != "" {
						return idStr, nil
					}
				}
			}
		}
	}

	// Flat format: {"id": "..."} — this is what POST /v1/workouts actually returns.
	// The generated client (ParsePostV1WorkoutsResponse) unmarshals the body directly
	// into a Workout struct, confirming the response is not wrapped.
	if id, ok := rawResp["id"]; ok {
		if idStr, ok := id.(string); ok && idStr != "" {
			return idStr, nil
		}
	}

	return "", nil
}
