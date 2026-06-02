package fit_file_heart_rate

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/fit_parser"

	pendinginput "github.com/fitglue/server/src/go/pkg/pending_input"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// FitFileHRProvider allows users to upload a FIT file containing heart rate data
// which is then merged with the activity. This is useful for activities recorded
// without HR (e.g., treadmill with GPS watch) where HR was captured separately
// (e.g., chest strap syncing to a different device or Peloton).
type FitFileHRProvider struct {
	service *bootstrap.Service
}

func init() {
	providers.Register(NewFitFileHRProvider())
}

func NewFitFileHRProvider() *FitFileHRProvider {
	return &FitFileHRProvider{}
}

func (p *FitFileHRProvider) SetService(s *bootstrap.Service) {
	p.service = s
}

func (p *FitFileHRProvider) Name() string {
	return "fit-file-heart-rate"
}

func (p *FitFileHRProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_FIT_FILE_HEART_RATE
}

// Enrich checks if the activity already has heart rate data. If not, it creates
// a pending input requesting a FIT file upload.
func (p *FitFileHRProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	// Check force option - skip if activity already has heartrate data and force is not set
	forceOverwrite := inputs["force"] == "true"
	if !forceOverwrite && hasExistingHeartRateData(activity) {
		logger.Info("Skipping FIT file HR enrichment: activity already has heartrate data and force=false")
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

	if p.service == nil {
		logger.Debug("fit-file-hr: error - service not initialized")
		return nil, fmt.Errorf("service not initialized")
	}

	stableID := pendinginput.GenerateID(activity.Source.String(), activity.ExternalId, p.Name())

	// Check if there's already a pending input for this activity
	pending, err := p.service.DB.GetPendingInput(ctx, user.UserId, stableID)
	if err == nil && pending != nil {
		if pending.Status == pbpipeline.PendingInput_STATUS_WAITING {
			logger.Debug("fit-file-hr: already waiting for user input")
			return &providers.EnrichmentResult{
				Metadata: map[string]string{
					"hr_source":     "pending",
					"status_detail": "Waiting for FIT file upload",
				},
			}, nil
		}
		if pending.Status == pbpipeline.PendingInput_STATUS_COMPLETED {
			// The FIT file was already uploaded in a prior resume pass. Re-apply it now so
			// that downstream enrichers (heart-rate-summary, heart-rate-zones, etc.) always
			// receive HR data, regardless of which pending input triggered this resume.
			// This covers the case where a second pending input (e.g. photo-upload) causes
			// a fresh enricher pass where only Enrich() is called, not EnrichResume().
			logger.Info("fit-file-hr: re-applying completed FIT file input")
			return p.EnrichResume(ctx, activity, user, pending)
		}
	}

	// Use the pre-generated activity_id from orchestrator for LinkedActivityId
	linkedActivityId := inputs["activity_id"]
	if linkedActivityId == "" {
		logger.Error("fit-file-hr: activity_id not in inputs - orchestrator bug")
		return nil, fmt.Errorf("activity_id not provided in enricher inputs")
	}

	logger.Info("fit-file-hr: requesting FIT file upload via pending input",
		"activity_id", stableID,
		"linked_activity_id", linkedActivityId)

	// Return WaitForInputError to halt the pipeline - the orchestrator's handleWaitError
	// will create the pending input with the original_payload_uri (GCS URI for payload retrieval)
	return nil, &user_input.WaitForInputError{
		ActivityID:         stableID,
		RequiredFields:     []string{"fit_file_base64"},
		EnricherProviderID: p.Name(),
		Metadata: map[string]string{
			"source_activity_id":   activity.ExternalId,
			"source_activity_type": activity.Source.String(),
			"linked_activity_id":   linkedActivityId,
			"pipeline_id":          inputs["pipeline_id"],
			"display.field_labels": `{"fit_file_base64":"Heart Rate File"}`,
			"display.field_types":  `{"fit_file_base64":"file:accept=.fit"}`,
			"display.summary":      "Upload a .fit file with heart rate data",
			"display.title":        "Upload Heart Rate Data",
			"display.help":         "Export the .fit file from your chest strap or secondary device",
		},
	}
}

// EnrichResume is called during resume mode to apply resolved pending input data.
// It parses the uploaded FIT file and extracts heart rate data for merging.
func (p *FitFileHRProvider) EnrichResume(ctx context.Context, activity *pbactivity.StandardizedActivity, user *user.Record, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	fitFileBase64 := pendingInput.InputData["fit_file_base64"]
	if fitFileBase64 == "" {
		// User dismissed the pending input without uploading a file — skip gracefully
		// so the pipeline can continue to destinations without HR data.
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "FIT file not uploaded",
			Metadata: map[string]string{
				"hr_source":     "dismissed",
				"status_detail": "FIT file upload dismissed by user",
			},
		}, nil
	}

	// Decode base64
	fitBytes, err := base64.StdEncoding.DecodeString(fitFileBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode FIT file base64: %w", err)
	}

	// Parse the FIT file
	parsedActivity, err := fit_parser.ParseFitFile(fitBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse FIT file: %w", err)
	}

	// Extract heart rate data from parsed activity
	hrSamples := extractHRSamples(parsedActivity)

	// Extract TimeMarkers from parsed FIT file (exercise transitions from Set messages).
	// These carry device-accurate timestamps and will be reconciled with StrengthSet
	// exercise names (e.g., from Hevy) by the orchestrator's reconcileTimeMarkerLabels.
	fitTimeMarkers := parsedActivity.GetTimeMarkers()

	if len(hrSamples) == 0 {
		return &providers.EnrichmentResult{
			TimeMarkers: fitTimeMarkers,
			Metadata: map[string]string{
				"hr_source":     "fit_file",
				"status_detail": "No heart rate data found in uploaded FIT file",
				"points_found":  "0",
				"time_markers":  fmt.Sprintf("%d", len(fitTimeMarkers)),
			},
		}, nil
	}

	// Build heart rate stream aligned to the target activity
	var stream []int
	alignmentMetadata := make(map[string]string)

	// Get target activity duration
	durationSec := 3600 // Default 1 hour
	if len(activity.Sessions) > 0 {
		durationSec = int(activity.Sessions[0].TotalElapsedTime)
	}

	// Check if GPS data exists for alignment
	if hasGPSData(activity) {
		// Use elastic matching for GPS+HR alignment
		gpsTimestamps := extractGPSTimestamps(activity)

		if len(gpsTimestamps) > 0 {
			alignResult, err := providers.AlignTimeSeries(gpsTimestamps, hrSamples, providers.DefaultAlignmentConfig, slog.Default())
			if err != nil {
				// Fallback to simple time-based mapping
				stream = buildStreamTimeBased(hrSamples, activity.StartTime.AsTime(), durationSec)
				alignmentMetadata["alignment_status"] = "fallback_time_based"
				alignmentMetadata["alignment_error"] = err.Error()
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
			stream = buildStreamTimeBased(hrSamples, activity.StartTime.AsTime(), durationSec)
			alignmentMetadata["alignment_status"] = "time_based_no_gps_timestamps"
		}
	} else {
		// No GPS data - use time-based mapping
		stream = buildStreamTimeBased(hrSamples, activity.StartTime.AsTime(), durationSec)
		alignmentMetadata["alignment_status"] = "time_based_no_gps"
	}

	return &providers.EnrichmentResult{
		HeartRateStream: stream,
		TimeMarkers:     fitTimeMarkers,
		Metadata: mergeMetadata(map[string]string{
			"hr_source":     "fit_file",
			"status_detail": "Success",
			"points_found":  fmt.Sprintf("%d", len(hrSamples)),
			"stream_length": fmt.Sprintf("%d", len(stream)),
			"time_markers":  fmt.Sprintf("%d", len(fitTimeMarkers)),
		}, alignmentMetadata),
	}, nil
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

// extractHRSamples extracts timed heart rate samples from a parsed FIT activity
func extractHRSamples(activity *pbactivity.StandardizedActivity) []providers.TimedSample {
	var samples []providers.TimedSample
	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate > 0 && record.Timestamp != nil {
					samples = append(samples, providers.TimedSample{
						Timestamp: record.Timestamp.AsTime(),
						Value:     int(record.HeartRate),
					})
				}
			}
		}
	}
	return samples
}

