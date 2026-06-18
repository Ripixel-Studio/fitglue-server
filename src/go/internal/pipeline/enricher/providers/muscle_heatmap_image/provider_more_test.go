package muscle_heatmap_image

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMuscleHeatmapImage_ShouldDefer(t *testing.T) {
	if !NewMuscleHeatmapImageProvider().ShouldDefer() {
		t.Error("muscle_heatmap_image should defer")
	}
}

func TestPercentageToColor(t *testing.T) {
	cases := []struct {
		pct  float64
		want string
	}{
		{0, ""},
		{-0.1, ""},
		{0.10, "#9370DB"},
		{0.30, "#8B5CF6"},
		{0.50, "#7C3AED"},
		{0.70, "#D946EF"},
		{0.90, "#EC4899"},
		{1.0, "#EC4899"},
	}
	for _, c := range cases {
		if got := percentageToColor(c.pct); got != c.want {
			t.Errorf("percentageToColor(%.2f) = %q, want %q", c.pct, got, c.want)
		}
	}
}

func TestGetMuscleSVGIDs(t *testing.T) {
	mapped := []pbactivity.MuscleGroup{
		pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_LATS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK,
		pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK,
		pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS,
		pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES,
		pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES,
	}
	for _, m := range mapped {
		if len(getMuscleSVGIDs(m)) == 0 {
			t.Errorf("expected SVG IDs for %v", m)
		}
	}

	// FULL_BODY / CARDIO / UNSPECIFIED have no SVG representation.
	for _, m := range []pbactivity.MuscleGroup{
		pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY,
		pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO,
		pbactivity.MuscleGroup_MUSCLE_GROUP_UNSPECIFIED,
	} {
		if got := getMuscleSVGIDs(m); got != nil {
			t.Errorf("expected nil SVG IDs for %v, got %v", m, got)
		}
	}
}

func TestExtractInnerSVG(t *testing.T) {
	in := []byte(`<?xml version="1.0"?><svg width="10"><rect/></svg>`)
	out := string(extractInnerSVG(in))
	if out != "<rect/>" {
		t.Errorf("extractInnerSVG = %q, want <rect/>", out)
	}

	// Content with no <svg> tag is returned unchanged.
	plain := []byte("no svg here")
	if string(extractInnerSVG(plain)) != "no svg here" {
		t.Error("expected unchanged content when no svg tag present")
	}
}

func TestGenerateSVG_Woman(t *testing.T) {
	p := NewMuscleHeatmapImageProvider()
	scores := []MuscleScore{
		{SVGIDs: []string{"pectoralis_major"}, Percentage: 1.0, Color: "#EC4899"},
	}
	svg, err := p.GenerateSVG("woman", scores)
	if err != nil {
		t.Fatalf("GenerateSVG(woman) error: %v", err)
	}
	if !strings.Contains(svg, "front-view") || !strings.Contains(svg, "back-view") {
		t.Error("expected front and back view groups")
	}
	if !strings.Contains(svg, "#EC4899") {
		t.Error("expected muscle color injected")
	}
}

func athleteUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{
		UserId: "u1",
		Tier:   pbuser.UserTier_USER_TIER_ATHLETE,
	}}
}

func TestMuscleHeatmapImage_Enrich_NonAthleteSkipped(t *testing.T) {
	p := NewMuscleHeatmapImageProvider()
	hobbyist := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1", Tier: pbuser.UserTier_USER_TIER_HOBBYIST}}
	res, err := p.Enrich(context.Background(), discardLogger(), &pbactivity.StandardizedActivity{}, hobbyist, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected skip for non-athlete, got %+v", res)
	}
}

func TestMuscleHeatmapImage_Enrich_NoStrengthSets(t *testing.T) {
	p := NewMuscleHeatmapImageProvider()
	activity := &pbactivity.StandardizedActivity{Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, athleteUser(), nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if !res.Skipped || res.SkipReason != "No strength sets found" {
		t.Errorf("expected no-strength-sets skip, got %+v", res)
	}
}

func TestMuscleHeatmapImage_Enrich_SuccessNoStore(t *testing.T) {
	p := NewMuscleHeatmapImageProvider() // no service → SVG generated, no asset stored
	activity := &pbactivity.StandardizedActivity{
		ExternalId: "act-1",
		Type:       pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
		Sessions: []*pbactivity.Session{
			{StrengthSets: []*pbactivity.StrengthSet{
				{ExerciseName: "Bench Press", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST, WeightKg: 100, Reps: 10},
				{ExerciseName: "Squat", PrimaryMuscleGroup: pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, WeightKg: 150, Reps: 8},
			}},
		},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, athleteUser(), map[string]string{"gender": "female"}, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Skipped {
		t.Fatalf("expected success, got skip: %s", res.SkipReason)
	}
	if res.Metadata["muscle_groups_activated"] == "" || res.Metadata["muscle_groups_activated"] == "0" {
		t.Errorf("expected activated muscle groups, got %q", res.Metadata["muscle_groups_activated"])
	}
	if _, ok := res.Metadata["asset_muscle_heatmap"]; ok {
		t.Error("did not expect asset URL without a configured store")
	}
}
