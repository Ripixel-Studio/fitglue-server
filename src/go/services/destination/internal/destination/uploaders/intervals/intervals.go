// nolint:proto-json
package intervals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/description"
	"github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	httputil "github.com/fitglue/server/src/go/pkg/infrastructure/http"
	"github.com/fitglue/server/src/go/pkg/loopprevention"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	baseURL = "https://intervals.icu/api/v1"
)

// Uploader implements destination.Destination for Intervals.icu
type Uploader struct {
	svc    *bootstrap.Service
	logger infra.Logger
}

// New returns a new Intervals Uploader initialized with dependencies.
func New(svc *bootstrap.Service) *Uploader {
	return &Uploader{
		svc:    svc,
		logger: infra.NewLoggerWithComponent("destination.intervals"),
	}
}

// Name returns the identifier for this uploader
func (u *Uploader) Name() string {
	return "intervals"
}

// Create uploads a new activity to Intervals.
func (u *Uploader) Create(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record) (string, error) {
	if userRec.Integrations == nil || userRec.Integrations.Intervals == nil || !userRec.Integrations.Intervals.Enabled {
		return "", fmt.Errorf("user has no Intervals integration configured")
	}

	integration := userRec.Integrations.Intervals
	if integration.ApiKey == "" || integration.AthleteId == "" {
		return "", fmt.Errorf("Intervals credentials incomplete: missing API key or athlete ID")
	}

	logger := slog.Default()
	httpClient := &http.Client{Timeout: 30 * time.Second}

	fitFileUri := ""
	if uri, ok := payload.Metadata["fit_file_uri"]; ok {
		fitFileUri = uri
	}
	if fitFileUri == "" {
		return "", fmt.Errorf("missing fit_file_uri in metadata")
	}

	bucketName := u.svc.Config.GCSArtifactBucket
	if bucketName == "" {
		bucketName = "fitglue-server-dev-artifacts"
	}
	objectName := strings.TrimPrefix(fitFileUri, "gs://"+bucketName+"/")

	fileData, err := u.svc.Store.Get(ctx, bucketName, objectName)
	if err != nil {
		return "", fmt.Errorf("GCS Read Error: %w", err)
	}

	uploadURL := fmt.Sprintf("%s/athlete/%s/activities", baseURL, integration.AthleteId)
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewBuffer(fileData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth("API_KEY", integration.ApiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Intervals API Error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", httputil.WrapResponseError(resp, "Intervals upload failed")
	}

	var uploadResp intervalsActivityResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	activityName := payload.Metadata["activity_name"]
	descriptionText := payload.Metadata["description"]

	if activityName != "" || descriptionText != "" {
		updateResp, err := u.updateIntervalsActivity(ctx, httpClient, integration, uploadResp.ID, payload, logger)
		if err != nil {
			logger.Warn("Failed to update activity metadata", "error", err)
		} else {
			uploadResp = *updateResp
		}
	}

	intervalsDestID := fmt.Sprintf("%d", uploadResp.ID)
	if intervalsDestID != "" {
		uploadRecord := &pbactivity.UploadedActivityRecord{
			Id:            loopprevention.BuildUploadedActivityID(pbplugin.DestinationType_DESTINATION_INTERVALS, intervalsDestID),
			UserId:        payload.UserId,
			Source:        payload.Source,
			ExternalId:    payload.StandardizedActivity.GetExternalId(),
			StartTime:     payload.Timestamp,
			Destination:   pbplugin.DestinationType_DESTINATION_INTERVALS,
			DestinationId: intervalsDestID,
			UploadedAt:    timestamppb.Now(),
		}
		if err := u.svc.DB.SetUploadedActivity(ctx, payload.UserId, uploadRecord); err != nil {
			u.logger.Error(ctx, "failed to store upload dedup record", "error", err, "destination", "intervals", "userId", payload.UserId)
		}
	}

	return intervalsDestID, nil
}

// Update modifies an existing Intervals activity.
func (u *Uploader) Update(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record, pipelineRun *pbpipeline.PipelineRun) error {
	if userRec.Integrations == nil || userRec.Integrations.Intervals == nil || !userRec.Integrations.Intervals.Enabled {
		return fmt.Errorf("user has no Intervals integration configured")
	}

	integration := userRec.Integrations.Intervals
	if integration.ApiKey == "" || integration.AthleteId == "" {
		return fmt.Errorf("Intervals credentials incomplete: missing API key or athlete ID")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	var intervalsIDStr string
	if pipelineRun != nil {
		for _, dest := range pipelineRun.Destinations {
			if dest.Destination == pbplugin.DestinationType_DESTINATION_INTERVALS && dest.ExternalId != nil && *dest.ExternalId != "" {
				intervalsIDStr = *dest.ExternalId
				break
			}
		}
	}
	if intervalsIDStr == "" {
		return fmt.Errorf("no Intervals destination found in pipeline run")
	}

	getURL := fmt.Sprintf("%s/athlete/%s/activities/%s", baseURL, integration.AthleteId, intervalsIDStr)
	getReq, err := http.NewRequestWithContext(ctx, "GET", getURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}
	getReq.SetBasicAuth("API_KEY", integration.ApiKey)

	getResp, err := httpClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("failed to GET existing activity: %w", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(getResp.Body)
		return fmt.Errorf("GET activity failed: status %d, body: %s", getResp.StatusCode, string(bodyBytes))
	}

	var existingActivity struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&existingActivity); err != nil {
		return fmt.Errorf("failed to decode existing activity: %w", err)
	}

	mergedDescription := existingActivity.Description
	payloadDesc := payload.Metadata["description"]

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

	updateBody := map[string]interface{}{}
	if mergedDescription != existingActivity.Description {
		updateBody["description"] = mergedDescription
	}

	if len(updateBody) == 0 {
		return nil
	}

	bodyJSON, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	putURL := fmt.Sprintf("%s/athlete/%s/activities/%s", baseURL, integration.AthleteId, intervalsIDStr)
	putReq, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create PUT request: %w", err)
	}
	putReq.SetBasicAuth("API_KEY", integration.ApiKey)
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := httpClient.Do(putReq)
	if err != nil {
		return fmt.Errorf("failed to PUT activity: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode >= 400 {
		return httputil.WrapResponseError(putResp, "Intervals PUT failed")
	}

	return nil
}

func (u *Uploader) updateIntervalsActivity(ctx context.Context, httpClient *http.Client, integration *pbuser.IntervalsIntegration, activityID int64, payload *pbevents.ActivityPayload, logger *slog.Logger) (*intervalsActivityResponse, error) {
	updateBody := map[string]interface{}{}
	if name, ok := payload.Metadata["activity_name"]; ok && name != "" {
		updateBody["name"] = name
	}
	if desc, ok := payload.Metadata["description"]; ok && desc != "" {
		updateBody["description"] = desc
	}

	activityTypeVal, ok := pbactivity.ActivityType_value[payload.Metadata["activity_type"]]
	activityType := pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED
	if ok {
		activityType = pbactivity.ActivityType(activityTypeVal)
	}

	if activityType != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		updateBody["type"] = activity.GetIntervalsActivityType(activityType)
	}

	bodyJSON, err := json.Marshal(updateBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update body: %w", err)
	}

	putURL := fmt.Sprintf("%s/athlete/%s/activities/%d", baseURL, integration.AthleteId, activityID)
	putReq, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create PUT request: %w", err)
	}
	putReq.SetBasicAuth("API_KEY", integration.ApiKey)
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := httpClient.Do(putReq)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT activity: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode >= 400 {
		return nil, httputil.WrapResponseError(putResp, "Intervals PUT failed")
	}

	var updatedActivity intervalsActivityResponse
	if err := json.NewDecoder(putResp.Body).Decode(&updatedActivity); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &updatedActivity, nil
}

type intervalsActivityResponse struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Type           string `json:"type"`
	StartDateLocal string `json:"start_date_local"`
}
