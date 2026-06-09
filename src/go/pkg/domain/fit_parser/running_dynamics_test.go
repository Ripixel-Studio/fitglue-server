package fit_parser

import (
	"os"
	"testing"
)

func TestParseFitFile_RunningDynamics(t *testing.T) {
	// Path to the problematic FIT file
	filePath := "../../../cmd/fit-inspect/examples/running_dynamics_example.fit"
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Skipf("Skipping test - could not read test file: %v", err)
	}

	activity, err := ParseFitFile(data)
	if err != nil {
		t.Fatalf("ParseFitFile failed: %v", err)
	}

	if len(activity.Sessions) == 0 {
		t.Fatal("Expected at least one session")
	}

	session := activity.Sessions[0]

	// Check for speed and altitude (which were missing before due to "Enhanced" fields)
	hasSpeed := false
	hasAltitude := false
	hasGCT := false
	hasVO := false
	hasSL := false

	totalRecords := 0
	for _, lap := range session.Laps {
		totalRecords += len(lap.Records)
		for _, record := range lap.Records {
			if record.Speed > 0 {
				hasSpeed = true
			}
			if record.Altitude != 0 {
				hasAltitude = true
			}
			if record.GroundContactTime != nil && *record.GroundContactTime > 0 {
				hasGCT = true
			}
			if record.VerticalOscillation != nil && *record.VerticalOscillation > 0 {
				hasVO = true
			}
			if record.StepLength != nil && *record.StepLength > 0 {
				hasSL = true
			}
		}
	}

	if !hasSpeed {
		t.Error("Expected speed data (from EnhancedSpeed fallback)")
	}
	if !hasAltitude {
		t.Error("Expected altitude data (from EnhancedAltitude fallback)")
	}
	if !hasGCT {
		t.Error("Expected Ground Contact Time data")
	}
	if !hasVO {
		t.Error("Expected Vertical Oscillation data")
	}
	if !hasSL {
		t.Error("Expected Step Length data")
	}

	t.Logf("Verified Running Dynamics: Speed=%v, Alt=%v, GCT=%v, VO=%v, SL=%v (Total Records: %d)",
		hasSpeed, hasAltitude, hasGCT, hasVO, hasSL, totalRecords)
}
