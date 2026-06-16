package activity

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const roundupAISummaryTimeout = 10 * time.Second

const (
	maxRoundupPhotos  = 24
	maxRoundupRoutes  = 12
	maxRoundupPlaces  = 8
	maxRoundupMuscles = 6
)

// wetWeatherKeywords flag a weather description as "grit" weather for the roundup.
var wetWeatherKeywords = []string{"rain", "drizzle", "shower", "snow", "sleet", "thunder"}

func isWetWeather(desc string) bool {
	d := strings.ToLower(desc)
	for _, k := range wetWeatherKeywords {
		if strings.Contains(d, k) {
			return true
		}
	}
	return false
}

// computeSessionPeaks finds the single-session maxima across the period for the
// "ceiling raised" highlights band: furthest distance, most calories in one
// session, and the heaviest strength session (reps × weight volume).
func computeSessionPeaks(entries []*pbactivity.ShowcaseProfileEntry) (furthestM float64, mostCal int32, biggestVolKg float64) {
	for _, e := range entries {
		if e.DistanceMeters > furthestM {
			furthestM = e.DistanceMeters
		}
		if e.CaloriesKcal != nil && *e.CaloriesKcal > mostCal {
			mostCal = *e.CaloriesKcal
		}
		if e.TotalWeightKg > biggestVolKg {
			biggestVolKg = e.TotalWeightKg
		}
	}
	return furthestM, mostCal, biggestVolKg
}

// aggregateRoundupEnrichments rolls the per-entry enrichment summaries up into
// the period-level place list, weather grit, best efforts and muscles worked.
func aggregateRoundupEnrichments(entries []*pbactivity.ShowcaseProfileEntry) (
	places []*pbactivity.RoundupPlace,
	weather *pbactivity.RoundupWeather,
	bestEfforts []*pbactivity.BestEffort,
	muscles []*pbactivity.RoundupMuscle,
) {
	placeData := map[string]*pbactivity.RoundupPlace{}
	var placeOrder []string
	muscleData := map[string]*pbactivity.RoundupMuscle{}
	var muscleOrder []string
	effortByKey := map[string]*pbactivity.BestEffort{}
	var effortOrder []string

	var w pbactivity.RoundupWeather
	weatherSeen, coldSet, hotSet := false, false, false

	for _, e := range entries {
		// Places trained
		if e.LocationName != nil && *e.LocationName != "" {
			name := *e.LocationName
			p, ok := placeData[name]
			if !ok {
				p = &pbactivity.RoundupPlace{Name: name}
				if e.Country != nil {
					p.Country = *e.Country
				}
				placeData[name] = p
				placeOrder = append(placeOrder, name)
			}
			p.ActivityCount++
		}

		// Weather grit
		if e.TempC != nil {
			weatherSeen = true
			w.SessionCount++
			t := *e.TempC
			if !coldSet || t < w.ColdestTempC {
				w.ColdestTempC = t
				coldSet = true
			}
			if !hotSet || t > w.HottestTempC {
				w.HottestTempC = t
				hotSet = true
			}
		}
		if e.WeatherDescription != nil && isWetWeather(*e.WeatherDescription) {
			w.RainCount++
		}

		// Best efforts — fastest per distance key
		for _, be := range e.BestEfforts {
			if be == nil || be.DistanceKey == "" || be.TimeSeconds <= 0 {
				continue
			}
			cur, ok := effortByKey[be.DistanceKey]
			if !ok {
				effortByKey[be.DistanceKey] = be
				effortOrder = append(effortOrder, be.DistanceKey)
			} else if be.TimeSeconds < cur.TimeSeconds {
				effortByKey[be.DistanceKey] = be
			}
		}

		// Muscles worked
		for _, m := range e.PrimaryMuscles {
			if m == "" {
				continue
			}
			md, ok := muscleData[m]
			if !ok {
				md = &pbactivity.RoundupMuscle{Name: m}
				muscleData[m] = md
				muscleOrder = append(muscleOrder, m)
			}
			md.Count++
		}
	}

	for _, name := range placeOrder {
		places = append(places, placeData[name])
	}
	sort.SliceStable(places, func(i, j int) bool { return places[i].ActivityCount > places[j].ActivityCount })
	if len(places) > maxRoundupPlaces {
		places = places[:maxRoundupPlaces]
	}

	for _, name := range muscleOrder {
		muscles = append(muscles, muscleData[name])
	}
	sort.SliceStable(muscles, func(i, j int) bool { return muscles[i].Count > muscles[j].Count })
	if len(muscles) > maxRoundupMuscles {
		muscles = muscles[:maxRoundupMuscles]
	}

	for _, k := range effortOrder {
		bestEfforts = append(bestEfforts, effortByKey[k])
	}
	sort.SliceStable(bestEfforts, func(i, j int) bool { return bestEfforts[i].DistanceM < bestEfforts[j].DistanceM })

	if weatherSeen {
		weather = &w
	}
	return places, weather, bestEfforts, muscles
}

