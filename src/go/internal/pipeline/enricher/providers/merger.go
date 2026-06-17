package providers

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"
)

// TimedSample represents a single data point with timestamp
type TimedSample struct {
	Timestamp time.Time
	Value     int
}

// AlignmentResult contains the merged HR data aligned to GPS timestamps
type AlignmentResult struct {
	AlignedHR      []int             // HR values aligned to target timestamps
	DriftPercent   float64           // Duration difference percentage
	WarningMessage string            // If drift > threshold
	Metadata       map[string]string // Alignment metadata for logging
}

// AlignmentConfig contains parameters for alignment
type AlignmentConfig struct {
	MaxDriftPercent float64       // Threshold for warning (default: 1.0 = 1%)
	TargetAccuracy  time.Duration // Target accuracy for alignment (default: 2s)
}

// DefaultAlignmentConfig provides sensible defaults
var DefaultAlignmentConfig = AlignmentConfig{
	MaxDriftPercent: 1.0,
	TargetAccuracy:  2 * time.Second,
}

// AlignTimeSeries performs the "Elastic Match" alignment between GPS timestamps and HR samples.
// It aligns HR data to the GPS timeline, handling clock drift between devices.
//
// Algorithm:
// 1. Align start times of both streams
// 2. Calculate duration difference (drift)
// 3. If drift < MaxDriftPercent: apply elastic stretch/compress with linear interpolation
// 4. If drift >= MaxDriftPercent: still apply alignment but log warning
// 5. Handle edge cases (missing data at start/end, gaps)
func AlignTimeSeries(gpsTimestamps []time.Time, hrSamples []TimedSample, config AlignmentConfig, logger *slog.Logger) (*AlignmentResult, error) {
	result := &AlignmentResult{
		AlignedHR: make([]int, len(gpsTimestamps)),
		Metadata:  make(map[string]string),
	}

	// Edge case: No GPS timestamps
	if len(gpsTimestamps) == 0 {
		result.Metadata["alignment_status"] = "skipped_no_gps"
		logger.Info("HR alignment skipped: no GPS timestamps provided")
		return result, nil
	}

	// Edge case: No HR samples
	if len(hrSamples) == 0 {
		result.Metadata["alignment_status"] = "skipped_no_hr"
		result.WarningMessage = "No HR data available for alignment"
		logger.Warn("HR alignment skipped: no HR samples provided")
		return result, nil
	}

	// Sort samples by timestamp to ensure correct ordering
	sortedHR := make([]TimedSample, len(hrSamples))
	copy(sortedHR, hrSamples)
	sort.Slice(sortedHR, func(i, j int) bool {
		return sortedHR[i].Timestamp.Before(sortedHR[j].Timestamp)
	})

	sortedGPS := make([]time.Time, len(gpsTimestamps))
	copy(sortedGPS, gpsTimestamps)
	sort.Slice(sortedGPS, func(i, j int) bool {
		return sortedGPS[i].Before(sortedGPS[j])
	})

	// Calculate durations
	gpsStart := sortedGPS[0]
	gpsEnd := sortedGPS[len(sortedGPS)-1]
	gpsDuration := gpsEnd.Sub(gpsStart)

	hrStart := sortedHR[0].Timestamp
	hrEnd := sortedHR[len(sortedHR)-1].Timestamp
	hrDuration := hrEnd.Sub(hrStart)

	// Calculate drift percentage
	if gpsDuration > 0 {
		driftDuration := math.Abs(float64(gpsDuration - hrDuration))
		result.DriftPercent = (driftDuration / float64(gpsDuration)) * 100
	}

	// Log drift detection
	result.Metadata["gps_duration_sec"] = fmt.Sprintf("%.1f", gpsDuration.Seconds())
	result.Metadata["hr_duration_sec"] = fmt.Sprintf("%.1f", hrDuration.Seconds())
	result.Metadata["drift_percent"] = fmt.Sprintf("%.2f", result.DriftPercent)
	result.Metadata["gps_samples"] = fmt.Sprintf("%d", len(sortedGPS))
	result.Metadata["hr_samples"] = fmt.Sprintf("%d", len(sortedHR))

	// Check drift threshold
	if result.DriftPercent > config.MaxDriftPercent {
		result.WarningMessage = fmt.Sprintf("Clock drift of %.2f%% detected (threshold: %.2f%%), applying best-effort alignment", result.DriftPercent, config.MaxDriftPercent)
		logger.Warn("High clock drift detected during HR alignment",
			"drift_percent", result.DriftPercent,
			"threshold_percent", config.MaxDriftPercent,
			"gps_duration_sec", gpsDuration.Seconds(),
			"hr_duration_sec", hrDuration.Seconds(),
		)
		result.Metadata["alignment_status"] = "high_drift_best_effort"
	} else {
		result.Metadata["alignment_status"] = "success"
	}

	// Align HR to the GPS timeline by ABSOLUTE timestamp — each GPS point takes the HR value at
	// that same real instant — NOT by relative position. Relative-position ("elastic") mapping
	// stretches a partial HR pull (e.g. Fitbit only synced the first 20 min of a 60 min ride)
	// across the entire GPS track, fabricating data that never existed. Absolute anchoring keeps
	// every HR sample at its true time; GPS points outside the HR coverage window — whether the
	// device started HR late or stopped early — are left as gaps (0) so the downstream consumer
	// skips them rather than holding a stale or stretched value.
	//
	// This relies on both series sharing one real-time frame: GPS record timestamps are UTC and
	// the Fitbit samples must be built in the Fitbit profile timezone (see ConvertHRResponseToSamples
	// callers) so the instants line up. Slow clock drift between devices is tolerated — HR varies
	// slowly, so a few seconds of skew is immaterial compared to the distortion of stretching.
	tolerance := config.TargetAccuracy
	coverageStart := hrStart.Add(-tolerance)
	coverageEnd := hrEnd.Add(tolerance)
	covered := 0
	for i, gpsTime := range sortedGPS {
		// Outside the HR coverage window: leave a gap rather than stretch/hold a stale value.
		if gpsTime.Before(coverageStart) || gpsTime.After(coverageEnd) {
			result.AlignedHR[i] = 0
			continue
		}

		result.AlignedHR[i] = interpolateHR(sortedHR, gpsTime)
		covered++
	}

	coveragePercent := 100.0
	if len(sortedGPS) > 0 {
		coveragePercent = float64(covered) / float64(len(sortedGPS)) * 100
	}
	result.Metadata["coverage_percent"] = fmt.Sprintf("%.1f", coveragePercent)
	result.Metadata["gps_points_covered"] = fmt.Sprintf("%d", covered)
	if covered < len(sortedGPS) {
		// Surface the shortfall as a warning, but leave alignment_status as the drift block set
		// it: a genuine partial pull is always a large duration mismatch, so it is already
		// flagged as high_drift_best_effort. coverage_percent above carries the precise figure.
		coverageWarning := fmt.Sprintf("HR covered %.1f%% of GPS track (%d/%d points); uncovered points left as gaps", coveragePercent, covered, len(sortedGPS))
		result.WarningMessage = strings.TrimSpace(result.WarningMessage + " " + coverageWarning)
	}

	logger.Info("HR alignment completed (absolute offset anchoring)",
		"gps_duration_sec", gpsDuration.Seconds(),
		"hr_duration_sec", hrDuration.Seconds(),
		"drift_percent", result.DriftPercent,
		"samples_aligned", len(result.AlignedHR),
		"coverage_percent", coveragePercent,
	)

	return result, nil
}

