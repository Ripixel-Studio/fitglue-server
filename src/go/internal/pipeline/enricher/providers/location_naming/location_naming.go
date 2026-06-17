// nolint:proto-json
package location_naming

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// Rate limiting: Nominatim requires max 1 request per second
var (
	lastRequestTime time.Time
	rateLimitMutex  sync.Mutex
	// Simple in-memory cache for location lookups (lat/lon key -> location name)
	locationCache      = make(map[string]string)
	locationCacheMutex sync.RWMutex
)

type LocationNaming struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewLocationNaming())
}

func NewLocationNaming() *LocationNaming {
	return &LocationNaming{}
}

func (p *LocationNaming) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *LocationNaming) Name() string {
	return "location_naming"
}

func (p *LocationNaming) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_LOCATION_NAMING
}

func (p *LocationNaming) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	// Extract GPS coordinates from first record
	var latitude, longitude float64
	var hasGPS bool
	var hintLabel string

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.PositionLat != 0 && record.PositionLong != 0 {
					latitude = record.PositionLat
					longitude = record.PositionLong
					hasGPS = true
					break
				}
			}
			if hasGPS {
				break
			}
		}
		if hasGPS {
			break
		}
	}

	// Fall back to a pinned location hint (set by the location-pinner enricher) when the
	// activity has no real GPS. If the hint carries a label (the place the user searched
	// and selected), use it directly so the resolved name is the venue rather than a
	// reverse-geocoded suburb — this is also what flows into roundup "places".
	if !hasGPS && activity.HintLocation != nil &&
		(activity.HintLocation.Latitude != 0 || activity.HintLocation.Longitude != 0) {
		latitude = activity.HintLocation.Latitude
		longitude = activity.HintLocation.Longitude
		hasGPS = true
		hintLabel = activity.HintLocation.LocationName
		logger.Info("Using pinned location hint for location naming",
			"latitude", latitude,
			"longitude", longitude,
			"label", hintLabel,
		)
	}

	if !hasGPS {
		logger.Info("No GPS data found for location naming enricher, skipping")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "No GPS data found",
			Metadata: map[string]string{
				"location_naming_status": "skipped",
				"status_detail":          "No GPS data found",
			},
		}, nil
	}

	var locationName, cityName string

	// If a pinned hint provided an explicit label, use it verbatim and skip geocoding —
	// the user already told us the place name (e.g. "Code Fitness, Newark").
	if hintLabel != "" {
		locationName = hintLabel
		logger.Info("Using pinned hint label as location name", "location", locationName)
	} else {
		// Try to get location from cache first
		cacheKey := fmt.Sprintf("%.4f,%.4f", latitude, longitude)
		locationCacheMutex.RLock()
		cachedLocation, cached := locationCache[cacheKey]
		locationCacheMutex.RUnlock()

		if cached {
			logger.Info("Using cached location", "key", cacheKey, "location", cachedLocation)
			// Parse cached value (format: "location|city")
			parts := strings.SplitN(cachedLocation, "|", 2)
			locationName = parts[0]
			if len(parts) > 1 {
				cityName = parts[1]
			}
		} else {
			// Rate limiting: ensure at least 1 second between requests
			rateLimitMutex.Lock()
			elapsed := time.Since(lastRequestTime)
			if elapsed < time.Second {
				time.Sleep(time.Second - elapsed)
			}
			lastRequestTime = time.Now()
			rateLimitMutex.Unlock()

			// Call Nominatim reverse geocode API
			url := fmt.Sprintf(
				"https://nominatim.openstreetmap.org/reverse?lat=%.6f&lon=%.6f&format=json&zoom=16",
				latitude, longitude,
			)

			logger.Info("Fetching location data from Nominatim",
				"latitude", latitude,
				"longitude", longitude,
				"url", url,
			)

			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				logger.Error("Failed to create Nominatim request", "error", err)
				return nil, &providers.RetryableError{Err: fmt.Errorf("failed to create request: %w", err)}
			}

			// Required by Nominatim usage policy
			req.Header.Set("User-Agent", "FitGlue/1.0")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				logger.Error("Failed to fetch location data", "error", err)
				return nil, &providers.RetryableError{Err: fmt.Errorf("nominatim API request failed: %w", err)}
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				logger.Error("Nominatim API returned non-200 status", "status", resp.StatusCode, "body", string(body))
				return nil, &providers.RetryableError{Err: fmt.Errorf("nominatim API returned status %d", resp.StatusCode)}
			}

			// Parse response
			var nominatimResp NominatimResponse
			if err := json.NewDecoder(resp.Body).Decode(&nominatimResp); err != nil {
				logger.Error("Failed to decode nominatim response", "error", err)
				return &providers.EnrichmentResult{
					Skipped:    true,
					SkipReason: "Failed to parse location response",
					Metadata: map[string]string{
						"location_naming_status": "skipped",
						"status_detail":          "Failed to parse location response",
					},
				}, nil
			}

			// Extract location name with priority: park/leisure > suburb > city
			locationName = getLocationName(nominatimResp.Address)
			cityName = getCityName(nominatimResp.Address)

			// Cache the result
			cacheValue := locationName + "|" + cityName
			locationCacheMutex.Lock()
			locationCache[cacheKey] = cacheValue
			locationCacheMutex.Unlock()

			logger.Info("Location resolved",
				"location", locationName,
				"city", cityName,
			)
		}
	}

	// Get config options
	mode := inputs["mode"]
	if mode == "" {
		mode = "title" // Default to title mode
	}
	titleTemplate := inputs["title_template"]
	if titleTemplate == "" {
		titleTemplate = "{activity_type} in {location}"
	}
	fallbackEnabled := inputs["fallback_enabled"] != "false" // Default to true
	timeContext := inputs["time_context"]                    // none (default), solar

	// If no specific location found and fallback is disabled, skip
	if locationName == "" && !fallbackEnabled {
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "No specific location found and fallback disabled",
			Metadata: map[string]string{
				"location_naming_status": "skipped",
				"status_detail":          "No specific location found and fallback disabled",
			},
		}, nil
	}

	// Use city as fallback if no specific location
	displayLocation := locationName
	if displayLocation == "" && fallbackEnabled {
		displayLocation = cityName
	}
	if displayLocation == "" {
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "Could not determine location name",
			Metadata: map[string]string{
				"location_naming_status": "skipped",
				"status_detail":          "Could not determine location name",
			},
		}, nil
	}

	// Calculate time context if enabled
	var timePrefix string
	var timeEmoji string
	if timeContext == "solar" && activity.StartTime != nil {
		activityTime := activity.StartTime.AsTime()
		sunrise, sunset := calculateSunriseSunset(activityTime, latitude, longitude)
		timePrefix, timeEmoji = getSolarTimeContext(activityTime, sunrise, sunset)
		logger.Info("Solar time calculated",
			"activity_time", activityTime,
			"sunrise", sunrise,
			"sunset", sunset,
			"time_prefix", timePrefix,
		)
	}

	result := &providers.EnrichmentResult{
		Metadata: map[string]string{
			"location_naming_status": "success",
			"location_name":          displayLocation,
			"city":                   cityName,
		},
	}

	result.Enrichments = &pbactivity.ActivityEnrichments{
		Location: &pbactivity.LocationSummary{
			LocationName: displayLocation,
			Country:      cityName,
			Latitude:     latitude,
			Longitude:    longitude,
		},
	}

	if timePrefix != "" {
		result.Metadata["time_context"] = timePrefix
	}

	// Build the activity type string for template
	activityType := getActivityTypeStr(activity.Type)

	switch mode {
	case "title":
		// Generate title using template
		newTitle := titleTemplate
		if timePrefix != "" {
			// Prepend time context: "Sunrise Run in Hyde Park"
			newTitle = strings.ReplaceAll(newTitle, "{activity_type}", timePrefix+" "+activityType)
		} else {
			newTitle = strings.ReplaceAll(newTitle, "{activity_type}", activityType)
		}
		newTitle = strings.ReplaceAll(newTitle, "{location}", displayLocation)
		result.Name = newTitle
		logger.Info("Generated location-based title", "title", newTitle)

	case "description":
		// Append location line to description
		var locationLine string
		locationLine = fmt.Sprintf("📍 Location: %s", displayLocation)
		if cityName != "" && cityName != displayLocation {
			locationLine = fmt.Sprintf("%s, %s", locationLine, cityName)
		}
		if timeEmoji != "" && timePrefix != "" {
			locationLine = fmt.Sprintf("%s (%s %s)", locationLine, timeEmoji, timePrefix)
		}
		result.Description = locationLine
		logger.Info("Generated location description", "location_line", locationLine)
	}

	return result, nil
}

