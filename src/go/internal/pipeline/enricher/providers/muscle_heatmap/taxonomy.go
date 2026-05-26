package muscle_heatmap

import (
	"strings"
	"unicode"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// ExerciseMapping defines muscle targeting for a canonical exercise
type ExerciseMapping struct {
	CanonicalName  string
	Primary        pbactivity.MuscleGroup
	Secondary      []pbactivity.MuscleGroup
	PrimaryCoeff   float64  // Coefficient for primary muscle (default 1.0)
	SecondaryCoeff float64  // Coefficient for secondary muscles (default 0.5)
	Aliases        []string // Alternative names for matching
}

// LookupResult is returned by the fuzzy matcher
type LookupResult struct {
	Matched        bool
	CanonicalName  string
	Primary        pbactivity.MuscleGroup
	Secondary      []pbactivity.MuscleGroup
	PrimaryCoeff   float64
	SecondaryCoeff float64
	Confidence     float64 // 0.0-1.0 match confidence
}

// Common abbreviation expansions
var abbreviations = map[string]string{
	"db":   "dumbbell",
	"bb":   "barbell",
	"kb":   "kettlebell",
	"ez":   "ez bar",
	"ohp":  "overhead press",
	"rdl":  "romanian deadlift",
	"sldl": "stiff leg deadlift",
	"lat":  "lateral",
	"incl": "incline",
	"decl": "decline",
	"ext":  "extension",
	"curl": "curl",
}

// ExerciseDatabase contains all canonical exercises with their muscle mappings
var ExerciseDatabase = []ExerciseMapping{
	// ============================================================================
	// CHEST EXERCISES
	// ============================================================================
	{
		CanonicalName:  "Bench Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Flat Bench", "Barbell Bench Press", "BB Bench", "Chest Press", "Flat Bench Press"},
	},
	{
		CanonicalName:  "Incline Bench Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Incline Press", "Incline Barbell Press", "Incline BB Press"},
	},
	{
		CanonicalName:  "Decline Bench Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Decline Press", "Decline Barbell Press"},
	},
	{
		CanonicalName:  "Dumbbell Bench Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"DB Bench", "DB Bench Press", "Dumbbell Press", "Flat DB Press"},
	},
	{
		CanonicalName:  "Incline Dumbbell Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Incline DB Press", "Incline Dumbbell Bench Press"},
	},
	{
		CanonicalName:  "Chest Fly",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Dumbbell Fly", "DB Fly", "Pec Fly", "Chest Flye", "Flyes"},
	},
	{
		CanonicalName:  "Cable Fly",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Cable Crossover", "Cable Chest Fly", "Low Cable Fly", "High Cable Fly"},
	},
	{
		CanonicalName:  "Push Up",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Pushup", "Push-Up", "Press Up"},
	},
	{
		CanonicalName:  "Dip",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.6,
		Aliases:        []string{"Chest Dip", "Parallel Bar Dip", "Dips"},
	},
	{
		CanonicalName:  "Machine Chest Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Chest Press Machine", "Seated Chest Press"},
	},

	// ============================================================================
	// BACK EXERCISES
	// ============================================================================
	{
		CanonicalName:  "Deadlift",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS, pbactivity.MuscleGroup_MUSCLE_GROUP_LATS, pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.6,
		Aliases:        []string{"Conventional Deadlift", "Barbell Deadlift", "BB Deadlift"},
	},
	{
		CanonicalName:  "Romanian Deadlift",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.6,
		Aliases:        []string{"RDL", "Stiff Leg Deadlift", "SLDL", "Romanian Deadlift (Dumbbell)", "Romanian Deadlift (Barbell)"},
	},
	{
		CanonicalName:  "Pull Up",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_LATS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Pullup", "Pull-Up", "Wide Grip Pull Up"},
	},
	{
		CanonicalName:  "Chin Up",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_LATS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.6,
		Aliases:        []string{"Chinup", "Chin-Up", "Close Grip Pull Up"},
	},
	{
		CanonicalName:  "Lat Pulldown",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_LATS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Lat Pull Down", "Cable Pulldown", "Wide Grip Pulldown"},
	},
	{
		CanonicalName:  "Bent Over Row",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_LATS, pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Barbell Row", "BB Row", "Bent Over Barbell Row", "Bent Over Row (Barbell)", "Bent Over Row (Dumbbell)"},
	},
	{
		CanonicalName:  "Dumbbell Row",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_LATS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK, pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"DB Row", "One Arm Row", "Single Arm Row", "One Arm Dumbbell Row"},
	},
	{
		CanonicalName:  "Seated Cable Row",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_LATS, pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Cable Row", "Seated Row", "Low Row"},
	},
	{
		CanonicalName:  "T-Bar Row",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_LATS, pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"T Bar Row", "Landmine Row"},
	},
	{
		CanonicalName:  "Face Pull",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Cable Face Pull", "Rope Face Pull"},
	},
	{
		CanonicalName:  "Shrug",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Barbell Shrug", "Dumbbell Shrug", "DB Shrug", "Trap Shrug", "Shrugs"},
	},
	{
		CanonicalName:  "Back Extension",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Hyperextension", "Back Raise", "Lower Back Extension"},
	},

	// ============================================================================
	// SHOULDER EXERCISES
	// ============================================================================
	{
		CanonicalName:  "Overhead Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"OHP", "Military Press", "Shoulder Press", "Standing Press", "Barbell Overhead Press", "Overhead Press (Barbell)"},
	},
	{
		CanonicalName:  "Dumbbell Shoulder Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"DB Shoulder Press", "Seated Dumbbell Press", "Overhead Press (Dumbbell)", "Dumbbell Press"},
	},
	{
		CanonicalName:  "Arnold Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Arnold Dumbbell Press"},
	},
	{
		CanonicalName:  "Lateral Raise",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Side Raise", "Dumbbell Lateral Raise", "DB Lateral Raise", "Lateral Raise (Dumbbell)", "Side Lateral Raise"},
	},
	{
		CanonicalName:  "Front Raise",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Dumbbell Front Raise", "DB Front Raise", "Front Delt Raise"},
	},
	{
		CanonicalName:  "Rear Delt Fly",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Reverse Fly", "Rear Fly", "Bent Over Fly", "Rear Delt Raise"},
	},
	{
		CanonicalName:  "Upright Row",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS, pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Barbell Upright Row", "Dumbbell Upright Row"},
	},
	{
		CanonicalName:  "Shoulder Taps",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST, pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Plank Shoulder Taps"},
	},

	// ============================================================================
	// ARM EXERCISES - BICEPS
	// ============================================================================
	{
		CanonicalName:  "Bicep Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Dumbbell Curl", "DB Curl", "Arm Curl", "Standing Curl", "Bicep Curl (Dumbbell)", "Bicep Curl (Barbell)"},
	},
	{
		CanonicalName:  "Barbell Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"BB Curl", "Straight Bar Curl"},
	},
	{
		CanonicalName:  "Hammer Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Dumbbell Hammer Curl", "DB Hammer Curl", "Neutral Grip Curl"},
	},
	{
		CanonicalName:  "Preacher Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"EZ Bar Preacher Curl", "Dumbbell Preacher Curl", "Scott Curl"},
	},
	{
		CanonicalName:  "Concentration Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Seated Concentration Curl"},
	},
	{
		CanonicalName:  "Cable Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Cable Bicep Curl", "Rope Curl"},
	},

	// ============================================================================
	// ARM EXERCISES - TRICEPS
	// ============================================================================
	{
		CanonicalName:  "Tricep Extension",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Overhead Tricep Extension", "Dumbbell Tricep Extension", "Triceps Extension"},
	},
	{
		CanonicalName:  "Tricep Pushdown",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Cable Pushdown", "Rope Pushdown", "Tricep Rope Pushdown", "Triceps Pushdown"},
	},
	{
		CanonicalName:  "Skull Crusher",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Lying Tricep Extension", "EZ Bar Skull Crusher", "Skullcrusher"},
	},
	{
		CanonicalName:  "Tricep Dip",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Bench Dip", "Chair Dip", "Triceps Dip"},
	},
	{
		CanonicalName:  "Close Grip Bench Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"CGBP", "Narrow Grip Bench"},
	},
	{
		CanonicalName:  "Tricep Kickback",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Dumbbell Kickback", "DB Kickback", "Triceps Kickback", "Triceps Kickback (Dumbbell)"},
	},

	// ============================================================================
	// LEG EXERCISES - QUADRICEPS
	// ============================================================================
	{
		CanonicalName:  "Squat",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.6,
		Aliases:        []string{"Back Squat", "Barbell Squat", "BB Squat", "Squat (Barbell)"},
	},
	{
		CanonicalName:  "Front Squat",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Barbell Front Squat"},
	},
	{
		CanonicalName:  "Goblet Squat",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Kettlebell Goblet Squat", "Dumbbell Goblet Squat"},
	},
	{
		CanonicalName:  "Leg Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Machine Leg Press", "45 Degree Leg Press"},
	},
	{
		CanonicalName:  "Leg Extension",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Machine Leg Extension", "Quad Extension"},
	},
	{
		CanonicalName:  "Lunge",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Walking Lunge", "Dumbbell Lunge", "Barbell Lunge", "Lunge (Dumbbell)", "Lunge (Barbell)", "Forward Lunge"},
	},
	{
		CanonicalName:  "Walking Lunge",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Walking Lunge (Dumbbell)", "Walking Lunge (Barbell)", "DB Walking Lunge"},
	},
	{
		CanonicalName:  "Bulgarian Split Squat",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Split Squat", "Rear Foot Elevated Split Squat"},
	},
	{
		CanonicalName:  "Hack Squat",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Machine Hack Squat"},
	},
	{
		CanonicalName:  "Wall Sit",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Wall Squat", "Wall Hold"},
	},
	{
		CanonicalName:  "Sumo Squat",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_ADDUCTORS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Sumo Squat (Kettlebell)", "Wide Stance Squat", "Plie Squat"},
	},
	{
		CanonicalName:  "Curtsy Lunge",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_ADDUCTORS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Curtsy Lunge (Dumbbell)", "Curtsey Lunge"},
	},

	// ============================================================================
	// LEG EXERCISES - HAMSTRINGS/GLUTES
	// ============================================================================
	{
		CanonicalName:  "Leg Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.2,
		Aliases:        []string{"Lying Leg Curl", "Seated Leg Curl", "Hamstring Curl", "Machine Leg Curl"},
	},
	{
		CanonicalName:  "Hip Thrust",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Barbell Hip Thrust", "Glute Bridge", "Weighted Glute Bridge"},
	},
	{
		CanonicalName:  "Glute Kickback",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Cable Kickback", "Donkey Kick", "Glute Kickback (Cable)"},
	},
	{
		CanonicalName:  "Good Morning",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK, pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Barbell Good Morning"},
	},

	// ============================================================================
	// LEG EXERCISES - CALVES
	// ============================================================================
	{
		CanonicalName:  "Calf Raise",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Standing Calf Raise", "Seated Calf Raise", "Machine Calf Raise", "Calf Press"},
	},

	// ============================================================================
	// CORE EXERCISES
	// ============================================================================
	{
		CanonicalName:  "Crunch",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Ab Crunch", "Abdominal Crunch", "Sit Up", "Situp"},
	},
	{
		CanonicalName:  "Plank",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Front Plank", "Forearm Plank", "High Plank"},
	},
	{
		CanonicalName:  "Russian Twist",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Russian Twist (Weighted)", "Seated Russian Twist"},
	},
	{
		CanonicalName:  "Leg Raise",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Lying Leg Raise", "Hanging Leg Raise", "Leg Raises"},
	},
	{
		CanonicalName:  "Mountain Climber",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Mountain Climbers"},
	},
	{
		CanonicalName:  "Bicycle Crunch",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Bicycle", "Elbow to Knee"},
	},
	{
		CanonicalName:  "Dead Bug",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Dead Bugs"},
	},
	{
		CanonicalName:  "Heel Taps",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Heel Touches", "Alternating Heel Taps"},
	},
	{
		CanonicalName:  "Ab Wheel Rollout",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_LATS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Ab Wheel", "Rollout"},
	},

	// ============================================================================
	// FULL BODY / COMPOUND EXERCISES
	// ============================================================================
	{
		CanonicalName:  "Kettlebell Swing",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES, pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"KB Swing", "Russian Swing", "American Swing"},
	},
	{
		CanonicalName:  "Burpee",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST, pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Burpees", "Burpee Box Jump"},
	},
	{
		CanonicalName:  "Clean and Press",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Clean & Press", "Power Clean and Press"},
	},
	{
		CanonicalName:  "Thruster",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Thrusters", "Barbell Thruster", "Dumbbell Thruster"},
	},
	{
		CanonicalName:  "Snatch",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.5,
		Aliases:        []string{"Power Snatch", "Barbell Snatch", "Dumbbell Snatch", "KB Snatch"},
	},
	{
		CanonicalName:  "Farmers Walk",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS, pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS, pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Farmer's Walk", "Farmer Carry", "Farmers Carry", "Loaded Carry"},
	},
	{
		CanonicalName:  "Battle Ropes",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Battle Rope", "Rope Slams", "Rope Waves"},
	},

	// ============================================================================
	// CARDIO EXERCISES
	// ============================================================================
	{
		CanonicalName:  "Running",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Run", "Jogging", "Jog", "Treadmill Run", "Treadmill"},
	},
	{
		CanonicalName:  "Rowing",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_LATS, pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.4,
		Aliases:        []string{"Row", "Rowing Machine", "Erg", "Ergometer"},
	},
	{
		CanonicalName:  "Cycling",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS, pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Bike", "Cycling Machine", "Stationary Bike", "Spin"},
	},
	{
		CanonicalName:  "Jumping Jack",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO,
		Secondary:      []pbactivity.MuscleGroup{pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS, pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.3,
		Aliases:        []string{"Jumping Jacks", "Star Jumps"},
	},

	// ============================================================================
	// FOREARM EXERCISES
	// ============================================================================
	{
		CanonicalName:  "Wrist Curl",
		Primary:        pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS,
		Secondary:      []pbactivity.MuscleGroup{},
		PrimaryCoeff:   1.0,
		SecondaryCoeff: 0.0,
		Aliases:        []string{"Barbell Wrist Curl", "Dumbbell Wrist Curl", "Reverse Wrist Curl"},
	},
}

