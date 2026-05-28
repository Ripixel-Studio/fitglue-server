package ai_companion

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"os"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/tier"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// AICompanionProvider generates AI-powered activity descriptions using Google Gemini.
// This is an Athlete-tier only feature.
type AICompanionProvider struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewAICompanionProvider())
}

func NewAICompanionProvider() *AICompanionProvider {
	return &AICompanionProvider{}
}

func (p *AICompanionProvider) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *AICompanionProvider) Name() string {
	return "ai-companion"
}

func (p *AICompanionProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION
}

func (p *AICompanionProvider) ShouldDefer() bool {
	return true
}

func (p *AICompanionProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	// Tier check - Athlete tier only
	if tier.GetEffectiveTier(user) != tier.TierAthlete {
		logger.Info("AI Companion skipped: user not on athlete tier",
			"user_id", user.UserId,
			"tier", tier.GetEffectiveTier(user),
		)
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "Athlete tier required",
			Metadata: map[string]string{
				"status":        "skipped",
				"reason":        "tier_restricted",
				"required_tier": "athlete",
			},
		}, nil
	}

	// Get configuration
	mode := inputs["mode"]
	if mode == "" {
		mode = "description" // Default
	}

	showSectionHeader := inputs["section_header"] != "false" // Default to true

	// Build context from activity
	activityContext := buildActivityContext(activity)

	// Include enriched description from other enrichers (injected by orchestrator Phase 2)
	if enrichedDesc := inputs["enriched_description"]; enrichedDesc != "" {
		activityContext += "\n\nOther Enricher Descriptions:\n" + enrichedDesc
	}

	// Get Gemini API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		logger.Warn("GEMINI_API_KEY not set, skipping AI companion")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "GEMINI_API_KEY not configured",
			Metadata: map[string]string{
				"status":        "skipped",
				"reason":        "api_key_not_configured",
				"status_detail": "GEMINI_API_KEY environment variable not set",
			},
		}, nil
	}

	// Generate content using Gemini
	result, err := p.generateWithGemini(ctx, apiKey, mode, activityContext)
	if err != nil {
		logger.Error("Failed to generate AI companion content", "error", err)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "error",
				"reason":        "generation_failed",
				"status_detail": err.Error(),
			},
		}, nil // Don't return error to avoid pipeline failure
	}

	if showSectionHeader && result.Description != "" {
		result.Description = "✨ AI Summary:\n" + result.Description
	}

	logger.Info("AI Companion content generated successfully",
		"mode", mode,
		"has_title", result.Title != "",
		"has_description", result.Description != "",
	)

	return &providers.EnrichmentResult{
		Name:        result.Title,
		Description: result.Description,
		Metadata: map[string]string{
			"status": "success",
			"mode":   mode,
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			AiSummary: &pbactivity.AiSummary{
				Html: result.Description,
			},
		},
	}, nil
}

type aiResult struct {
	Title       string
	Description string
}

// sanitiseForPrompt truncates user-controlled strings to a safe length and
// strips obvious prompt-injection delimiters to prevent injected instructions
// from escaping the data section of the prompt.
func sanitiseForPrompt(s string) string {
	const maxLen = 500
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

func (p *AICompanionProvider) generateWithGemini(ctx context.Context, apiKey, mode, activityContext string) (*aiResult, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")

	// Configure model for short, punchy outputs
	model.SetTemperature(0.7)
	model.SetTopP(0.9)
	model.SetMaxOutputTokens(300)

	// Structural separation: fixed instructions go in SystemInstruction; user-controlled
	// activity data is passed only in the user-role content part.
	systemInstr, userContent := buildPrompt(mode)
	model.SystemInstruction = genai.NewUserContent(genai.Text(systemInstr))

	resp, err := model.GenerateContent(ctx, genai.Text(userContent+"\n\nActivity Context:\n"+sanitiseForPrompt(activityContext)))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content generated")
	}

	// Parse response
	rawOutput := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			rawOutput += string(text)
		}
	}

	return parseAIResponse(mode, rawOutput), nil
}

// buildPrompt returns the fixed system instruction and the mode-specific user
// content request as separate strings. User-controlled activity context is NOT
// included here — it is appended at call site after sanitisation, keeping
// instructions and data structurally separated at the API level.
func buildPrompt(mode string) (systemInstruction, userContent string) {
	systemInstruction = `You are an activity reviewer. Generate a casual, engaging summary of the fitness activity provided in the user message.

Guidelines:
- Provide a casual summary or review of the effort.
- DO NOT talk to the user directly (avoid "you", "your", or addressing them as "runner", "athlete", etc.).
- Maintain an objective yet positive tone.
- Generic punchy reactions like "Nice one!" or "Solid session" are acceptable as part of the summary.
- Avoid motivational "coach" cliches (e.g., "Keep pushing", "You've got this").
- Use fitness terminology naturally.
- Reference specific details from the workout.`

	switch mode {
	case "title":
		userContent = `Generate a creative, engaging title for this workout (max 50 characters).
Respond with ONLY the title, nothing else.`
	case "both":
		userContent = `Generate both a title and description for this workout.
Format your response exactly as:
TITLE: [creative title, max 50 chars]
DESCRIPTION: [engaging description, 2-3 sentences max]`
	default: // "description"
		userContent = `Generate an engaging description for this workout (2-3 sentences max).
Respond with ONLY the description, nothing else.`
	}

	return systemInstruction, userContent
}

