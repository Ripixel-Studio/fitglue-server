package recovery_advisor

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/encoding/protojson"
)

// RecoveryAdvisor calculates training load and suggests recovery time.
// Uses TRIMP (Training Impulse) with an Acute:Chronic Workload Ratio (ACWR) model.
type RecoveryAdvisor struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewRecoveryAdvisor())
}

func NewRecoveryAdvisor() *RecoveryAdvisor {
	return &RecoveryAdvisor{}
}

func (p *RecoveryAdvisor) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *RecoveryAdvisor) Name() string {
	return "recovery-advisor"
}

func (p *RecoveryAdvisor) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_RECOVERY_ADVISOR
}

func (p *RecoveryAdvisor) IsIdempotent() bool { return false }

func (p *RecoveryAdvisor) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("recovery_advisor: starting", "activity_name", activity.Name)

	// --- Configurable inputs (matching training-load pattern) ---
	maxHR := 190.0
	restHR := 60.0
	gender := "male"

	if v, ok := inputs["max_hr"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			maxHR = f
		}
	}
	if v, ok := inputs["rest_hr"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			restHR = f
		}
	}
	if v, ok := inputs["gender"]; ok {
		gender = v
	}

	genderCoeff := 1.92
	if gender == "female" {
		genderCoeff = 1.67
	}

	// --- Calculate session TRIMP ---
	var durationMinutes float64
	var avgHR float64
	var hrSamples int

	for _, session := range activity.Sessions {
		durationMinutes += session.TotalElapsedTime / 60
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.HeartRate > 0 {
					avgHR += float64(record.HeartRate)
					hrSamples++
				}
			}
		}
	}

	if hrSamples > 0 {
		avgHR = avgHR / float64(hrSamples)
	}

	hrRange := maxHR - restHR
	var trimp float64

	if avgHR > 0 && hrRange > 0 {
		hrReserve := (avgHR - restHR) / hrRange
		if hrReserve < 0 {
			hrReserve = 0
		}
		if hrReserve > 1 {
			hrReserve = 1
		}
		trimp = durationMinutes * hrReserve * 0.64 * math.Exp(genderCoeff*hrReserve)
	} else {
		// Estimate from duration only (less accurate)
		trimp = durationMinutes * 0.5
	}

	// --- Fetch 28-day training load history ---
	boosterId := "recovery_advisor"
	// Anchor the 7/28-day windows on the activity's own date, not processing
	// time: backdated uploads and history replays must land on the day they
	// happened, otherwise every replayed session piles into "today" and the
	// acute/chronic loads become the sum of the whole history.
	now := time.Now()
	if activity.StartTime != nil {
		now = activity.StartTime.AsTime()
	}
	var data map[string]interface{}

	var lastExternalId string
	var cachedDescription string
	var cachedMetadata map[string]string

	if p.Service != nil && p.Service.DB != nil {
		var err error
		data, err = p.Service.DB.GetBoosterData(ctx, user.UserId, boosterId)
		if err != nil {
			logger.Warn("Failed to fetch recovery data", "error", err)
		}
		if data != nil {
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

	// Same-source dedup: if this activity was already processed, return cached result.
	// Only use cache if structured enrichments were also cached — if last_result_enrichments
	// is absent (written by older code), fall through and run fresh so the showcase GCS blob
	// gets proper typed enrichment data.
	externalId := inputs["external_id"]
	enrichmentsJSON, hasEnrichments := data["last_result_enrichments"].(string)
	if externalId != "" && lastExternalId == externalId && cachedDescription != "" && hasEnrichments && enrichmentsJSON != "" {
		logger.Info("recovery_advisor: returning cached result for same-source activity",
			"external_id", externalId)
		if cachedMetadata == nil {
			cachedMetadata = map[string]string{}
		}
		cachedMetadata["dedup"] = "true"
		result := &providers.EnrichmentResult{
			Description: cachedDescription,
			Metadata:    cachedMetadata,
		}
		var enrichments pbactivity.ActivityEnrichments
		if err := protojson.Unmarshal([]byte(enrichmentsJSON), &enrichments); err == nil {
			result.Enrichments = &enrichments
		}
		return result, nil
	}

	// Compute acute (7-day) and chronic (28-day) loads from stored data
	var acuteLoad float64   // days 1-7
	var chronicLoad float64 // days 1-28
	var consecutiveHardDays int

	if data != nil {
		// Count consecutive hard days (most recent first)
		checkingConsecutive := true

		for i := 1; i <= 28; i++ {
			dateKey := now.AddDate(0, 0, -i).Format("2006-01-02")
			dayLoad := providers.ToFloat64(data[dateKey])
			chronicLoad += dayLoad
			if i <= 7 {
				acuteLoad += dayLoad
			}
			// Track consecutive hard days (TRIMP > 60 per day)
			if checkingConsecutive && i <= 7 {
				if dayLoad > 60 {
					consecutiveHardDays++
				} else {
					checkingConsecutive = false
				}
			}
		}
	}

	// Add today's load, accumulating with any previously stored TRIMP for today.
	// On a full-pipeline repost the original run already stored the correct value;
	// use it as-is so we don't double-count this activity's TRIMP.
	today := now.Format("2006-01-02")
	todayLoad := trimp
	if data != nil {
		if inputs["is_repost"] == "true" {
			todayLoad = providers.ToFloat64(data[today])
		} else {
			todayLoad += providers.ToFloat64(data[today])
		}
	}

	// Include today in acute and chronic totals
	totalAcuteLoad := acuteLoad + todayLoad
	totalChronicLoad := chronicLoad + todayLoad

	// Check if today is also a hard day for consecutive count
	if todayLoad > 60 {
		consecutiveHardDays++
	} else {
		// Today is not hard, so reset the consecutive count
		// (consecutive means unbroken from today backward)
		consecutiveHardDays = 0
	}

	// Re-count consecutive hard days including today properly
	consecutiveHardDays = countConsecutiveHardDays(data, todayLoad, now)

	// Calculate ACWR (Acute:Chronic Workload Ratio)
	// Chronic load averaged per day over 28 days, acute averaged per day over 7 days
	chronicDailyAvg := totalChronicLoad / 28.0
	acuteDailyAvg := totalAcuteLoad / 7.0
	var acwr float64
	if chronicDailyAvg > 0 {
		acwr = acuteDailyAvg / chronicDailyAvg
	}

	// Calculate recovery recommendation
	recoveryHours, intensity := getRecoveryRecommendation(trimp, totalAcuteLoad, acwr, consecutiveHardDays)

	// ACWR status label
	acwrLabel := getACWRLabel(acwr)

	// Build output
	var sb strings.Builder

	sb.WriteString("💤 Recovery Advisor:\n")
	sb.WriteString(fmt.Sprintf("• Session load: %.0f TRIMP (%s)\n", trimp, intensity))
	sb.WriteString(fmt.Sprintf("• 7-day load: %.0f TRIMP • 28-day avg: %.0f TRIMP\n", totalAcuteLoad, totalChronicLoad))

	if chronicDailyAvg > 0 {
		acwrLine := fmt.Sprintf("• ACWR: %.2f (%s", acwr, acwrLabel)
		if acwr > 1.5 {
			acwrLine += " ⚠️"
		}
		acwrLine += ")\n"
		sb.WriteString(acwrLine)
	}

	if consecutiveHardDays >= 3 {
		sb.WriteString(fmt.Sprintf("• ⚠️ %d consecutive hard days — fatigue risk\n", consecutiveHardDays))
	}

	sb.WriteString(fmt.Sprintf("• 💡 Suggested recovery: %s", formatRecoveryTime(recoveryHours)))

	logger.Info("Recovery advisor processed",
		"trimp", trimp,
		"acute_load", totalAcuteLoad,
		"chronic_load", totalChronicLoad,
		"acwr", acwr,
		"consecutive_hard_days", consecutiveHardDays,
		"recovery_hours", recoveryHours,
		"intensity", intensity,
	)

	resultMetadata := map[string]string{
		"trimp":                 fmt.Sprintf("%.0f", trimp),
		"acute_load":            fmt.Sprintf("%.0f", totalAcuteLoad),
		"chronic_load":          fmt.Sprintf("%.0f", totalChronicLoad),
		"acwr":                  fmt.Sprintf("%.2f", acwr),
		"acwr_label":            acwrLabel,
		"recovery_hours":        fmt.Sprintf("%.0f", recoveryHours),
		"intensity":             intensity,
		"consecutive_hard_days": fmt.Sprintf("%d", consecutiveHardDays),
	}

	alertText := ""
	if acwr > 0 {
		alertText = fmt.Sprintf("ACWR %.2f — %s", acwr, acwrLabel)
	}

	resultEnrichments := &pbactivity.ActivityEnrichments{
		Recovery: &pbactivity.RecoverySummary{
			SessionLoad:           int32(trimp),
			AcuteChronicRatio:     acwr,
			HoursToRecover:        int32(recoveryHours),
			Alert:                 acwr > 1.5 || consecutiveHardDays >= 3,
			AlertText:             alertText,
			SevenDayLoad:          int32(totalAcuteLoad),
			TwentyEightDayAvgLoad: int32(chronicDailyAvg),
		},
	}

	// Persist today's load + cached result for same-source dedup.
	// Skip on full-pipeline repost: today's TRIMP was already accumulated on the original run.
	if p.Service != nil && p.Service.DB != nil && inputs["is_repost"] != "true" {
		metadataMap := make(map[string]interface{})
		for k, v := range resultMetadata {
			metadataMap[k] = v
		}
		updateData := map[string]interface{}{
			today:                     todayLoad,
			"last_external_id":        externalId,
			"last_result_description": sb.String(),
			"last_result_metadata":    metadataMap,
		}
		// Cache structured enrichments so dedup hits restore full typed data
		if b, err := protojson.Marshal(resultEnrichments); err == nil {
			updateData["last_result_enrichments"] = string(b)
		}
		if err := p.Service.DB.SetBoosterData(ctx, user.UserId, boosterId, updateData); err != nil {
			logger.Warn("Failed to save recovery data", "error", err)
		}
	}

	return &providers.EnrichmentResult{
		Description: sb.String(),
		Metadata:    resultMetadata,
		Enrichments: resultEnrichments,
	}, nil
}

// countConsecutiveHardDays counts unbroken streak of hard days (TRIMP > 60)
// working backward from today.
func countConsecutiveHardDays(data map[string]interface{}, todayLoad float64, now time.Time) int {
	if todayLoad <= 60 {
		return 0
	}
	count := 1 // today counts
	for i := 1; i <= 7; i++ {
		dateKey := now.AddDate(0, 0, -i).Format("2006-01-02")
		dayLoad := 0.0
		if data != nil {
			dayLoad = providers.ToFloat64(data[dateKey])
		}
		if dayLoad > 60 {
			count++
		} else {
			break
		}
	}
	return count
}

func getRecoveryRecommendation(trimp, acuteLoad, acwr float64, consecutiveHardDays int) (hours float64, intensity string) {
	// Base recovery on session intensity
	switch {
	case trimp >= 150:
		intensity = "Very Hard"
		hours = 48
	case trimp >= 90:
		intensity = "Hard"
		hours = 36
	case trimp >= 60:
		intensity = "Moderate"
		hours = 24
	case trimp >= 30:
		intensity = "Easy"
		hours = 12
	default:
		intensity = "Recovery"
		hours = 8
	}

	// Graduated weekly load scaling (replaces binary >500)
	switch {
	case acuteLoad > 700:
		hours += 12
	case acuteLoad > 500:
		hours += 8
	case acuteLoad > 300:
		hours += 4
	}

	// ACWR-based adjustment
	switch {
	case acwr > 1.5:
		hours += 16 // Overreaching — significant extra rest
	case acwr > 1.2:
		hours += 8 // Building — moderate extra rest
	case acwr < 0.8 && acwr > 0:
		hours -= 4 // Detraining — slightly less rest needed
	}

	// Consecutive hard days adjustment
	if consecutiveHardDays >= 3 {
		hours += 8
	}

	// Floor at minimum 4 hours
	if hours < 4 {
		hours = 4
	}

	return hours, intensity
}

func getACWRLabel(acwr float64) string {
	switch {
	case acwr > 1.5:
		return "Overreaching"
	case acwr > 1.2:
		return "Building"
	case acwr >= 0.8:
		return "Optimal"
	case acwr > 0:
		return "Detraining"
	default:
		return "No History"
	}
}

func formatRecoveryTime(hours float64) string {
	return fmt.Sprintf("%.0f hours", hours)
}
