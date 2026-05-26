package hevy

// Exercise template handling for Hevy
// This file provides template resolution via the Hevy API with strict matching and caching
// For Hyrox/ATHX/GymRace activities, we use strict matching to preserve exercise specificity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	hevy "github.com/fitglue/server/src/go/pkg/api/hevy"
	"github.com/fitglue/server/src/go/pkg/infrastructure/oauth"
)

// ExerciseTypeConfig holds the exercise type and muscle group for custom templates
type ExerciseTypeConfig struct {
	ExerciseType      string // Hevy CustomExerciseType: weight_reps, distance_duration, weight_duration, etc.
	MuscleGroup       string // Hevy MuscleGroup: full_body, quadriceps, etc.
	EquipmentCategory string // Hevy EquipmentCategory: "none", "barbell", "dumbbell", "kettlebell", "machine", "plate", "resistance_band", "suspension", "other"
}

// strictExerciseAliases maps normalized exercise names to acceptable alternatives
// These are intentionally restrictive - only truly equivalent exercises should be listed
// Key: normalized source name, Value: list of acceptable normalized Hevy names
var strictExerciseAliases = map[string][]string{
	"farmers carry":      {"farmers walk", "farmer walk", "farmer carry"},
	"farmers walk":       {"farmers carry", "farmer walk", "farmer carry"},
	"skierg":             {"ski erg"},
	"ski erg":            {"skierg"},
	"burpee broad jump":  {"burpee broad jumps"},
	"burpee broad jumps": {"burpee broad jump"},
	"sandbag lunges":     {"sandbag lunge", "weighted lunges", "weighted lunge"},
	"sandbag lunge":      {"sandbag lunges", "weighted lunges", "weighted lunge"},
	"sled push":          {"prowler push"},
	"sled pull":          {"prowler pull"},
	"wall balls":         {"wall ball"},
	"wall ball":          {"wall balls"},
	"rowing":             {"row", "rowing machine", "row machine"},
	"row":                {"rowing", "rowing machine", "row machine"},
}

