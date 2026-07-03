// nolint:proto-json
package fitbit_heart_rate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/user"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/infrastructure/oauth"

	fitbit "github.com/fitglue/server/src/go/pkg/integrations/fitbit"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

type FitBitHeartRate struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewFitBitHeartRate())
}

func NewFitBitHeartRate() *FitBitHeartRate {
	return &FitBitHeartRate{}
}

func (p *FitBitHeartRate) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *FitBitHeartRate) Name() string {
	return "fitbit-heart-rate"
}

func (p *FitBitHeartRate) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_FITBIT_HEART_RATE
}

// IsIdempotent returns false: the fetched stream depends on Fitbit's own sync lag at
// query time, so re-running on resume can silently commit a less-complete stream than
// the one already applied. Marking this non-idempotent makes the orchestrator replay
// the previously-fetched stream from the journal instead of re-querying Fitbit.
func (p *FitBitHeartRate) IsIdempotent() bool { return false }

func (p *FitBitHeartRate) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	return p.EnrichWithClient(ctx, logger, activity, user, inputs, nil, doNotRetry)
}

// EnrichWithClient allows HTTP client injection for testing
func (p *FitBitHeartRate) EnrichWithClient(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, httpClient *http.Client, doNotRetry bool) (*providers.EnrichmentResult, error) {
	// 0. Check force option - skip if activity already has heartrate data and force is not set
	forceOverwrite := inputs["force"] == "true"
	if !forceOverwrite && hasExistingHeartRateData(activity) {
		logger.Info("Skipping Fitbit HR enrichment: activity already has heartrate data and force=false")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "Activity already has heartrate data",
			Metadata: map[string]string{
				"hr_source":     "skipped",
				"status_detail": "Activity already has heartrate data",
				"force":         "false",
			},
		}, nil
	}

	// 1. Check Credentials
	if user.Integrations == nil || user.Integrations.Fitbit == nil || !user.Integrations.Fitbit.Enabled {
		return nil, fmt.Errorf("fitbit integration not enabled")
	}

	// 2. Parse Activity Times
	// 2. Parse Activity Times
	startTime := activity.StartTime.AsTime()
	if startTime.IsZero() {
		return nil, fmt.Errorf("invalid start time: zero")
	}

	// Calculate end time
	durationSec := 3600 // Default
	if len(activity.Sessions) > 0 {
		durationSec = int(activity.Sessions[0].TotalElapsedTime)
	}
	endTime := startTime.Add(time.Duration(durationSec) * time.Second)

	// 3. Initialize OAuth HTTP Client if not provided (for testing)
	if httpClient == nil {
		tokenSource := oauth.NewFirestoreTokenSource(p.Service, user.UserId, "fitbit")
		httpClient = oauth.NewClientWithUsageTracking(tokenSource, p.Service, user.UserId, "fitbit", infra.WrapSlogLogger(logger))
	}

	// 4. Create Fitbit Client with OAuth transport
	client, err := fitbit.NewClient("https://api.fitbit.com", fitbit.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create fitbit client: %w", err)
	}

	// 4a. Fetch user's Fitbit profile timezone so we query with local time.
	// Fitbit's intraday API interprets all date/time parameters in the user's profile timezone,
	// not UTC. Sending UTC times causes the wrong hour's data to be fetched (e.g. BST = UTC+1
	// means we'd retrieve pre-workout resting HR instead of the actual session).
	loc := fetchFitbitTimezone(ctx, client)
	if loc != time.UTC {
		logger.Info("Using Fitbit profile timezone for HR query", "timezone", loc.String())
	}

	// Format for Fitbit API using local timezone
	startTimeLocal := startTime.In(loc)
	endTimeLocal := endTime.In(loc)
	startTimeStr := startTimeLocal.Format("15:04")
	endTimeStr := endTimeLocal.Format("15:04")
	startDate := startTimeLocal.Format("2006-01-02")
	endDate := endTimeLocal.Format("2006-01-02")

	// Check if activity spans midnight (crosses day boundary)
	spansMidnight := startDate != endDate

	// 5. Request Data (Intraday HR)
	// Use date range API when activity spans midnight to avoid "start time after end time" error
	var resp *http.Response
	if spansMidnight {
		logger.Info(fmt.Sprintf("Activity spans midnight, using date range API: %s %s to %s %s", startDate, startTimeStr, endDate, endTimeStr))
		resp, err = client.GetHeartByDateRangeTimestampIntraday(ctx, startDate, endDate, "1sec", startTimeStr, endTimeStr)
	} else {
		resp, err = client.GetHeartByDateTimestampIntraday(ctx, startDate, "1sec", startTimeStr, endTimeStr)
	}
	if err != nil {
		return nil, fmt.Errorf("fitbit api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fitbit api error %d: %s", resp.StatusCode, string(body))
	}

	// 6. Parse Response
	var hrResponse struct {
		ActivitiesHeartIntraday struct {
			Dataset []struct {
				Time  string `json:"time"`
				Value int    `json:"value"`
			} `json:"dataset"`
		} `json:"activities-heart-intraday"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&hrResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// 7. Build Stream - Check if GPS data exists for alignment
	var stream []int
	alignmentMetadata := make(map[string]string)

	if hasGPSData(activity) {
		// Align HR to GPS by absolute timestamp (see providers.AlignTimeSeries).
		logger.Info("GPS data detected, applying absolute-timestamp HR alignment")

		// Convert HR response to timed samples. Use the start time in the Fitbit profile
		// timezone (the frame Fitbit returns clock times in) so the samples land on the same
		// real-time line as the UTC GPS timestamps — absolute-timestamp alignment depends on it.
		hrSamples := providers.ConvertHRResponseToSamples(hrResponse.ActivitiesHeartIntraday.Dataset, startTimeLocal)

		// Extract GPS timestamps from activity records
		gpsTimestamps := extractGPSTimestamps(activity)

		if len(gpsTimestamps) > 0 && len(hrSamples) > 0 {
			alignResult, err := providers.AlignTimeSeries(gpsTimestamps, hrSamples, providers.DefaultAlignmentConfig, logger)
			if err != nil {
				logger.Warn("HR alignment failed, falling back to index-based mapping", "error", err)
				stream = buildStreamIndexBased(hrResponse.ActivitiesHeartIntraday.Dataset, startTimeStr, durationSec)
			} else {
				stream = alignResult.AlignedHR
				for k, v := range alignResult.Metadata {
					alignmentMetadata[k] = v
				}
				if alignResult.WarningMessage != "" {
					alignmentMetadata["alignment_warning"] = alignResult.WarningMessage
				}
			}
		} else {
			// Fallback if no meaningful data
			stream = buildStreamIndexBased(hrResponse.ActivitiesHeartIntraday.Dataset, startTimeStr, durationSec)
		}
	} else {
		// No GPS data - use original index-based mapping
		stream = buildStreamIndexBased(hrResponse.ActivitiesHeartIntraday.Dataset, startTimeStr, durationSec)
		alignmentMetadata["alignment_status"] = "skipped_no_gps"
	}

	pointsFound := len(hrResponse.ActivitiesHeartIntraday.Dataset)
	logger.Info(fmt.Sprintf("Retrieved Fitbit HR points=%d duration=%d start_time=%s", pointsFound, durationSec, startTimeStr))

	// Lag Detection (Start/End Coverage)
	hasStart := false
	hasEnd := false
	startThreshold := 120 // 2 minutes (or 10% logic)
	endThreshold := durationSec - 120

	if pointsFound > 0 {
		// Calculate coverage
		// Sort just in case? API returns sorted usually.
		firstPt := hrResponse.ActivitiesHeartIntraday.Dataset[0]
		lastPt := hrResponse.ActivitiesHeartIntraday.Dataset[pointsFound-1]

		t1, _ := time.Parse("15:04:05", firstPt.Time)
		t2, _ := time.Parse("15:04:05", lastPt.Time)
		startBase, _ := time.Parse("15:04", startTimeStr)

		offset1 := int(t1.Sub(startBase).Seconds())
		offset2 := int(t2.Sub(startBase).Seconds())

		if offset1 <= startThreshold {
			hasStart = true
		}
		if offset2 >= endThreshold {
			hasEnd = true
		}
	}
	logger.Info(fmt.Sprintf("Retrieved Fitbit HR points=%d duration=%d start_time=%s has_start=%v has_end=%v", pointsFound, durationSec, startTimeStr, hasStart, hasEnd))

	// Decision logic
	timeSinceEnd := time.Since(endTime)
	isRecent := timeSinceEnd < 1*time.Hour

	// On resume the activity is usually no longer "recent" (it sat paused as a pending input),
	// so the recency gate below would normally skip the lag retry and accept whatever partial
	// data Fitbit has synced so far — which then gets aligned with gaps (no stretching) but
	// still misses HR Fitbit simply hadn't delivered yet. Re-arm the retry on resume so the lag
	// queue gives Fitbit time to finish syncing before we commit to partial coverage.
	isResume := inputs["is_resume"] == "true"
	requireCoverage := isRecent || isResume

	var lagErr error
	if (!hasStart || !hasEnd) && requireCoverage {
		reason := fmt.Sprintf("incomplete data (start:%v end:%v, recent:%v resume:%v) %v after end", hasStart, hasEnd, isRecent, isResume, timeSinceEnd.Round(time.Second))

		// Check if we exhausted retries
		if doNotRetry {
			logger.Warn("Incomplete data detected but forced to continue: " + reason)
			// DO NOT return error, accept whatever data we have
		} else {
			logger.Warn("Incomplete data detected: " + reason)
			// Return RetryableError to trigger lag mechanism
			lagErr = providers.NewRetryableError(fmt.Errorf("incomplete data"), 1*time.Minute, reason)
			// Logic: If it's a RetryableError, the system will discard this result anyway.
			return nil, lagErr
		}
	} else if pointsFound == 0 && !isRecent {
		// If old and empty, likely no data ever. Just return empty.
		logger.Warn(fmt.Sprintf("No heart rate data points found in Fitbit response start_time=%s end_time=%s", startTimeStr, endTimeStr))
	}

	return &providers.EnrichmentResult{
		Name:            "", // Don't wipe name
		HeartRateStream: stream,
		Metadata: mergeMetadata(map[string]string{
			"hr_source":      "fitbit",
			"query_date":     startDate,
			"query_end_date": endDate,
			"query_start":    startTimeStr,
			"query_end":      endTimeStr,
			"spans_midnight": fmt.Sprintf("%v", spansMidnight),
			"points_found":   fmt.Sprintf("%d", pointsFound),
			"status_detail":  "Success",
			"do_not_retry":   fmt.Sprintf("%v", doNotRetry),
		}, alignmentMetadata),
	}, nil
}

// fetchFitbitTimezone retrieves the IANA timezone from the user's Fitbit profile.
// Falls back to UTC on any error so enrichment still proceeds.
func fetchFitbitTimezone(ctx context.Context, client *fitbit.Client) *time.Location {
	resp, err := client.GetProfile(ctx)
	if err != nil {
		return time.UTC
	}
	defer resp.Body.Close()

	var profileResp struct {
		User *struct {
			Timezone *string `json:"timezone,omitempty"`
		} `json:"user,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profileResp); err != nil {
		return time.UTC
	}

	if profileResp.User == nil || profileResp.User.Timezone == nil || *profileResp.User.Timezone == "" {
		return time.UTC
	}

	loc, err := time.LoadLocation(*profileResp.User.Timezone)
	if err != nil {
		return time.UTC
	}

	return loc
}

