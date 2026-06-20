package muscle_heatmap

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"sort"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// StandardCoefficients is the default coefficient map for muscle group normalization.
// Smaller muscles (arms, shoulders) have lower max loads, so we multiply their volume
// to match legs/back. This is exported for use by the muscle_heatmap_image provider.
var StandardCoefficients = map[pbactivity.MuscleGroup]float64{
	// Legs: baseline 1.0
	pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS: 1.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS: 1.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES:     1.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES:     1.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_ADDUCTORS:  1.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_ABDUCTORS:  1.0,

	// Back: 1.2x
	pbactivity.MuscleGroup_MUSCLE_GROUP_LATS:       1.2,
	pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK: 1.2,
	pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK: 1.2,
	pbactivity.MuscleGroup_MUSCLE_GROUP_NECK:       1.2,
	pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS:      1.2,

	// Chest: 1.5x
	pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST: 1.5,

	// Shoulders: 2.5x
	pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS: 2.5,

	// Arms: 4.0x (100kg curl is impossible, 100kg squat is warmup)
	pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS:   4.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS:  4.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS: 4.0,

	// Core & Misc
	pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS: 3.0,
	pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO:     0.5,
	pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY:  1.0,
}

// MuscleGroupCategories maps individual muscle groups to broader display categories.
// This is exported for use by the muscle_heatmap_image provider.
var MuscleGroupCategories = map[pbactivity.MuscleGroup]string{
	// Legs
	pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS: "Legs",
	pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS: "Legs",
	pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES:     "Legs",
	pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES:     "Legs",
	pbactivity.MuscleGroup_MUSCLE_GROUP_ADDUCTORS:  "Legs",
	pbactivity.MuscleGroup_MUSCLE_GROUP_ABDUCTORS:  "Legs",

	// Back
	pbactivity.MuscleGroup_MUSCLE_GROUP_LATS:       "Back",
	pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK: "Back",
	pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK: "Back",
	pbactivity.MuscleGroup_MUSCLE_GROUP_NECK:       "Back",
	pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS:      "Back",

	// Chest
	pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST: "Chest",

	// Shoulders
	pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS: "Shoulders",

	// Arms
	pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS:   "Arms",
	pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS:  "Arms",
	pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS: "Arms",

	// Core
	pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS: "Core",

	// Cardio & Full Body
	pbactivity.MuscleGroup_MUSCLE_GROUP_CARDIO:    "Cardio",
	pbactivity.MuscleGroup_MUSCLE_GROUP_FULL_BODY: "Full Body",
}

// MuscleHeatmapProvider generates an emoji-based "heatmap" of muscle volume.
type MuscleHeatmapProvider struct {
	coefficients map[pbactivity.MuscleGroup]float64
}

func init() {
	providers.Register(NewMuscleHeatmapProvider())
}

func NewMuscleHeatmapProvider() *MuscleHeatmapProvider {
	return &MuscleHeatmapProvider{
		coefficients: StandardCoefficients,
	}
}

func (p *MuscleHeatmapProvider) Name() string {
	return "muscle-heatmap"
}

func (p *MuscleHeatmapProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MUSCLE_HEATMAP
}

// GetMuscleCoefficient returns the normalization coefficient for a muscle group.
// This is exported for use by the muscle_heatmap_image provider.
func GetMuscleCoefficient(coeffs map[pbactivity.MuscleGroup]float64, muscle pbactivity.MuscleGroup) float64 {
	if v, ok := coeffs[muscle]; ok {
		return v
	}
	return 1.0 // Default fallback
}