// getExerciseTypeConfig returns the appropriate exercise_type, muscle_group, and equipment_category for an exercise name
// This is used when creating custom templates to ensure the right measurement types are supported
func getExerciseTypeConfig(exerciseName string) ExerciseTypeConfig {
	normalized := strings.ToLower(exerciseName)

	equip := "other"
	if strings.Contains(normalized, "barbell") {
		equip = "barbell"
	} else if strings.Contains(normalized, "dumbbell") || strings.Contains(normalized, "dumbell") {
		equip = "dumbbell"
	} else if strings.Contains(normalized, "kettlebell") {
		equip = "kettlebell"
	} else if strings.Contains(normalized, "machine") {
		equip = "machine"
	} else if strings.Contains(normalized, "plate") {
		equip = "plate"
	} else if strings.Contains(normalized, "band") {
		equip = "resistance_band"
	} else if strings.Contains(normalized, "suspension") || strings.Contains(normalized, "trx") {
		equip = "suspension"
	} else if strings.Contains(normalized, "bodyweight") || strings.Contains(normalized, "running") || strings.Contains(normalized, "walking") || strings.Contains(normalized, "swimming") || strings.Contains(normalized, "burpee") {
		equip = "none"
	}

	// Duration-only exercises (pure cardio activities tracked only by time, no distance component).
	// These are fallback cardio exercises where only elapsed time is meaningful.
	durationOnlyPatterns := []string{
		"other cardio",
		"crossfit",
		"hiit",
		"yoga",
		"pilates",
		"weightlifting",
		"alpine skiing",
		"skiing",
		"cross country skiing",
		"snowboarding",
		"snowshoeing",
		"ice skating",
		"roller skiing",
		"inline skating",
		"skateboarding",
		"tennis",
		"badminton",
		"racquetball",
		"squash",
		"pickleball",
		"table tennis",
		"football",
		"soccer",
		"golf",
		"rock climbing",
		"hiking",
		"kayaking",
		"canoeing",
		"stand up paddle",
		"surfing",
		"windsurfing",
		"kitesurfing",
		"mountain biking",
		"trail running",
		"stair stepper",
		"elliptical",
	}
	for _, pattern := range durationOnlyPatterns {
		if strings.Contains(normalized, pattern) {
			return ExerciseTypeConfig{ExerciseType: "duration", MuscleGroup: "full_body", EquipmentCategory: "none"}
		}
	}

	// Distance + Duration exercises (pure cardio stations with no weight component).
	// These exercises track distance covered and time taken.
	distanceDurationPatterns := []string{
		"skierg", "ski erg",
		"rowing", "row",
		"burpee broad jump",
		"running", "cycling", "swimming", "walking",
	}

	for _, pattern := range distanceDurationPatterns {
		if strings.Contains(normalized, pattern) {
			if strings.Contains(pattern, "running") || strings.Contains(pattern, "walking") || strings.Contains(pattern, "swimming") || strings.Contains(pattern, "burpee") {
				equip = "none"
			} else if strings.Contains(pattern, "rowing") || strings.Contains(pattern, "row") || strings.Contains(pattern, "skierg") || strings.Contains(pattern, "ski erg") || strings.Contains(pattern, "cycling") {
				equip = "machine"
			}
			return ExerciseTypeConfig{ExerciseType: "distance_duration", MuscleGroup: "full_body", EquipmentCategory: equip}
		}
	}

	// Weight + Duration exercises.
	// Weighted carries and sleds: the preset distance is fixed and known (e.g. 50m Sled Push),
	// so weight + time is the meaningful variable to record in Hevy.
	// Wall Balls: Hevy has no weight+reps+duration type, so weight+duration is the best fit.
	weightDurationPatterns := []string{
		"sled push", "sled pull", "prowler",
		"farmers carry", "farmers walk", "farmer carry", "farmer walk",
		"sandbag lunges", "sandbag lunge", "weighted lunges", "weighted lunge",
		"wall ball",
	}
	for _, pattern := range weightDurationPatterns {
		if strings.Contains(normalized, pattern) {
			return ExerciseTypeConfig{ExerciseType: "weight_duration", MuscleGroup: "full_body", EquipmentCategory: equip}
		}
	}

	// Default to weight_reps for unknown strength exercises
	return ExerciseTypeConfig{ExerciseType: "weight_reps", MuscleGroup: "other", EquipmentCategory: equip}
}

// TemplateResolver fetches, caches, and resolves exercise template IDs from Hevy
type TemplateResolver struct {
	apiKey    string
	templates []hevy.ExerciseTemplate
	cache     map[string]*hevy.ExerciseTemplate // normalized name -> template
	fetched   bool
	logger    *slog.Logger
}

// NewTemplateResolver creates a resolver with the user's Hevy API key
func NewTemplateResolver(apiKey string, logger *slog.Logger) *TemplateResolver {
	return &TemplateResolver{
		apiKey: apiKey,
		cache:  make(map[string]*hevy.ExerciseTemplate),
		logger: logger,
	}
}

// ResolveTemplate resolves an exercise name to a valid Hevy template
// It will:
// 1. Check local cache first
// 2. Fetch templates from API if not yet fetched
// 3. Strict match against fetched templates (exact or known aliases only)
// 4. Create a custom template if no match found
func (r *TemplateResolver) ResolveTemplate(ctx context.Context, exerciseName string) (*hevy.ExerciseTemplate, error) {
	normalized := normalizeExerciseName(exerciseName)

	// Check cache first
	if tmpl, ok := r.cache[normalized]; ok {
		r.logger.Debug("Template cache hit",
			"exerciseName", exerciseName,
			"templateID", *tmpl.Id)
		return tmpl, nil
	}

	// Fetch templates from API if not yet done
	if !r.fetched {
		if err := r.fetchAllTemplates(ctx); err != nil {
			r.logger.Warn("Failed to fetch templates, will create custom",
				"error", err)
			// Continue to create custom template
		}
	}

	// Strict match against fetched templates (exact or known aliases only)
	if tmpl := r.strictMatch(normalized); tmpl != nil {
		r.cache[normalized] = tmpl
		r.logger.Info("Template matched",
			"exerciseName", exerciseName,
			"templateID", *tmpl.Id)
		return tmpl, nil
	}

	// No match found - create custom template with appropriate exercise type
	r.logger.Info("No template match, creating custom",
		"exerciseName", exerciseName)
	tmpl, err := r.createCustomTemplate(ctx, exerciseName)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom template for %q: %w", exerciseName, err)
	}

	r.cache[normalized] = tmpl
	r.logger.Info("Created custom template",
		"exerciseName", exerciseName,
		"templateID", *tmpl.Id)

	return tmpl, nil
}