// hasGPSData checks if any record in the activity has GPS coordinates
func hasGPSData(activity *pbactivity.StandardizedActivity) bool {
	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.PositionLat != 0 || record.PositionLong != 0 {
					return true
				}
			}
		}
	}
	return false
}

// extractGPSTimestamps extracts all record timestamps from the activity
func extractGPSTimestamps(activity *pbactivity.StandardizedActivity) []time.Time {
	var timestamps []time.Time
	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Timestamp != nil {
					timestamps = append(timestamps, record.Timestamp.AsTime())
				}
			}
		}
	}
	return timestamps
}

// buildStreamIndexBased creates HR stream using original index-based mapping
func buildStreamIndexBased(dataset []struct {
	Time  string `json:"time"`
	Value int    `json:"value"`
}, startTimeStr string, durationSec int) []int {
	stream := make([]int, durationSec)

	for _, dataPoint := range dataset {
		ptTime, _ := time.Parse("15:04:05", dataPoint.Time)
		startDayTime, _ := time.Parse("15:04", startTimeStr)

		offset := int(ptTime.Sub(startDayTime).Seconds())

		if offset >= 0 && offset < durationSec {
			stream[offset] = dataPoint.Value
		}
	}

	// Forward-fill internal gaps only, up to the last point Fitbit actually returned. The tail
	// after the last real sample (e.g. a partial pull that stopped early) is left as gaps (0)
	// rather than holding the last value across the rest of the session — that hold is the
	// non-GPS equivalent of stretching. Leading entries before the first sample also stay 0.
	lastDataIdx := -1
	for i := len(stream) - 1; i >= 0; i-- {
		if stream[i] != 0 {
			lastDataIdx = i
			break
		}
	}

	lastVal := 0
	for i := 0; i <= lastDataIdx; i++ {
		if stream[i] != 0 {
			lastVal = stream[i]
		} else {
			stream[i] = lastVal
		}
	}

	return stream
}

// hasExistingHeartRateData checks if the activity already has heart rate data in its records
func hasExistingHeartRateData(activity *pbactivity.StandardizedActivity) bool {
	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate > 0 {
					return true
				}
			}
		}
	}
	return false
}

// mergeMetadata combines two metadata maps, with second map taking precedence
func mergeMetadata(base, overlay map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}
