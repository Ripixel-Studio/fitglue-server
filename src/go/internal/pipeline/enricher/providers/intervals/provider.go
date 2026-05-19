package intervals

import (
	"context"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"math"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Intervals detects structured interval data in activities and produces a
// human-readable summary following Rule G40 (SectionHeader).
type Intervals struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewIntervals())
}

func NewIntervals() *Intervals {
	return &Intervals{}
}

func (p *Intervals) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *Intervals) Name() string {
	return "intervals"
}

func (p *Intervals) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_INTERVALS
}

// ---------- internal types ----------

// intervalLap holds parsed data for a single interval.
type intervalLap struct {
	intensity string
	duration  float64                // seconds
	distance  float64                // metres
	avgSpeed  float64                // m/s (derived from distance/duration)
	avgHR     float64                // bpm (average of records)
	peakHR    float64                // bpm (max of records)
	startTime *timestamppb.Timestamp // original lap start time for time markers
}

// intervalGroup collects consecutive laps that share the same intensity and
// similar duration (±25%) – these are "repeats".
type intervalGroup struct {
	intensity string
	laps      []intervalLap
}

// ---------- Enrich ----------

func (p *Intervals) Enrich(
	ctx context.Context,
	logger *slog.Logger,
	activity *pbactivity.StandardizedActivity,
	user *user.Record,
	inputs map[string]string,
	doNotRetry bool,
) (*providers.EnrichmentResult, error) {
	logger.Debug("intervals: starting", "activity_name", activity.Name)

	// Config options
	showAllIntervals := inputs["show_all_intervals"] == "true"
	showProgression := inputs["show_progression"] != "false" // default true
	showSummary := inputs["show_summary"] != "false"         // default true

	// Collect interval laps from all sessions
	laps := collectIntervalLaps(activity)
	if len(laps) == 0 {
		logger.Info("No interval data found")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"intervals_status": "skipped",
				"status_detail":    "No interval data found",
			},
		}, nil
	}

	// Require at least 2 distinct intensity types to confirm structured intervals.
	// Auto-split laps (every km/mile) only have "active" intensity on every lap,
	// which duplicates the Splits enricher output.
	intensitySet := make(map[string]bool)
	for _, l := range laps {
		intensitySet[l.intensity] = true
	}
	if len(intensitySet) < 2 {
		logger.Info("Skipping non-structured intervals (single intensity type)", "intensity", laps[0].intensity)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"intervals_status": "skipped",
				"status_detail":    "Non-structured intervals (single intensity type)",
			},
		}, nil
	}

	// Determine workout name
	workoutName := ""
	if activity.Workout != nil && activity.Workout.Name != "" {
		workoutName = activity.Workout.Name
	}

	// Group consecutive same-intensity same-duration laps as repeats
	groups := groupIntervals(laps)

	// Build description - section header is prepended to align with other enrichers
	// (Parkrun, AI Companion) that inline their headers in the description content.
	var sb strings.Builder

	// Active stats accumulators for summary
	var activeDistance, activeDuration float64
	var recoveryDistance, recoveryDuration float64
	var firstActiveGroup, lastActiveGroup *intervalGroup
	var activeGroupSpeeds []float64

	// Generate time markers for graph visualization
	timeMarkers := generateIntervalTimeMarkers(laps)

	for _, g := range groups {
		switch g.intensity {
		case "warmup":
			writeWarmupCooldown(&sb, "🔥 Warmup", g, showAllIntervals)
		case "cooldown":
			writeWarmupCooldown(&sb, "❄️ Cooldown", g, showAllIntervals)
		case "active":
			writeActiveGroup(&sb, g, showAllIntervals)
			avgSpeed := groupAvgSpeed(g)
			activeGroupSpeeds = append(activeGroupSpeeds, avgSpeed)
			if firstActiveGroup == nil {
				firstActiveGroup = &g
			}
			lastActiveGroup = &g
			for _, l := range g.laps {
				activeDistance += l.distance
				activeDuration += l.duration
			}
		case "recovery":
			// Recovery laps are not listed individually in grouped mode
			for _, l := range g.laps {
				recoveryDistance += l.distance
				recoveryDuration += l.duration
			}
		}
	}

	// Summary: active vs recovery
	if showSummary && activeDuration > 0 && recoveryDuration > 0 {
		activeSpeed := activeDistance / activeDuration
		recoverySpeed := recoveryDistance / recoveryDuration
		ratio := activeSpeed / recoverySpeed
		sb.WriteString(fmt.Sprintf("\n📊 Active vs Recovery: %.1f m/s active • %.1f m/s recovery • %.2f× ratio",
			activeSpeed, recoverySpeed, ratio))
	}

	// Progression / fade
	if showProgression && len(activeGroupSpeeds) >= 2 && firstActiveGroup != nil && lastActiveGroup != nil {
		firstPace := speedToPace(activeGroupSpeeds[0])
		lastPace := speedToPace(activeGroupSpeeds[len(activeGroupSpeeds)-1])
		if firstPace > 0 && lastPace > 0 {
			fadePercent := ((lastPace - firstPace) / firstPace) * 100
			if fadePercent > 0 {
				sb.WriteString(fmt.Sprintf("\n📉 Fade: %s → %s/km • +%.0f%% slower",
					formatPace(firstPace), formatPace(lastPace), fadePercent))
			} else if fadePercent < -1 {
				sb.WriteString(fmt.Sprintf("\n📈 Getting faster: %s → %s/km • %.0f%% improvement",
					formatPace(firstPace), formatPace(lastPace), -fadePercent))
			}
		}
	}

	// Count total active intervals
	totalActive := 0
	totalRecovery := 0
	for _, g := range groups {
		switch g.intensity {
		case "active":
			totalActive += len(g.laps)
		case "recovery":
			totalRecovery += len(g.laps)
		}
	}

	metadataWorkoutName := workoutName
	if metadataWorkoutName == "" {
		metadataWorkoutName = "Structured Intervals"
	}
	metadata := map[string]string{
		"intervals_status":     "success",
		"intervals_workout":    metadataWorkoutName,
		"intervals_active":     fmt.Sprintf("%d", totalActive),
		"intervals_recovery":   fmt.Sprintf("%d", totalRecovery),
		"intervals_total_laps": fmt.Sprintf("%d", len(laps)),
		"time_markers":         fmt.Sprintf("%d", len(timeMarkers)),
	}

	logger.Info("Intervals enrichment complete",
		"workout", workoutName,
		"active_intervals", totalActive,
		"recovery_intervals", totalRecovery,
		"total_laps", len(laps),
		"time_markers", len(timeMarkers))

	// Prepend section header to description (same pattern as Parkrun and AI Companion)
	sectionHeader := formatSectionHeader(workoutName)
	desc := sectionHeader + "\n" + strings.TrimLeft(sb.String(), "\n")

	// Build interval segments for Enrichments
	intervalSegments := make([]*pbactivity.IntervalSegment, 0, len(laps))
	activeIdx := 0
	recoveryIdx := 0
	lapIdx := 0
	for _, l := range laps {
		var label string
		switch l.intensity {
		case "warmup":
			label = "Warmup"
		case "cooldown":
			label = "Cooldown"
		case "active":
			activeIdx++
			label = fmt.Sprintf("Interval %d", activeIdx)
		case "recovery":
			recoveryIdx++
			label = fmt.Sprintf("Recovery %d", recoveryIdx)
		default:
			lapIdx++
			label = fmt.Sprintf("Lap %d", lapIdx)
		}
		intervalSegments = append(intervalSegments, &pbactivity.IntervalSegment{
			Type:            l.intensity,
			Label:           label,
			DurationSeconds: l.duration,
			DistanceMeters:  l.distance,
			AvgHr:           l.avgHR,
			AvgSpeedMs:      l.avgSpeed,
		})
	}

	return &providers.EnrichmentResult{
		Description:   desc,
		SectionHeader: sectionHeader,
		TimeMarkers:   timeMarkers,
		Metadata:      metadata,
		Enrichments: &pbactivity.ActivityEnrichments{
			Intervals: &pbactivity.IntervalsSummary{
				Segments:    intervalSegments,
				WorkoutName: metadataWorkoutName,
			},
		},
	}, nil
}

