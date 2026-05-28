// nolint:proto-json
package effort_score

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

const (
	boosterID       = "effort_score"
	historyKey      = "activities"
	maxHistory      = 14
	minHistory      = 3
	weightHR        = 0.35
	weightPace      = 0.25
	weightDuration  = 0.20
	weightElevation = 0.10
	weightIntensity = 0.10
)

// activitySnapshot stores the metrics for a single activity in the rolling history.
type activitySnapshot struct {
	Date     string  `json:"date"`
	AvgHR    float64 `json:"avg_hr"`
	AvgPace  float64 `json:"avg_pace"`  // min/km (0 if no pace data)
	Duration float64 `json:"duration"`  // minutes
	ElevGain float64 `json:"elev_gain"` // meters
	TRIMP    float64 `json:"trimp"`
}

// EffortScore computes a relative difficulty score 0-100 against the user's rolling history.
type EffortScore struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewEffortScore())
}

func NewEffortScore() *EffortScore {
	return &EffortScore{}
}

func (p *EffortScore) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *EffortScore) Name() string {
	return "effort-score"
}

func (p *EffortScore) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_EFFORT_SCORE
}

func (p *EffortScore) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("effort_score: starting", "activity_name", activity.Name)

	// 1. Extract current activity metrics
	current := extractMetrics(activity)
	logger.Debug("effort_score: extracted metrics",
		"avg_hr", current.AvgHR,
		"avg_pace", current.AvgPace,
		"duration", current.Duration,
		"elev_gain", current.ElevGain,
		"trimp", current.TRIMP,
	)

	// 2. Fetch rolling history from booster_data
	var history []activitySnapshot
	var lastExternalId string
	var cachedDescription string
	var cachedMetadata map[string]string

	if p.Service != nil && p.Service.DB != nil {
		data, err := p.Service.DB.GetBoosterData(ctx, user.UserId, boosterID)
		if err != nil {
			logger.Warn("effort_score: failed to fetch history", "error", err)
		} else if data != nil {
			history = parseHistory(data)
			if v, ok := data["last_external_id"].(string); ok {
				lastExternalId = v
			}
			if v, ok := data["last_result_description"].(string); ok {
				cachedDescription = v
			}
			if v, ok := data["last_result_metadata"].(map[string]interface{}); ok {
				cachedMetadata = make(map[string]string)
				for k, val := range v {
					if s, ok := val.(string); ok {
						cachedMetadata[k] = s
					}
				}
			}
		}
	}

	logger.Debug("effort_score: history loaded", "count", len(history))

	// Same-source dedup: return cached result without re-appending to history
	externalId := inputs["external_id"]
	if externalId != "" && lastExternalId == externalId && cachedDescription != "" {
		logger.Info("effort_score: returning cached result for same-source activity",
			"external_id", externalId)
		if cachedMetadata == nil {
			cachedMetadata = map[string]string{}
		}
		cachedMetadata["dedup"] = "true"
		return &providers.EnrichmentResult{
			Description: cachedDescription,
			Metadata:    cachedMetadata,
		}, nil
	}

	// 3. Check minimum history
	if len(history) < minHistory {
		logger.Debug("effort_score: insufficient history, skipping", "count", len(history))

		// Still persist the current activity to build history (no dedup fields — nothing to cache yet)
		p.persistHistory(ctx, logger, user.UserId, history, current, nil)

		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "skipped",
				"status_detail": fmt.Sprintf("insufficient_history (%d/%d)", len(history), minHistory),
			},
		}, nil
	}

	// 4. Calculate rolling averages
	avgHR, avgPace, avgDuration, avgElevGain, avgTRIMP := calculateAverages(history)

	// 5. Compute weighted effort score
	score, factors := computeEffortScore(current, avgHR, avgPace, avgDuration, avgElevGain, avgTRIMP)
	label := getScoreLabel(score)

	logger.Info("effort_score: calculated",
		"score", score,
		"label", label,
	)

	// 6. Build output before persisting so we can cache the description for dedup
	description := buildDescription(score, label, factors)

	resultMetadata := map[string]string{
		"status": "success",
		"score":  fmt.Sprintf("%.0f", score),
		"label":  label,
	}
	metadataMap := make(map[string]interface{})
	for k, v := range resultMetadata {
		metadataMap[k] = v
	}

	// 7. Persist updated history + dedup fields
	p.persistHistory(ctx, logger, user.UserId, history, current, map[string]interface{}{
		"last_external_id":        externalId,
		"last_result_description": description,
		"last_result_metadata":    metadataMap,
	})

	return &providers.EnrichmentResult{
		Description: description,
		Metadata:    resultMetadata,
		Enrichments: &pbactivity.ActivityEnrichments{
			Effort: &pbactivity.EffortScoreSummary{
				Score: int32(score),
				Band:  label,
			},
		},
	}, nil
}