// Cached normalized database for faster lookups
var normalizedDB map[string]*ExerciseMapping
var aliasMap map[string]*ExerciseMapping

func init() {
	buildIndexes()
}

// buildIndexes creates normalized lookup maps for faster matching
func buildIndexes() {
	normalizedDB = make(map[string]*ExerciseMapping)
	aliasMap = make(map[string]*ExerciseMapping)

	for i := range ExerciseDatabase {
		ex := &ExerciseDatabase[i]

		// Index by normalized canonical name
		normalizedDB[normalize(ex.CanonicalName)] = ex

		// Index by normalized aliases
		for _, alias := range ex.Aliases {
			aliasMap[normalize(alias)] = ex
		}
	}
}

// stripEquipmentSuffix removes parenthetical equipment modifiers from exercise names.
// e.g. "Bench Press (Barbell)" → "Bench Press", "Chest Fly (Machine)" → "Chest Fly"
func stripEquipmentSuffix(name string) string {
	if idx := strings.Index(name, "("); idx > 0 {
		return strings.TrimSpace(name[:idx])
	}
	return name
}

// LookupExercise attempts to find a matching exercise for the given name
func LookupExercise(name string) LookupResult {
	if name == "" {
		return LookupResult{Matched: false, Primary: pbactivity.MuscleGroup_MUSCLE_GROUP_OTHER}
	}

	normalized := normalize(name)

	// 1. Exact match on canonical name
	if ex, ok := normalizedDB[normalized]; ok {
		return resultFromMapping(ex, 1.0)
	}

	// 2. Exact match on alias
	if ex, ok := aliasMap[normalized]; ok {
		return resultFromMapping(ex, 1.0)
	}

	// 3. Try with abbreviation expansion
	expanded := expandAbbreviations(normalized)
	if expanded != normalized {
		if ex, ok := normalizedDB[expanded]; ok {
			return resultFromMapping(ex, 0.95)
		}
		if ex, ok := aliasMap[expanded]; ok {
			return resultFromMapping(ex, 0.95)
		}
	}

	// 4. Strip equipment modifiers (e.g. "Bench Press (Barbell)" → "Bench Press") and retry.
	// Only do this when the full name didn't already match — parentheticals like
	// "Overhead Press (Dumbbell)" may themselves be registered aliases.
	stripped := stripEquipmentSuffix(name)
	if stripped != name {
		strippedNorm := normalize(stripped)
		if ex, ok := normalizedDB[strippedNorm]; ok {
			return resultFromMapping(ex, 0.95)
		}
		if ex, ok := aliasMap[strippedNorm]; ok {
			return resultFromMapping(ex, 0.95)
		}
		expandedStripped := expandAbbreviations(strippedNorm)
		if expandedStripped != strippedNorm {
			if ex, ok := normalizedDB[expandedStripped]; ok {
				return resultFromMapping(ex, 0.90)
			}
			if ex, ok := aliasMap[expandedStripped]; ok {
				return resultFromMapping(ex, 0.90)
			}
		}
	}

	// 5. Fuzzy match with Levenshtein distance (90% threshold)
	bestMatch, confidence := fuzzyMatch(normalized)
	if bestMatch != nil && confidence >= 0.90 {
		return resultFromMapping(bestMatch, confidence)
	}

	// 6. No match found
	return LookupResult{
		Matched:   false,
		Primary:   pbactivity.MuscleGroup_MUSCLE_GROUP_OTHER,
		Secondary: nil,
	}
}

