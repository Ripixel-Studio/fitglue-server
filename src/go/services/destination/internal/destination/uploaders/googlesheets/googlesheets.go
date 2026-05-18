// nolint:proto-json
package googlesheets

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

// Uploader implements destination.Destination for Google Sheets
type Uploader struct {
	svc *bootstrap.Service
}

// New returns a new Google Sheets Uploader initialized with dependencies.
func New(svc *bootstrap.Service) *Uploader {
	return &Uploader{
		svc: svc,
	}
}

// Name returns the identifier for this uploader
func (u *Uploader) Name() string {
	return "googlesheets"
}

// Create uploads a new activity to Google Sheets by appending a row.
func (u *Uploader) Create(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record) (string, error) {
	if userRec.Integrations == nil || userRec.Integrations.Google == nil || !userRec.Integrations.Google.Enabled {
		return "", fmt.Errorf("user has no Google integration configured")
	}

	spreadsheetID := ""
	sheetName := "Activities"
	includeVisuals := true

	if id, ok := payload.Metadata["googlesheets_spreadsheet_id"]; ok && id != "" {
		spreadsheetID = id
	}
	if name, ok := payload.Metadata["googlesheets_sheet_name"]; ok && name != "" {
		sheetName = name
	}
	if visuals, ok := payload.Metadata["googlesheets_include_visuals"]; ok && visuals == "false" {
		includeVisuals = false
	}

	if spreadsheetID == "" {
		return "", fmt.Errorf("spreadsheet_id not configured in metadata")
	}

	tokenSource := oauth.NewFirestoreTokenSource(u.svc, payload.UserId, "google")
	httpClient := oauth.NewClientWithUsageTracking(tokenSource, u.svc, payload.UserId, "google", infra.NewLogger())
	logger := slog.Default()

	if err := u.ensureHeaderRow(ctx, httpClient, spreadsheetID, sheetName, logger); err != nil {
		logger.Warn("Failed to ensure header row", "error", err)
	}

	row := u.buildSheetRow(payload, includeVisuals)

	rowNumber, err := u.appendToSheet(ctx, httpClient, spreadsheetID, sheetName, row, logger)
	if err != nil {
		return "", fmt.Errorf("failed to append to Google Sheets: %w", err)
	}

	googlesheetsDestID := fmt.Sprintf("%d", rowNumber)
	if googlesheetsDestID != "" {
		uploadRecord := &pbactivity.UploadedActivityRecord{
			Id:            loopprevention.BuildUploadedActivityID(pbplugin.DestinationType_DESTINATION_GOOGLESHEETS, googlesheetsDestID),
			UserId:        payload.UserId,
			Source:        payload.Source,
			ExternalId:    payload.StandardizedActivity.GetExternalId(),
			StartTime:     payload.Timestamp,
			Destination:   pbplugin.DestinationType_DESTINATION_GOOGLESHEETS,
			DestinationId: googlesheetsDestID,
			UploadedAt:    timestamppb.Now(),
		}
		_ = u.svc.DB.SetUploadedActivity(ctx, payload.UserId, uploadRecord)
	}

	return googlesheetsDestID, nil
}

// Update modifies an existing activity. For Google Sheets, this appends a new row.
func (u *Uploader) Update(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record, pipelineRun *pbpipeline.PipelineRun) error {
	logger := slog.Default()
	logger.Info("Handling Google Sheets UPDATE (append-only mode)", "activity_id", payload.ActivityId)

	_, err := u.Create(ctx, payload, userRec)
	return err
}

func (u *Uploader) getHeaderRow() []interface{} {
	return []interface{}{
		"Synced At",
		"Date",
		"Source",
		"Activity Type",
		"Title",
		"Duration",
		"Distance (km)",
		"Calories",
		"Avg HR",
		"Max HR",
		"Elevation Gain (m)",
		"Generated Images",
		"Description",
	}
}

