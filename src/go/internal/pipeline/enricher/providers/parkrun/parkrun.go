package parkrun

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	_ "time/tzdata" // embed timezone database for distroless containers

	"github.com/fitglue/server/src/go/pkg/domain/user"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	parkrunutil "github.com/fitglue/server/src/go/pkg/parkrun"

	pendinginput "github.com/fitglue/server/src/go/pkg/pending_input"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Shared location service instance - initialized at startup
var locationService *ParkrunLocationsService

func init() {
	locationService = NewParkrunLocationsService()
	providers.Register(NewParkrunProvider())
}

// ParkrunProvider detects if an activity is a Parkrun event.
type ParkrunProvider struct {
	locationService *ParkrunLocationsService
	service         *bootstrap.Service
}

func NewParkrunProvider() *ParkrunProvider {
	return &ParkrunProvider{
		locationService: locationService,
	}
}

// NewParkrunProviderWithService creates a provider with a custom location service (for testing).
func NewParkrunProviderWithService(svc *ParkrunLocationsService) *ParkrunProvider {
	return &ParkrunProvider{
		locationService: svc,
	}
}

// SetService injects the bootstrap service for database access (resume mode support)
func (p *ParkrunProvider) SetService(s *bootstrap.Service) {
	p.service = s
}

func (p *ParkrunProvider) Name() string {
	return "parkrun"
}

func (p *ParkrunProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_PARKRUN
}

// EnrichResume is called during resume mode to apply resolved pending input data
func (p *ParkrunProvider) EnrichResume(ctx context.Context, activity *pbactivity.StandardizedActivity, user *user.Record, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	// Extract resolved data from the pending input
	description := pendingInput.InputData["description"]
	position := pendingInput.InputData["position"]
	timeStr := pendingInput.InputData["time"]
	ageGrade := pendingInput.InputData["age_grade"]

	result := &providers.EnrichmentResult{
		Description:   description, // description already contains the header from FormatResultsDescription
		SectionHeader: "🏃 Parkrun Results:",
		Metadata: map[string]string{
			"status":                "success",
			"is_parkrun":            "true",
			"results_applied":       "true",
			"parkrun_position":      position,
			"parkrun_time":          timeStr,
			"parkrun_age_grade":     ageGrade,
			"parkrun_results_state": "COMPLETE",
		},
	}

	return result, nil
}

