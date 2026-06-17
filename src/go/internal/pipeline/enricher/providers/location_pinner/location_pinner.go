// nolint:proto-json
package location_pinner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/fitglue/server/src/go/pkg/domain/user"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// LocationPinnerProvider maps activities to a fixed real-world location based on a
// title substring match. It is intended for indoor/GPS-less activities (gym classes,
// home workouts) that always happen at the same place.
//
// On a match it attaches the location to the transient StandardizedActivity.HintLocation
// field (NOT to records), which the weather and location-naming enrichers consume as a
// GPS fallback. Because nothing is written to records, the location never ends up in the
// generated FIT file and therefore never produces a GPS track/map on destinations.
type LocationPinnerProvider struct{}

func init() {
	providers.Register(NewLocationPinnerProvider())
}

func NewLocationPinnerProvider() *LocationPinnerProvider {
	return &LocationPinnerProvider{}
}

func (p *LocationPinnerProvider) Name() string {
	return "location-pinner"
}

func (p *LocationPinnerProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_LOCATION_PINNER
}

// pinnedLocation holds a parsed location rule value.
type pinnedLocation struct {
	lat   float64
	lng   float64
	label string
}

// parseLocationValue parses a config map value of the form "<lat>|<lng>|<label>".
// The label is optional and may itself contain "|" characters (rejoined). Returns
// ok=false if lat/lng are missing or not valid finite coordinates.
func parseLocationValue(value string) (pinnedLocation, bool) {
	parts := strings.Split(value, "|")
	if len(parts) < 2 {
		return pinnedLocation{}, false
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return pinnedLocation{}, false
	}
	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return pinnedLocation{}, false
	}

	// Reject out-of-range / zero-zero (null-island) coordinates.
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return pinnedLocation{}, false
	}
	if lat == 0 && lng == 0 {
		return pinnedLocation{}, false
	}

	label := ""
	if len(parts) > 2 {
		label = strings.TrimSpace(strings.Join(parts[2:], "|"))
	}

	return pinnedLocation{lat: lat, lng: lng, label: label}, true
}

func skipped(reason string, detail map[string]string) *providers.EnrichmentResult {
	meta := map[string]string{"status": "skipped", "reason": reason}
	for k, v := range detail {
		meta[k] = v
	}
	return &providers.EnrichmentResult{Skipped: true, SkipReason: reason, Metadata: meta}
}

// activityHasRecordGPS reports whether any record already carries GPS coordinates.
// This protects a legitimate GPS track and any location established by an earlier
// enricher (e.g. Parkrun, which only acts when GPS already exists).
func activityHasRecordGPS(activity *pbactivity.StandardizedActivity) bool {
	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.PositionLat != 0 || record.PositionLong != 0 {
					return true
				}
			}
		}
	}
	return false
}

func (p *LocationPinnerProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	force := inputConfig["force"] == "true"

	logger.Debug("location_pinner: starting",
		"activity_title", activity.Name,
		"force", force,
		"has_rules", inputConfig["location_rules"] != "",
	)

	// 1. Validation: need at least one session for the activity to be meaningful.
	if len(activity.Sessions) == 0 {
		logger.Debug("location_pinner: skipping - no sessions")
		return skipped("no_sessions", nil), nil
	}

	// 2. Don't override a real GPS track (or a location set by an earlier enricher) unless forced.
	if !force && activityHasRecordGPS(activity) {
		logger.Debug("location_pinner: skipping - GPS already present")
		return skipped("gps_already_exists", map[string]string{"force": "false"}), nil
	}

	// 3. Don't override a hint set by an earlier location-pinner instance unless forced.
	if !force && activity.HintLocation != nil {
		logger.Debug("location_pinner: skipping - hint location already set")
		return skipped("hint_location_already_set", map[string]string{"force": "false"}), nil
	}

	// 4. Need a title to match against.
	if activity.Name == "" {
		logger.Debug("location_pinner: skipping - no activity title")
		return skipped("no_activity_title", nil), nil
	}

	// 5. Parse rules. Format matches type_mapper's type_rules: {"<title substring>": "<lat>|<lng>|<label>"}.
	rulesJSON := inputConfig["location_rules"]
	if rulesJSON == "" {
		logger.Debug("location_pinner: skipping - no rules configured")
		return skipped("no_rules_configured", nil), nil
	}
	var rules map[string]string
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		logger.Debug("location_pinner: skipping - failed to parse location_rules", "error", err.Error())
		return skipped("invalid_rules_json", nil), nil
	}
	if len(rules) == 0 {
		logger.Debug("location_pinner: skipping - no rules configured")
		return skipped("no_rules_configured", nil), nil
	}

	// 6. First matching substring wins (case-insensitive), mirroring type_mapper.
	lowerTitle := strings.ToLower(activity.Name)
	for substring, value := range rules {
		if substring == "" {
			continue
		}
		if !strings.Contains(lowerTitle, strings.ToLower(substring)) {
			continue
		}

		loc, ok := parseLocationValue(value)
		if !ok {
			logger.Debug("location_pinner: rule matched but value invalid",
				"matched_substring", substring,
				"value", value,
			)
			// A malformed value shouldn't mask other rules; keep checking.
			continue
		}

		logger.Info("location_pinner: matched rule - pinning location",
			"matched_substring", substring,
			"lat", loc.lat,
			"lng", loc.lng,
			"label", loc.label,
		)

		return &providers.EnrichmentResult{
			HintLocation: &pbactivity.LocationSummary{
				Latitude:     loc.lat,
				Longitude:    loc.lng,
				LocationName: loc.label,
			},
			Metadata: map[string]string{
				"status":          "success",
				"matched_pattern": substring,
				"location_label":  loc.label,
				"lat":             fmt.Sprintf("%.6f", loc.lat),
				"lng":             fmt.Sprintf("%.6f", loc.lng),
			},
		}, nil
	}

	logger.Debug("location_pinner: no rules matched", "title", activity.Name)
	return skipped("no_matching_rule", map[string]string{"title": activity.Name}), nil
}