// NominatimResponse represents the Nominatim reverse geocode response
type NominatimResponse struct {
	Address NominatimAddress `json:"address"`
}

type NominatimAddress struct {
	Leisure string `json:"leisure"`
	Park    string `json:"park"`
	Suburb  string `json:"suburb"`
	City    string `json:"city"`
	Town    string `json:"town"`
	Village string `json:"village"`
	County  string `json:"county"`
	State   string `json:"state"`
	Country string `json:"country"`
}

// getLocationName returns the most specific location name available
// Priority: park > leisure > suburb
func getLocationName(addr NominatimAddress) string {
	if addr.Park != "" {
		return addr.Park
	}
	if addr.Leisure != "" {
		return addr.Leisure
	}
	if addr.Suburb != "" {
		return addr.Suburb
	}
	return ""
}

// getCityName returns the city/town/village name
func getCityName(addr NominatimAddress) string {
	if addr.City != "" {
		return addr.City
	}
	if addr.Town != "" {
		return addr.Town
	}
	if addr.Village != "" {
		return addr.Village
	}
	return ""
}

// getActivityTypeStr returns a human-readable activity type string
func getActivityTypeStr(activityType pbactivity.ActivityType) string {
	// Convert enum to user-friendly string
	switch activityType {
	case pbactivity.ActivityType_ACTIVITY_TYPE_RUN:
		return "Run"
	case pbactivity.ActivityType_ACTIVITY_TYPE_RIDE:
		return "Ride"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WALK:
		return "Walk"
	case pbactivity.ActivityType_ACTIVITY_TYPE_HIKE:
		return "Hike"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SWIM:
		return "Swim"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING:
		return "Workout"
	case pbactivity.ActivityType_ACTIVITY_TYPE_YOGA:
		return "Yoga"
	case pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE:
		return "Virtual Ride"
	case pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN:
		return "Virtual Run"
	default:
		return "Activity"
	}
}