func (p *ParkrunProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	hasParkrunIntegration := user != nil && user.Integrations != nil && user.Integrations.Parkrun != nil && user.Integrations.Parkrun.Enabled
	logger.Debug("parkrun: starting",
		"activity_type", activity.Type.String(),
		"activity_name", activity.Name,
		"start_time", activity.StartTime.AsTime().Format(time.RFC3339),
		"has_parkrun_integration", hasParkrunIntegration,
	)

	// 0. Ensure locations are loaded
	if err := p.locationService.EnsureLoaded(ctx); err != nil {
		logger.Debug("parkrun: failed to load locations",
			"error", err.Error(),
			"do_not_retry", doNotRetry,
		)
		if doNotRetry {
			return &providers.EnrichmentResult{
				Metadata: map[string]string{
					"status":       "skipped",
					"reason":       "location_data_unavailable",
					"error_detail": err.Error(),
				},
			}, nil
		}
		return nil, providers.NewRetryableError(err, 5*time.Minute, "failed to load Parkrun locations")
	}

	logger.Debug("parkrun: locations loaded successfully")

	// 1. Parse Inputs
	enableTitling := inputs["enable_titling"] != "false" // Default true
	titlePattern := inputs["title_pattern"]
	if titlePattern == "" {
		titlePattern = "{location}"
	}
	specialTitlePattern := inputs["special_title_pattern"]
	if specialTitlePattern == "" {
		specialTitlePattern = "{location} - {special} Edition"
	}
	tagValueStr := inputs["tags"]
	if _, ok := inputs["tags"]; !ok {
		tagValueStr = "Parkrun"
	}

	// 2. Basic Checks - Only care about Runs
	if activity.Type != pbactivity.ActivityType_ACTIVITY_TYPE_RUN &&
		activity.Type != pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN &&
		activity.Type != pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN {
		logger.Debug("parkrun: skipping - not a run activity",
			"activity_type", activity.Type.String(),
		)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"status": "skipped", "reason": "not_run_activity_type", "activity_type": activity.Type.String()},
		}, nil
	}

	// 3. Location Check
	lat, long, found := getStartLocation(activity)
	if !found {
		logger.Debug("parkrun: skipping - no GPS data")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"status": "skipped", "reason": "no_gps_data"},
		}, nil
	}

	logger.Debug("parkrun: found GPS coordinates",
		"latitude", lat,
		"longitude", long,
	)

	// 4. Time Check
	startTime := activity.StartTime.AsTime()
	if startTime.IsZero() {
		logger.Debug("parkrun: error - zero start time")
		return nil, fmt.Errorf("invalid start time: zero")
	}

	// 5. Find nearest Parkrun location (5km search for debug visibility)
	matchedLocation := p.locationService.FindNearest(lat, long, 5000.0)

	if matchedLocation == nil {
		logger.Debug("parkrun: skipping - no parkrun within 5km",
			"latitude", lat,
			"longitude", long,
		)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":        "skipped",
				"reason":        "no_parkrun_within_5km",
				"activity_lat":  fmt.Sprintf("%f", lat),
				"activity_long": fmt.Sprintf("%f", long),
			},
		}, nil
	}

	dist := distanceMeters(lat, long, matchedLocation.Latitude, matchedLocation.Longitude)
	logger.Debug("parkrun: found nearest location",
		"parkrun_name", matchedLocation.Name,
		"parkrun_slug", matchedLocation.EventSlug,
		"distance_meters", dist,
	)

	if dist > 1500.0 {
		logger.Debug("parkrun: skipping - too far from parkrun (>1500m)",
			"distance_meters", dist,
		)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status":          "skipped",
				"reason":          "not_near_parkrun",
				"nearest_parkrun": matchedLocation.Name,
				"distance_meters": fmt.Sprintf("%.0f", dist),
				"activity_lat":    fmt.Sprintf("%f", lat),
				"activity_long":   fmt.Sprintf("%f", long),
			},
		}, nil
	}

	// 6. Determine local time using IANA timezone (handles DST correctly)
	estimatedLocalTime := localTimeForLocation(startTime, *matchedLocation)

	// 7. Check for Parkrun day (Saturday or special event)
	isSaturday := estimatedLocalTime.Weekday() == time.Saturday
	specialDay := getSpecialDay(estimatedLocalTime)
	isSpecial := specialDay != ""

	logger.Debug("parkrun: checking day",
		"estimated_local_time", estimatedLocalTime.Format(time.RFC3339),
		"weekday", estimatedLocalTime.Weekday().String(),
		"is_saturday", isSaturday,
		"is_special", isSpecial,
		"special_day", specialDay,
	)

	if !isSaturday && !isSpecial {
		logger.Debug("parkrun: skipping - not parkrun day")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status": "skipped",
				"reason": "not_parkrun_day",
				"day":    estimatedLocalTime.Weekday().String(),
			},
		}, nil
	}

	// 8. Time Window Check (08:45 to 09:15 local time)
	hour := estimatedLocalTime.Hour()
	minute := estimatedLocalTime.Minute()
	totalMinutes := hour*60 + minute

	startWindow := 8*60 + 45 // 08:45
	endWindow := 9*60 + 15   // 09:15

	if totalMinutes < startWindow || totalMinutes > endWindow {
		logger.Debug("parkrun: skipping - outside time window",
			"local_time", fmt.Sprintf("%02d:%02d", hour, minute),
			"window", "08:45-09:15",
		)
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"status": "skipped",
				"reason": "outside_time_window",
				"time":   fmt.Sprintf("%02d:%02d", hour, minute),
			},
		}, nil
	}

	logger.Debug("parkrun: MATCHED - activity is a parkrun!",
		"parkrun_name", matchedLocation.Name,
		"parkrun_slug", matchedLocation.EventSlug,
		"distance_meters", dist,
		"local_time", fmt.Sprintf("%02d:%02d", hour, minute),
	)

	// 9. Match Found! Build result
	result := &providers.EnrichmentResult{
		Metadata: map[string]string{
			"status":             "success",
			"is_parkrun":         "true",
			"parkrun_event":      matchedLocation.Name,
			"parkrun_slug":       matchedLocation.EventSlug,
			"parkrun_country":    matchedLocation.CountryURL,
			"parkrun_is_special": fmt.Sprintf("%v", isSpecial),
		},
	}

	if isSpecial {
		result.Metadata["parkrun_special_day"] = specialDay
	}

	// 10. Apply title using pattern
	if enableTitling {
		result.Name = applyTitlePattern(titlePattern, specialTitlePattern, matchedLocation.Name, estimatedLocalTime, specialDay)
	}

	// 11. Apply tags
	if tagValueStr != "" {
		tags := strings.Split(tagValueStr, ",")
		result.Tags = make([]string, 0, len(tags))
		for _, t := range tags {
			if val := strings.TrimSpace(t); val != "" {
				result.Tags = append(result.Tags, val)
			}
		}
	}

	// 12. Results Enrichment: Attempt immediate fetch, fall back to pending input if not available
	// Default to true unless explicitly set to "false"
	fetchResults := inputs["fetch_results"] != "false"

	if fetchResults {
		// Check if user has Parkrun integration configured
		if user != nil && user.Integrations != nil && user.Integrations.Parkrun != nil && user.Integrations.Parkrun.Enabled {
			integration := user.Integrations.Parkrun

			// Step 1: Attempt IMMEDIATE fetch of results
			logger.Debug("parkrun: attempting immediate results fetch",
				"athlete_id", integration.AthleteId,
				"event_slug", matchedLocation.EventSlug)

			parkrunResults, fetchErr := parkrunutil.FetchResultsForAthlete(
				ctx, logger,
				integration.AthleteId,
				integration.CountryUrl,
				matchedLocation.EventSlug,
				estimatedLocalTime,
			)

			if fetchErr != nil {
				logger.Debug("parkrun: immediate fetch failed, will poll later",
					"error", fetchErr.Error())
				result.Metadata["immediate_fetch_error"] = fetchErr.Error()
			}

			if parkrunResults != nil {
				// SUCCESS: Results are immediately available!
				logger.Info("parkrun: immediate results found!",
					"position", parkrunResults.Position,
					"time", parkrunResults.Time,
					"age_grade", parkrunResults.AgeGrade)

				desc := parkrunutil.FormatResultsDescription(parkrunResults, matchedLocation.Name)
				result.Description = desc
				result.SectionHeader = "🏃 Parkrun Results:"

				result.Metadata["parkrun_results_state"] = "IMMEDIATE"
				result.Metadata["parkrun_position"] = fmt.Sprintf("%d", parkrunResults.Position)
				result.Metadata["parkrun_time"] = parkrunResults.Time
				result.Metadata["parkrun_age_grade"] = parkrunResults.AgeGrade
				result.Metadata["results_source"] = "immediate_fetch"
			} else if p.service != nil {
				// Step 2: Results not available yet - create pending input for background polling
				logger.Debug("parkrun: results not yet available, creating pending input")

				// stableID is used as the pending input document ID (unique per source activity + enricher)
				stableID := pendinginput.GenerateID(activity.Source.String(), activity.ExternalId, p.Name())

				// Use the pre-generated activity_id from orchestrator for LinkedActivityId
				// This is the UUID that will be used for the synchronized activity
				linkedActivityId := inputs["activity_id"]
				if linkedActivityId == "" {
					// activity_id must be provided by orchestrator - this is a bug if missing
					logger.Error("parkrun: activity_id not in inputs - orchestrator bug")
					return nil, fmt.Errorf("activity_id not provided in enricher inputs")
				}

				// Calculate auto deadline (48 hours from now - Parkrun results usually come within 24h)
				autoDeadline := time.Now().Add(48 * time.Hour)

				pendingInput := &pbpipeline.PendingInput{
					ActivityId:                 stableID, // Document ID stays as stableID for uniqueness
					UserId:                     user.UserId,
					Status:                     pbpipeline.PendingInput_STATUS_WAITING,
					RequiredFields:             []string{"description", "position", "time", "age_grade"},
					AutoPopulated:              true,
					ContinuedWithoutResolution: true,
					EnricherProviderId:         "parkrun",
					AutoDeadline:               timestamppb.New(autoDeadline),
					LinkedActivityId:           linkedActivityId,      // Now uses the correct UUID!
					PipelineId:                 inputs["pipeline_id"], // For resume mode
					// OriginalPayload is now stored in GCS via original_payload_uri (set by orchestrator)
					ProviderMetadata: map[string]string{
						"parkrun_event_slug":   matchedLocation.EventSlug,
						"parkrun_event_name":   matchedLocation.Name,
						"parkrun_country":      matchedLocation.CountryURL,
						"expected_date":        estimatedLocalTime.Format("02/01/2006"),
						"source_activity_id":   activity.ExternalId,
						"source_activity_type": activity.Source.String(),
						"display.field_labels": `{"description":"Results Summary","position":"Finish Position","time":"Finish Time","age_grade":"Age Grade %"}`,
						"display.field_types":  `{"description":"textarea:rows=3","position":"text:placeholder=e.g. 42","time":"text:placeholder=e.g. 25:30","age_grade":"text:placeholder=e.g. 55.5%"}`,
						"display.summary":      "Waiting for Parkrun results",
						"display.title":        "Enter Parkrun Results",
					},
					CreatedAt: timestamppb.Now(),
					UpdatedAt: timestamppb.Now(),
				}

				if err := p.service.DB.CreatePendingInput(ctx, user.UserId, pendingInput); err != nil {
					// Log but don't fail - we can still continue without results enrichment
					logger.Warn("parkrun: failed to create pending input", "error", err)
					result.Metadata["results_pending_input_error"] = err.Error()
				} else {
					result.Metadata["results_pending_input_created"] = "true"
					result.Metadata["results_auto_deadline"] = autoDeadline.Format(time.RFC3339)
				}

				// Add placeholder description for destinations while waiting for official results
				result.Description = "🏃 Parkrun Results:\nWaiting for results to be released..."
				result.SectionHeader = "🏃 Parkrun Results:"

				result.Metadata["parkrun_results_state"] = "PENDING"
			} else {
				// No service available for pending input creation, skip results enrichment
				logger.Debug("parkrun: results not available and no service for pending input")
				result.Metadata["parkrun_results_state"] = "UNAVAILABLE"
			}
		} else {
			result.Metadata["parkrun_results_state"] = "DISABLED"
			result.Metadata["results_enrichment_skipped"] = "no_parkrun_integration"
			// Debug: Show what's missing
			if user == nil {
				result.Metadata["debug_user_nil"] = "true"
			} else if user.Integrations == nil {
				result.Metadata["debug_integration_nil"] = "true"
			} else if user.Integrations.Parkrun == nil {
				result.Metadata["debug_parkrun_nil"] = "true"
			} else {
				result.Metadata["debug_parkrun_enabled"] = fmt.Sprintf("%v", user.Integrations.Parkrun.Enabled)
			}
		}
	} else {
		// Results enrichment not enabled - mark as immediate (no pending results)
		result.Metadata["parkrun_results_state"] = "IMMEDIATE"
	}

	return result, nil
}