// fetchAllTemplates retrieves all exercise templates from Hevy API (paginated)
func (r *TemplateResolver) fetchAllTemplates(ctx context.Context) error {
	r.templates = []hevy.ExerciseTemplate{}
	page := 1
	pageSize := 100

	client := oauth.NewClientWithErrorLogging(r.logger, "hevy", 30*time.Second)

	for {
		url := fmt.Sprintf("https://api.hevyapp.com/v1/exercise_templates?page=%d&page_size=%d", page, pageSize)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("api-key", r.apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var errorBody bytes.Buffer
			errorBody.ReadFrom(resp.Body)
			return fmt.Errorf("API error (status %d): %s", resp.StatusCode, errorBody.String())
		}

		rawBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read response body: %w", readErr)
		}

		var result struct {
			ExerciseTemplates []hevy.ExerciseTemplate `json:"exercise_templates"`
			Page              int                     `json:"page"`
			PageCount         int                     `json:"page_count"`
		}
		if err := json.Unmarshal(rawBody, &result); err != nil {
			rawStr := strings.TrimSpace(string(rawBody))
			if len(rawStr) > 0 && !strings.HasPrefix(rawStr, "{") && !strings.HasPrefix(rawStr, "[") {
				return fmt.Errorf("hevy returned non-JSON response: %s", rawStr)
			}
			return fmt.Errorf("decode response (raw: %q): %w", rawStr, err)
		}

		r.templates = append(r.templates, result.ExerciseTemplates...)
		r.logger.Debug("Fetched exercise templates page",
			"page", page,
			"count", len(result.ExerciseTemplates),
			"totalSoFar", len(r.templates))

		if page >= result.PageCount || len(result.ExerciseTemplates) == 0 {
			break
		}
		page++
	}

	r.fetched = true
	r.logger.Info("Fetched all exercise templates",
		"totalCount", len(r.templates))

	return nil
}

// strictMatch finds a template ID using exact matching or known aliases only
// This is intentionally restrictive to preserve exercise specificity for Hyrox/ATHX/GymRace
func (r *TemplateResolver) strictMatch(normalizedName string) *hevy.ExerciseTemplate {
	// Exact match first
	for i := range r.templates {
		t := &r.templates[i]
		if t.Title != nil && normalizeExerciseName(*t.Title) == normalizedName {
			if t.Id != nil {
				return t
			}
		}
	}

	// Check known aliases (strict equivalents only)
	if aliases, ok := strictExerciseAliases[normalizedName]; ok {
		for _, alias := range aliases {
			for i := range r.templates {
				t := &r.templates[i]
				if t.Title != nil && normalizeExerciseName(*t.Title) == alias {
					r.logger.Debug("Matched via strict alias",
						"source", normalizedName,
						"alias", alias,
						"templateTitle", *t.Title)
					if t.Id != nil {
						return t
					}
				}
			}
		}
	}

	return nil
}

