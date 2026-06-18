package file_generators

import (
	"bytes"
	"testing"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/typedef"
	"google.golang.org/protobuf/types/known/timestamppb"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func TestGenerateFitFile_NilActivity(t *testing.T) {
	if _, err := GenerateFitFile(nil); err == nil {
		t.Fatal("expected error for nil activity")
	}
}

func TestGenerateFitFile_NoSessions(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(time.Now()),
		Sessions:  nil,
	}
	if _, err := GenerateFitFile(act); err == nil {
		t.Fatal("expected error for activity with no sessions")
	}
}

func TestGenerateFitFile_RichRecords(t *testing.T) {
	start := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	startPb := timestamppb.New(start)
	act := &pbactivity.StandardizedActivity{
		StartTime: startPb,
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		Source:    pbactivity.ActivitySource_SOURCE_STRAVA,
		Sessions: []*pbactivity.Session{
			{
				StartTime:        startPb,
				TotalElapsedTime: 2,
				TotalDistance:    1000,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{
								Timestamp:    timestamppb.New(start),
								HeartRate:    150,
								Power:        200,
								Cadence:      90,
								Speed:        5.0,
								Altitude:     100,
								PositionLat:  51.5,
								PositionLong: -0.12,
							},
							{
								// invalid (zero) timestamp -> skipped
								Timestamp: timestamppb.New(time.Time{}),
								HeartRate: 99,
							},
							{
								Timestamp: timestamppb.New(start.Add(time.Second)),
								HeartRate: 152,
							},
						},
					},
				},
			},
		},
	}

	out, err := GenerateFitFile(act)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dec := decoder.New(bytes.NewReader(out))
	data, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	var recordCount, sessionCount int
	for _, m := range data.Messages {
		switch m.Num {
		case typedef.MesgNumRecord:
			recordCount++
		case typedef.MesgNumSession:
			sessionCount++
		}
	}
	// One record skipped (zero timestamp) -> 2 valid records.
	if recordCount != 2 {
		t.Errorf("expected 2 records (1 skipped for zero ts), got %d", recordCount)
	}
	if sessionCount != 1 {
		t.Errorf("expected 1 session, got %d", sessionCount)
	}
}

func TestGenerateFitFile_NegativeAltitudeSkipped(t *testing.T) {
	// Altitude below -500m makes scaled value negative; SetAltitude is skipped but the record still encodes.
	start := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	startPb := timestamppb.New(start)
	act := &pbactivity.StandardizedActivity{
		StartTime: startPb,
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Sessions: []*pbactivity.Session{
			{
				StartTime: startPb,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{Timestamp: startPb, Altitude: -600},
						},
					},
				},
			},
		},
	}
	if _, err := GenerateFitFile(act); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
