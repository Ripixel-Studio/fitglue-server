// nolint:proto-json
package ai_banner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/tier"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"github.com/google/generative-ai-go/genai"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

// AIBannerProvider generates custom header images for activities using Vertex AI Imagen.
// This is an Athlete-tier only feature.
// Generated images are stored in Cloud Storage and referenced in activity metadata.
type AIBannerProvider struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewAIBannerProvider())
}

func NewAIBannerProvider() *AIBannerProvider {
	return &AIBannerProvider{}
}

func (p *AIBannerProvider) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *AIBannerProvider) Name() string {
	return "ai-banner"
}

func (p *AIBannerProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_BANNER
}

func (p *AIBannerProvider) ShouldDefer() bool {
	return true
}

func (p *AIBannerProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	// Tier check - Athlete tier only
	if tier.GetEffectiveTier(user) != tier.TierAthlete {
		logger.Info("AI Banner skipped: user not on athlete tier",
			"user_id", user.UserId,
			"tier", tier.GetEffectiveTier(user),
		)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "skipped",
				"reason":        "tier_restricted",
				"required_tier": "athlete",
			},
		}, nil
	}

	// Get style configuration
	style := inputs["style"]
	if style == "" {
		style = "vibrant" // Default
	}

	// Get subject configuration
	subject := inputs["subject"]
	if subject == "" {
		subject = "abstract" // Default to no people
	}

	// Get asset folder ID for storage path
	// Use pipeline_execution_id (unique per pipeline execution) with fallback to activity.ExternalId
	assetFolderID := inputs["pipeline_execution_id"]
	if assetFolderID == "" {
		assetFolderID = activity.ExternalId
	}
	if assetFolderID == "" {
		logger.Warn("AI Banner skipped: no pipeline execution ID or activity ID")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "skipped",
				"reason":        "no_asset_folder_id",
				"status_detail": "Pipeline execution ID or Activity ID is required for image storage",
			},
		}, nil
	}

	// Get Gemini API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		logger.Warn("GEMINI_API_KEY not set, skipping AI banner")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "skipped",
				"reason":        "api_key_not_configured",
				"status_detail": "GEMINI_API_KEY environment variable not set",
			},
		}, nil
	}

	// Step 1: Build activity context (structured data)
	activityContext := buildActivityContext(activity)

	// Include enriched description from other enrichers (injected by orchestrator Phase 2)
	// The full booster output (muscle heatmap, heart rate, training load, etc.) provides
	// valuable context for generating relevant fitness imagery.
	if enrichedDesc := inputs["enriched_description"]; enrichedDesc != "" {
		activityContext += "\n\nEnriched Activity Description:\n" + enrichedDesc
	}

	// Step 2: Use text LLM to generate an image description
	imagePrompt, err := p.generateImagePromptWithLLM(ctx, apiKey, activityContext, style, subject)
	if err != nil {
		logger.Error("Failed to generate image prompt with LLM", "error", err)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "error",
				"reason":        "prompt_generation_failed",
				"status_detail": err.Error(),
			},
		}, nil
	}

	logger.Info("Generated image prompt via LLM",
		"prompt_length", len(imagePrompt),
		"prompt_preview", truncatePrompt(imagePrompt, 200),
	)

	// Step 3: Generate image using Imagen with the LLM-generated prompt
	imageData, err := p.generateBannerWithGemini(ctx, apiKey, imagePrompt)
	if err != nil {
		logger.Error("Failed to generate AI banner", "error", err)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "error",
				"reason":        "generation_failed",
				"status_detail": err.Error(),
			},
		}, nil // Don't return error to avoid pipeline failure
	}

	// Store image in Cloud Storage
	bucketName := os.Getenv("SHOWCASE_ASSETS_BUCKET")
	if bucketName == "" {
		bucketName = "fitglue-server-dev-showcase-assets" // Fallback for local development
	}

	objectPath := fmt.Sprintf("%s/banner.png", assetFolderID)
	bannerURL, err := p.storeImage(ctx, bucketName, objectPath, imageData)
	if err != nil {
		logger.Error("Failed to store AI banner", "error", err)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "error",
				"reason":        "storage_failed",
				"status_detail": err.Error(),
			},
		}, nil
	}

	logger.Info("AI Banner generated successfully",
		"asset_folder_id", assetFolderID,
		"banner_url", bannerURL,
		"style", style,
	)

	return &providers.EnrichmentResult{
		Metadata: map[string]string{
			"status":          "success",
			"asset_ai_banner": bannerURL,
			"style":           style,
			"image_prompt":    imagePrompt,
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			AiBanner: &pbactivity.AiBanner{
				ImageUrl: bannerURL,
			},
		},
	}, nil
}