// createCustomTemplate creates a new custom exercise template in Hevy
// Uses getExerciseTypeConfig to determine the appropriate exercise_type
func (r *TemplateResolver) createCustomTemplate(ctx context.Context, exerciseName string) (*hevy.ExerciseTemplate, error) {
	client := oauth.NewClientWithErrorLogging(r.logger, "hevy", 30*time.Second)

	// Get the appropriate exercise type for this exercise
	config := getExerciseTypeConfig(exerciseName)

	payload := map[string]interface{}{
		"exercise": map[string]interface{}{
			"title":              exerciseName,
			"exercise_type":      config.ExerciseType,
			"muscle_group":       config.MuscleGroup,
			"equipment_category": config.EquipmentCategory,
		},
	}

	r.logger.Debug("Creating custom template",
		"exerciseName", exerciseName,
		"exerciseType", config.ExerciseType,
		"muscleGroup", config.MuscleGroup,
		"equipmentCategory", config.EquipmentCategory)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.hevyapp.com/v1/exercise_templates", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("api-key", r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the full body upfront so we can include it in any error message for diagnosis.
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		r.logger.Warn("Hevy create custom template API error",
			"exerciseName", exerciseName,
			"status", resp.StatusCode,
			"body", string(rawBody))
		return nil, fmt.Errorf("create template failed (status %d): %s", resp.StatusCode, string(rawBody))
	}

	r.logger.Debug("Hevy create custom template raw response",
		"exerciseName", exerciseName,
		"status", resp.StatusCode,
		"body", string(rawBody))

	rawStr := strings.TrimSpace(string(rawBody))

	// Hevy may return just the UUID as a plain string (not wrapped in JSON) on successful creation.
	// Detect this by checking if the response is non-empty and doesn't start with a JSON delimiter.
	if len(rawStr) > 0 && !strings.HasPrefix(rawStr, "{") && !strings.HasPrefix(rawStr, "[") {
		// Strip surrounding quotes if present (e.g. "\"some-uuid\"")
		templateID := strings.Trim(rawStr, "\"")
		r.logger.Info("Hevy returned plain-string template ID on creation",
			"exerciseName", exerciseName,
			"templateID", templateID)

		exType := config.ExerciseType
		title := exerciseName
		return &hevy.ExerciseTemplate{
			Id:    &templateID,
			Title: &title,
			Type:  &exType,
		}, nil
	}

	var result struct {
		ExerciseTemplate hevy.ExerciseTemplate `json:"exercise_template"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("decode response (raw: %q): %w", string(rawBody), err)
	}

	// Add to local cache
	r.templates = append(r.templates, result.ExerciseTemplate)

	if result.ExerciseTemplate.Id != nil {
		return &result.ExerciseTemplate, nil
	}
	return nil, fmt.Errorf("created template has no ID")
}

// normalizeExerciseName normalizes an exercise name for comparison
// This does NOT simplify Hyrox-specific exercises - we preserve their specificity
func normalizeExerciseName(name string) string {
	// Lowercase and trim
	name = strings.ToLower(strings.TrimSpace(name))

	// Common substitutions for punctuation/formatting only
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "'", "") // farmer's -> farmers
	name = strings.ReplaceAll(name, "'", "") // curly apostrophe

	// Normalize plural/singular for common variations
	name = strings.ReplaceAll(name, "carries", "carry")

	// Remove common equipment suffixes that vary between platforms
	suffixes := []string{"(barbell)", "(dumbbell)", "(machine)", "(cable)", "(smith)", "(outdoor)", "(treadmill)"}
	for _, suffix := range suffixes {
		name = strings.TrimSuffix(name, suffix)
	}

	return strings.TrimSpace(name)
}

// CardioTemplateNames maps FitGlue activity types to Hevy exercise search terms
var CardioTemplateNames = map[string][]string{
	"run":  {"Running (Outdoor)", "Running", "Running (Treadmill)"},
	"walk": {"Walking", "Walk"},
	"ride": {"Cycling (Outdoor)", "Cycling", "Cycling (Stationary)"},
	"swim": {"Swimming", "Swim"},
	"row":  {"Rowing", "Rowing Machine"},
}
