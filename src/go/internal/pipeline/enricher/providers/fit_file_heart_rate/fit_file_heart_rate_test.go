package fit_file_heart_rate

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"encoding/base64"
	"log/slog"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFitFileHR_Name(t *testing.T) {
	provider := NewFitFileHRProvider()
	expected := "fit-file-heart-rate"
	if provider.Name() != expected {
		t.Errorf("Expected provider name %q, got %q", expected, provider.Name())
	}
}

func TestFitFileHR_ProviderType(t *testing.T) {
	provider := NewFitFileHRProvider()
	expected := pbplugin.EnricherProviderType_ENRICHER_PROVIDER_FIT_FILE_HEART_RATE
	if provider.ProviderType() != expected {
		t.Errorf("Expected provider type %v, got %v", expected, provider.ProviderType())
	}
}

func TestFitFileHR_SkipsIfExistingHRData(t *testing.T) {
	provider := NewFitFileHRProvider()

	// Create activity WITH existing heart rate data
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 3600,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{HeartRate: 120}, // Existing HR data
							{HeartRate: 130},
						},
					},
				},
			},
		},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
	}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !result.Skipped {
		t.Error("Expected result.Skipped=true so orchestrator treats this as a skip, not a success")
	}
	if result.Metadata["hr_source"] != "skipped" {
		t.Errorf("Expected hr_source=skipped, got %s", result.Metadata["hr_source"])
	}
	if result.Metadata["force"] != "false" {
		t.Errorf("Expected force=false in metadata, got %s", result.Metadata["force"])
	}
}

func TestFitFileHR_NoServiceError(t *testing.T) {
	provider := NewFitFileHRProvider()
	// Don't set service

	// Activity without HR data
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Source:    pbactivity.ActivitySource_SOURCE_FILE_UPLOAD,
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 3600,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{Timestamp: timestamppb.New(startTime)},
						},
					},
				},
			},
		},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
	}

	_, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err == nil {
		t.Error("Expected error when service not initialized")
	}
}

func TestFitFileHR_EnrichResume_InvalidBase64(t *testing.T) {
	provider := NewFitFileHRProvider()

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(time.Now()),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	pendingInput := &pbpipeline.PendingInput{
		InputData: map[string]string{
			"fit_file_base64": "not-valid-base64!!!",
		},
	}

	_, err := provider.EnrichResume(context.Background(), activity, user, pendingInput)
	if err == nil {
		t.Error("Expected error for invalid base64")
	}
}

func TestFitFileHR_EnrichResume_EmptyInput(t *testing.T) {
	provider := NewFitFileHRProvider()

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(time.Now()),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	pendingInput := &pbpipeline.PendingInput{
		InputData: map[string]string{},
	}

	res, err := provider.EnrichResume(context.Background(), activity, user, pendingInput)
	if err != nil {
		t.Errorf("Expected no error for dismissed pending input, got: %v", err)
	}
	if res == nil || !res.Skipped {
		t.Error("Expected skipped result for dismissed pending input")
	}
}