// ImagenRequest represents the request body for Vertex AI Imagen API
type ImagenRequest struct {
	Instances  []ImagenInstance `json:"instances"`
	Parameters ImagenParameters `json:"parameters"`
}

type ImagenInstance struct {
	Prompt string `json:"prompt"`
}

type ImagenParameters struct {
	SampleCount      int    `json:"sampleCount"`
	AspectRatio      string `json:"aspectRatio"`
	AddWatermark     bool   `json:"addWatermark"`
	IncludeRaiReason bool   `json:"includeRaiReason"`
	// PersonGeneration removed - causes silent RAI filtering issues with abstract prompts
}

// ImagenResponse represents the response from Vertex AI Imagen API
type ImagenResponse struct {
	Predictions       []ImagenPrediction `json:"predictions"`
	RaiFilteredReason string             `json:"raiFilteredReason,omitempty"`
}

type ImagenPrediction struct {
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	MimeType           string `json:"mimeType"`
}

// fallbackPrompt is a simple, safe prompt used when the primary prompt triggers content filters
const fallbackPrompt = "Abstract digital art with colorful geometric shapes and gradient backgrounds, professional quality artistic composition, no text"

// generateImagePromptWithLLM uses Gemini text model to generate a clean image description
// from the activity context. This ensures the prompt is purely visual with no text elements.
func (p *AIBannerProvider) generateImagePromptWithLLM(ctx context.Context, apiKey, activityContext, style, subject string) (string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")

	// Configure for concise, focused output
	model.SetTemperature(0.8) // Slightly creative
	model.SetTopP(0.9)
	model.SetMaxOutputTokens(200) // Keep prompts concise

	prompt := buildLLMPrompt(activityContext, style, subject)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate image prompt: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}

	// Extract the generated prompt
	var result string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			result += string(text)
		}
	}

	return strings.TrimSpace(result), nil
}

// buildLLMPrompt creates the prompt for the text LLM to generate an image description
func buildLLMPrompt(activityContext, style, subject string) string {
	subjectGuidance := ""
	switch subject {
	case "male":
		subjectGuidance = `SUBJECT: Feature a male athlete as the main subject.
- Show them mid-action performing one of the specific exercises listed in the activity data
- Focus on the movement, form, and environment rather than physique
- Avoid generic "muscular man posing" or stock-photo bodybuilder imagery
- The person should look natural and focused, not a model`
	case "female":
		subjectGuidance = `SUBJECT: Feature a female athlete as the main subject.
- Show them mid-action performing one of the specific exercises listed in the activity data
- Focus on the movement, form, and environment rather than physique
- Avoid generic fitness model or stock-photo imagery
- The person should look natural and focused, not a model`
	default: // "abstract"
		subjectGuidance = `SUBJECT: No people or human figures in the image.
- Instead, depict the EQUIPMENT, ENVIRONMENT, or SETTING relevant to the workout
- For strength/gym workouts: dumbbells, barbells, kettlebells, weight plates, gym floor, exercise mat, pull-up bar
- For running: running shoes, track, trail path, road at dawn/dusk
- For cycling: bicycle, road, velodrome, cycling gear
- For swimming: pool lanes, open water, goggles
- For stretching/yoga: yoga mat, foam roller, resistance bands in a calm space
- Show the equipment as if someone just used it or is about to — lived-in, not a catalog photo`
	}

	styleGuidance := ""
	switch style {
	case "minimal":
		styleGuidance = "Style: minimalist, clean lines, muted colors, shallow depth of field, simple composition."
	case "dramatic":
		styleGuidance = "Style: dramatic lighting, bold contrast, dynamic angles, intense atmosphere."
	default: // "vibrant"
		styleGuidance = "Style: vibrant, energetic, warm lighting, bold colors, athletic mood."
	}

	return fmt.Sprintf(`You are an image prompt generator for a fitness activity banner.

Given the activity data below, generate a short visual description for an image that DEPICTS THIS SPECIFIC WORKOUT.

CRITICAL RULES:
1. Output ONLY the image description — no explanations, preamble, or quotes
2. NEVER mention text, titles, captions, watermarks, or written words
3. The image MUST be grounded in a REAL-WORLD FITNESS SETTING (gym, track, trail, pool, home workout space, park)
4. NEVER use sci-fi, fantasy, cosmic, space, underwater, or surreal themes
5. NEVER use abstract art, geometric shapes, or non-fitness imagery
6. Match the image to the SPECIFIC exercises and activity type — a core workout should look different from a heavy leg day
7. Consider the time of day for lighting (morning = golden light, night = warm indoor/ambient lighting)
8. Keep it under 80 words

%s
%s

Activity Data:
%s

Generate the image prompt now:`, subjectGuidance, styleGuidance, activityContext)
}