func (u *Uploader) ensureHeaderRow(ctx context.Context, httpClient *http.Client, spreadsheetID, sheetName string, logger *slog.Logger) error {
	url := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s!A1:M1", spreadsheetID, sheetName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create header check request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("header check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return httputil.WrapResponseError(resp, "Google Sheets API error checking headers")
	}

	var respBody struct {
		Values [][]interface{} `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return fmt.Errorf("failed to decode header check response: %w", err)
	}

	if len(respBody.Values) == 0 || len(respBody.Values[0]) == 0 {
		logger.Info("Sheet has no header row, writing headers")
		headerRow := u.getHeaderRow()
		_, err := u.appendToSheet(ctx, httpClient, spreadsheetID, sheetName, headerRow, logger)
		if err != nil {
			return fmt.Errorf("failed to write header row: %w", err)
		}
	}

	return nil
}

func (u *Uploader) buildSheetRow(payload *pbevents.ActivityPayload, includeVisuals bool) []interface{} {
	row := []interface{}{}

	row = append(row, time.Now().UTC().Format("2006-01-02 15:04:05"))

	date := ""
	if payload.Timestamp != nil {
		date = payload.Timestamp.AsTime().Format("2006-01-02")
	}
	row = append(row, date)

	source := payload.Source.String()
	source = strings.TrimPrefix(source, "SOURCE_")
	source = strings.ReplaceAll(source, "_", " ")
	row = append(row, source)

	activityTypeStr := payload.Metadata["activity_type"]
	activityTypeStr = strings.TrimPrefix(activityTypeStr, "ACTIVITY_TYPE_")
	activityTypeStr = strings.ReplaceAll(activityTypeStr, "_", " ")
	if activityTypeStr == "" {
		activityTypeStr = "UNSPECIFIED"
	}
	row = append(row, activityTypeStr)

	row = append(row, payload.Metadata["activity_name"])

	duration := ""
	if payload.StandardizedActivity != nil && len(payload.StandardizedActivity.Sessions) > 0 {
		totalSeconds := int(payload.StandardizedActivity.Sessions[0].TotalElapsedTime)
		hours := totalSeconds / 3600
		minutes := (totalSeconds % 3600) / 60
		seconds := totalSeconds % 60
		duration = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	row = append(row, duration)

	distance := ""
	if payload.StandardizedActivity != nil && len(payload.StandardizedActivity.Sessions) > 0 {
		distanceMeters := payload.StandardizedActivity.Sessions[0].TotalDistance
		if distanceMeters > 0 {
			distance = fmt.Sprintf("%.2f", distanceMeters/1000.0)
		}
	}
	row = append(row, distance)

	calories := ""
	if payload.StandardizedActivity != nil && len(payload.StandardizedActivity.Sessions) > 0 {
		if cal := payload.StandardizedActivity.Sessions[0].GetTotalCalories(); cal > 0 {
			calories = fmt.Sprintf("%.0f", cal)
		}
	}
	if calories == "" {
		if cal, ok := payload.Metadata["calories"]; ok && cal != "" {
			calories = cal
		}
	}
	row = append(row, calories)

	avgHR := ""
	if payload.StandardizedActivity != nil && len(payload.StandardizedActivity.Sessions) > 0 {
		var hrSum, hrCount int
		for _, lap := range payload.StandardizedActivity.Sessions[0].Laps {
			for _, record := range lap.Records {
				if record.HeartRate > 0 {
					hrSum += int(record.HeartRate)
					hrCount++
				}
			}
		}
		if hrCount > 0 {
			avgHR = fmt.Sprintf("%.0f", float64(hrSum)/float64(hrCount))
		}
	}
	if avgHR == "" {
		if v, ok := payload.Metadata["hr_avg"]; ok && v != "" {
			avgHR = v
		}
	}
	row = append(row, avgHR)

	maxHR := ""
	if payload.StandardizedActivity != nil && len(payload.StandardizedActivity.Sessions) > 0 {
		var maxHRVal int32
		for _, lap := range payload.StandardizedActivity.Sessions[0].Laps {
			for _, record := range lap.Records {
				if record.HeartRate > maxHRVal {
					maxHRVal = record.HeartRate
				}
			}
		}
		if maxHRVal > 0 {
			maxHR = fmt.Sprintf("%d", maxHRVal)
		} else if mhr := payload.StandardizedActivity.Sessions[0].GetMaxHeartRate(); mhr > 0 {
			maxHR = fmt.Sprintf("%d", mhr)
		}
	}
	if maxHR == "" {
		if v, ok := payload.Metadata["hr_max"]; ok && v != "" {
			maxHR = v
		}
	}
	row = append(row, maxHR)

	elevationGain := ""
	if eg, ok := payload.Metadata["elevation_gain"]; ok && eg != "" {
		elevationGain = eg
	}
	row = append(row, elevationGain)

	var imageURLs []string
	if includeVisuals {
		if url, ok := payload.Metadata["asset_muscle_heatmap"]; ok && url != "" {
			imageURLs = append(imageURLs, url)
		}
		if url, ok := payload.Metadata["asset_route_thumbnail"]; ok && url != "" {
			imageURLs = append(imageURLs, url)
		}
	}
	row = append(row, strings.Join(imageURLs, "\n"))

	description := payload.Metadata["description"]
	if len(description) > 5000 {
		description = description[:5000] + "..."
	}
	row = append(row, description)

	return row
}

func (u *Uploader) appendToSheet(ctx context.Context, httpClient *http.Client, spreadsheetID, sheetName string, row []interface{}, logger *slog.Logger) (int, error) {
	payload := map[string]interface{}{
		"values": [][]interface{}{row},
	}

	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s:append?valueInputOption=USER_ENTERED", spreadsheetID, sheetName)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	logger.Debug("Sending POST to Google Sheets API",
		"url", url,
		"bodyLength", len(bodyJSON))

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Google Sheets API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, httputil.WrapResponseError(resp, "Google Sheets API error")
	}

	var respBody struct {
		Updates struct {
			UpdatedRange string `json:"updatedRange"`
		} `json:"updates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return 0, fmt.Errorf("failed to decode Google Sheets response: %w", err)
	}

	rowNumber := 0
	if respBody.Updates.UpdatedRange != "" {
		parts := strings.Split(respBody.Updates.UpdatedRange, "!")
		if len(parts) == 2 {
			rangePart := parts[1]
			if idx := strings.Index(rangePart, ":"); idx > 0 {
				rowStr := rangePart[1:idx]
				var dummyCol string
				// e.g. "A2"
				fmt.Sscanf(rowStr, "%[A-Z]%d", &dummyCol, &rowNumber)
			}
		}
	}

	return rowNumber, nil
}