// ---------- data collection ----------

func collectIntervalLaps(activity *pbactivity.StandardizedActivity) []intervalLap {
	var laps []intervalLap
	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			if lap.Intensity == "" {
				continue
			}
			il := intervalLap{
				intensity: lap.Intensity,
				duration:  lap.TotalElapsedTime,
				distance:  lap.TotalDistance,
				startTime: lap.StartTime,
			}
			if il.duration > 0 {
				il.avgSpeed = il.distance / il.duration
			}

			// HR from records
			var sumHR float64
			var hrCount int
			var maxHR float64
			for _, r := range lap.Records {
				if r.HeartRate > 0 {
					sumHR += float64(r.HeartRate)
					hrCount++
					if float64(r.HeartRate) > maxHR {
						maxHR = float64(r.HeartRate)
					}
				}
			}
			if hrCount > 0 {
				il.avgHR = sumHR / float64(hrCount)
			}
			il.peakHR = maxHR

			laps = append(laps, il)
		}
	}
	return laps
}

// ---------- grouping ----------

func groupIntervals(laps []intervalLap) []intervalGroup {
	if len(laps) == 0 {
		return nil
	}

	// Two-pass approach:
	// 1. Extract warmup prefix and cooldown suffix
	// 2. Group remaining active laps by similar duration into repeat groups

	var groups []intervalGroup

	// Pass 1: Extract warmup prefix
	start := 0
	if laps[0].intensity == "warmup" {
		warmup := intervalGroup{intensity: "warmup", laps: []intervalLap{laps[0]}}
		// Merge consecutive warmup laps
		for i := 1; i < len(laps) && laps[i].intensity == "warmup"; i++ {
			warmup.laps = append(warmup.laps, laps[i])
			start = i + 1
		}
		if start == 0 {
			start = 1
		}
		groups = append(groups, warmup)
	}

	// Extract cooldown suffix
	end := len(laps)
	var cooldownGroup *intervalGroup
	if end > start && laps[end-1].intensity == "cooldown" {
		cooldownLaps := []intervalLap{laps[end-1]}
		for i := end - 2; i >= start && laps[i].intensity == "cooldown"; i-- {
			cooldownLaps = append([]intervalLap{laps[i]}, cooldownLaps...)
			end = i
		}
		if end == len(laps) {
			end = len(laps) - 1
		}
		cooldownGroup = &intervalGroup{intensity: "cooldown", laps: cooldownLaps}
	}

	// Pass 2: Group the middle section (active + recovery intervals).
	// Active laps with similar durations that are part of the same block
	// (separated only by recovery) get grouped together.
	var activeLaps []intervalLap
	var recoveryLaps []intervalLap

	for i := start; i < end; i++ {
		switch laps[i].intensity {
		case "active":
			activeLaps = append(activeLaps, laps[i])
		case "recovery":
			recoveryLaps = append(recoveryLaps, laps[i])
		default:
			// Other intensity types – treat as active for grouping
			activeLaps = append(activeLaps, laps[i])
		}
	}

	// Group active laps by similar duration
	activeGroups := groupByDuration(activeLaps)
	for _, ag := range activeGroups {
		groups = append(groups, intervalGroup{intensity: "active", laps: ag})
	}

	// Add recovery as a single hidden group (used for summary stats)
	if len(recoveryLaps) > 0 {
		groups = append(groups, intervalGroup{intensity: "recovery", laps: recoveryLaps})
	}

	// Add cooldown at the end
	if cooldownGroup != nil {
		groups = append(groups, *cooldownGroup)
	}

	return groups
}

