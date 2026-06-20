package muscle_heatmap

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
)

func makeTestActivity(sets []*pbactivity.StrengthSet) *pbactivity.StandardizedActivity {
	return &pbactivity.StandardizedActivity{
		Name: "Test Workout",
		Sessions: []*pbactivity.Session{
			{StrengthSets: sets},
		},
	}
}

func TestEnrich_DefaultNoGroupBy(t *testing.T) {
	p := NewMuscleHeatmapProvider()

	sets := []*pbactivity.StrengthSet{
		{ExerciseName: "Squat", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, WeightKg: 100, Reps: 10},
		{ExerciseName: "Leg Curl", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS, WeightKg: 40, Reps: 10},
	}

	result, err := p.Enrich(context.Background(), slog.Default(), makeTestActivity(sets), &user.Record{}, map[string]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default: individual muscles should appear
	if !strings.Contains(result.Description, "Quadriceps") {
		t.Errorf("expected 'Quadriceps' in default output, got:\n%s", result.Description)
	}
	if !strings.Contains(result.Description, "Hamstrings") {
		t.Errorf("expected 'Hamstrings' in default output, got:\n%s", result.Description)
	}
	// Should NOT contain "Legs" as a rolled-up group
	if strings.Contains(result.Description, "Legs") {
		t.Errorf("did not expect rolled-up 'Legs' in default output, got:\n%s", result.Description)
	}
}

func TestEnrich_GroupByMuscleGroup(t *testing.T) {
	p := NewMuscleHeatmapProvider()

	sets := []*pbactivity.StrengthSet{
		{ExerciseName: "Squat", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, WeightKg: 100, Reps: 10},
		{ExerciseName: "Leg Curl", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS, WeightKg: 40, Reps: 10},
		{ExerciseName: "Bench Press", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST, WeightKg: 80, Reps: 10},
		{ExerciseName: "Bicep Curl", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS, WeightKg: 20, Reps: 10},
		{ExerciseName: "Tricep Pushdown", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS, WeightKg: 30, Reps: 10},
	}

	config := map[string]string{"group_by": "muscle_group"}
	result, err := p.Enrich(context.Background(), slog.Default(), makeTestActivity(sets), &user.Record{}, config, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Rolled-up categories should appear
	if !strings.Contains(result.Description, "Legs") {
		t.Errorf("expected 'Legs' in grouped output, got:\n%s", result.Description)
	}
	if !strings.Contains(result.Description, "Chest") {
		t.Errorf("expected 'Chest' in grouped output, got:\n%s", result.Description)
	}
	if !strings.Contains(result.Description, "Arms") {
		t.Errorf("expected 'Arms' in grouped output, got:\n%s", result.Description)
	}

	// Individual muscles should NOT appear
	if strings.Contains(result.Description, "Quadriceps") {
		t.Errorf("did not expect 'Quadriceps' in grouped output, got:\n%s", result.Description)
	}
	if strings.Contains(result.Description, "Hamstrings") {
		t.Errorf("did not expect 'Hamstrings' in grouped output, got:\n%s", result.Description)
	}
	if strings.Contains(result.Description, "Biceps") {
		t.Errorf("did not expect 'Biceps' in grouped output, got:\n%s", result.Description)
	}
	if strings.Contains(result.Description, "Triceps") {
		t.Errorf("did not expect 'Triceps' in grouped output, got:\n%s", result.Description)
	}
}

func TestEnrich_GroupByMuscleGroup_ScoresAreSummed(t *testing.T) {
	p := NewMuscleHeatmapProvider()

	// Two quad exercises should combine into one Legs entry
	sets := []*pbactivity.StrengthSet{
		{ExerciseName: "Squat", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, WeightKg: 100, Reps: 10},
		{ExerciseName: "Leg Curl", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS, WeightKg: 100, Reps: 10},
	}

	config := map[string]string{"group_by": "muscle_group", "style": "percentage"}
	result, err := p.Enrich(context.Background(), slog.Default(), makeTestActivity(sets), &user.Record{}, config, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With both muscles contributing equally and both being "Legs",
	// we should see exactly one "Legs" line at 100%
	lines := strings.Split(result.Description, "\n")
	legsCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "Legs:") {
			legsCount++
			if !strings.Contains(line, "100%") {
				t.Errorf("expected Legs at 100%%, got: %s", line)
			}
		}
	}
	if legsCount != 1 {
		t.Errorf("expected exactly 1 Legs line, got %d in:\n%s", legsCount, result.Description)
	}
}

func TestEnrich_EmptySets(t *testing.T) {
	p := NewMuscleHeatmapProvider()

	result, err := p.Enrich(context.Background(), slog.Default(), makeTestActivity(nil), &user.Record{}, map[string]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Description != "" {
		t.Errorf("expected empty description for no sets, got: %s", result.Description)
	}
}

func TestEnrich_SetsButNoRecognizedMuscleGroups(t *testing.T) {
	p := NewMuscleHeatmapProvider()

	// Sets exist but their exercises map to no recognized muscle group
	// (e.g. Pilates). The enricher should produce no output at all rather
	// than a header with an empty body.
	sets := []*pbactivity.StrengthSet{
		{ExerciseName: "Pilates Roll Up", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_OTHER, WeightKg: 0, Reps: 10},
		{ExerciseName: "Pilates Hundred", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_UNSPECIFIED, WeightKg: 0, Reps: 100},
	}

	result, err := p.Enrich(context.Background(), slog.Default(), makeTestActivity(sets), &user.Record{}, map[string]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Description != "" {
		t.Errorf("expected empty description when no muscle groups have volume, got: %q", result.Description)
	}
}

func TestRollUpScores(t *testing.T) {
	input := map[string]float64{
		"Quadriceps": 500.0,
		"Hamstrings": 300.0,
		"Glutes":     200.0,
		"Chest":      400.0,
		"Biceps":     150.0,
		"Triceps":    180.0,
		"Lats":       350.0,
	}

	result := RollUpScores(input)

	// Legs = 500 + 300 + 200 = 1000
	if result["Legs"] != 1000.0 {
		t.Errorf("expected Legs=1000, got %f", result["Legs"])
	}

	// Chest stays as-is
	if result["Chest"] != 400.0 {
		t.Errorf("expected Chest=400, got %f", result["Chest"])
	}

	// Arms = 150 + 180 = 330
	if result["Arms"] != 330.0 {
		t.Errorf("expected Arms=330, got %f", result["Arms"])
	}

	// Back = 350
	if result["Back"] != 350.0 {
		t.Errorf("expected Back=350, got %f", result["Back"])
	}

	// Individual muscles should not be present
	if _, ok := result["Quadriceps"]; ok {
		t.Error("did not expect 'Quadriceps' in rolled-up result")
	}
	if _, ok := result["Biceps"]; ok {
		t.Error("did not expect 'Biceps' in rolled-up result")
	}
}

func TestRollUpScores_UnknownMuscle(t *testing.T) {
	input := map[string]float64{
		"CustomMuscle": 100.0,
		"Chest":        200.0,
	}

	result := RollUpScores(input)

	// Unknown muscles keep their name
	if result["CustomMuscle"] != 100.0 {
		t.Errorf("expected CustomMuscle=100, got %f", result["CustomMuscle"])
	}
	if result["Chest"] != 200.0 {
		t.Errorf("expected Chest=200, got %f", result["Chest"])
	}
}

// Verify that Enrich produces a valid EnrichmentResult (not nil)
func TestEnrich_ReturnsMetadata(t *testing.T) {
	p := NewMuscleHeatmapProvider()

	sets := []*pbactivity.StrengthSet{
		{ExerciseName: "Squat", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, WeightKg: 100, Reps: 10},
	}

	result, err := p.Enrich(context.Background(), slog.Default(), makeTestActivity(sets), &user.Record{}, map[string]string{"group_by": "muscle_group"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil {
		t.Fatal("expected non-nil metadata")
	}
	if result.Metadata["muscle_groups_displayed"] == "" {
		t.Error("expected muscle_groups_displayed in metadata")
	}
}

// Ensure no crash when the providers package is used (compile-time check for interface)
var _ providers.Provider = (*MuscleHeatmapProvider)(nil)
