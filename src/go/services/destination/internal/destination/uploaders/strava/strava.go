// nolint:proto-json
package strava

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/api/strava"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/description"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	httputil "github.com/fitglue/server/src/go/pkg/infrastructure/http"
	"github.com/fitglue/server/src/go/pkg/infrastructure/oauth"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// stravaAPIBase is the Strava REST API base URL.
// Changing to https://www.api-v3.strava.com is required before June 1, 2027.
const stravaAPIBase = "https://www.strava.com/api/v3"

// Uploader implements destination.Destination for Strava
type Uploader struct {
	svc *bootstrap.Service
}

// New returns a new Strava Uploader initialized with dependencies.
func New(svc *bootstrap.Service) *Uploader {
	return &Uploader{
		svc: svc,
	}
}

// Name returns the identifier for this uploader
func (u *Uploader) Name() string {
	return "strava"
}

// Create uploads a new activity to Strava.
func (u *Uploader) Create(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record) (string, error) {
	// Idempotency guard: check destination subcollection for an existing SUCCESS entry.
	// Strava's own API deduplicates by FIT file hash, but a Pub/Sub redelivery before
	// the first delivery's UpdateStatus call writes SUCCESS can slip past the executor's
	// guard and lose the real ExternalId when the second delivery returns ("", nil).
	if payload.PipelineExecutionId != nil && *payload.PipelineExecutionId != "" {
		outcomes, err := u.svc.DB.GetDestinationOutcomes(ctx, payload.UserId, *payload.PipelineExecutionId)
		if err == nil {
			for _, outcome := range outcomes {
				if outcome.Destination == pbplugin.DestinationType_DESTINATION_STRAVA &&
					outcome.Status == pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS &&
					outcome.ExternalId != nil && *outcome.ExternalId != "" {
					slog.Default().Info("Strava activity already uploaded for this pipeline execution, skipping duplicate create",
						"strava_id", *outcome.ExternalId,
						"pipeline_execution_id", *payload.PipelineExecutionId)
					return *outcome.ExternalId, nil
				}
			}
		}
	}

	// Initialize OAuth HTTP Client — force HTTP/1.1 to avoid HTTP/2 stream errors
	// from Strava's /uploads endpoint when sending multipart file bodies.
	tokenSource := oauth.NewFirestoreTokenSource(u.svc, payload.UserId, "strava")
	httpClient := oauth.NewClientWithUsageTrackingHTTP1(tokenSource, u.svc, payload.UserId, "strava", infra.NewLogger())

	// Get fit_file_uri from metadata, which is injected by executor
	fitFileUri := ""
	if uri, ok := payload.Metadata["fit_file_uri"]; ok {
		fitFileUri = uri
	}
	if fitFileUri == "" {
		return "", fmt.Errorf("missing fit_file_uri in metadata")
	}

	activityName := "FitGlue Activity"
	if name, ok := payload.Metadata["activity_name"]; ok {
		activityName = name
	}

	description := ""
	if desc, ok := payload.Metadata["description"]; ok {
		description = desc
	}

	// Note: activityType is no longer needed since executor injects strava_sport_type directly

	bucketName := u.svc.Config.GCSArtifactBucket
	if bucketName == "" {
		bucketName = "fitglue-server-dev-artifacts"
	}
	objectName := strings.TrimPrefix(fitFileUri, "gs://"+bucketName+"/")

	fileData, err := u.svc.Store.Get(ctx, bucketName, objectName)
	if err != nil {
		return "", fmt.Errorf("GCS Read Error: %w", err)
	}

	// Build multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "activity.fit")
	part.Write(fileData)
	writer.WriteField("data_type", "fit")
	if activityName != "" {
		writer.WriteField("name", activityName)
	}
	if description != "" {
		writer.WriteField("description", description)
	}

	stravaType := "Run"
	if sType, ok := payload.Metadata["strava_sport_type"]; ok {
		stravaType = sType
	}
	writer.WriteField("sport_type", stravaType)
	writer.WriteField("activity_type", stravaType)
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", stravaAPIBase+"/uploads", body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpResp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Strava API Error: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		err := httputil.WrapResponseError(httpResp, "Strava upload failed")
		return "", err
	}

	var uploadResp strava.Upload
	json.NewDecoder(httpResp.Body).Decode(&uploadResp)

	if uploadResp.ActivityId == nil || *uploadResp.ActivityId == 0 {
		finalResp, err := waitForUploadCompletion(ctx, httpClient, *uploadResp.Id)
		if err == nil {
			uploadResp = *finalResp
		}
	}

	if uploadResp.Error != nil && strings.Contains(strings.ToLower(*uploadResp.Error), "duplicate") {
		// Treat duplicate as skipped
		return "", nil // or handle as specific error to skip
	}

	if uploadResp.ActivityId != nil && *uploadResp.ActivityId != 0 {
		return fmt.Sprintf("%d", *uploadResp.ActivityId), nil
	}

	if uploadResp.Error != nil && *uploadResp.Error != "" {
		return "", fmt.Errorf("strava upload incomplete: %s", *uploadResp.Error)
	}

	return "", fmt.Errorf("upload pending processing")
}

