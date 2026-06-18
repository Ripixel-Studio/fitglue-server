package file_generators

import (
	"testing"

	"github.com/muktihari/fit/profile/typedef"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func TestMapExerciseToCategory(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want typedef.ExerciseCategory
	}{
		{"bench press", "Barbell Bench Press", typedef.ExerciseCategoryBenchPress},
		{"chest press push up", "Push Up", typedef.ExerciseCategoryBenchPress},
		{"flye", "Cable Flye", typedef.ExerciseCategoryFlye},
		{"deadlift", "Romanian Deadlift", typedef.ExerciseCategoryDeadlift},
		{"row", "Barbell Row", typedef.ExerciseCategoryRow},
		{"pull up", "Pull Up", typedef.ExerciseCategoryPullUp},
		{"pulldown", "Lat Pulldown", typedef.ExerciseCategoryPullUp},
		{"squat", "Back Squat", typedef.ExerciseCategorySquat},
		{"lunge", "Walking Lunge", typedef.ExerciseCategoryLunge},
		{"leg press", "Leg Press", typedef.ExerciseCategorySquat},
		{"leg curl", "Seated Leg Curl", typedef.ExerciseCategoryLegCurl},
		{"leg extension", "Leg Extension", typedef.ExerciseCategoryLegCurl},
		{"calf raise", "Standing Calf Raise", typedef.ExerciseCategoryCalfRaise},
		{"shoulder press", "Overhead Press", typedef.ExerciseCategoryShoulderPress},
		{"lateral raise", "Lateral Raise", typedef.ExerciseCategoryLateralRaise},
		{"front raise", "Front Raise", typedef.ExerciseCategoryLateralRaise},
		// "rear delt" maps to lateral raise only when the name does not also contain "fly".
		{"rear delt", "Rear Delt Raise", typedef.ExerciseCategoryLateralRaise},
		// NOTE: possible bug: the "fly" branch (exercise_mapping.go:22) precedes the
		// "rear delt"/"reverse fly" branch (exercise_mapping.go:70), so any name containing
		// "fly" returns Flye and the "reverse fly" alias is unreachable. Asserting actual behavior.
		{"reverse fly is flye not lateral raise", "Reverse Fly", typedef.ExerciseCategoryFlye},
		{"shrug", "Barbell Shrug", typedef.ExerciseCategoryShrug},
		{"curl", "Bicep Curl", typedef.ExerciseCategoryCurl},
		{"triceps extension", "Triceps Extension", typedef.ExerciseCategoryTricepsExtension},
		{"dip", "Tricep Dip", typedef.ExerciseCategoryTricepsExtension},
		{"crunch", "Crunch", typedef.ExerciseCategoryCrunch},
		{"plank", "Plank", typedef.ExerciseCategoryPlank},
		{"clean", "Power Clean", typedef.ExerciseCategoryOlympicLift},
		{"snatch", "Snatch", typedef.ExerciseCategoryOlympicLift},
		{"default", "Unknown Movement", typedef.ExerciseCategoryTotalBody},
		{"trims whitespace and case", "   SQUAT   ", typedef.ExerciseCategorySquat},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MapExerciseToCategory(c.in); got != c.want {
				t.Errorf("MapExerciseToCategory(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestMapSport(t *testing.T) {
	cases := []struct {
		in           pbactivity.ActivityType
		wantSport    typedef.Sport
		wantSubSport typedef.SubSport
	}{
		{pbactivity.ActivityType_ACTIVITY_TYPE_RUN, typedef.SportRunning, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN, typedef.SportRunning, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, typedef.SportCycling, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE, typedef.SportCycling, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SWIM, typedef.SportSwimming, typedef.SubSportLapSwimming},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WALK, typedef.SportWalking, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_HIKE, typedef.SportHiking, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SNOWSHOE, typedef.SportHiking, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING, typedef.SportTraining, typedef.SubSportStrengthTraining},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT, typedef.SportTraining, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT, typedef.SportTraining, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_YOGA, typedef.SportTraining, typedef.SubSportYoga},
		{pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING, typedef.SportTraining, typedef.SubSportHiit},
		{pbactivity.ActivityType_ACTIVITY_TYPE_ROWING, typedef.SportRowing, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_KAYAKING, typedef.SportRowing, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_ALPINE_SKI, typedef.SportGeneric, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SOCCER, typedef.SportGeneric, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING, typedef.SportGeneric, typedef.SubSportGeneric},
		{pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, typedef.SportGeneric, typedef.SubSportGeneric},
	}
	for _, c := range cases {
		t.Run(c.in.String(), func(t *testing.T) {
			gotSport, gotSub := mapSport(c.in)
			if gotSport != c.wantSport {
				t.Errorf("mapSport(%v) sport = %v, want %v", c.in, gotSport, c.wantSport)
			}
			if gotSub != c.wantSubSport {
				t.Errorf("mapSport(%v) subSport = %v, want %v", c.in, gotSub, c.wantSubSport)
			}
		})
	}
}

func TestMapSourceToDevice(t *testing.T) {
	cases := []struct {
		in          string
		wantProduct string
	}{
		{"SOURCE_HEVY", "Hevy"},
		{"SOURCE_TEST", "FitGlue Test"},
		{"SOURCE_STRAVA", "FitGlue"},
		{"", "FitGlue"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			manuf, product := mapSourceToDevice(c.in)
			if manuf != typedef.Manufacturer(255) {
				t.Errorf("mapSourceToDevice(%q) manufacturer = %v, want 255 (development)", c.in, manuf)
			}
			if product != c.wantProduct {
				t.Errorf("mapSourceToDevice(%q) product = %q, want %q", c.in, product, c.wantProduct)
			}
		})
	}
}
