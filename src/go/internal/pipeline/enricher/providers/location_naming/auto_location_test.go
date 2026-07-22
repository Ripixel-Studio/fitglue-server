package location_naming

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// stubGeocode returns fixed values and records that it was called, so tests can assert whether
// the network path would have been taken without making a real request.
func stubGeocode(name, city string, called *bool) GeocodeFunc {
	return func(_ context.Context, _ *slog.Logger, _, _ float64) (string, string) {
		if called != nil {
			*called = true
		}
		return name, city
	}
}

// nil activity and GPS-less activities yield no location (nothing to extract).
func TestResolveLocationSummary_NoCoordinates(t *testing.T) {
	if got := ResolveLocationSummary(context.Background(), slog.Default(), nil, nil); got != nil {
		t.Errorf("nil activity: expected nil, got %+v", got)
	}

	// Sessions/laps/records present but no GPS on any record.
	act := &pbactivity.StandardizedActivity{
		Name: "Treadmill Run",
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{
				{Records: []*pbactivity.Record{{}, {}}},
			}},
		},
	}
	var called bool
	if got := ResolveLocationSummary(context.Background(), slog.Default(), act, stubGeocode("X", "Y", &called)); got != nil {
		t.Errorf("GPS-less activity: expected nil, got %+v", got)
	}
	if called {
		t.Error("geocoder should not be called when there are no coordinates")
	}
}

// A record carrying GPS is promoted to a LocationSummary with the raw coordinates, and the
// injected geocoder supplies the name/country.
func TestResolveLocationSummary_ExtractsRecordCoordinates(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Name: "Morning Run",
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{
				{Records: []*pbactivity.Record{
					{}, // first record has no GPS — should be skipped
					{PositionLat: 51.4120, PositionLong: -0.3010},
				}},
			}},
		},
	}

	var called bool
	got := ResolveLocationSummary(context.Background(), slog.Default(), act, stubGeocode("Bushy Park", "London", &called))
	if got == nil {
		t.Fatal("expected a LocationSummary, got nil")
	}
	if !called {
		t.Error("expected geocoder to be called for a real GPS track")
	}
	if got.Latitude != 51.4120 || got.Longitude != -0.3010 {
		t.Errorf("coordinates = (%v,%v), want (51.4120,-0.3010)", got.Latitude, got.Longitude)
	}
	if got.LocationName != "Bushy Park" {
		t.Errorf("location name = %q, want %q", got.LocationName, "Bushy Park")
	}
	if got.Country != "London" {
		t.Errorf("country = %q, want %q (city mirrored, matching enricher parity)", got.Country, "London")
	}
}

// With a nil geocoder the coordinates are still promoted — no network, no name.
func TestResolveLocationSummary_NilGeocoderCoordinatesOnly(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{
				{Records: []*pbactivity.Record{{PositionLat: 12.34, PositionLong: 56.78}}},
			}},
		},
	}
	got := ResolveLocationSummary(context.Background(), slog.Default(), act, nil)
	if got == nil {
		t.Fatal("expected a LocationSummary, got nil")
	}
	if got.Latitude != 12.34 || got.Longitude != 56.78 {
		t.Errorf("coordinates = (%v,%v), want (12.34,56.78)", got.Latitude, got.Longitude)
	}
	if got.LocationName != "" {
		t.Errorf("expected no name with nil geocoder, got %q", got.LocationName)
	}
}

// A pinned hint with an explicit label is used verbatim with no geocoding.
func TestResolveLocationSummary_UsesHintLabel(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Name: "Evening Pilates",
		Sessions: []*pbactivity.Session{
			{TotalElapsedTime: 1800},
		},
		HintLocation: &pbactivity.LocationSummary{
			Latitude:     52.99,
			Longitude:    -0.78,
			LocationName: "Anasa Fernwood",
		},
	}

	var called bool
	got := ResolveLocationSummary(context.Background(), slog.Default(), act, stubGeocode("Should", "NotUse", &called))
	if got == nil {
		t.Fatal("expected a LocationSummary, got nil")
	}
	if called {
		t.Error("geocoder should not be called when a hint label is present")
	}
	if got.LocationName != "Anasa Fernwood" {
		t.Errorf("location name = %q, want hint label", got.LocationName)
	}
	if got.Latitude != 52.99 || got.Longitude != -0.78 {
		t.Errorf("coordinates = (%v,%v), want hint coords", got.Latitude, got.Longitude)
	}
}

// ReverseGeocode hits Nominatim, parses the address, returns name+city, and caches the result
// so a second lookup for the same rounded coordinates makes no further request.
func TestReverseGeocode_HTTPAndCache(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		fmt.Fprint(w, `{"address":{"park":"Richmond Park","city":"London","country":"United Kingdom"}}`)
	}))
	defer srv.Close()

	prev := nominatimBaseURL
	nominatimBaseURL = srv.URL
	defer func() { nominatimBaseURL = prev }()

	// Use coordinates unlikely to collide with other tests' cache keys.
	lat, lng := 40.7128, -74.0060
	name, city := ReverseGeocode(context.Background(), slog.Default(), lat, lng)
	if name != "Richmond Park" || city != "London" {
		t.Fatalf("ReverseGeocode = (%q,%q), want (Richmond Park, London)", name, city)
	}
	if hits != 1 {
		t.Fatalf("expected 1 upstream request, got %d", hits)
	}

	// Second call: same rounded key → served from cache, no extra request.
	name2, city2 := ReverseGeocode(context.Background(), slog.Default(), lat, lng)
	if name2 != "Richmond Park" || city2 != "London" {
		t.Errorf("cached ReverseGeocode = (%q,%q), want (Richmond Park, London)", name2, city2)
	}
	if hits != 1 {
		t.Errorf("expected cache hit (still 1 request), got %d", hits)
	}
}

// A non-200 from Nominatim is swallowed: ReverseGeocode returns empty strings so the caller
// keeps the raw coordinates rather than failing the pipeline.
func TestReverseGeocode_NonOKReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	prev := nominatimBaseURL
	nominatimBaseURL = srv.URL
	defer func() { nominatimBaseURL = prev }()

	name, city := ReverseGeocode(context.Background(), slog.Default(), 1.2345, 6.7890)
	if name != "" || city != "" {
		t.Errorf("expected empty on non-200, got (%q,%q)", name, city)
	}
}

// A real GPS track takes precedence over any pinned hint.
func TestResolveLocationSummary_RecordGPSBeatsHint(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{
				{Records: []*pbactivity.Record{{PositionLat: 10.0, PositionLong: 20.0}}},
			}},
		},
		HintLocation: &pbactivity.LocationSummary{
			Latitude:     52.99,
			Longitude:    -0.78,
			LocationName: "Ignored Hint",
		},
	}

	got := ResolveLocationSummary(context.Background(), slog.Default(), act, stubGeocode("Real Place", "Real City", nil))
	if got == nil {
		t.Fatal("expected a LocationSummary, got nil")
	}
	if got.Latitude != 10.0 || got.Longitude != 20.0 {
		t.Errorf("coordinates = (%v,%v), want record GPS (10,20)", got.Latitude, got.Longitude)
	}
	if got.LocationName == "Ignored Hint" {
		t.Errorf("expected record GPS to win over hint, got hint label")
	}
}