// calculateSunriseSunset calculates approximate sunrise and sunset times for a given date and location
// Uses a simplified solar position algorithm
func calculateSunriseSunset(date time.Time, lat, lon float64) (sunrise, sunset time.Time) {
	// Day of year
	dayOfYear := float64(date.YearDay())

	// Solar declination (simplified)
	declination := 23.45 * math.Sin(2*math.Pi*(284+dayOfYear)/365)
	declinationRad := declination * math.Pi / 180

	// Latitude in radians
	latRad := lat * math.Pi / 180

	// Hour angle at sunrise/sunset
	cosHourAngle := -math.Tan(latRad) * math.Tan(declinationRad)

	// Clamp to valid range (for extreme latitudes)
	if cosHourAngle < -1 {
		cosHourAngle = -1 // Midnight sun
	} else if cosHourAngle > 1 {
		cosHourAngle = 1 // Polar night
	}

	hourAngle := math.Acos(cosHourAngle) * 180 / math.Pi

	// Solar noon (in hours UTC)
	// Equation of time approximation
	B := 2 * math.Pi * (dayOfYear - 81) / 365
	eot := 9.87*math.Sin(2*B) - 7.53*math.Cos(B) - 1.5*math.Sin(B) // minutes

	solarNoon := 12 - lon/15 - eot/60 // hours UTC

	// Sunrise and sunset in hours
	sunriseHours := solarNoon - hourAngle/15
	sunsetHours := solarNoon + hourAngle/15

	// Convert to time.Time
	year, month, day := date.Date()
	loc := date.Location()

	sunrise = time.Date(year, month, day, 0, 0, 0, 0, loc).Add(time.Duration(sunriseHours * float64(time.Hour)))
	sunset = time.Date(year, month, day, 0, 0, 0, 0, loc).Add(time.Duration(sunsetHours * float64(time.Hour)))

	return sunrise, sunset
}

// getSolarTimeContext returns time-of-day context based on solar position
func getSolarTimeContext(activityTime, sunrise, sunset time.Time) (prefix, emoji string) {
	// Define time windows relative to sunrise/sunset
	goldenHourDuration := 45 * time.Minute

	// Before sunrise
	if activityTime.Before(sunrise.Add(-goldenHourDuration)) {
		return "Night", "🌙"
	}

	// Dawn/Sunrise window (45 min before to 45 min after sunrise)
	if activityTime.Before(sunrise.Add(goldenHourDuration)) {
		return "Sunrise", "🌅"
	}

	// Morning (after sunrise golden hour until solar noon-ish)
	midday := sunrise.Add(sunset.Sub(sunrise) / 2)
	if activityTime.Before(midday) {
		return "Morning", "☀️"
	}

	// Afternoon (solar noon to sunset golden hour)
	if activityTime.Before(sunset.Add(-goldenHourDuration)) {
		return "Afternoon", "☀️"
	}

	// Sunset/Dusk window (45 min before to 45 min after sunset)
	if activityTime.Before(sunset.Add(goldenHourDuration)) {
		return "Sunset", "🌄"
	}

	// After sunset
	return "Night", "🌙"
}
