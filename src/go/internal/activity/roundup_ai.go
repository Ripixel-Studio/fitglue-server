package activity

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const roundupAISummaryTimeout = 10 * time.Second

// generateRoundupAISummary calls Gemini to produce a one-paragraph narrative
// for the period. Returns an empty string if the API key is absent or the
// call fails — callers should treat an empty string as "not available".
func generateRoundupAISummary(ctx context.Context, logger infra.Logger, roundup *pbactivity.ShowcaseRoundup) string {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return ""
	}

	tctx, cancel := context.WithTimeout(ctx, roundupAISummaryTimeout)
	defer cancel()

	client, err := genai.NewClient(tctx, option.WithAPIKey(apiKey))
	if err != nil {
		logger.Warn(ctx, "roundup AI summary: failed to create Gemini client", "error", err)
		return ""
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash")
	model.SetTemperature(0.7)
	model.SetTopP(0.9)
	model.SetMaxOutputTokens(512)
	model.SystemInstruction = genai.NewUserContent(genai.Text(roundupSystemInstruction))

	periodCtx := buildRoundupSummaryContext(roundup)
	resp, err := model.GenerateContent(tctx, genai.Text(roundupUserPrompt+"\n\nPeriod data:\n"+periodCtx))
	if err != nil {
		logger.Warn(ctx, "roundup AI summary: generation failed", "error", err)
		return ""
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return ""
	}

	raw := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			raw += string(text)
		}
	}
	return strings.TrimSpace(raw)
}

const roundupSystemInstruction = `You are a sports journalist writing brief training period summaries.

Guidelines:
- Write exactly one paragraph, 2–4 sentences.
- DO NOT address the athlete directly (no "you", "your"). Write in third person or impersonal voice.
- Avoid motivational coach clichés ("crushed it", "smashed", "killed it", "great work", "keep it up").
- Reference specific numbers and details from the data.
- Tone: casual, analytical, objective. Like a match report, not a pep talk.
- No emojis. No markdown.`

const roundupUserPrompt = `Write a brief summary paragraph of this athlete's training period.`

// sanitiseRoundupString truncates user-controlled strings and strips prompt-injection
// delimiters before they are embedded in the Gemini prompt.
func sanitiseRoundupString(s string) string {
	const maxLen = 200
	s = strings.NewReplacer("<", "", ">", "", "{", "", "}", "").Replace(s)
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return strings.TrimSpace(s)
}

// buildRoundupSummaryContext produces the plaintext data section of the Gemini
// prompt from an already-computed ShowcaseRoundup.
func buildRoundupSummaryContext(roundup *pbactivity.ShowcaseRoundup) string {
	var lines []string

	// Period
	period := periodTypeName(roundup.PeriodType)
	lines = append(lines, fmt.Sprintf("Period: %s", period))
	if roundup.PeriodStart != nil && roundup.PeriodEnd != nil {
		start := roundup.PeriodStart.AsTime().UTC().Format("2 Jan 2006")
		end := roundup.PeriodEnd.AsTime().UTC().AddDate(0, 0, -1).Format("2 Jan 2006")
		lines = append(lines, fmt.Sprintf("Date range: %s – %s", start, end))
	}

	if name := sanitiseRoundupString(roundup.OwnerDisplayName); name != "" {
		lines = append(lines, fmt.Sprintf("Athlete: %s", name))
	}

	lines = append(lines, fmt.Sprintf("Sessions: %d", roundup.TotalActivities))

	if roundup.TotalDurationSeconds > 0 {
		h := int(roundup.TotalDurationSeconds) / 3600
		m := (int(roundup.TotalDurationSeconds) % 3600) / 60
		if h > 0 {
			lines = append(lines, fmt.Sprintf("Total time: %dh %dm", h, m))
		} else {
			lines = append(lines, fmt.Sprintf("Total time: %dm", m))
		}
	}

	if roundup.TotalDistanceMeters > 500 {
		km := roundup.TotalDistanceMeters / 1000
		lines = append(lines, fmt.Sprintf("Total distance: %.1f km", km))
	}

	if roundup.TotalCaloriesKcal > 0 {
		lines = append(lines, fmt.Sprintf("Calories burned: %d kcal", roundup.TotalCaloriesKcal))
	}

	// Sport breakdown
	if len(roundup.ActivityTypeBreakdowns) > 0 {
		var sports []string
		for _, bd := range roundup.ActivityTypeBreakdowns {
			label := activityTypeShortLabel(bd.ActivityType)
			sports = append(sports, fmt.Sprintf("%d %s", bd.ActivityCount, label))
		}
		lines = append(lines, fmt.Sprintf("Sports: %s", strings.Join(sports, ", ")))
	}

	if len(roundup.PrsAchieved) > 0 {
		lines = append(lines, fmt.Sprintf("Personal records broken: %d", len(roundup.PrsAchieved)))
	}

	if roundup.LongestActivityDurationSeconds > 60 {
		h := int(roundup.LongestActivityDurationSeconds) / 3600
		m := (int(roundup.LongestActivityDurationSeconds) % 3600) / 60
		if h > 0 {
			lines = append(lines, fmt.Sprintf("Longest session: %dh %dm", h, m))
		} else {
			lines = append(lines, fmt.Sprintf("Longest session: %dm", m))
		}
	}

	if roundup.HighestAvgBpm > 0 {
		if title := sanitiseRoundupString(roundup.HighestAvgBpmActivityTitle); title != "" {
			lines = append(lines, fmt.Sprintf("Peak avg heart rate: %d bpm (%s)", roundup.HighestAvgBpm, title))
		} else {
			lines = append(lines, fmt.Sprintf("Peak avg heart rate: %d bpm", roundup.HighestAvgBpm))
		}
	}

	if roundup.HighestCaloriesPerHourKcal > 0 {
		lines = append(lines, fmt.Sprintf("Highest burn rate: %.0f kcal/h", math.Round(roundup.HighestCaloriesPerHourKcal)))
	}

	// Effort distribution
	easy := roundup.EffortEasyCount
	mod := roundup.EffortModerateCount
	hard := roundup.EffortHardCount
	if easy+mod+hard > 0 {
		lines = append(lines, fmt.Sprintf("Effort distribution: %d easy, %d moderate, %d hard", easy, mod, hard))
	}

	return strings.Join(lines, "\n")
}

func periodTypeName(t pbactivity.RoundupPeriodType) string {
	switch t {
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK:
		return "week"
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH:
		return "month"
	case pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR:
		return "year"
	default:
		return "period"
	}
}

func activityTypeShortLabel(t pbactivity.ActivityType) string {
	s := t.String()
	s = strings.TrimPrefix(s, "ACTIVITY_TYPE_")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", " ")
	return s
}
