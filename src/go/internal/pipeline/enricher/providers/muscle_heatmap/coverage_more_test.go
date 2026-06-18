package muscle_heatmap

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	user "github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

func TestGetPresetCoefficients_AllPresets(t *testing.T) {
	pl := GetPresetCoefficients("powerlifting")
	if pl[pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS] != 3.5 {
		t.Errorf("powerlifting biceps coeff = %f, want 3.5", pl[pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS])
	}
	bb := GetPresetCoefficients("bodybuilding")
	if bb[pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS] != 4.0 {
		t.Errorf("bodybuilding forearms coeff = %f, want 4.0", bb[pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS])
	}
	std := GetPresetCoefficients("anything-else")
	if std[pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS] != 1.0 {
		t.Errorf("standard quads coeff = %f, want 1.0", std[pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS])
	}
}

func TestGetMuscleGroupCategory(t *testing.T) {
	if cat := GetMuscleGroupCategory(pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS); cat != "Arms" {
		t.Errorf("expected Arms, got %s", cat)
	}
	if cat := GetMuscleGroupCategory(pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST); cat != "Chest" {
		t.Errorf("expected Chest, got %s", cat)
	}
	// UNSPECIFIED is not in the category map -> falls back to formatted name.
	got := GetMuscleGroupCategory(pbactivity.MuscleGroup_MUSCLE_GROUP_UNSPECIFIED)
	if got == "" {
		t.Error("expected non-empty fallback category for unspecified muscle")
	}
}

func TestGetMuscleCoefficient_Fallback(t *testing.T) {
	coeffs := map[pbactivity.MuscleGroup]float64{
		pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST: 1.5,
	}
	if v := GetMuscleCoefficient(coeffs, pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST); v != 1.5 {
		t.Errorf("expected 1.5, got %f", v)
	}
	// Missing key -> default 1.0
	if v := GetMuscleCoefficient(coeffs, pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS); v != 1.0 {
		t.Errorf("expected fallback 1.0, got %f", v)
	}
}

func TestFormatMuscleRow_Styles(t *testing.T) {
	p := NewMuscleHeatmapProvider()

	// Percentage style
	pct := p.formatMuscleRow("Chest", 50, 3, 100, 5, pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_PERCENTAGE)
	if !strings.Contains(pct, "50%") {
		t.Errorf("expected 50%% row, got %q", pct)
	}

	// Percentage style with zero max -> 0%
	pctZero := p.formatMuscleRow("Chest", 50, 3, 0, 5, pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_PERCENTAGE)
	if !strings.Contains(pctZero, "0%") {
		t.Errorf("expected 0%% row when maxScore=0, got %q", pctZero)
	}

	// Text-only: cover all four level buckets with barLength=8.
	cases := []struct {
		rating int
		want   string
	}{
		{8, "Very High"}, // >= 6
		{4, "High"},      // >= 4
		{2, "Medium"},    // >= 2
		{0, "Low"},       // below
	}
	for _, c := range cases {
		row := p.formatMuscleRow("Chest", 10, c.rating, 100, 8, pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_TEXT_ONLY)
		if !strings.Contains(row, c.want) {
			t.Errorf("rating %d: expected level %q, got %q", c.rating, c.want, row)
		}
	}

	// Default emoji bars
	bars := p.formatMuscleRow("Chest", 10, 3, 100, 5, pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_EMOJI_BARS)
	if !strings.Contains(bars, "🟪") || !strings.Contains(bars, "⬜") {
		t.Errorf("expected emoji bar row, got %q", bars)
	}
}

func TestCalculateLoad_Branches(t *testing.T) {
	tests := []struct {
		name string
		set  *pbactivity.StrengthSet
		want float64
	}{
		{"distance-based", &pbactivity.StrengthSet{DistanceMeters: 1000}, 100},
		{"duration-based", &pbactivity.StrengthSet{DurationSeconds: 60}, 30},
		{"weight x reps", &pbactivity.StrengthSet{WeightKg: 50, Reps: 10}, 500},
		{"bodyweight heuristic", &pbactivity.StrengthSet{Reps: 5}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateLoad(tt.set); got != tt.want {
				t.Errorf("CalculateLoad = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestStripEquipmentSuffix(t *testing.T) {
	if got := stripEquipmentSuffix("Bench Press (Barbell)"); got != "Bench Press" {
		t.Errorf("expected 'Bench Press', got %q", got)
	}
	if got := stripEquipmentSuffix("Squat"); got != "Squat" {
		t.Errorf("expected unchanged 'Squat', got %q", got)
	}
	// Leading paren (idx 0) is not stripped.
	if got := stripEquipmentSuffix("(weird)"); got != "(weird)" {
		t.Errorf("expected unchanged '(weird)', got %q", got)
	}
}

func TestLookupExercise_Cases(t *testing.T) {
	// Empty name -> not matched.
	if r := LookupExercise(""); r.Matched {
		t.Error("expected empty name to not match")
	}

	// Equipment suffix should strip and still match a known exercise.
	r := LookupExercise("Bench Press (Barbell)")
	if !r.Matched {
		t.Error("expected 'Bench Press (Barbell)' to match via equipment stripping")
	}

	// Total nonsense -> no match, OTHER fallback.
	if r := LookupExercise("zzzqqq nonexistent exercise"); r.Matched {
		t.Error("expected nonsense exercise to not match")
	}
}

// TestEnrich_PresetAndStyleAndTaxonomyFallback drives the preset coefficient path,
// bar_length clamping, percentage style, and the taxonomy lookup fallback when a
// set has no primary muscle group.
func TestEnrich_PresetAndStyleAndTaxonomyFallback(t *testing.T) {
	p := NewMuscleHeatmapProvider()
	sets := []*pbactivity.StrengthSet{
		// Unspecified primary -> taxonomy LookupExercise fallback by name.
		{ExerciseName: "Bench Press", WeightKg: 80, Reps: 8},
		{ExerciseName: "Squat", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
			SecondaryMuscleGroups: []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES},
			WeightKg:              100, Reps: 5},
	}
	activity := makeTestActivity(sets)

	cfg := map[string]string{
		"preset":     "powerlifting",
		"style":      "percentage",
		"bar_length": "99", // clamped to 10
	}
	res, err := p.Enrich(context.Background(), slog.Default(), activity, &user.Record{}, cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Description == "" {
		t.Error("expected non-empty description")
	}
	if !strings.Contains(res.Description, "%") {
		t.Errorf("expected percentage style output, got:\n%s", res.Description)
	}
	if res.Enrichments == nil || res.Enrichments.MuscleHeatmap == nil {
		t.Fatal("expected muscle heatmap enrichments")
	}
}

// TestEnrich_TextStyleAndLowBarClamp covers text style output and the lower bar
// length clamp (<3 -> 3).
func TestEnrich_TextStyleAndLowBarClamp(t *testing.T) {
	p := NewMuscleHeatmapProvider()
	sets := []*pbactivity.StrengthSet{
		{ExerciseName: "Squat", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, WeightKg: 100, Reps: 10},
		{ExerciseName: "Curl", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS, WeightKg: 20, Reps: 10},
	}
	cfg := map[string]string{"style": "text", "bar_length": "1"}
	res, err := p.Enrich(context.Background(), slog.Default(), makeTestActivity(sets), &user.Record{}, cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Text style emits level words rather than emoji bars.
	if strings.Contains(res.Description, "🟪") {
		t.Errorf("did not expect emoji bars in text style, got:\n%s", res.Description)
	}
}