// extractMetrics pulls all relevant signals from the activity.
func extractMetrics(activity *pbactivity.StandardizedActivity) activitySnapshot {
	var totalHR float64
	var hrSamples int
	var totalSpeed float64
	var speedSamples int
	var elevGain float64
	var lastAlt float64
	var hasAlt bool
	var durationMinutes float64

	for _, session := range activity.Sessions {
		durationMinutes += session.TotalElapsedTime / 60
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate > 0 {
					totalHR += float64(record.HeartRate)
					hrSamples++
				}
				if record.Speed > 0 {
					totalSpeed += record.Speed
					speedSamples++
				}
				if record.Altitude > 0 {
					if hasAlt {
						diff := record.Altitude - lastAlt
						if diff > 0 {
							elevGain += diff
						}
					}
					lastAlt = record.Altitude
					hasAlt = true
				}
			}
		}
	}

	activityDate := time.Now().Format("2006-01-02")
	if activity.StartTime != nil {
		activityDate = activity.StartTime.AsTime().Format("2006-01-02")
	}

	snap := activitySnapshot{
		Date:     activityDate,
		Duration: durationMinutes,
		ElevGain: elevGain,
	}

	if hrSamples > 0 {
		snap.AvgHR = totalHR / float64(hrSamples)
	}

	if speedSamples > 0 {
		avgSpeed := totalSpeed / float64(speedSamples) // m/s
		if avgSpeed > 0 {
			snap.AvgPace = (1000 / avgSpeed) / 60 // min/km
		}
	}

	// Calculate simplified TRIMP
	if snap.AvgHR > 0 {
		maxHR := 190.0
		restHR := 60.0
		hrReserve := (snap.AvgHR - restHR) / (maxHR - restHR)
		if hrReserve < 0 {
			hrReserve = 0
		}
		if hrReserve > 1 {
			hrReserve = 1
		}
		snap.TRIMP = durationMinutes * hrReserve * 0.64 * math.Exp(1.92*hrReserve)
	} else {
		snap.TRIMP = durationMinutes * 0.5
	}

	return snap
}

// parseHistory deserializes the activity history from Firestore data.
func parseHistory(data map[string]interface{}) []activitySnapshot {
	raw, ok := data[historyKey]
	if !ok {
		return nil
	}

	// Firestore stores arrays as []interface{}
	arr, ok := raw.([]interface{})
	if !ok {
		// Try JSON string fallback
		if str, ok := raw.(string); ok {
			var result []activitySnapshot
			if err := json.Unmarshal([]byte(str), &result); err == nil {
				return result
			}
		}
		return nil
	}

	var result []activitySnapshot
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		snap := activitySnapshot{
			AvgHR:    providers.ToFloat64(m["avg_hr"]),
			AvgPace:  providers.ToFloat64(m["avg_pace"]),
			Duration: providers.ToFloat64(m["duration"]),
			ElevGain: providers.ToFloat64(m["elev_gain"]),
			TRIMP:    providers.ToFloat64(m["trimp"]),
		}
		if d, ok := m["date"].(string); ok {
			snap.Date = d
		}
		result = append(result, snap)
	}
	return result
}

// calculateAverages computes the mean of each metric across history entries.
func calculateAverages(history []activitySnapshot) (avgHR, avgPace, avgDuration, avgElevGain, avgTRIMP float64) {
	var sumHR, sumPace, sumDuration, sumElevGain, sumTRIMP float64
	var countHR, countPace int

	for _, h := range history {
		sumDuration += h.Duration
		sumElevGain += h.ElevGain
		sumTRIMP += h.TRIMP

		if h.AvgHR > 0 {
			sumHR += h.AvgHR
			countHR++
		}
		if h.AvgPace > 0 {
			sumPace += h.AvgPace
			countPace++
		}
	}

	n := float64(len(history))
	avgDuration = sumDuration / n
	avgElevGain = sumElevGain / n
	avgTRIMP = sumTRIMP / n

	if countHR > 0 {
		avgHR = sumHR / float64(countHR)
	}
	if countPace > 0 {
		avgPace = sumPace / float64(countPace)
	}

	return
}

// factorDetail describes one factor's contribution to the effort score.
type factorDetail struct {
	Name  string
	Ratio float64
	Emoji string
}