// OverlapResult contains the overlap calculation results
type OverlapResult struct {
	OverlapPercent float64   // Percentage of activity duration covered by HR data
	OverlapStart   time.Time // Start of overlapping period
	OverlapEnd     time.Time // End of overlapping period
	HRStart        time.Time // FIT file start time
	HREnd          time.Time // FIT file end time
	NeedsReindex   bool      // True if <50% natural overlap
	Strategy       string    // "direct", "interpolate", or "reindex"
}

// calculateOverlap determines the overlap between HR samples and activity time range
func calculateOverlap(samples []providers.TimedSample, activityStart time.Time, durationSec int) OverlapResult {
	if len(samples) == 0 {
		return OverlapResult{OverlapPercent: 0, Strategy: "none"}
	}

	activityEnd := activityStart.Add(time.Duration(durationSec) * time.Second)
	hrStart := samples[0].Timestamp
	hrEnd := samples[len(samples)-1].Timestamp

	// Calculate overlap window
	overlapStart := activityStart
	if hrStart.After(activityStart) {
		overlapStart = hrStart
	}
	overlapEnd := activityEnd
	if hrEnd.Before(activityEnd) {
		overlapEnd = hrEnd
	}

	// Calculate overlap duration
	var overlapDuration time.Duration
	if overlapEnd.After(overlapStart) {
		overlapDuration = overlapEnd.Sub(overlapStart)
	}

	activityDuration := time.Duration(durationSec) * time.Second
	overlapPercent := float64(overlapDuration) / float64(activityDuration) * 100

	// Determine strategy
	var strategy string
	needsReindex := false
	if overlapPercent >= 90 {
		strategy = "direct"
	} else if overlapPercent >= 50 {
		strategy = "interpolate"
	} else {
		strategy = "reindex"
		needsReindex = true
	}

	return OverlapResult{
		OverlapPercent: overlapPercent,
		OverlapStart:   overlapStart,
		OverlapEnd:     overlapEnd,
		HRStart:        hrStart,
		HREnd:          hrEnd,
		NeedsReindex:   needsReindex,
		Strategy:       strategy,
	}
}