// localTimeForLocation converts a UTC time to the local time for a parkrun location.
// Uses IANA timezone names for DST-correct conversion; falls back to longitude estimation.
func localTimeForLocation(utc time.Time, loc ParkrunLocation) time.Time {
	if tzName := resolveTimezone(loc); tzName != "" {
		if tzLoc, err := time.LoadLocation(tzName); err == nil {
			return utc.In(tzLoc)
		}
	}
	// Fallback: rough longitude-based offset (no DST awareness)
	offsetHours := loc.Longitude / 15.0
	return utc.Add(time.Duration(offsetHours * float64(time.Hour)))
}

// resolveTimezone returns the IANA timezone name for a parkrun location.
// Returns "" for unknown countries, triggering longitude-based fallback.
func resolveTimezone(loc ParkrunLocation) string {
	switch loc.CountryURL {
	case "www.parkrun.org.uk":
		return "Europe/London"
	case "www.parkrun.ie":
		return "Europe/Dublin"
	case "www.parkrun.de":
		return "Europe/Berlin"
	case "www.parkrun.dk":
		return "Europe/Copenhagen"
	case "www.parkrun.fi":
		return "Europe/Helsinki"
	case "www.parkrun.fr":
		return "Europe/Paris"
	case "www.parkrun.it":
		return "Europe/Rome"
	case "www.parkrun.nl":
		return "Europe/Amsterdam"
	case "www.parkrun.no":
		return "Europe/Oslo"
	case "www.parkrun.pl":
		return "Europe/Warsaw"
	case "www.parkrun.se":
		return "Europe/Stockholm"
	case "www.parkrun.jp":
		return "Asia/Tokyo"
	case "www.parkrun.sg":
		return "Asia/Singapore"
	case "www.parkrun.my":
		return "Asia/Kuala_Lumpur"
	case "www.parkrun.co.nz":
		return "Pacific/Auckland"
	case "www.parkrun.co.za":
		return "Africa/Johannesburg"
	case "www.parkrun.com.au":
		// Australia spans three main zones; use longitude to pick the right one
		switch {
		case loc.Longitude < 129:
			return "Australia/Perth"
		case loc.Longitude < 138:
			return "Australia/Darwin"
		case loc.Longitude < 141:
			return "Australia/Adelaide"
		default:
			return "Australia/Sydney"
		}
	case "www.parkrun.ca":
		switch {
		case loc.Longitude > -60:
			return "America/Halifax"
		case loc.Longitude > -75:
			return "America/Toronto"
		case loc.Longitude > -90:
			return "America/Winnipeg"
		case loc.Longitude > -110:
			return "America/Denver"
		default:
			return "America/Vancouver"
		}
	case "www.parkrun.us":
		switch {
		case loc.Longitude > -75:
			return "America/New_York"
		case loc.Longitude > -90:
			return "America/Chicago"
		case loc.Longitude > -115:
			return "America/Denver"
		default:
			return "America/Los_Angeles"
		}
	}
	return ""
}

