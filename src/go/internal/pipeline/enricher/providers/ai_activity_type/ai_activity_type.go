// nolint:proto-json
package ai_activity_type

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	domainactivity "github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/domain/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ambiguousTypes are activity types that are too generic to be meaningful.
// When an activity has one of these types, we ask Gemini to infer a better one.
var ambiguousTypes = map[pbactivity.ActivityType]bool{
	pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT:     true,
	pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED: true,
}

// AIActivityTypeProvider infers the correct ActivityType from activity context using Gemini AI.
// It runs in Phase 2 (deferred) so it has access to the full enriched description.
type AIActivityTypeProvider struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewAIActivityTypeProvider())
}

func NewAIActivityTypeProvider() *AIActivityTypeProvider {
	return &AIActivityTypeProvider{}
}

func (p *AIActivityTypeProvider) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *AIActivityTypeProvider) Name() string {
	return "ai-activity-type"
}

func (p *AIActivityTypeProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_ACTIVITY_TYPE
}

// ShouldDefer returns true so this provider runs in Phase 2 after all other enrichers,
// giving it access to the accumulated enriched_description in inputConfig.
func (p *AIActivityTypeProvider) ShouldDefer() bool {
	return true
}

func (p *AIActivityTypeProvider) Enrich(
	ctx context.Context,
	logger *slog.Logger,
	activity *pbactivity.StandardizedActivity,
	user *user.Record,
	inputConfig map[string]string,
	doNotRetry bool,
) (*providers.EnrichmentResult, error) {
	// Only run when the activity type is ambiguous/generic.
	if !ambiguousTypes[activity.Type] {
		logger.Debug("ai-activity-type: skipping — activity already has a specific type",
			"type", activity.Type.String(),
		)
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: fmt.Sprintf("activity type %s is already specific", activity.Type.String()),
			Metadata: map[string]string{
				"status":       "skipped",
				"reason":       "type_already_specific",
				"current_type": activity.Type.String(),
			},
		}, nil
	}

	// Require GEMINI_API_KEY.
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		logger.Warn("ai-activity-type: GEMINI_API_KEY not set, skipping")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "GEMINI_API_KEY not configured",
			Metadata: map[string]string{
				"status": "skipped",
				"reason": "api_key_not_configured",
			},
		}, nil
	}

	// Build prompt (system instruction + sanitised user data kept separate).
	systemInstruction, userData := buildPrompt(activity, inputConfig)

	// Call Gemini.
	rawResponse, err := callGemini(ctx, apiKey, systemInstruction, userData)
	if err != nil {
		logger.Error("ai-activity-type: Gemini call failed", "error", err)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status": "error",
				"reason": err.Error(),
			},
		}, nil // Don't fail the pipeline
	}

	// Parse the response — expect a single proto enum string like "ACTIVITY_TYPE_RUN".
	candidate := strings.TrimSpace(rawResponse)

	newType := domainactivity.ParseActivityTypeFromString(candidate)
	if newType == pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		logger.Warn("ai-activity-type: Gemini returned unrecognised activity type",
			"raw_response", rawResponse,
		)
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: fmt.Sprintf("Gemini returned unrecognised value: %q", candidate),
			Metadata: map[string]string{
				"status":       "skipped",
				"reason":       "unrecognised_response",
				"raw_response": rawResponse,
			},
		}, nil
	}

	originalType := activity.Type.String()

	// Apply the type override directly (same pattern as type_mapper).
	activity.Type = newType

	logger.Info("ai-activity-type: activity type updated",
		"original_type", originalType,
		"new_type", newType.String(),
	)

	return &providers.EnrichmentResult{
		Metadata: map[string]string{
			"status":        "success",
			"original_type": originalType,
			"new_type":      newType.String(),
			"raw_response":  rawResponse,
		},
	}, nil
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

// buildPrompt returns the fixed system instruction and the sanitised user-data
// content as separate strings, keeping instructions and data structurally
// separated at the API level.
func buildPrompt(activity *pbactivity.StandardizedActivity, inputConfig map[string]string) (systemInstruction, userData string) {
	// Collect activity signals.
	var durationMins float64
	var distanceKm float64
	hasHeartRate := false
	hasGPS := false
	hasStrengthSets := false

	for _, session := range activity.Sessions {
		durationMins += session.TotalElapsedTime / 60.0
		distanceKm += session.TotalDistance / 1000.0

		// Heart rate and GPS presence are inferred from lap records
		for _, lap := range session.Laps {
			for _, rec := range lap.Records {
				if rec.HeartRate > 0 {
					hasHeartRate = true
				}
				if rec.PositionLat != 0 || rec.PositionLong != 0 {
					hasGPS = true
				}
			}
		}
		if len(session.StrengthSets) > 0 {
			hasStrengthSets = true
		}
	}

	// Build valid type list (exclude UNSPECIFIED).
	var validTypes []string
	for name, val := range pbactivity.ActivityType_value {
		if pbactivity.ActivityType(val) == pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
			continue
		}
		validTypes = append(validTypes, name)
	}

	systemInstruction = fmt.Sprintf(`You are a fitness data classifier. Given the activity details provided by the user, respond with ONLY the single most appropriate ActivityType enum value from the list below. Do not add any explanation, punctuation, or extra text — just the enum string.

Valid ActivityType values:
%s

Respond with exactly one value from the list above.`,
		strings.Join(validTypes, "\n"),
	)

	// User-controlled fields are sanitised before embedding.
	userData = fmt.Sprintf(`Activity details:
- Name: %s
- Source: %s
- Duration: %.1f minutes
- Distance: %.2f km
- Has heart rate data: %v
- Has GPS data: %v
- Has strength/weight sets: %v
- Enriched description context: %s`,
		sanitiseForPrompt(activity.Name),
		activity.Source.String(),
		durationMins,
		distanceKm,
		hasHeartRate,
		hasGPS,
		hasStrengthSets,
		sanitiseForPrompt(inputConfig["enriched_description"]),
	)

	return systemInstruction, userData
}

// callGemini sends the prompt to Gemini and returns the raw text response.
// systemInstruction contains the fixed classifier directives and is set on the
// model directly; userData contains only sanitised user-controlled content and
// is passed as the user-role message, keeping instructions and data separated
// at the API level.
func callGemini(ctx context.Context, apiKey, systemInstruction, userData string) (string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash")
	model.SetTemperature(0.1) // Low temperature for deterministic classification
	model.SetTopP(0.9)
	model.SetMaxOutputTokens(50) // We only need a short enum string

	// Structural separation: fixed instructions in SystemInstruction field,
	// user-controlled data only in the user-role content.
	model.SystemInstruction = genai.NewUserContent(genai.Text(systemInstruction))

	resp, err := model.GenerateContent(ctx, genai.Text(userData))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}

	var raw string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			raw += string(text)
		}
	}

	return raw, nil
}