// Update modifies an existing Strava activity.
func (u *Uploader) Update(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record, pipelineRun *pbpipeline.PipelineRun) error {
	isSameSource := false
	if val, ok := payload.Metadata["same_source_destination_strava"]; ok && val == "true" {
		isSameSource = true
	}

	var stravaIDStr string
	if pipelineRun != nil {
		for _, dest := range pipelineRun.Destinations {
			if dest.Destination == pbplugin.DestinationType_DESTINATION_STRAVA && dest.ExternalId != nil && *dest.ExternalId != "" {
				stravaIDStr = *dest.ExternalId
				break
			}
		}
	}

	if stravaIDStr == "" && isSameSource {
		stravaIDStr = payload.StandardizedActivity.GetExternalId()
	}

	if stravaIDStr == "" {
		return fmt.Errorf("activity_not_found") // Treated as skipped in new framework
	}

	tokenSource := oauth.NewFirestoreTokenSource(u.svc, payload.UserId, "strava")
	httpClient := oauth.NewClientWithUsageTracking(tokenSource, u.svc, payload.UserId, "strava", infra.NewLogger())

	getURL := fmt.Sprintf("%s/activities/%s", stravaAPIBase, stravaIDStr)
	getReq, err := http.NewRequestWithContext(ctx, "GET", getURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}

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
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&existingActivity); err != nil {
		return fmt.Errorf("failed to decode existing activity: %w", err)
	}

	activityName := payload.Metadata["activity_name"]
	payloadDescription := payload.Metadata["description"]

	var mergedDescription string
	if isSameSource {
		mergedDescription = payloadDescription
	} else {
		mergedDescription = existingActivity.Description
		if payloadDescription != "" {
			sectionHeader := ""
			for key, val := range payload.Metadata {
				if strings.HasPrefix(key, "section_header_") {
					sectionHeader = val
					break
				}
			}

			if sectionHeader != "" && description.HasSection(mergedDescription, sectionHeader) {
				newSectionContent := description.ExtractSection(payloadDescription, sectionHeader)
				if newSectionContent != "" {
					mergedDescription = description.ReplaceSection(mergedDescription, sectionHeader, newSectionContent)
				}
			} else if mergedDescription != "" {
				mergedDescription += "\n\n" + payloadDescription
			} else {
				mergedDescription = payloadDescription
			}
		}
	}

	updateBody := map[string]interface{}{}
	if isSameSource && activityName != "" && activityName != existingActivity.Name {
		updateBody["name"] = activityName
	}
	if mergedDescription != existingActivity.Description {
		updateBody["description"] = mergedDescription
	}

	if len(updateBody) == 0 {
		return nil // Success, no update needed
	}

	bodyJSON, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	putURL := fmt.Sprintf("%s/activities/%s", stravaAPIBase, stravaIDStr)
	putReq, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create PUT request: %w", err)
	}
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := httpClient.Do(putReq)
	if err != nil {
		return fmt.Errorf("failed to PUT activity: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode >= 400 {
		return httputil.WrapResponseError(putResp, "Strava PUT failed")
	}

	return nil
}

func waitForUploadCompletion(ctx context.Context, client *http.Client, uploadID int64) (*strava.Upload, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timeout := time.After(15 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for upload processing")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/uploads/%d", stravaAPIBase, uploadID), nil)
			if err != nil {
				return nil, err
			}

			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}

			var status strava.Upload
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("failed to decode poll response: %w", err)
			}
			resp.Body.Close()

			if status.ActivityId != nil && *status.ActivityId != 0 || (status.Error != nil && *status.Error != "") {
				return &status, nil
			}
		}
	}
}