// buildStreamTimeBased creates an HR stream aligned by timestamps with overlap-aware logic
func buildStreamTimeBased(samples []providers.TimedSample, activityStart time.Time, durationSec int) []int {
	if len(samples) == 0 {
		return make([]int, durationSec)
	}

	overlap := calculateOverlap(samples, activityStart, durationSec)

	var stream []int
	switch overlap.Strategy {
	case "direct":
		// ≥90% overlap: use samples directly with timestamp matching
		stream = buildStreamDirect(samples, activityStart, durationSec)
	case "interpolate":
		// 50-90% overlap: interpolate/scale HR data to fit activity duration
		stream = buildStreamInterpolated(samples, activityStart, durationSec, overlap)
	case "reindex":
		// <50% overlap: re-index HR data to align start times, then interpolate
		reindexedSamples := reindexSamples(samples, activityStart)
		newOverlap := calculateOverlap(reindexedSamples, activityStart, durationSec)
		if newOverlap.OverlapPercent >= 90 {
			stream = buildStreamDirect(reindexedSamples, activityStart, durationSec)
		} else {
			stream = buildStreamInterpolated(reindexedSamples, activityStart, durationSec, newOverlap)
		}
	default:
		stream = make([]int, durationSec)
	}

	return stream
}

// buildStreamDirect maps HR samples directly by absolute timestamp (for ≥90% overlap)
func buildStreamDirect(samples []providers.TimedSample, activityStart time.Time, durationSec int) []int {
	stream := make([]int, durationSec)

	for _, sample := range samples {
		offset := int(sample.Timestamp.Sub(activityStart).Seconds())
		if offset >= 0 && offset < durationSec {
			stream[offset] = sample.Value
		}
	}

	// Forward fill gaps
	forwardFillStream(stream)
	return stream
}

// buildStreamInterpolated scales HR samples to fit the activity duration (for 50-90% overlap)
func buildStreamInterpolated(samples []providers.TimedSample, activityStart time.Time, durationSec int, overlap OverlapResult) []int {
	stream := make([]int, durationSec)

	if len(samples) == 0 {
		return stream
	}

	hrDuration := overlap.HREnd.Sub(overlap.HRStart).Seconds()
	if hrDuration <= 0 {
		hrDuration = 1 // Prevent division by zero
	}

	activityDuration := float64(durationSec)

	// Scale factor: how much to stretch/compress HR data
	scaleFactor := activityDuration / hrDuration

	for _, sample := range samples {
		// Calculate the relative position in HR timeline (0.0 to 1.0)
		hrOffset := sample.Timestamp.Sub(overlap.HRStart).Seconds()
		relativePosition := hrOffset / hrDuration

		// Map to activity timeline
		activityOffset := int(relativePosition * activityDuration)
		if activityOffset >= 0 && activityOffset < durationSec {
			stream[activityOffset] = sample.Value
		}
	}

	// Forward fill gaps
	forwardFillStream(stream)

	_ = scaleFactor // Used for documentation purposes
	return stream
}

// reindexSamples shifts HR sample timestamps so the first sample aligns with activityStart
func reindexSamples(samples []providers.TimedSample, activityStart time.Time) []providers.TimedSample {
	if len(samples) == 0 {
		return samples
	}

	// Calculate the offset needed to align first HR sample with activity start
	offset := activityStart.Sub(samples[0].Timestamp)

	reindexed := make([]providers.TimedSample, len(samples))
	for i, sample := range samples {
		reindexed[i] = providers.TimedSample{
			Timestamp: sample.Timestamp.Add(offset),
			Value:     sample.Value,
		}
	}

	return reindexed
}

// forwardFillStream fills zero gaps in the stream with the last non-zero value
func forwardFillStream(stream []int) {
	lastVal := 0
	for i := 0; i < len(stream); i++ {
		if stream[i] != 0 {
			lastVal = stream[i]
		} else {
			stream[i] = lastVal
		}
	}
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