// getSpecialDay returns the special day name if applicable, or empty string.
func getSpecialDay(t time.Time) string {
	month := t.Month()
	day := t.Day()

	if month == time.December && day == 25 {
		return "Christmas Day"
	}
	if month == time.January && day == 1 {
		return "New Year's Day"
	}
	return ""
}

// applyTitlePattern applies the appropriate title pattern.
func applyTitlePattern(normalPattern, specialPattern, location string, t time.Time, specialDay string) string {
	pattern := normalPattern
	if specialDay != "" {
		pattern = specialPattern
	}

	dateFormatted := t.Format("2 Jan 2006")

	result := pattern
	result = strings.ReplaceAll(result, "{location}", location)
	result = strings.ReplaceAll(result, "{date}", dateFormatted)
	result = strings.ReplaceAll(result, "{special}", specialDay)

	return result
}

func getStartLocation(activity *pbactivity.StandardizedActivity) (float64, float64, bool) {
	if len(activity.Sessions) == 0 {
		return 0, 0, false
	}
	for _, session := range activity.Sessions {
		if len(session.Laps) == 0 {
			continue
		}
		for _, lap := range session.Laps {
			if len(lap.Records) == 0 {
				continue
			}
			for _, rec := range lap.Records {
				if rec.PositionLat != 0 || rec.PositionLong != 0 {
					return rec.PositionLat, rec.PositionLong, true
				}
			}
		}
	}
	return 0, 0, false
}

// Haversine formula for distance calculation
func distanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	deltaPhi := (lat2 - lat1) * math.Pi / 180
	deltaLambda := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*
			math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}
