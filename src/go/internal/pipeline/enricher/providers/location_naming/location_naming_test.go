package location_naming

import (
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"log/slog"
	"testing"
)

func TestGetLocationName(t *testing.T) {
	tests := []struct {
		name     string
		address  NominatimAddress
		expected string
	}{
		{
			name: "park takes priority",
			address: NominatimAddress{
				Park:    "Hyde Park",
				Leisure: "Sports Centre",
				Suburb:  "Kensington",
			},
			expected: "Hyde Park",
		},
		{
			name: "leisure takes priority over suburb",
			address: NominatimAddress{
				Leisure: "Sports Centre",
				Suburb:  "Kensington",
			},
			expected: "Sports Centre",
		},
		{
			name: "suburb used as fallback",
			address: NominatimAddress{
				Suburb: "Kensington",
				City:   "London",
			},
			expected: "Kensington",
		},
		{
			name:     "empty when no location available",
			address:  NominatimAddress{City: "London"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLocationName(tt.address)
			if result != tt.expected {
				t.Errorf("getLocationName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetCityName(t *testing.T) {
	tests := []struct {
		name     string
		address  NominatimAddress
		expected string
	}{
		{
			name:     "city preferred",
			address:  NominatimAddress{City: "London", Town: "Westminster"},
			expected: "London",
		},
		{
			name:     "town as fallback",
			address:  NominatimAddress{Town: "Windsor"},
			expected: "Windsor",
		},
		{
			name:     "village as last resort",
			address:  NominatimAddress{Village: "Little Gaddesden"},
			expected: "Little Gaddesden",
		},
		{
			name:     "empty when no city available",
			address:  NominatimAddress{County: "Hertfordshire"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCityName(tt.address)
			if result != tt.expected {
				t.Errorf("getCityName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetActivityTypeStr(t *testing.T) {
	tests := []struct {
		name         string
		activityType pbactivity.ActivityType
		expected     string
	}{
		{name: "run", activityType: pbactivity.ActivityType_ACTIVITY_TYPE_RUN, expected: "Run"},
		{name: "ride", activityType: pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, expected: "Ride"},
		{name: "walk", activityType: pbactivity.ActivityType_ACTIVITY_TYPE_WALK, expected: "Walk"},
		{name: "hike", activityType: pbactivity.ActivityType_ACTIVITY_TYPE_HIKE, expected: "Hike"},
		{name: "unknown defaults to Activity", activityType: pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, expected: "Activity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getActivityTypeStr(tt.activityType)
			if result != tt.expected {
				t.Errorf("getActivityTypeStr() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestProviderMetadata(t *testing.T) {
	provider := NewLocationNaming()

	if provider.Name() != "location_naming" {
		t.Errorf("Name() = %q, want 'location_naming'", provider.Name())
	}

	if provider.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_LOCATION_NAMING {
		t.Errorf("ProviderType() = %v, want ENRICHER_PROVIDER_LOCATION_NAMING", provider.ProviderType())
	}
}

// TestEnrich_UsesPinnedHintLabel verifies that when an activity has no record GPS but
// carries a location hint with a label (set by the location-pinner enricher), the
// resolved location name is the hint label verbatim (no network/geocoding) and is
// surfaced via Enrichments.Location so it rolls up into roundup "places".
func TestEnrich_UsesPinnedHintLabel(t *testing.T) {
	provider := NewLocationNaming()
	act := &pbactivity.StandardizedActivity{
		Name: "Evening Pilates Class",
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_YOGA,
		Sessions: []*pbactivity.Session{
			{TotalElapsedTime: 1800},
		},
		HintLocation: &pbactivity.LocationSummary{
			Latitude:     52.99,
			Longitude:    -0.78,
			LocationName: "Anasa Fernwood",
		},
	}

	res, err := provider.Enrich(context.Background(), slog.Default(), act, nil, map[string]string{"mode": "title"}, true)
	if err != nil {
		t.Fatalf("Enrich returned error: %v", err)
	}
	if res.Skipped {
		t.Fatalf("expected enrichment, got skip: %s", res.SkipReason)
	}
	if res.Enrichments == nil || res.Enrichments.Location == nil {
		t.Fatalf("expected Enrichments.Location to be set, got %+v", res.Enrichments)
	}
	if got := res.Enrichments.Location.LocationName; got != "Anasa Fernwood" {
		t.Errorf("expected location name from hint label, got %q", got)
	}
	if res.Enrichments.Location.Latitude != 52.99 || res.Enrichments.Location.Longitude != -0.78 {
		t.Errorf("expected hint coords on Enrichments.Location, got (%v,%v)",
			res.Enrichments.Location.Latitude, res.Enrichments.Location.Longitude)
	}
	if res.Name == "" {
		t.Error("expected a generated title in title mode")
	}
}
