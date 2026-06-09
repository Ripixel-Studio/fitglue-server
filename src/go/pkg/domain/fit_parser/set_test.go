package fit_parser

import (
	"os"
	"testing"

	"github.com/muktihari/fit/profile/typedef"
)

func TestParseFitFile_StrengthSets(t *testing.T) {
	// This test uses a sample FIT file which contains 13 Set messages
	data, err := os.ReadFile("../../../../../web/.cache/21808300906_ACTIVITY.fit")
	if err != nil {
		t.Skip("Skipping: sample FIT file not available")
	}

	activity, err := ParseFitFile(data)
	if err != nil {
		t.Fatalf("ParseFitFile failed: %v", err)
	}

	// Verify sessions exist
	if len(activity.Sessions) == 0 {
		t.Fatal("Expected at least one session")
	}

	session := activity.Sessions[0]

	// The file has 13 active Set messages
	if len(session.StrengthSets) != 13 {
		t.Errorf("Expected 13 StrengthSets, got %d", len(session.StrengthSets))
	}

	// Verify first set (Jump Rope equivalent - category: cardio)
	if len(session.StrengthSets) > 0 {
		first := session.StrengthSets[0]
		if first.StartTime == nil {
			t.Error("Expected first set to have StartTime")
		}
		if first.Reps != 9 {
			t.Errorf("Expected first set reps=9, got %d", first.Reps)
		}
		if first.DurationSeconds == 0 {
			t.Error("Expected first set to have non-zero duration")
		}
		if first.ExerciseName == "" {
			t.Error("Expected first set to have exercise name from category")
		}
		t.Logf("First set: name=%q, reps=%d, dur=%ds, start=%s",
			first.ExerciseName, first.Reps, first.DurationSeconds, first.StartTime.AsTime().Format("15:04:05"))
	}

	// Verify time markers were generated
	if len(activity.TimeMarkers) == 0 {
		t.Error("Expected TimeMarkers to be generated from Set messages")
	}

	// Log all markers for verification
	for i, m := range activity.TimeMarkers {
		t.Logf("Marker %d: %s @ %s (type=%s)",
			i+1, m.Label, m.Timestamp.AsTime().Format("15:04:05"), m.MarkerType)
	}

	// Each Set message gets its own marker — no grouping
	if len(activity.TimeMarkers) != 13 {
		t.Errorf("Expected 13 time markers (one per set), got %d", len(activity.TimeMarkers))
	}

	// All markers should have exercise_start type
	for _, m := range activity.TimeMarkers {
		if m.MarkerType != "exercise_start" {
			t.Errorf("Expected MarkerType 'exercise_start', got %q", m.MarkerType)
		}
	}

	t.Logf("Successfully parsed %d StrengthSets and %d TimeMarkers from FIT file",
		len(session.StrengthSets), len(activity.TimeMarkers))
}

func TestGenerateExerciseTimeMarkers(t *testing.T) {
	tests := []struct {
		name    string
		sets    []setInfo
		wantLen int
		wantNil bool
	}{
		{
			name:    "empty sets",
			sets:    nil,
			wantNil: true,
		},
		{
			name: "single set",
			sets: []setInfo{
				{exerciseName: "Bench Press"},
			},
			wantLen: 1,
		},
		{
			name: "consecutive same exercise not grouped",
			sets: []setInfo{
				{exerciseName: "Bench Press"},
				{exerciseName: "Bench Press"},
				{exerciseName: "Bench Press"},
			},
			wantLen: 3,
		},
		{
			name: "different exercises get separate markers",
			sets: []setInfo{
				{exerciseName: "Bench Press"},
				{exerciseName: "Squat"},
				{exerciseName: "Deadlift"},
			},
			wantLen: 3,
		},
		{
			name: "alternating exercises",
			sets: []setInfo{
				{exerciseName: "Sit Up"},
				{exerciseName: "Plank"},
				{exerciseName: "Sit Up"},
			},
			wantLen: 3, // Sit Up, Plank, Sit Up (not grouped because non-consecutive)
		},
		{
			name: "empty name defaults to Exercise",
			sets: []setInfo{
				{exerciseName: ""},
				{exerciseName: ""},
			},
			wantLen: 2, // Each set gets its own marker
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markers := generateExerciseTimeMarkers(tt.sets)
			if tt.wantNil {
				if markers != nil {
					t.Errorf("Expected nil, got %d markers", len(markers))
				}
				return
			}
			if len(markers) != tt.wantLen {
				t.Errorf("Expected %d markers, got %d", tt.wantLen, len(markers))
			}
		})
	}
}

func TestFormatExerciseCategory(t *testing.T) {
	tests := []struct {
		name string
		cat  string
		want string
	}{
		{"bench press", "bench_press", "Bench Press"},
		{"sit up", "sit_up", "Sit Up"},
		{"cardio", "cardio", "Cardio"},
		{"lateral raise", "lateral_raise", "Lateral Raise"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the typedef to get the ExerciseCategory from string
			cat := typedef.ExerciseCategoryFromString(tt.cat)
			got := formatExerciseCategory(cat)
			if got != tt.want {
				t.Errorf("formatExerciseCategory(%q) = %q, want %q", tt.cat, got, tt.want)
			}
		})
	}
}