// resultFromMapping creates a LookupResult from an ExerciseMapping
func resultFromMapping(ex *ExerciseMapping, confidence float64) LookupResult {
	return LookupResult{
		Matched:        true,
		CanonicalName:  ex.CanonicalName,
		Primary:        ex.Primary,
		Secondary:      ex.Secondary,
		PrimaryCoeff:   ex.PrimaryCoeff,
		SecondaryCoeff: ex.SecondaryCoeff,
		Confidence:     confidence,
	}
}

// normalize converts a string to lowercase and removes non-alphanumeric characters
func normalize(s string) string {
	var result strings.Builder
	s = strings.ToLower(s)
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			result.WriteRune(r)
		}
	}
	// Collapse multiple spaces into one
	return strings.Join(strings.Fields(result.String()), " ")
}

// expandAbbreviations replaces common abbreviations with full words
func expandAbbreviations(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if expanded, ok := abbreviations[word]; ok {
			words[i] = expanded
		}
	}
	return strings.Join(words, " ")
}

// fuzzyMatch finds the best matching exercise using Levenshtein distance
func fuzzyMatch(normalized string) (*ExerciseMapping, float64) {
	var bestMatch *ExerciseMapping
	var bestScore float64

	// Check all canonical names
	for i := range ExerciseDatabase {
		ex := &ExerciseDatabase[i]

		// Score against canonical name
		canon := normalize(ex.CanonicalName)
		score := similarityScore(normalized, canon)
		if score > bestScore {
			bestScore = score
			bestMatch = ex
		}

		// Score against aliases
		for _, alias := range ex.Aliases {
			aliasNorm := normalize(alias)
			score := similarityScore(normalized, aliasNorm)
			if score > bestScore {
				bestScore = score
				bestMatch = ex
			}
		}
	}

	return bestMatch, bestScore
}

// similarityScore calculates a 0-1 similarity score based on Levenshtein distance
func similarityScore(a, b string) float64 {
	if a == b {
		return 1.0
	}

	dist := levenshteinDistance(a, b)
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}