// buildActivityContext assembles structured data about the activity for the LLM
func buildActivityContext(activity *pbactivity.StandardizedActivity) string {
	var parts []string

	// Activity type
	if activity.Type != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		activityType := strings.ToLower(strings.ReplaceAll(activity.Type.String(), "ACTIVITY_TYPE_", ""))
		activityType = strings.ReplaceAll(activityType, "_", " ")
		parts = append(parts, fmt.Sprintf("Activity type: %s", activityType))
	}

	// Calculate totals from sessions
	var totalDuration float64
	var totalDistance float64
	var strengthSets []*pbactivity.StrengthSet

	for _, session := range activity.Sessions {
		totalDuration += session.TotalElapsedTime
		totalDistance += session.TotalDistance
		strengthSets = append(strengthSets, session.StrengthSets...)
	}

	// Duration
	if totalDuration > 0 {
		mins := totalDuration / 60
		parts = append(parts, fmt.Sprintf("Duration: %.0f minutes", mins))
	}

	// Distance with scale context for cardio activities
	if totalDistance > 0 {
		km := totalDistance / 1000
		parts = append(parts, fmt.Sprintf("Distance: %.1f km", km))

		// Add distance scale context for runs/rides/swims
		activityType := activity.Type
		if activityType == pbactivity.ActivityType_ACTIVITY_TYPE_RUN || activityType == pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN {
			var scaleContext string
			switch {
			case km >= 42:
				scaleContext = "marathon or ultra distance - epic endurance achievement"
			case km >= 21:
				scaleContext = "half marathon distance - significant endurance effort"
			case km >= 15:
				scaleContext = "long run - serious training distance"
			case km >= 10:
				scaleContext = "10K distance - solid training run"
			case km >= 5:
				scaleContext = "5K distance - classic race distance"
			default:
				scaleContext = "short run - easy/recovery pace"
			}
			parts = append(parts, fmt.Sprintf("Run category: %s", scaleContext))
		} else if activityType == pbactivity.ActivityType_ACTIVITY_TYPE_RIDE || activityType == pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE {
			var scaleContext string
			switch {
			case km >= 160:
				scaleContext = "century ride or gran fondo - epic distance"
			case km >= 100:
				scaleContext = "long endurance ride"
			case km >= 50:
				scaleContext = "substantial ride - solid training"
			case km >= 20:
				scaleContext = "moderate ride"
			default:
				scaleContext = "short ride - commute or easy spin"
			}
			parts = append(parts, fmt.Sprintf("Ride category: %s", scaleContext))
		} else if activityType == pbactivity.ActivityType_ACTIVITY_TYPE_SWIM {
			var scaleContext string
			switch {
			case km >= 3.8:
				scaleContext = "ironman swim distance - open water endurance"
			case km >= 1.9:
				scaleContext = "half ironman swim distance"
			case km >= 1.5:
				scaleContext = "olympic triathlon swim distance"
			case km >= 0.75:
				scaleContext = "sprint triathlon swim distance"
			default:
				scaleContext = "pool training session"
			}
			parts = append(parts, fmt.Sprintf("Swim category: %s", scaleContext))
		}
	}

	// Calculate intensity indicators from pace (for cardio) or heart rate
	if totalDistance > 0 && totalDuration > 0 {
		// Calculate average pace/speed
		avgSpeedKmh := (totalDistance / 1000) / (totalDuration / 3600)

		activityType := activity.Type
		if activityType == pbactivity.ActivityType_ACTIVITY_TYPE_RUN || activityType == pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN {
			// Running pace context (min/km)
			paceMinPerKm := (totalDuration / 60) / (totalDistance / 1000)
			var intensityContext string
			switch {
			case paceMinPerKm < 4:
				intensityContext = "elite/race pace - very fast"
			case paceMinPerKm < 5:
				intensityContext = "fast tempo pace"
			case paceMinPerKm < 6:
				intensityContext = "steady moderate pace"
			case paceMinPerKm < 7:
				intensityContext = "easy conversational pace"
			default:
				intensityContext = "recovery/walk pace"
			}
			parts = append(parts, fmt.Sprintf("Intensity: %s", intensityContext))
		} else if activityType == pbactivity.ActivityType_ACTIVITY_TYPE_RIDE || activityType == pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE {
			var intensityContext string
			switch {
			case avgSpeedKmh > 35:
				intensityContext = "race pace - very fast"
			case avgSpeedKmh > 28:
				intensityContext = "fast training pace"
			case avgSpeedKmh > 22:
				intensityContext = "moderate endurance pace"
			default:
				intensityContext = "easy/recovery pace"
			}
			parts = append(parts, fmt.Sprintf("Intensity: %s", intensityContext))
		}
	}

	// Add trail run context
	if activity.Type == pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN {
		parts = append(parts, "Environment: trail running - natural terrain, forests, mountains")
	}

	// Strength exercises - include actual exercise names and detect if bodyweight
	if len(strengthSets) > 0 {
		// Collect unique exercise names (limit to 8 for prompt brevity)
		exercisesSeen := make(map[string]bool)
		var exercises []string
		var hasWeightedExercise bool
		var hasBodyweightExercise bool

		for _, set := range strengthSets {
			// Check if weighted or bodyweight
			if set.WeightKg > 0 {
				hasWeightedExercise = true
			} else {
				hasBodyweightExercise = true
			}

			// Collect exercise names
			if set.ExerciseName != "" && !exercisesSeen[set.ExerciseName] {
				exercisesSeen[set.ExerciseName] = true
				exercises = append(exercises, set.ExerciseName)
			}
		}

		// Include exercise names - this is crucial for accurate image generation
		if len(exercises) > 0 {
			// Limit to 8 exercises for prompt brevity
			displayExercises := exercises
			if len(displayExercises) > 8 {
				displayExercises = displayExercises[:8]
			}
			parts = append(parts, fmt.Sprintf("Exercises performed: %s", strings.Join(displayExercises, ", ")))
		}

		// Indicate workout style (bodyweight vs weighted)
		if hasBodyweightExercise && !hasWeightedExercise {
			parts = append(parts, "Workout style: bodyweight only (no equipment/weights)")
		} else if hasWeightedExercise && !hasBodyweightExercise {
			parts = append(parts, "Workout style: weighted exercises with equipment")
		} else if hasWeightedExercise && hasBodyweightExercise {
			parts = append(parts, "Workout style: mixed bodyweight and weighted exercises")
		}

		// Collect muscle groups
		musclesSeen := make(map[pbactivity.MuscleGroup]bool)
		var muscles []string
		for _, set := range strengthSets {
			if set.PrimaryMuscleGroup != pbactivity.MuscleGroup_MUSCLE_GROUP_UNSPECIFIED && !musclesSeen[set.PrimaryMuscleGroup] {
				musclesSeen[set.PrimaryMuscleGroup] = true
				muscleName := strings.ToLower(strings.ReplaceAll(set.PrimaryMuscleGroup.String(), "MUSCLE_GROUP_", ""))
				muscleName = strings.ReplaceAll(muscleName, "_", " ")
				muscles = append(muscles, muscleName)
			}
		}
		if len(muscles) > 0 {
			parts = append(parts, fmt.Sprintf("Muscle focus: %s", strings.Join(muscles, ", ")))
		}
		parts = append(parts, fmt.Sprintf("Total sets: %d", len(strengthSets)))
	}

	// Time of day
	if activity.StartTime != nil {
		startTime := activity.StartTime.AsTime()
		hour := startTime.Hour()
		var timeOfDay string
		switch {
		case hour >= 5 && hour < 9:
			timeOfDay = "early morning (sunrise)"
		case hour >= 9 && hour < 12:
			timeOfDay = "morning"
		case hour >= 12 && hour < 17:
			timeOfDay = "afternoon"
		case hour >= 17 && hour < 20:
			timeOfDay = "evening (golden hour)"
		default:
			timeOfDay = "night"
		}
		parts = append(parts, fmt.Sprintf("Time of day: %s", timeOfDay))
	}

	return strings.Join(parts, "\n")
}