// groupByDuration groups active laps into clusters of similar duration.
// Laps within ±25% of the first lap in a cluster belong together.
func groupByDuration(laps []intervalLap) [][]intervalLap {
	if len(laps) == 0 {
		return nil
	}
	var result [][]intervalLap
	current := []intervalLap{laps[0]}
	for i := 1; i < len(laps); i++ {
		if similarDuration(current[0].duration, laps[i].duration) {
			current = append(current, laps[i])
		} else {
			result = append(result, current)
			current = []intervalLap{laps[i]}
		}
	}
	result = append(result, current)
	return result
}

// similarDuration returns true if two durations are within 25% of each other.
func similarDuration(a, b float64) bool {
	if a == 0 || b == 0 {
		return a == b
	}
	ratio := a / b
	return ratio >= 0.75 && ratio <= 1.333
}

// ---------- output builders ----------

func writeWarmupCooldown(sb *strings.Builder, emoji string, g intervalGroup, showAll bool) {
	// Merge all laps in the group
	var totalDist, totalDur, sumHR float64
	var hrCount int
	for _, l := range g.laps {
		totalDist += l.distance
		totalDur += l.duration
		if l.avgHR > 0 {
			sumHR += l.avgHR
			hrCount++
		}
	}
	pace := speedToPace(totalDist / totalDur)
	hrStr := ""
	if hrCount > 0 {
		hrStr = fmt.Sprintf(" (%dbpm)", int(sumHR/float64(hrCount)))
	}
	sb.WriteString(fmt.Sprintf("\n%s: %s • %s/km • %dm%s",
		emoji, formatDuration(totalDur), formatPace(pace), int(totalDist), hrStr))
}

