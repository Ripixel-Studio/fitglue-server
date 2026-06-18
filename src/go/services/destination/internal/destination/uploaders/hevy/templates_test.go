package hevy

import (
	"context"
	"io"
	"log/slog"
	"testing"

	hevy "github.com/fitglue/server/src/go/pkg/api/hevy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func strPtr(s string) *string { return &s }

func TestNormalizeExerciseName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Deadlift", "deadlift"},
		{"  Bench Press  ", "bench press"},
		{"Farmer's Carry", "farmers carry"},
		{"Sled-Push", "sled push"},
		{"Sled_Pull", "sled pull"},
		{"Farmers Carries", "farmers carry"},
		{"Squat (Barbell)", "squat"},
		{"Running (Outdoor)", "running"},
		{"Running (Treadmill)", "running"},
		{"Row (Machine)", "row"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeExerciseName(tc.in))
		})
	}
}

func TestGetExerciseTypeConfig(t *testing.T) {
	tests := []struct {
		name      string
		exercise  string
		wantType  string
		wantEquip string
	}{
		{"barbell strength", "Barbell Bench Press", "weight_reps", "barbell"},
		{"dumbbell strength", "Dumbbell Curl", "weight_reps", "dumbbell"},
		{"dumbell typo", "Dumbell Curl", "weight_reps", "dumbbell"},
		{"kettlebell", "Kettlebell Swing", "weight_reps", "kettlebell"},
		{"plate", "Plate Front Raise", "weight_reps", "plate"},
		{"band", "Band Pull Apart", "weight_reps", "resistance_band"},
		{"trx suspension", "TRX Row Suspension", "distance_duration", "machine"},
		{"duration only yoga", "Yoga", "duration", "none"},
		{"duration only crossfit", "CrossFit", "duration", "none"},
		{"duration only other cardio", "Other Cardio", "duration", "none"},
		{"distance duration skierg", "SkiErg", "distance_duration", "machine"},
		{"distance duration running", "Running (Outdoor)", "distance_duration", "none"},
		{"distance duration cycling", "Cycling (Outdoor)", "distance_duration", "machine"},
		{"weight duration sled", "Sled Push", "weight_duration", "other"},
		{"weight duration farmers", "Farmers Carry", "weight_duration", "other"},
		{"weight duration wall ball", "Wall Ball", "weight_duration", "other"},
		{"default weight reps", "Mystery Move", "weight_reps", "other"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := getExerciseTypeConfig(tc.exercise)
			assert.Equal(t, tc.wantType, cfg.ExerciseType, "exercise type")
			assert.Equal(t, tc.wantEquip, cfg.EquipmentCategory, "equipment category")
		})
	}
}

func TestNewTemplateResolver(t *testing.T) {
	r := NewTemplateResolver("api-key", testLogger())
	require.NotNil(t, r)
	assert.Equal(t, "api-key", r.apiKey)
	assert.NotNil(t, r.cache)
	assert.False(t, r.fetched)
}

func TestResolveTemplate_CacheHit(t *testing.T) {
	r := NewTemplateResolver("api-key", testLogger())
	r.fetched = true // avoid network fetch
	tmpl := &hevy.ExerciseTemplate{Id: strPtr("tmpl-123"), Title: strPtr("Deadlift")}
	r.cache[normalizeExerciseName("Deadlift")] = tmpl

	got, err := r.ResolveTemplate(context.Background(), "Deadlift")
	require.NoError(t, err)
	assert.Equal(t, "tmpl-123", *got.Id)
}

func TestResolveTemplate_StrictMatchPopulatesCache(t *testing.T) {
	r := NewTemplateResolver("api-key", testLogger())
	r.fetched = true
	r.templates = []hevy.ExerciseTemplate{
		{Id: strPtr("t-bench"), Title: strPtr("Bench Press")},
	}
	got, err := r.ResolveTemplate(context.Background(), "Bench Press")
	require.NoError(t, err)
	assert.Equal(t, "t-bench", *got.Id)
	// second call should hit cache
	got2, err := r.ResolveTemplate(context.Background(), "Bench Press")
	require.NoError(t, err)
	assert.Equal(t, "t-bench", *got2.Id)
}

func TestStrictMatch(t *testing.T) {
	r := NewTemplateResolver("api-key", testLogger())
	r.templates = []hevy.ExerciseTemplate{
		{Id: strPtr("t-row"), Title: strPtr("Rowing Machine")},
		{Id: strPtr("t-skierg"), Title: strPtr("Ski Erg")},
		{Title: strPtr("No ID Template")}, // nil Id should be ignored
	}

	t.Run("ExactMatch", func(t *testing.T) {
		got := r.strictMatch(normalizeExerciseName("Rowing Machine"))
		require.NotNil(t, got)
		assert.Equal(t, "t-row", *got.Id)
	})

	t.Run("AliasMatch", func(t *testing.T) {
		// "skierg" is a known alias of "ski erg"
		got := r.strictMatch("skierg")
		require.NotNil(t, got)
		assert.Equal(t, "t-skierg", *got.Id)
	})

	t.Run("NoMatch", func(t *testing.T) {
		got := r.strictMatch("nonexistent exercise")
		assert.Nil(t, got)
	})

	t.Run("NilIdIgnored", func(t *testing.T) {
		got := r.strictMatch("no id template")
		assert.Nil(t, got)
	})
}
