// nolint:proto-json
package location_naming

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// nominatimBaseURL is the Nominatim host for ReverseGeocode. Overridable in tests so the HTTP
// path can be exercised against an httptest server instead of the live service.
var nominatimBaseURL = "https://nominatim.openstreetmap.org"

// GeocodeFunc reverse-geocodes coordinates to a place name and city. A nil GeocodeFunc means
// "coordinates only" — the caller wants the raw lat/lng promoted to a location but no network
// lookup (used to keep unit tests and offline paths free of external calls).
type GeocodeFunc func(ctx context.Context, logger *slog.Logger, latitude, longitude float64) (name, city string)

// ResolveLocationSummary derives a LocationSummary for an activity straight from its GPS data,
// independent of whether the (opt-in, title-generating) location_naming enricher is configured
// on the pipeline. The pipeline finalizer calls this so that EVERY GPS-tracked activity carries
// Enrichments.Location — which is what the showcased activity page and the roundup "where it
// happened" section read. This mirrors how the weather enricher already derives conditions
// directly from GPS records: the coordinates were always present, they just were never promoted
// to a displayable location unless location_naming happened to run.
//
// Coordinate resolution order: the first record carrying GPS, else a pinned location hint (set
// by the location-pinner enricher for indoor/GPS-less activities). When a hint carries an
// explicit label — a venue the user searched and picked — it is used verbatim with no network
// call. Otherwise, if geocode is non-nil, the coordinates are reverse-geocoded to a human name;
// the raw coordinates are always retained so the activity carries a map pin even when geocoding
// yields nothing.
//
// Returns nil when the activity has no usable coordinates (a truly GPS-less activity).
func ResolveLocationSummary(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, geocode GeocodeFunc) *pbactivity.LocationSummary {
	if activity == nil {
		return nil
	}

	var latitude, longitude float64
	var hasGPS bool
	var hintLabel string

	// Extract GPS coordinates from the first record that has them (matches the weather
	// and location_naming enrichers' extraction).
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

	// Fall back to a pinned location hint when there is no real GPS track.
	if !hasGPS && activity.HintLocation != nil &&
		(activity.HintLocation.Latitude != 0 || activity.HintLocation.Longitude != 0) {
		latitude = activity.HintLocation.Latitude
		longitude = activity.HintLocation.Longitude
		hasGPS = true
		hintLabel = activity.HintLocation.LocationName
	}

	if !hasGPS {
		return nil
	}

	loc := &pbactivity.LocationSummary{
		Latitude:  latitude,
		Longitude: longitude,
	}

	// A pinned hint label is the venue the user already named — use it directly, no geocoding.
	if hintLabel != "" {
		loc.LocationName = hintLabel
		return loc
	}

	if geocode == nil {
		return loc // coordinates only
	}

	locationName, cityName := geocode(ctx, logger, latitude, longitude)
	displayLocation := locationName
	if displayLocation == "" {
		displayLocation = cityName
	}
	loc.LocationName = displayLocation
	// Country mirrors the location_naming enricher's projection (it stores the city here);
	// keeping parity means both code paths roll up identically into roundup "places".
	loc.Country = cityName
	return loc
}

// ReverseGeocode resolves a place name and city for coordinates using Nominatim, backed by the
// package-level in-memory cache and 1-req/sec rate limiter shared with the enricher's Enrich.
//
// Unlike the enricher's own geocoding, this never returns an error: the pipeline finalizer that
// calls it (via ResolveLocationSummary) must not fail or retry the whole pipeline just because a
// best-effort location lookup was unavailable. On any failure it returns empty strings and the
// caller keeps the raw coordinates. It satisfies GeocodeFunc.
func ReverseGeocode(ctx context.Context, logger *slog.Logger, latitude, longitude float64) (locationName, cityName string) {
	// Cache first — key rounded to ~11m so repeated activities at the same place hit cache
	// and avoid hammering Nominatim (same key format as Enrich).
	cacheKey := fmt.Sprintf("%.4f,%.4f", latitude, longitude)
	locationCacheMutex.RLock()
	cached, ok := locationCache[cacheKey]
	locationCacheMutex.RUnlock()
	if ok {
		parts := strings.SplitN(cached, "|", 2)
		locationName = parts[0]
		if len(parts) > 1 {
			cityName = parts[1]
		}
		return locationName, cityName
	}

	// Rate limiting: ensure at least 1 second between requests (Nominatim usage policy).
	rateLimitMutex.Lock()
	elapsed := time.Since(lastRequestTime)
	if elapsed < time.Second {
		time.Sleep(time.Second - elapsed)
	}
	lastRequestTime = time.Now()
	rateLimitMutex.Unlock()

	url := fmt.Sprintf(
		"%s/reverse?lat=%.6f&lon=%.6f&format=json&zoom=16",
		nominatimBaseURL, latitude, longitude,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		logger.Warn("Auto-location: failed to build Nominatim request", "error", err)
		return "", ""
	}
	req.Header.Set("User-Agent", "FitGlue/1.0") // required by Nominatim usage policy

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("Auto-location: Nominatim request failed", "error", err)
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Warn("Auto-location: Nominatim returned non-200", "status", resp.StatusCode, "body", string(body))
		return "", ""
	}

	var nominatimResp NominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&nominatimResp); err != nil {
		logger.Warn("Auto-location: failed to decode Nominatim response", "error", err)
		return "", ""
	}

	locationName = getLocationName(nominatimResp.Address)
	cityName = getCityName(nominatimResp.Address)

	// Cache the resolved value (same "location|city" format as Enrich so the caches are shared).
	locationCacheMutex.Lock()
	locationCache[cacheKey] = locationName + "|" + cityName
	locationCacheMutex.Unlock()

	logger.Info("Auto-location resolved from GPS", "location", locationName, "city", cityName)
	return locationName, cityName
}
