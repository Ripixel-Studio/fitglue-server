package fit_file_heart_rate

import (
	"context"
	"encoding/base64"
	"log/slog"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/file_generators"
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ffhrUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}
}

func TestFitFileHR_SetService(t *testing.T) {
	p := NewFitFileHRProvider()
	p.SetService(nil)
	if p.service != nil {
		t.Error("expected nil service")
	}
	svc := &bootstrap.Service{}
	p.SetService(svc)
	if p.service != svc {
		t.Error("expected service to be set")
	}
}

func TestExtractGPSTimestamps(t *testing.T) {
	start := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{
			Laps: []*pbactivity.Lap{{
				Records: []*pbactivity.Record{
					{Timestamp: timestamppb.New(start)},
					{Timestamp: timestamppb.New(start.Add(time.Second))},
					{}, // nil timestamp -> skipped
				},
			}},
		}},
	}
	ts := extractGPSTimestamps(activity)
	if len(ts) != 2 {
		t.Fatalf("expected 2 timestamps, got %d", len(ts))
	}
	if !ts[0].Equal(start) {
		t.Errorf("expected first timestamp %v, got %v", start, ts[0])
	}
}

// noHRActivity builds an activity with no HR data so Enrich proceeds past the skip gate.
func noHRActivity() *pbactivity.StandardizedActivity {
	start := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	return &pbactivity.StandardizedActivity{
		StartTime:  timestamppb.New(start),
		Source:     pbactivity.ActivitySource_SOURCE_FILE_UPLOAD,
		ExternalId: "ext-1",
		Sessions: []*pbactivity.Session{{
			TotalElapsedTime: 3600,
			Laps: []*pbactivity.Lap{{
				Records: []*pbactivity.Record{{Timestamp: timestamppb.New(start)}},
			}},
		}},
	}
}

// TestFitFileHR_Enrich_AlreadyWaiting covers the pending-input STATUS_WAITING branch.
func TestFitFileHR_Enrich_AlreadyWaiting(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
			return &pbpipeline.PendingInput{Status: pbpipeline.PendingInput_STATUS_WAITING}, nil
		},
	}
	p := NewFitFileHRProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	res, err := p.Enrich(context.Background(), slog.Default(), noHRActivity(), ffhrUser(), nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["hr_source"] != "pending" {
		t.Errorf("expected hr_source=pending, got %s", res.Metadata["hr_source"])
	}
}

// TestFitFileHR_Enrich_CompletedReapply covers the STATUS_COMPLETED branch which
// re-applies the stored FIT file via EnrichResume. With an empty fit_file_base64 the
// resume path returns a graceful skip (dismissed).
func TestFitFileHR_Enrich_CompletedReapply(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
			return &pbpipeline.PendingInput{
				Status:    pbpipeline.PendingInput_STATUS_COMPLETED,
				InputData: map[string]string{}, // no file -> dismissed path
			}, nil
		},
	}
	p := NewFitFileHRProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	res, err := p.Enrich(context.Background(), slog.Default(), noHRActivity(), ffhrUser(), nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Skipped || res.Metadata["hr_source"] != "dismissed" {
		t.Errorf("expected dismissed skip from re-applied completed input, got %v", res.Metadata)
	}
}

// TestFitFileHR_Enrich_MissingActivityID covers the orchestrator-bug branch where
// no pending input exists and activity_id is absent from inputs.
func TestFitFileHR_Enrich_MissingActivityID(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
			return nil, nil // no existing pending input
		},
	}
	p := NewFitFileHRProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	_, err := p.Enrich(context.Background(), slog.Default(), noHRActivity(), ffhrUser(), nil, false)
	if err == nil {
		t.Fatal("expected error when activity_id is missing from inputs")
	}
}