// interpolateHR finds the HR value at a specific target time using linear interpolation.
// If the target time is before the first sample, returns the first sample's value.
// If the target time is after the last sample, returns the last sample's value.
// Otherwise, linearly interpolates between the two surrounding samples.
func interpolateHR(samples []TimedSample, targetTime time.Time) int {
	if len(samples) == 0 {
		return 0
	}

	// Before first sample - forward fill
	if targetTime.Before(samples[0].Timestamp) || targetTime.Equal(samples[0].Timestamp) {
		return samples[0].Value
	}

	// After last sample - backward fill
	lastIdx := len(samples) - 1
	if targetTime.After(samples[lastIdx].Timestamp) || targetTime.Equal(samples[lastIdx].Timestamp) {
		return samples[lastIdx].Value
	}

	// Find surrounding samples using binary search
	beforeIdx := findSampleBefore(samples, targetTime)
	afterIdx := beforeIdx + 1

	if afterIdx >= len(samples) {
		return samples[beforeIdx].Value
	}

	before := samples[beforeIdx]
	after := samples[afterIdx]

	// If timestamps are the same (shouldn't happen but be safe)
	if after.Timestamp.Equal(before.Timestamp) {
		return before.Value
	}

	// Linear interpolation
	totalDuration := float64(after.Timestamp.Sub(before.Timestamp))
	elapsed := float64(targetTime.Sub(before.Timestamp))
	ratio := elapsed / totalDuration

	interpolatedValue := float64(before.Value) + ratio*float64(after.Value-before.Value)
	return int(math.Round(interpolatedValue))
}

// findSampleBefore returns the index of the sample immediately before or at the target time.
// Uses binary search for efficiency.
func findSampleBefore(samples []TimedSample, targetTime time.Time) int {
	left, right := 0, len(samples)-1

	for left < right {
		mid := (left + right + 1) / 2
		if samples[mid].Timestamp.After(targetTime) {
			right = mid - 1
		} else {
			left = mid
		}
	}

	return left
}

// ConvertHRResponseToSamples converts the Fitbit API response format to TimedSamples.
// The baseDate is the date of the activity (used to construct full timestamps).
func ConvertHRResponseToSamples(dataset []struct {
	Time  string `json:"time"`
	Value int    `json:"value"`
}, baseDate time.Time) []TimedSample {
	samples := make([]TimedSample, 0, len(dataset))

	for _, point := range dataset {
		// Parse time in "15:04:05" format
		ptTime, err := time.Parse("15:04:05", point.Time)
		if err != nil {
			// Skip invalid timestamps silently - not critical for functionality
			continue
		}

		// Combine with base date
		fullTime := time.Date(
			baseDate.Year(), baseDate.Month(), baseDate.Day(),
			ptTime.Hour(), ptTime.Minute(), ptTime.Second(), 0,
			baseDate.Location(),
		)

		samples = append(samples, TimedSample{
			Timestamp: fullTime,
			Value:     point.Value,
		})
	}

	return samples
}
