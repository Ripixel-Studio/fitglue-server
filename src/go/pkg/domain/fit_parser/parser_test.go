package fit_parser

import (
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"os"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/typedef"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestParseFitFile_GarminExample(t *testing.T) {
	// Load the actual Garmin FIT file
	data, err := os.ReadFile("../../../cmd/fit-inspect/examples/21583826023_ACTIVITY.fit")
	if err != nil {
		t.Skipf("Skipping test - could not read test file: %v", err)
	}

	activity, err := ParseFitFile(data)
	if err != nil {
		t.Fatalf("ParseFitFile failed: %v", err)
	}

	// Verify basic structure
	if activity == nil {
		t.Fatal("Expected non-nil activity")
	}

	if activity.Source != pbactivity.ActivitySource_SOURCE_FILE_UPLOAD {
		t.Errorf("Expected source 'SOURCE_FILE_UPLOAD', got %q", activity.Source)
	}

	if len(activity.Sessions) != 1 {
		t.Errorf("Expected exactly 1 session after merging, got %d", len(activity.Sessions))
	}

	session := activity.Sessions[0]

	// Verify we got records (the file has 1622 records)
	totalRecords := 0
	for _, lap := range session.Laps {
		totalRecords += len(lap.Records)
	}
	if totalRecords < 100 {
		t.Errorf("Expected at least 100 records, got %d", totalRecords)
	}

	// Verify we have GPS data (the file has position data)
	hasGPS := false
	for _, lap := range session.Laps {
		for _, record := range lap.Records {
			if record.PositionLat != 0 || record.PositionLong != 0 {
				hasGPS = true
				break
			}
		}
		if hasGPS {
			break
		}
	}
	if !hasGPS {
		t.Error("Expected GPS data in records")
	}

	// Verify we have heart rate data
	hasHR := false
	for _, lap := range session.Laps {
		for _, record := range lap.Records {
			if record.HeartRate > 0 {
				hasHR = true
				break
			}
		}
		if hasHR {
			break
		}
	}
	if !hasHR {
		t.Error("Expected heart rate data in records")
	}

	// The file is a Run activity
	if activity.Type != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("Expected activity type RUN, got %v", activity.Type)
	}

	t.Logf("Successfully parsed Garmin file: %d laps, %d records, type=%v",
		len(session.Laps), totalRecords, activity.Type)
}

func TestParseFitFile_EmptyData(t *testing.T) {
	_, err := ParseFitFile([]byte{})
	if err == nil {
		t.Error("Expected error for empty data")
	}
}

func TestParseFitFile_InvalidData(t *testing.T) {
	_, err := ParseFitFile([]byte("not a fit file"))
	if err == nil {
		t.Error("Expected error for invalid data")
	}
}

func TestMergeSessions(t *testing.T) {
	now := time.Now()
	sessions := []*pbactivity.Session{
		{
			StartTime:        timestamppb.New(now),
			TotalElapsedTime: 100,
			TotalDistance:    1000,
			Laps: []*pbactivity.Lap{
				{StartTime: timestamppb.New(now), TotalElapsedTime: 100, TotalDistance: 1000},
			},
		},
		{
			StartTime:        timestamppb.New(now.Add(2 * time.Minute)),
			TotalElapsedTime: 200,
			TotalDistance:    2000,
			Laps: []*pbactivity.Lap{
				{StartTime: timestamppb.New(now.Add(2 * time.Minute)), TotalElapsedTime: 200, TotalDistance: 2000},
			},
		},
	}

	merged := MergeSessions(sessions)

	if merged.TotalElapsedTime != 300 {
		t.Errorf("Expected merged elapsed time 300, got %v", merged.TotalElapsedTime)
	}

	if merged.TotalDistance != 3000 {
		t.Errorf("Expected merged distance 3000, got %v", merged.TotalDistance)
	}

	if len(merged.Laps) != 2 {
		t.Errorf("Expected 2 laps, got %d", len(merged.Laps))
	}

	// Start time should be from first session
	if !merged.StartTime.AsTime().Equal(now) {
		t.Errorf("Expected start time from first session")
	}
}

func TestMergeSessions_Single(t *testing.T) {
	now := time.Now()
	sessions := []*pbactivity.Session{
		{
			StartTime:        timestamppb.New(now),
			TotalElapsedTime: 100,
			TotalDistance:    1000,
		},
	}

	merged := MergeSessions(sessions)

	if merged != sessions[0] {
		t.Error("Expected single session to be returned as-is")
	}
}

func TestMergeSessions_Empty(t *testing.T) {
	merged := MergeSessions([]*pbactivity.Session{})
	if merged != nil {
		t.Error("Expected nil for empty sessions")
	}
}

func TestMapFitSportToActivityType(t *testing.T) {
	tests := []struct {
		sport    typedef.Sport
		subSport typedef.SubSport
		expected pbactivity.ActivityType
	}{
		{typedef.SportRunning, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_RUN},
		{typedef.SportRunning, typedef.SubSportTrail, pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN},
		{typedef.SportRunning, typedef.SubSportVirtualActivity, pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN},
		{typedef.SportCycling, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_RIDE},
		{typedef.SportCycling, typedef.SubSportVirtualActivity, pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE},
		{typedef.SportCycling, typedef.SubSportMountain, pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE},
		{typedef.SportSwimming, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_SWIM},
		{typedef.SportWalking, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_WALK},
		{typedef.SportHiking, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_HIKE},
		{typedef.SportTraining, typedef.SubSportStrengthTraining, pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING},
		{typedef.SportTraining, typedef.SubSportYoga, pbactivity.ActivityType_ACTIVITY_TYPE_YOGA},
		{typedef.SportTraining, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT},
		{typedef.SportGolf, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_GOLF},
		{typedef.SportGeneric, typedef.SubSportGeneric, pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT},
	}

	for _, tc := range tests {
		result := mapFitSportToActivityType(tc.sport, tc.subSport)
		if result != tc.expected {
			t.Errorf("mapFitSportToActivityType(%v, %v) = %v, want %v",
				tc.sport, tc.subSport, result, tc.expected)
		}
	}
}

func TestGenerateActivityName(t *testing.T) {
	morningTime := time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC)
	afternoonTime := time.Date(2024, 1, 1, 14, 0, 0, 0, time.UTC)
	eveningTime := time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC)

	tests := []struct {
		activityType pbactivity.ActivityType
		startTime    time.Time
		expected     string
	}{
		{pbactivity.ActivityType_ACTIVITY_TYPE_RUN, morningTime, "Morning Run"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_RUN, afternoonTime, "Afternoon Run"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, eveningTime, "Evening Ride"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SWIM, morningTime, "Morning Swim"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING, afternoonTime, "Afternoon Workout"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_YOGA, eveningTime, "Evening Yoga"},
	}

	for _, tc := range tests {
		result := GenerateActivityName(tc.activityType, tc.startTime)
		if result != tc.expected {
			t.Errorf("GenerateActivityName(%v, %v) = %q, want %q",
				tc.activityType, tc.startTime, result, tc.expected)
		}
	}
}
