package personal_records

import (
	"strings"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// --- formatDuration ---

func TestFormatDurationExtra(t *testing.T) {
	cases := []struct {
		seconds  float64
		expected string
	}{
		{0, "0:00"},
		{59, "0:59"},
		{60, "1:00"},
		{90, "1:30"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{5423, "1:30:23"},
	}
	for _, c := range cases {
		got := formatDuration(c.seconds)
		if got != c.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", c.seconds, got, c.expected)
		}
	}
}

// --- formatWeight ---

func TestFormatWeightExtra(t *testing.T) {
	cases := []struct {
		kg       float64
		expected string
	}{
		{20.0, "20kg"},
		{20.5, "20.5kg"},
		{100.0, "100kg"},
		{62.5, "62.5kg"},
	}
	for _, c := range cases {
		got := formatWeight(c.kg)
		if got != c.expected {
			t.Errorf("formatWeight(%v) = %q, want %q", c.kg, got, c.expected)
		}
	}
}

// --- formatVolume ---

func TestFormatVolumeExtra(t *testing.T) {
	cases := []struct {
		kg       float64
		expected string
	}{
		{500.0, "500kg"},
		{999.0, "999kg"},
		{1000.0, "1.0 tonnes"},
		{2500.0, "2.5 tonnes"},
	}
	for _, c := range cases {
		got := formatVolume(c.kg)
		if got != c.expected {
			t.Errorf("formatVolume(%v) = %q, want %q", c.kg, got, c.expected)
		}
	}
}

// --- formatExerciseName ---

func TestFormatExerciseNameExtra(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"bench_press", "Bench Press"},
		{"squat", "Squat"},
		{"overhead_press", "Overhead Press"},
	}
	for _, c := range cases {
		got := formatExerciseName(c.input)
		if got != c.expected {
			t.Errorf("formatExerciseName(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

// --- detectHybridRaceType ---

func TestDetectHybridRaceType(t *testing.T) {
	cases := []struct {
		name     string
		tags     []string
		actName  string
		expected string
	}{
		{"hyrox tag", []string{"HYROX", "Race"}, "Run", "hyrox"},
		{"athx tag", []string{"ATHX"}, "Run", "athx"},
		{"hyrox in name (no tag)", nil, "My Hyrox Race", ""},
		{"athx in name (no tag)", nil, "ATHX 2026", ""},
		{"no match", []string{"parkrun"}, "Morning Run", ""},
	}
	for _, c := range cases {
		act := &pbactivity.StandardizedActivity{
			Name: c.actName,
			Tags: c.tags,
		}
		got := detectHybridRaceType(act)
		if got != c.expected {
			t.Errorf("[%s] detectHybridRaceType() = %q, want %q", c.name, got, c.expected)
		}
	}
}

// --- normalizeStationName ---

func TestNormalizeStationName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"SkiErg", "skierg"},
		{"Sled Push", "sled_push"},
		{"Sled Pull", "sled_pull"},
		{"Burpee Broad Jump", "burpee_broad_jump"},
		{"Rowing Machine", "rowing"},
		{"Farmers Carry", "farmers_carry"},
		{"Sandbag Lunges", "sandbag_lunges"},
		{"Wall Ball", "wall_balls"},
		{"Unknown Exercise", ""},
	}
	for _, c := range cases {
		got := normalizeStationName(c.input)
		if got != c.expected {
			t.Errorf("normalizeStationName(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

// --- formatRecordTypeForDisplay ---

func TestFormatRecordTypeForDisplay_Cardio(t *testing.T) {
	cases := []struct {
		input    string
		contains string
	}{
		{"longest_run", "Longest Run"},
		{"longest_ride", "Longest Ride"},
		{"highest_elevation_gain", "Highest Elevation Gain"},
	}
	for _, c := range cases {
		got := formatRecordTypeForDisplay(c.input)
		if !strings.Contains(got, c.contains) {
			t.Errorf("formatRecordTypeForDisplay(%q) = %q, want to contain %q", c.input, got, c.contains)
		}
	}
}

func TestFormatRecordTypeForDisplay_Strength(t *testing.T) {
	cases := []struct {
		input    string
		contains string
	}{
		{"bench_press_1rm", "1RM"},
		{"squat_set_volume", "Best Set Volume"},
		{"deadlift_volume", "Total Volume"},
		{"pullup_reps", "Max Reps"},
	}
	for _, c := range cases {
		got := formatRecordTypeForDisplay(c.input)
		if !strings.Contains(got, c.contains) {
			t.Errorf("formatRecordTypeForDisplay(%q) = %q, want to contain %q", c.input, got, c.contains)
		}
	}
}

func TestFormatRecordTypeForDisplay_HeaviestWeight(t *testing.T) {
	got := formatRecordTypeForDisplay("deadlift" + string(SuffixHeaviestWeight))
	if got != "Deadlift Heaviest Weight" {
		t.Errorf("formatRecordTypeForDisplay(deadlift_heaviest_weight) = %q, want %q", got, "Deadlift Heaviest Weight")
	}
}

func TestFormatRecordTypeForDisplay_HybridRace(t *testing.T) {
	cases := []struct {
		input    string
		contains string
	}{
		{"hybrid_race_hyrox_total_time", "Total Time"},
		{"hybrid_race_hyrox_skierg", "HYROX"},
		{"hybrid_race_athx_total_time", "Total Time"},
	}
	for _, c := range cases {
		got := formatRecordTypeForDisplay(c.input)
		if !strings.Contains(got, c.contains) {
			t.Errorf("formatRecordTypeForDisplay(%q) = %q, want to contain %q", c.input, got, c.contains)
		}
	}
}

func TestFormatRecordTypeForDisplay_Fallback(t *testing.T) {
	got := formatRecordTypeForDisplay("some_custom_type")
	if got == "" {
		t.Error("expected non-empty fallback for unknown record type")
	}
}

// --- formatPRMessage ---

func TestFormatPRMessage_Speed(t *testing.T) {
	p := NewPersonalRecordsProvider()
	// fastest_ prefix -> 🎉 emoji
	msg := p.formatPRMessage("fastest_5km", 1200.0, nil, nil, "seconds", true)
	if !strings.Contains(msg, "🎉") {
		t.Errorf("expected 🎉 emoji for fastest_ record, got %q", msg)
	}
	if !strings.Contains(msg, ":") {
		t.Errorf("expected formatted duration in message, got %q", msg)
	}
}

func TestFormatPRMessage_Distance(t *testing.T) {
	p := NewPersonalRecordsProvider()
	// meters >= 1000 -> km
	msg := p.formatPRMessage("longest_run", 21097.0, nil, nil, "meters", false)
	if !strings.Contains(msg, "km") {
		t.Errorf("expected km in message for large distance, got %q", msg)
	}
}

func TestFormatPRMessage_DistanceUnderKm(t *testing.T) {
	p := NewPersonalRecordsProvider()
	msg := p.formatPRMessage("longest_run", 800.0, nil, nil, "meters", false)
	if !strings.Contains(msg, "m") || strings.Contains(msg, "km") {
		t.Errorf("expected m (not km) in message for short distance, got %q", msg)
	}
}

func TestFormatPRMessage_WithPreviousValue(t *testing.T) {
	p := NewPersonalRecordsProvider()
	prev := 1300.0
	imp := 8.33
	msg := p.formatPRMessage("fastest_5km", 1200.0, &prev, &imp, "seconds", true)
	if !strings.Contains(msg, "previous") {
		t.Errorf("expected 'previous' in message, got %q", msg)
	}
}

func TestFormatPRMessage_Strength(t *testing.T) {
	p := NewPersonalRecordsProvider()
	msg := p.formatPRMessage("bench_press_1rm", 100.0, nil, nil, "kg", false)
	if !strings.Contains(msg, "kg") {
		t.Errorf("expected kg in strength message, got %q", msg)
	}
}

func TestFormatPRMessage_Reps(t *testing.T) {
	p := NewPersonalRecordsProvider()
	msg := p.formatPRMessage("pullup_reps", 15.0, nil, nil, "reps", false)
	if !strings.Contains(msg, "reps") {
		t.Errorf("expected 'reps' in message, got %q", msg)
	}
}

func TestFormatPRMessage_HeaviestWeight(t *testing.T) {
	p := NewPersonalRecordsProvider()
	msg := p.formatPRMessage("deadlift"+string(SuffixHeaviestWeight), 140.0, nil, nil, "kg", false)
	if !strings.Contains(msg, "🏋️") {
		t.Errorf("expected 🏋️ emoji for heaviest weight record, got %q", msg)
	}
	if !strings.Contains(msg, "Heaviest Weight") {
		t.Errorf("expected 'Heaviest Weight' label in message, got %q", msg)
	}
	if !strings.Contains(msg, "140kg") {
		t.Errorf("expected weight value in message, got %q", msg)
	}
}

func TestFormatPRMessage_VolumeEmoji(t *testing.T) {
	p := NewPersonalRecordsProvider()
	msg := p.formatPRMessage("bench_press_set_volume", 1200.0, nil, nil, "kg", false)
	if !strings.Contains(msg, "💪") {
		t.Errorf("expected 💪 emoji for set_volume record, got %q", msg)
	}
}

func TestFormatPRMessage_DefaultUnit(t *testing.T) {
	p := NewPersonalRecordsProvider()
	msg := p.formatPRMessage("some_custom", 42.0, nil, nil, "custom_unit", false)
	if !strings.Contains(msg, "custom_unit") {
		t.Errorf("expected custom unit in message, got %q", msg)
	}
}