func writeActiveGroup(sb *strings.Builder, g intervalGroup, showAll bool) {
	n := len(g.laps)
	durLabel := formatDuration(g.laps[0].duration)

	// Group-level stats
	var sumSpeed, sumDist float64
	var peakHR float64
	for _, l := range g.laps {
		sumSpeed += l.avgSpeed
		sumDist += l.distance
		if l.peakHR > peakHR {
			peakHR = l.peakHR
		}
	}
	avgPace := speedToPace(sumSpeed / float64(n))
	hrStr := ""
	if peakHR > 0 {
		hrStr = fmt.Sprintf(", peak %dbpm", int(peakHR))
	}
	sb.WriteString(fmt.Sprintf("\n💨 %d×%s intervals: avg %s/km%s",
		n, durLabel, formatPace(avgPace), hrStr))

	// Individual intervals when show_all_intervals is on
	if showAll {
		for i, l := range g.laps {
			lapHR := ""
			if l.avgHR > 0 {
				lapHR = fmt.Sprintf(" (%dbpm)", int(l.avgHR))
			}
			lapPace := speedToPace(l.avgSpeed)
			sb.WriteString(fmt.Sprintf("\n  💨 Run %d: %s • %s/km • %dm%s",
				i+1, formatDuration(l.duration), formatPace(lapPace), int(l.distance), lapHR))
		}
	}
}

// ---------- helpers ----------

func groupAvgSpeed(g intervalGroup) float64 {
	if len(g.laps) == 0 {
		return 0
	}
	var sum float64
	for _, l := range g.laps {
		sum += l.avgSpeed
	}
	return sum / float64(len(g.laps))
}

// speedToPace converts m/s to min/km. Returns 0 if speed is 0.
func speedToPace(speed float64) float64 {
	if speed <= 0 {
		return 0
	}
	return (1000.0 / speed) / 60.0
}

func formatPace(paceMinutes float64) string {
	if paceMinutes <= 0 || math.IsInf(paceMinutes, 0) {
		return "0:00"
	}
	minutes := int(paceMinutes)
	seconds := int((paceMinutes - float64(minutes)) * 60)
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func formatDuration(seconds float64) string {
	mins := int(seconds) / 60
	secs := int(seconds) % 60
	if mins > 0 {
		return fmt.Sprintf("%d:%02d", mins, secs)
	}
	return fmt.Sprintf("0:%02d", secs)
}

// formatSectionHeader builds the G40 section header.
// With a workout name: "⏱️ Intervals — 4×6:"
// Without: "⏱️ Intervals:"
func formatSectionHeader(workoutName string) string {
	if workoutName == "" {
		return "⏱️ Intervals:"
	}
	return fmt.Sprintf("⏱️ Intervals — %s:", workoutName)
}

// generateIntervalTimeMarkers creates TimeMarker entries for each interval
// transition (warmup → active → recovery → cooldown) for graph visualization.
func generateIntervalTimeMarkers(laps []intervalLap) []*pbactivity.TimeMarker {
	var markers []*pbactivity.TimeMarker
	activeCount := 0
	recoveryCount := 0

	for _, l := range laps {
		if l.startTime == nil {
			continue
		}

		var label, markerType string
		switch l.intensity {
		case "warmup":
			label = "🔥 Warmup"
			markerType = "warmup_start"
		case "active":
			activeCount++
			label = fmt.Sprintf("💨 Interval %d", activeCount)
			markerType = "interval_start"
		case "recovery":
			recoveryCount++
			label = fmt.Sprintf("😮\u200d💨 Recovery %d", recoveryCount)
			markerType = "recovery_start"
		case "cooldown":
			label = "❄️ Cooldown"
			markerType = "cooldown_start"
		default:
			continue
		}

		markers = append(markers, &pbactivity.TimeMarker{
			Timestamp:  l.startTime,
			Label:      label,
			MarkerType: markerType,
		})
	}

	return markers
}