func TestHasExistingHeartRateData(t *testing.T) {
	tests := []struct {
		name     string
		activity *pbactivity.StandardizedActivity
		expected bool
	}{
		{
			name: "has HR data",
			activity: &pbactivity.StandardizedActivity{
				Sessions: []*pbactivity.Session{{
					Laps: []*pbactivity.Lap{{
						Records: []*pbactivity.Record{{HeartRate: 120}},
					}},
				}},
			},
			expected: true,
		},
		{
			name: "no HR data",
			activity: &pbactivity.StandardizedActivity{
				Sessions: []*pbactivity.Session{{
					Laps: []*pbactivity.Lap{{
						Records: []*pbactivity.Record{{HeartRate: 0}},
					}},
				}},
			},
			expected: false,
		},
		{
			name: "empty sessions",
			activity: &pbactivity.StandardizedActivity{
				Sessions: []*pbactivity.Session{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasExistingHeartRateData(tt.activity)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasGPSData(t *testing.T) {
	tests := []struct {
		name     string
		activity *pbactivity.StandardizedActivity
		expected bool
	}{
		{
			name: "has GPS data",
			activity: &pbactivity.StandardizedActivity{
				Sessions: []*pbactivity.Session{{
					Laps: []*pbactivity.Lap{{
						Records: []*pbactivity.Record{{PositionLat: 53.07, PositionLong: -1.02}},
					}},
				}},
			},
			expected: true,
		},
		{
			name: "no GPS data",
			activity: &pbactivity.StandardizedActivity{
				Sessions: []*pbactivity.Session{{
					Laps: []*pbactivity.Lap{{
						Records: []*pbactivity.Record{{PositionLat: 0, PositionLong: 0}},
					}},
				}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasGPSData(tt.activity)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractHRSamples(t *testing.T) {
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{
			Laps: []*pbactivity.Lap{{
				Records: []*pbactivity.Record{
					{HeartRate: 100, Timestamp: timestamppb.New(startTime)},
					{HeartRate: 110, Timestamp: timestamppb.New(startTime.Add(1 * time.Second))},
					{HeartRate: 0, Timestamp: timestamppb.New(startTime.Add(2 * time.Second))}, // Zero HR - should skip
					{HeartRate: 120, Timestamp: timestamppb.New(startTime.Add(3 * time.Second))},
				},
			}},
		}},
	}

	samples := extractHRSamples(activity)

	if len(samples) != 3 {
		t.Errorf("Expected 3 samples (skipping zero HR), got %d", len(samples))
	}

	if samples[0].Value != 100 {
		t.Errorf("Expected first sample value 100, got %d", samples[0].Value)
	}
}

func TestBuildStreamTimeBased(t *testing.T) {
	// Test with samples that cover the full duration to trigger "direct" strategy
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	samples := make([]providers.TimedSample, 30)
	for i := 0; i < 30; i++ {
		samples[i] = providers.TimedSample{
			Timestamp: startTime.Add(time.Duration(i) * time.Second),
			Value:     100 + i, // 100, 101, 102, ...
		}
	}

	stream := buildStreamTimeBased(samples, startTime, 30)

	if len(stream) != 30 {
		t.Errorf("Expected stream length 30, got %d", len(stream))
	}

	// First point
	if stream[0] != 100 {
		t.Errorf("Expected stream[0]=100, got %d", stream[0])
	}

	// At second 10
	if stream[10] != 110 {
		t.Errorf("Expected stream[10]=110, got %d", stream[10])
	}

	// At second 20
	if stream[20] != 120 {
		t.Errorf("Expected stream[20]=120, got %d", stream[20])
	}

	// Last point
	if stream[29] != 129 {
		t.Errorf("Expected stream[29]=129, got %d", stream[29])
	}
}

func TestMergeMetadata(t *testing.T) {
	base := map[string]string{"a": "1", "b": "2"}
	overlay := map[string]string{"b": "3", "c": "4"}

	result := mergeMetadata(base, overlay)

	if result["a"] != "1" {
		t.Errorf("Expected a=1, got %s", result["a"])
	}
	if result["b"] != "3" { // Overlay wins
		t.Errorf("Expected b=3 (overlay), got %s", result["b"])
	}
	if result["c"] != "4" {
		t.Errorf("Expected c=4, got %s", result["c"])
	}
}

// TestFitFileHR_EnrichResume_ValidFIT tests with a minimal valid FIT-like structure
// Note: This is a unit test with base64-encoded data - real FIT parsing is tested in fit_parser package
func TestFitFileHR_EnrichResume_InvalidFitFile(t *testing.T) {
	provider := NewFitFileHRProvider()

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(time.Now()),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	// Valid base64 but not a valid FIT file
	invalidFitData := base64.StdEncoding.EncodeToString([]byte("not a fit file"))
	pendingInput := &pbpipeline.PendingInput{
		InputData: map[string]string{
			"fit_file_base64": invalidFitData,
		},
	}

	_, err := provider.EnrichResume(context.Background(), activity, user, pendingInput)
	if err == nil {
		t.Error("Expected error for invalid FIT file format")
	}
}

func TestCalculateOverlap_HighOverlap(t *testing.T) {
	// Activity: 10:00:00 to 10:30:00 (30 minutes)
	// HR data: 10:00:00 to 10:30:00 (100% overlap)
	activityStart := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	durationSec := 1800 // 30 minutes

	samples := make([]providers.TimedSample, 1800)
	for i := 0; i < 1800; i++ {
		samples[i] = providers.TimedSample{
			Timestamp: activityStart.Add(time.Duration(i) * time.Second),
			Value:     120,
		}
	}

	result := calculateOverlap(samples, activityStart, durationSec)

	if result.Strategy != "direct" {
		t.Errorf("Expected strategy 'direct', got '%s'", result.Strategy)
	}
	if result.OverlapPercent < 99 {
		t.Errorf("Expected ~100%% overlap, got %.1f%%", result.OverlapPercent)
	}
}

func TestCalculateOverlap_MediumOverlap(t *testing.T) {
	// Activity: 10:00:00 to 10:30:00 (30 minutes)
	// HR data: 10:00:00 to 10:20:00 (67% overlap)
	activityStart := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	durationSec := 1800 // 30 minutes

	samples := make([]providers.TimedSample, 1200) // 20 minutes
	for i := 0; i < 1200; i++ {
		samples[i] = providers.TimedSample{
			Timestamp: activityStart.Add(time.Duration(i) * time.Second),
			Value:     120,
		}
	}

	result := calculateOverlap(samples, activityStart, durationSec)

	if result.Strategy != "interpolate" {
		t.Errorf("Expected strategy 'interpolate', got '%s'", result.Strategy)
	}
	if result.OverlapPercent < 65 || result.OverlapPercent > 70 {
		t.Errorf("Expected ~67%% overlap, got %.1f%%", result.OverlapPercent)
	}
}

func TestCalculateOverlap_LowOverlap_FITStartsBeforeActivity(t *testing.T) {
	// Spinning class scenario:
	// Activity (Hevy): 19:26:15 to 20:06:15 (40 minutes = 2400 seconds)
	// HR data (Garmin): 19:04:47 to 19:40:46 (36 minutes = ~2160 seconds)
	// Natural overlap: 19:26:15 to 19:40:46 = ~14 minutes = 35% of activity
	activityStart := time.Date(2026, 2, 4, 19, 26, 15, 0, time.UTC)
	durationSec := 2400 // 40 minutes

	hrStart := time.Date(2026, 2, 4, 19, 4, 47, 0, time.UTC)
	samples := make([]providers.TimedSample, 2159) // ~36 minutes
	for i := 0; i < 2159; i++ {
		samples[i] = providers.TimedSample{
			Timestamp: hrStart.Add(time.Duration(i) * time.Second),
			Value:     100 + (i % 60), // Varying HR
		}
	}

	result := calculateOverlap(samples, activityStart, durationSec)

	if result.Strategy != "reindex" {
		t.Errorf("Expected strategy 'reindex', got '%s'", result.Strategy)
	}
	if result.NeedsReindex != true {
		t.Error("Expected NeedsReindex to be true")
	}
	// Overlap should be ~35% (14 minutes out of 40)
	if result.OverlapPercent > 50 {
		t.Errorf("Expected <50%% overlap, got %.1f%%", result.OverlapPercent)
	}
}

func TestReindexSamples(t *testing.T) {
	// HR samples start at 10:00:00
	// Activity starts at 10:30:00
	hrStart := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activityStart := time.Date(2025, 12, 25, 10, 30, 0, 0, time.UTC)

	samples := []providers.TimedSample{
		{Timestamp: hrStart, Value: 100},
		{Timestamp: hrStart.Add(10 * time.Second), Value: 110},
		{Timestamp: hrStart.Add(20 * time.Second), Value: 120},
	}

	reindexed := reindexSamples(samples, activityStart)

	if len(reindexed) != 3 {
		t.Fatalf("Expected 3 reindexed samples, got %d", len(reindexed))
	}

	// First sample should now be at activity start
	if !reindexed[0].Timestamp.Equal(activityStart) {
		t.Errorf("Expected first sample at %v, got %v", activityStart, reindexed[0].Timestamp)
	}

	// Values should be preserved
	if reindexed[0].Value != 100 || reindexed[1].Value != 110 || reindexed[2].Value != 120 {
		t.Error("Sample values were not preserved during reindexing")
	}

	// Time gaps should be preserved
	gap := reindexed[1].Timestamp.Sub(reindexed[0].Timestamp)
	if gap != 10*time.Second {
		t.Errorf("Expected 10s gap between samples, got %v", gap)
	}
}

func TestBuildStreamTimeBased_SpinningClassScenario(t *testing.T) {
	// This tests the exact spinning class scenario:
	// Activity (Hevy): 19:26:15, duration 40 minutes (2400s)
	// HR (Garmin FIT): starts 19:04:47, ~36 minutes of data
	activityStart := time.Date(2026, 2, 4, 19, 26, 15, 0, time.UTC)
	durationSec := 2400

	// HR data starts ~21 minutes before activity
	hrStart := time.Date(2026, 2, 4, 19, 4, 47, 0, time.UTC)
	samples := make([]providers.TimedSample, 2159)
	for i := 0; i < 2159; i++ {
		samples[i] = providers.TimedSample{
			Timestamp: hrStart.Add(time.Duration(i) * time.Second),
			Value:     100 + (i / 36), // Gradual increase from 100 to ~160
		}
	}

	stream := buildStreamTimeBased(samples, activityStart, durationSec)

	// Stream should have correct length
	if len(stream) != durationSec {
		t.Errorf("Expected stream length %d, got %d", durationSec, len(stream))
	}

	// First value should NOT be zero (the old bug)
	if stream[0] == 0 {
		t.Error("First HR value is zero - alignment failed (old bug!)")
	}

	// First value should be the first HR sample (reindexed)
	if stream[0] != 100 {
		t.Errorf("Expected first HR value ~100, got %d", stream[0])
	}

	// Mid-workout value should show progression
	if stream[1200] < 100 {
		t.Errorf("Expected mid-workout HR > 100, got %d", stream[1200])
	}

	// All values should be filled (no zeros in the middle)
	zeroCount := 0
	for i := 0; i < len(stream); i++ {
		if stream[i] == 0 {
			zeroCount++
		}
	}
	if zeroCount > 0 {
		t.Errorf("Stream has %d zero values, expected none", zeroCount)
	}
}

func TestBuildStreamDirect(t *testing.T) {
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	samples := []providers.TimedSample{
		{Timestamp: startTime, Value: 100},
		{Timestamp: startTime.Add(10 * time.Second), Value: 120},
		{Timestamp: startTime.Add(20 * time.Second), Value: 130},
	}

	stream := buildStreamDirect(samples, startTime, 30)

	if len(stream) != 30 {
		t.Errorf("Expected stream length 30, got %d", len(stream))
	}
	if stream[0] != 100 {
		t.Errorf("Expected stream[0]=100, got %d", stream[0])
	}
	if stream[10] != 120 {
		t.Errorf("Expected stream[10]=120, got %d", stream[10])
	}
	if stream[20] != 130 {
		t.Errorf("Expected stream[20]=130, got %d", stream[20])
	}
}

func TestBuildStreamInterpolated(t *testing.T) {
	// HR data is 20 seconds, activity is 40 seconds (2x stretch)
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	samples := []providers.TimedSample{
		{Timestamp: startTime, Value: 100},
		{Timestamp: startTime.Add(10 * time.Second), Value: 150},
		{Timestamp: startTime.Add(20 * time.Second), Value: 200},
	}

	overlap := OverlapResult{
		HRStart: startTime,
		HREnd:   startTime.Add(20 * time.Second),
	}

	stream := buildStreamInterpolated(samples, startTime, 40, overlap)

	if len(stream) != 40 {
		t.Errorf("Expected stream length 40, got %d", len(stream))
	}

	// First value should be 100
	if stream[0] != 100 {
		t.Errorf("Expected stream[0]=100, got %d", stream[0])
	}

	// Mid value (originally at 10s, should be at ~20s now)
	if stream[20] != 150 {
		t.Errorf("Expected stream[20]=150, got %d", stream[20])
	}

	// No zeros should remain
	for i, v := range stream {
		if v == 0 {
			t.Errorf("Unexpected zero at index %d", i)
			break
		}
	}
}

func TestForwardFillStream(t *testing.T) {
	stream := []int{100, 0, 0, 120, 0, 130}
	forwardFillStream(stream)

	expected := []int{100, 100, 100, 120, 120, 130}
	for i := range stream {
		if stream[i] != expected[i] {
			t.Errorf("At index %d: expected %d, got %d", i, expected[i], stream[i])
		}
	}
}