// truncatePrompt returns a truncated version of the prompt for logging
func truncatePrompt(prompt string, maxLen int) string {
	if len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen] + "..."
}

func (p *AIBannerProvider) generateBannerWithGemini(ctx context.Context, apiKey, prompt string) ([]byte, error) {
	// First attempt with the context-aware prompt
	imageData, err := p.callImagenAPI(ctx, apiKey, prompt)
	if err == nil {
		return imageData, nil
	}

	// Check if this looks like a content filter issue (empty response)
	// These errors indicate the API processed the request but returned no image
	if strings.Contains(err.Error(), "empty base64") || strings.Contains(err.Error(), "no predictions") {
		// Retry with simplified safe prompt that avoids content filter triggers
		imageData, retryErr := p.callImagenAPI(ctx, apiKey, fallbackPrompt)
		if retryErr == nil {
			return imageData, nil
		}
		// Both attempts failed - return original error with context
		return nil, fmt.Errorf("primary prompt failed (%w), fallback also failed (%v)", err, retryErr)
	}

	// Non-content-filter error (auth, network, etc.) - don't retry
	return nil, err
}

func (p *AIBannerProvider) callImagenAPI(ctx context.Context, apiKey, prompt string) ([]byte, error) {
	// Get GCP project ID and region from environment
	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if projectID == "" {
		return nil, fmt.Errorf("GCP_PROJECT_ID or GOOGLE_CLOUD_PROJECT environment variable not set")
	}

	region := os.Getenv("GCP_REGION")
	if region == "" {
		region = "us-central1" // Default region
	}

	// Use imagen-3.0-generate-002 model as specified in documentation
	modelVersion := "imagen-3.0-generate-002"

	// Build Vertex AI Imagen endpoint
	endpoint := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
		region, projectID, region, modelVersion,
	)

	// Prepare request body
	reqBody := ImagenRequest{
		Instances: []ImagenInstance{
			{Prompt: prompt},
		},
		Parameters: ImagenParameters{
			SampleCount:      1,
			AspectRatio:      "3:4", // Standard photograph ratio
			AddWatermark:     false, // Disable watermark for cleaner banners
			IncludeRaiReason: true,  // Include RAI filtering reasons for debugging
		},
	}

	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Get access token for authentication using Application Default Credentials
	// Note: Vertex AI requires OAuth 2.0 access tokens, not ID tokens
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to create token source: %w", err)
	}

	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imagen API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var imagenResp ImagenResponse
	if err := json.Unmarshal(respBody, &imagenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(imagenResp.Predictions) == 0 {
		// Include RAI filter reason and full response for debugging
		raiReason := imagenResp.RaiFilteredReason
		if raiReason == "" {
			raiReason = "unknown"
		}
		return nil, fmt.Errorf("no predictions in response (RAI reason: %s, full response: %s)", raiReason, string(respBody))
	}

	// Validate that we have actual image data
	base64Data := imagenResp.Predictions[0].BytesBase64Encoded
	if base64Data == "" {
		raiReason := imagenResp.RaiFilteredReason
		if raiReason == "" {
			raiReason = "none provided"
		}
		return nil, fmt.Errorf("empty base64 image data in response (prompt: %q, RAI reason: %s)", truncatePrompt(prompt, 100), raiReason)
	}

	// Decode base64 image data
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// Validate decoded image has content
	if len(imageData) == 0 {
		return nil, fmt.Errorf("decoded image data is empty")
	}

	return imageData, nil
}

func (p *AIBannerProvider) storeImage(ctx context.Context, bucketName, objectPath string, data []byte) (string, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create storage client: %w", err)
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	obj := bucket.Object(objectPath)

	writer := obj.NewWriter(ctx)
	writer.ContentType = "image/png"
	writer.CacheControl = "public, max-age=31536000, immutable"

	if _, err := bytes.NewReader(data).WriteTo(writer); err != nil {
		return "", fmt.Errorf("failed to write image data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	// Build URL using custom domain if configured, otherwise raw GCS URL
	// ASSETS_BASE_URL should be set per environment:
	//   - Dev: https://assets.dev.fitglue.tech
	//   - Prod: https://assets.fitglue.tech
	assetsBaseURL := os.Getenv("ASSETS_BASE_URL")
	if assetsBaseURL != "" {
		return fmt.Sprintf("%s/%s", assetsBaseURL, objectPath), nil
	}

	// Fallback to raw GCS URL
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, objectPath), nil
}