// collectRoundupMedia gathers photo and GPS route-thumbnail assets from the
// period's entries for the roundup photo mosaic and route wall. Photos are only
// included when the owner has the public photo gallery enabled; both lists are
// capped to keep the persisted roundup document small.
func collectRoundupMedia(entries []*pbactivity.ShowcaseProfileEntry, galleryEnabled bool) (photos []*pbactivity.RoundupPhoto, routes []*pbactivity.RoundupRoute) {
	for _, e := range entries {
		dateStr := ""
		if e.StartTime != nil {
			dateStr = e.StartTime.AsTime().UTC().Format("2 Jan")
		}
		if galleryEnabled && len(photos) < maxRoundupPhotos {
			for _, url := range e.PhotoUrls {
				if url == "" {
					continue
				}
				photos = append(photos, &pbactivity.RoundupPhoto{
					Url:           url,
					ActivityTitle: e.Title,
					Date:          dateStr,
					ActivityType:  e.ActivityType,
				})
				if len(photos) >= maxRoundupPhotos {
					break
				}
			}
		}
		if e.RouteThumbnailUrl != "" && len(routes) < maxRoundupRoutes {
			routes = append(routes, &pbactivity.RoundupRoute{
				ThumbnailUrl:   e.RouteThumbnailUrl,
				ActivityTitle:  e.Title,
				Date:           dateStr,
				DistanceMeters: e.DistanceMeters,
				ActivityType:   e.ActivityType,
			})
		}
	}
	return photos, routes
}

// deriveEffortLevel maps a single showcase entry to a 0–4 effort level for
// the consistency calendar heatmap (0=rest,1=easy,2=moderate,3=hard,4=peak).
func deriveEffortLevel(e *pbactivity.ShowcaseProfileEntry) int32 {
	if len(e.HrZoneMinutes) >= 6 {
		var total int32
		for i := 1; i <= 5; i++ {
			total += e.HrZoneMinutes[i]
		}
		if total > 10 {
			z5 := e.HrZoneMinutes[5]
			z45 := e.HrZoneMinutes[4] + z5
			z12 := e.HrZoneMinutes[1] + e.HrZoneMinutes[2]
			switch {
			case z5*100 > total*15:
				return 4 // >15 % in Z5 → peak
			case z45*100 > total*25:
				return 3 // >25 % in Z4+Z5 → hard
			case z12*100 > total*70:
				return 1 // >70 % in Z1+Z2 → easy
			default:
				return 2
			}
		}
	}
	// duration-based fallback
	mins := int32(e.DurationSeconds / 60)
	switch {
	case mins >= 120:
		return 4
	case mins >= 75:
		return 3
	case mins >= 30:
		return 2
	case mins > 0:
		return 1
	default:
		return 0
	}
}

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