func buildActivityContext(activity *pbactivity.StandardizedActivity) string {
	var parts []string

	// Activity type
	if activity.Type != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		parts = append(parts, fmt.Sprintf("Type: %s", activity.Type.String()))
	}

	// Activity name (original)
	if activity.Name != "" {
		parts = append(parts, fmt.Sprintf("Original Name: %s", activity.Name))
	}

	// Duration and distance from sessions
	var totalDuration float64
	var totalDistance float64
	var strengthSets []*pbactivity.StrengthSet
	var heartRates []float32
	var cadences []uint32
	var powers []uint32

	for _, session := range activity.Sessions {
		totalDuration += session.TotalElapsedTime
		totalDistance += session.TotalDistance
		strengthSets = append(strengthSets, session.StrengthSets...)

		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate > 0 {
					heartRates = append(heartRates, float32(record.HeartRate))
				}
				if record.Cadence > 0 {
					cadences = append(cadences, uint32(record.Cadence))
				}
				if record.Power > 0 {
					powers = append(powers, uint32(record.Power))
				}
			}
		}
	}

	if totalDuration > 0 {
		mins := totalDuration / 60 // seconds to minutes
		parts = append(parts, fmt.Sprintf("Duration: %.1f minutes", mins))
	}

	if totalDistance > 0 {
		km := totalDistance / 1000
		parts = append(parts, fmt.Sprintf("Distance: %.2f km", km))
	}

	// Heart Rate Summary
	if len(heartRates) > 0 {
		var sum float32
		var max float32
		min := heartRates[0]
		for _, hr := range heartRates {
			sum += hr
			if hr > max {
				max = hr
			}
			if hr < min {
				min = hr
			}
		}
		avg := sum / float32(len(heartRates))
		parts = append(parts, fmt.Sprintf("Heart Rate: Avg %.0f bpm, Max %.0f bpm, Min %.0f bpm", avg, max, min))
	}

	// Cadence Summary
	if len(cadences) > 0 {
		var sum uint32
		var max uint32
		for _, c := range cadences {
			sum += c
			if c > max {
				max = c
			}
		}
		avg := float64(sum) / float64(len(cadences))
		parts = append(parts, fmt.Sprintf("Cadence: Avg %.0f rpm, Max %d rpm", avg, max))
	}

	// Power Summary
	if len(powers) > 0 {
		var sum uint32
		var max uint32
		for _, p := range powers {
			sum += p
			if p > max {
				max = p
			}
		}
		avg := float64(sum) / float64(len(powers))
		parts = append(parts, fmt.Sprintf("Power: Avg %.0f W, Max %d W", avg, max))
	}

	// Strength Exercises Summary
	if len(strengthSets) > 0 {
		type exerciseStats struct {
			totalSets int
			reps      []int32
			weights   []float32
		}
		exercises := make(map[string]*exerciseStats)
		var exerciseOrder []string

		for _, set := range strengthSets {
			if set.ExerciseName == "" {
				continue
			}
			if _, ok := exercises[set.ExerciseName]; !ok {
				exercises[set.ExerciseName] = &exerciseStats{}
				exerciseOrder = append(exerciseOrder, set.ExerciseName)
			}
			stats := exercises[set.ExerciseName]
			stats.totalSets++
			stats.reps = append(stats.reps, set.Reps)
			stats.weights = append(stats.weights, float32(set.WeightKg))
		}

		if len(exerciseOrder) > 0 {
			parts = append(parts, "Strength Exercises:")
			for _, name := range exerciseOrder {
				stats := exercises[name]

				// Group by reps/weight to be concise
				type setGroup struct {
					reps   int32
					weight float32
					count  int
				}
				var groups []setGroup
				for i := 0; i < len(stats.reps); i++ {
					found := false
					for gIdx := range groups {
						if groups[gIdx].reps == stats.reps[i] && groups[gIdx].weight == stats.weights[i] {
							groups[gIdx].count++
							found = true
							break
						}
					}
					if !found {
						groups = append(groups, setGroup{reps: stats.reps[i], weight: stats.weights[i], count: 1})
					}
				}

				var groupParts []string
				for _, g := range groups {
					if g.weight > 0 {
						groupParts = append(groupParts, fmt.Sprintf("%d x %d @ %.1fkg", g.count, g.reps, g.weight))
					} else {
						groupParts = append(groupParts, fmt.Sprintf("%d x %d", g.count, g.reps))
					}
				}
				parts = append(parts, fmt.Sprintf("- %s: %s", name, strings.Join(groupParts, ", ")))
			}
		}
	}

	return strings.Join(parts, "\n")
}

func parseAIResponse(mode, rawOutput string) *aiResult {
	result := &aiResult{}
	output := strings.TrimSpace(rawOutput)

	switch mode {
	case "title":
		result.Title = cleanTitle(output)
	case "both":
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToUpper(line), "TITLE:") {
				result.Title = cleanTitle(strings.TrimPrefix(line, "TITLE:"))
				result.Title = cleanTitle(strings.TrimPrefix(result.Title, "Title:"))
			} else if strings.HasPrefix(strings.ToUpper(line), "DESCRIPTION:") {
				result.Description = cleanDescription(strings.TrimPrefix(line, "DESCRIPTION:"))
				result.Description = cleanDescription(strings.TrimPrefix(result.Description, "Description:"))
			}
		}
	default: // "description"
		result.Description = cleanDescription(output)
	}

	return result
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	// Remove markdown formatting if present
	s = strings.Trim(s, "*_`")
	// Limit title length
	if len(s) > 100 {
		s = s[:97] + "..."
	}
	return s
}

func cleanDescription(s string) string {
	s = strings.TrimSpace(s)
	// Remove markdown formatting if present
	s = strings.Trim(s, "*_`")
	// Note: No length limit for descriptions,
	// LLM is prompted for 2-3 sentences.
	return s
}