func (p *MuscleHeatmapProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("muscle_heatmap: starting", "activity_name", activity.Name)
	// Aggregate sets
	var allSets []*pbactivity.StrengthSet
	for _, s := range activity.Sessions {
		allSets = append(allSets, s.StrengthSets...)
	}

	if len(allSets) == 0 {
		return &providers.EnrichmentResult{}, nil
	}

	// Parse config options
	style := pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_EMOJI_BARS
	if styleStr, ok := inputConfig["style"]; ok {
		switch styleStr {
		case "percentage":
			style = pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_PERCENTAGE
		case "text":
			style = pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_TEXT_ONLY
		}
	}

	barLength := 5
	if barLenStr, ok := inputConfig["bar_length"]; ok {
		if len, err := fmt.Sscanf(barLenStr, "%d", &barLength); err == nil && len == 1 {
			if barLength < 3 {
				barLength = 3
			} else if barLength > 10 {
				barLength = 10
			}
		}
	}

	// Apply coefficient preset
	coeffs := p.coefficients
	if preset, ok := inputConfig["preset"]; ok {
		coeffs = GetPresetCoefficients(preset)
	}

	// Parse group_by option
	groupBy := ""
	if g, ok := inputConfig["group_by"]; ok {
		groupBy = g
	}

	// Calculate Weighted Volume per Muscle Group
	volumeScores := make(map[string]float64)
	maxScore := 0.0

	for _, set := range allSets {
		// Process Primary Muscle
		primary := set.PrimaryMuscleGroup
		secondary := set.SecondaryMuscleGroups
		load := CalculateLoad(set)

		// Fallback: if muscle group is unspecified, use taxonomy lookup
		if primary == pbactivity.MuscleGroup_MUSCLE_GROUP_UNSPECIFIED || primary == pbactivity.MuscleGroup_MUSCLE_GROUP_OTHER {
			result := LookupExercise(set.ExerciseName)
			if result.Matched {
				primary = result.Primary
				secondary = result.Secondary
			}
		}

		if primary != pbactivity.MuscleGroup_MUSCLE_GROUP_UNSPECIFIED && primary != pbactivity.MuscleGroup_MUSCLE_GROUP_OTHER {
			coeff := GetMuscleCoefficient(coeffs, primary)
			score := load * coeff

			name := formatMuscleName(primary)
			volumeScores[name] += score
			if volumeScores[name] > maxScore {
				maxScore = volumeScores[name]
			}
		}

		// Process Secondary Muscles (0.5x impact)
		for _, sec := range secondary {
			if sec != pbactivity.MuscleGroup_MUSCLE_GROUP_UNSPECIFIED && sec != pbactivity.MuscleGroup_MUSCLE_GROUP_OTHER {
				coeff := GetMuscleCoefficient(coeffs, sec)
				score := load * coeff * 0.5

				name := formatMuscleName(sec)
				volumeScores[name] += score
				if volumeScores[name] > maxScore {
					maxScore = volumeScores[name]
				}
			}
		}
	}

	// Roll up into broader groups if requested
	if groupBy == "muscle_group" {
		volumeScores = RollUpScores(volumeScores)
		// Recalculate max
		maxScore = 0
		for _, v := range volumeScores {
			if v > maxScore {
				maxScore = v
			}
		}
	}

	// Generate output based on style
	// Filter out muscle groups with zero volume
	keys := make([]string, 0, len(volumeScores))
	for k, score := range volumeScores {
		if score > 0 {
			keys = append(keys, k)
		}
	}

	// If no muscle groups have any volume (e.g. an activity whose exercises
	// map to no recognized muscle group, like Pilates), skip the section
	// entirely rather than emitting a header with no body.
	if len(keys) == 0 {
		return &providers.EnrichmentResult{}, nil
	}

	// Sort by volume (descending order)
	sort.Slice(keys, func(i, j int) bool {
		return volumeScores[keys[i]] > volumeScores[keys[j]]
	})

	var sb strings.Builder
	sb.WriteString("🔥 Muscle Heatmap:\n")

	for _, k := range keys {
		score := volumeScores[k]
		rating := 0
		if maxScore > 0 {
			rating = int((score / maxScore) * float64(barLength))
		}
		if rating == 0 && score > 0 {
			rating = 1
		}

		sb.WriteString(p.formatMuscleRow(k, score, rating, maxScore, barLength, style))
	}

	var primaryMuscles, secondaryMuscles []string
	for _, k := range keys {
		score := volumeScores[k]
		if maxScore > 0 && score/maxScore >= 0.5 {
			primaryMuscles = append(primaryMuscles, k)
		} else {
			secondaryMuscles = append(secondaryMuscles, k)
		}
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata: map[string]string{
			"muscle_groups_displayed": fmt.Sprintf("%d", len(keys)),
			"max_score":               fmt.Sprintf("%.2f", maxScore),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			MuscleHeatmap: &pbactivity.MuscleHeatmapSummary{
				Primary:   primaryMuscles,
				Secondary: secondaryMuscles,
			},
		},
	}, nil
}