// computeEffortScore calculates the weighted effort score and individual factor details.
func computeEffortScore(current activitySnapshot, avgHR, avgPace, avgDuration, avgElevGain, avgTRIMP float64) (float64, []factorDetail) {
	type weightedFactor struct {
		weight float64
		ratio  float64
		detail factorDetail
	}

	var factors []weightedFactor

	// Heart Rate factor
	if current.AvgHR > 0 && avgHR > 0 {
		ratio := current.AvgHR / avgHR
		factors = append(factors, weightedFactor{
			weight: weightHR,
			ratio:  ratio,
			detail: factorDetail{Name: "HR", Ratio: ratio, Emoji: "❤️"},
		})
	}

	// Pace factor (lower pace = harder, so invert: avgPace/currentPace)
	if current.AvgPace > 0 && avgPace > 0 {
		ratio := avgPace / current.AvgPace // faster = higher ratio
		factors = append(factors, weightedFactor{
			weight: weightPace,
			ratio:  ratio,
			detail: factorDetail{Name: "Pace", Ratio: ratio, Emoji: "🏃"},
		})
	}

	// Duration factor
	if current.Duration > 0 && avgDuration > 0 {
		ratio := current.Duration / avgDuration
		factors = append(factors, weightedFactor{
			weight: weightDuration,
			ratio:  ratio,
			detail: factorDetail{Name: "Duration", Ratio: ratio, Emoji: "⏱️"},
		})
	}

	// Elevation factor
	if current.ElevGain > 0 && avgElevGain > 0 {
		ratio := current.ElevGain / avgElevGain
		factors = append(factors, weightedFactor{
			weight: weightElevation,
			ratio:  ratio,
			detail: factorDetail{Name: "Elevation", Ratio: ratio, Emoji: "⛰️"},
		})
	}

	// Intensity (TRIMP) factor
	if current.TRIMP > 0 && avgTRIMP > 0 {
		ratio := current.TRIMP / avgTRIMP
		factors = append(factors, weightedFactor{
			weight: weightIntensity,
			ratio:  ratio,
			detail: factorDetail{Name: "Intensity", Ratio: ratio, Emoji: "💪"},
		})
	}

	if len(factors) == 0 {
		return 50, nil // Default to "normal" if no data
	}

	// Redistribute weights proportionally across available factors
	totalWeight := 0.0
	for _, f := range factors {
		totalWeight += f.weight
	}

	weightedSum := 0.0
	var details []factorDetail
	for _, f := range factors {
		normalizedWeight := f.weight / totalWeight
		weightedSum += normalizedWeight * f.ratio
		details = append(details, f.detail)
	}

	// Map combined ratio to 0-100 scale (1.0 ratio = 50 score)
	score := math.Min(100, weightedSum*50)
	if score < 0 {
		score = 0
	}

	return math.Round(score), details
}

// getScoreLabel returns a human-readable label for the effort score.
func getScoreLabel(score float64) string {
	switch {
	case score <= 30:
		return "Easy"
	case score <= 50:
		return "Moderate"
	case score <= 70:
		return "Hard"
	case score <= 85:
		return "Very Hard"
	default:
		return "All-Out"
	}
}

// buildDescription formats the effort score output using Rule G52 multi-line bullets.
func buildDescription(score float64, label string, factors []factorDetail) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("💥 Effort Score: %.0f/100 (%s)\n", score, label))

	for _, f := range factors {
		sb.WriteString(fmt.Sprintf("• %s %s: %.2f× avg\n", f.Emoji, f.Name, f.Ratio))
	}

	// Add trend indicator
	if score >= 70 {
		sb.WriteString("• 📈 Harder than usual")
	} else if score <= 30 {
		sb.WriteString("• 📉 Easier than usual")
	} else {
		sb.WriteString("• ➡️ Typical effort")
	}

	return sb.String()
}

// persistHistory saves the updated activity history to booster_data.
// extraFields is merged into the saved document (used for dedup fields when a score was computed).
func (p *EffortScore) persistHistory(ctx context.Context, logger *slog.Logger, userID string, history []activitySnapshot, current activitySnapshot, extraFields map[string]interface{}) {
	if p.Service == nil || p.Service.DB == nil {
		return
	}

	// Append current and trim to max entries
	history = append(history, current)
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}

	// Serialize history for Firestore storage
	historyData := make([]interface{}, len(history))
	for i, h := range history {
		historyData[i] = map[string]interface{}{
			"date":      h.Date,
			"avg_hr":    h.AvgHR,
			"avg_pace":  h.AvgPace,
			"duration":  h.Duration,
			"elev_gain": h.ElevGain,
			"trimp":     h.TRIMP,
		}
	}

	updateData := map[string]interface{}{
		historyKey: historyData,
	}
	for k, v := range extraFields {
		updateData[k] = v
	}

	if err := p.Service.DB.SetBoosterData(ctx, userID, boosterID, updateData); err != nil {
		logger.Warn("effort_score: failed to save history", "error", err)
	}
}