// TestFitFileHR_Enrich_ForceOverwriteProceeds confirms force=true bypasses the
// existing-HR skip gate and proceeds into the pending-input flow.
func TestFitFileHR_Enrich_ForceOverwriteProceeds(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
			return &pbpipeline.PendingInput{Status: pbpipeline.PendingInput_STATUS_WAITING}, nil
		},
	}
	p := NewFitFileHRProvider()
	p.SetService(&bootstrap.Service{DB: mockDB})

	start := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime:  timestamppb.New(start),
		Source:     pbactivity.ActivitySource_SOURCE_FILE_UPLOAD,
		ExternalId: "ext-2",
		Sessions: []*pbactivity.Session{{
			TotalElapsedTime: 3600,
			Laps: []*pbactivity.Lap{{
				Records: []*pbactivity.Record{{HeartRate: 140, Timestamp: timestamppb.New(start)}},
			}},
		}},
	}

	res, err := p.Enrich(context.Background(), slog.Default(), activity, ffhrUser(), map[string]string{"force": "true"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// force=true means we did NOT take the skip branch; with a waiting pending input we get "pending".
	if res.Metadata["hr_source"] != "pending" {
		t.Errorf("expected force to bypass skip and reach pending flow, got %v", res.Metadata)
	}
}

// generateHRFitBase64 builds a real, parseable FIT file containing per-second HR
// records starting at hrStart, returned as base64 for the pending-input payload.
func generateHRFitBase64(t *testing.T, hrStart time.Time, count int, withGPS bool) string {
	t.Helper()
	records := make([]*pbactivity.Record, 0, count)
	for i := 0; i < count; i++ {
		r := &pbactivity.Record{
			Timestamp: timestamppb.New(hrStart.Add(time.Duration(i) * time.Second)),
			HeartRate: int32(120 + (i % 40)),
		}
		if withGPS {
			r.PositionLat = 51.5 + float64(i)*0.00001
			r.PositionLong = -0.12
		}
		records = append(records, r)
	}
	src := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(hrStart),
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Sessions: []*pbactivity.Session{{
			StartTime:        timestamppb.New(hrStart),
			TotalElapsedTime: float64(count),
			Laps:             []*pbactivity.Lap{{StartTime: timestamppb.New(hrStart), Records: records}},
		}},
	}
	data, err := file_generators.GenerateFitFile(src)
	if err != nil {
		t.Fatalf("failed to generate FIT file: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

// TestFitFileHR_EnrichResume_TimeBasedSuccess drives EnrichResume's success path
// where the target activity has no GPS, so the time-based stream builder is used.
func TestFitFileHR_EnrichResume_TimeBasedSuccess(t *testing.T) {
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	fitB64 := generateHRFitBase64(t, start, 300, false)

	p := NewFitFileHRProvider()
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 300}}, // no records -> no GPS
	}
	pi := &pbpipeline.PendingInput{InputData: map[string]string{"fit_file_base64": fitB64}}

	res, err := p.EnrichResume(context.Background(), activity, ffhrUser(), pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["hr_source"] != "fit_file" {
		t.Errorf("expected hr_source=fit_file, got %s", res.Metadata["hr_source"])
	}
	if len(res.HeartRateStream) != 300 {
		t.Errorf("expected stream length 300, got %d", len(res.HeartRateStream))
	}
	if res.Metadata["alignment_status"] != "time_based_no_gps" {
		t.Errorf("expected time_based_no_gps alignment, got %s", res.Metadata["alignment_status"])
	}
}

// TestFitFileHR_EnrichResume_GPSAlignmentSuccess drives EnrichResume's GPS branch,
// where the target activity has GPS records and elastic alignment runs.
func TestFitFileHR_EnrichResume_GPSAlignmentSuccess(t *testing.T) {
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	fitB64 := generateHRFitBase64(t, start, 300, true)

	// Target activity carries GPS records on the same time line.
	records := make([]*pbactivity.Record, 0, 300)
	for i := 0; i < 300; i++ {
		records = append(records, &pbactivity.Record{
			Timestamp:    timestamppb.New(start.Add(time.Duration(i) * time.Second)),
			PositionLat:  51.5 + float64(i)*0.00001,
			PositionLong: -0.12,
		})
	}
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions: []*pbactivity.Session{{
			TotalElapsedTime: 300,
			Laps:             []*pbactivity.Lap{{Records: records}},
		}},
	}

	p := NewFitFileHRProvider()
	pi := &pbpipeline.PendingInput{InputData: map[string]string{"fit_file_base64": fitB64}}

	res, err := p.EnrichResume(context.Background(), activity, ffhrUser(), pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["hr_source"] != "fit_file" {
		t.Errorf("expected hr_source=fit_file, got %s", res.Metadata["hr_source"])
	}
	// GPS path: must NOT report a time-based-no-gps status.
	if res.Metadata["alignment_status"] == "time_based_no_gps" {
		t.Errorf("expected GPS alignment to run, got time_based_no_gps")
	}
}

// TestBuildStreamTimeBased_InterpolateBranch drives the "interpolate" strategy
// (50-90%% overlap) of buildStreamTimeBased.
func TestBuildStreamTimeBased_InterpolateBranch(t *testing.T) {
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	// Activity is 100s; HR data only covers the first ~70s -> ~70%% overlap -> interpolate.
	samples := make([]providers.TimedSample, 70)
	for i := 0; i < 70; i++ {
		samples[i] = providers.TimedSample{
			Timestamp: start.Add(time.Duration(i) * time.Second),
			Value:     100 + i,
		}
	}
	stream := buildStreamTimeBased(samples, start, 100)
	if len(stream) != 100 {
		t.Fatalf("expected stream length 100, got %d", len(stream))
	}
	if stream[0] == 0 {
		t.Error("expected non-zero HR at stream start after interpolation")
	}
}