// GetPresetCoefficients returns coefficient map for a given preset.
// This is exported for use by the muscle_heatmap_image provider.
func GetPresetCoefficients(preset string) map[pbactivity.MuscleGroup]float64 {
	switch preset {
	case "powerlifting":
		// Emphasize compounds (squat, deadlift, bench)
		return map[pbactivity.MuscleGroup]float64{
			pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS: 1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS: 1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES:     1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK: 1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST:      1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_LATS:       1.2,
			pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK: 1.2,
			pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS:      1.2,
			pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS:  2.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS:    3.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS:     3.5,
			pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS:   3.5,
			pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES:     2.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS: 2.5,
		}
	case "bodybuilding":
		// Emphasize isolation and hypertrophy
		return map[pbactivity.MuscleGroup]float64{
			pbactivity.MuscleGroup_MUSCLE_GROUP_QUADRICEPS: 1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_HAMSTRINGS: 1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_GLUTES:     1.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_CALVES:     0.8,
			pbactivity.MuscleGroup_MUSCLE_GROUP_CHEST:      1.2,
			pbactivity.MuscleGroup_MUSCLE_GROUP_LATS:       1.2,
			pbactivity.MuscleGroup_MUSCLE_GROUP_UPPER_BACK: 1.2,
			pbactivity.MuscleGroup_MUSCLE_GROUP_LOWER_BACK: 1.5,
			pbactivity.MuscleGroup_MUSCLE_GROUP_SHOULDERS:  2.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_TRAPS:      2.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_BICEPS:     3.5,
			pbactivity.MuscleGroup_MUSCLE_GROUP_TRICEPS:    3.5,
			pbactivity.MuscleGroup_MUSCLE_GROUP_FOREARMS:   4.0,
			pbactivity.MuscleGroup_MUSCLE_GROUP_ABDOMINALS: 2.5,
		}
	default: // standard
		return StandardCoefficients
	}
}

// GetMuscleGroupCategory returns the broad category name for a muscle group.
// This is exported for use by the muscle_heatmap_image provider.
func GetMuscleGroupCategory(muscle pbactivity.MuscleGroup) string {
	if cat, ok := MuscleGroupCategories[muscle]; ok {
		return cat
	}
	return formatMuscleName(muscle)
}

// RollUpScores aggregates per-muscle volume scores into broader category totals.
// Input keys are formatted muscle names (e.g. "Quadriceps", "Hamstrings").
// Output keys are category names (e.g. "Legs", "Back").
// This is exported for use by the muscle_heatmap_image provider.
func RollUpScores(perMuscle map[string]float64) map[string]float64 {
	// Build reverse lookup: formatted name -> category
	nameToCategory := make(map[string]string)
	for mg, cat := range MuscleGroupCategories {
		nameToCategory[formatMuscleName(mg)] = cat
	}

	rolled := make(map[string]float64)
	for name, score := range perMuscle {
		if cat, ok := nameToCategory[name]; ok {
			rolled[cat] += score
		} else {
			// Unknown muscles keep their original name
			rolled[name] += score
		}
	}
	return rolled
}

// formatMuscleRow formats a single muscle row based on style
func (p *MuscleHeatmapProvider) formatMuscleRow(name string, score float64, rating int, maxScore float64, barLength int, style pbplugin.MuscleHeatmapStyle) string {
	switch style {
	case pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_PERCENTAGE:
		percentage := 0
		if maxScore > 0 {
			percentage = int((score / maxScore) * 100)
		}
		return fmt.Sprintf("%s: %d%%\n", name, percentage)

	case pbplugin.MuscleHeatmapStyle_MUSCLE_HEATMAP_STYLE_TEXT_ONLY:
		level := "Low"
		if rating >= barLength*3/4 {
			level = "Very High"
		} else if rating >= barLength/2 {
			level = "High"
		} else if rating >= barLength/4 {
			level = "Medium"
		}
		return fmt.Sprintf("%s: %s\n", name, level)

	default: // EMOJI_BARS
		bar := ""
		for i := 0; i < barLength; i++ {
			if i < rating {
				bar += "🟪"
			} else {
				bar += "⬜"
			}
		}
		return fmt.Sprintf("%s: %s\n", name, bar)
	}
}

func formatMuscleName(m pbactivity.MuscleGroup) string {
	// E.g. MUSCLE_GROUP_UPPER_BACK -> "Upper Back"
	s := m.String()
	s = strings.TrimPrefix(s, "MUSCLE_GROUP_")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ToLower(s)

	// Convert to Title Case manually (strings.Title is deprecated)
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// CalculateLoad calculates the training load for a strength set.
// This is exported for use by the muscle_heatmap_image provider.
func CalculateLoad(set *pbactivity.StrengthSet) float64 {
	// Handle distance-based exercises (running, cycling, rowing, etc.)
	if set.DistanceMeters > 0 {
		// Use distance as primary metric: 10m = 1 unit of load
		return set.DistanceMeters * 0.1
	}

	// Handle duration-based exercises (without distance)
	if set.DurationSeconds > 0 && set.Reps == 0 && set.WeightKg == 0 {
		// Use duration as metric: 2 seconds = 1 unit of load
		return float64(set.DurationSeconds) * 0.5
	}

	// Handle weight-based exercises
	load := set.WeightKg * float64(set.Reps)
	if set.WeightKg == 0 && set.Reps > 0 {
		// Bodyweight exercises: use heuristic
		load = float64(set.Reps) * 40.0
	}
	return load
}
