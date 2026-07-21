package location_naming

import (
	"context"
	"log/slog"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// nil activity and GPS-less activities yield no location (nothing to extract).
func TestResolveLocationSummary_NoCoordinates(t *testing.T) {
	if got := ResolveLocationSummary(context.Background(), slog.Default(), nil); got != nil {
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
	if got := ResolveLocationSummary(context.Background(), slog.Default(), act); got != nil {
		t.Errorf("GPS-less activity: expected nil, got %+v", got)
	}
}

// A record carrying GPS is promoted to a LocationSummary with the raw coordinates, even
// with no name (reverse geocoding is best-effort and not exercised here).
func TestResolveLocationSummary_ExtractsRecordCoordinates(t *testing.T) {
	// Prime the cache so the reverse-geocode path is deterministic and makes no network call.
	locationCacheMutex.Lock()
	locationCache["51.4120,-0.3010"] = "Bushy Park|London"
	locationCacheMutex.Unlock()

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

	got := ResolveLocationSummary(context.Background(), slog.Default(), act)
	if got == nil {
		t.Fatal("expected a LocationSummary, got nil")
	}
	if got.Latitude != 51.4120 || got.Longitude != -0.3010 {
		t.Errorf("coordinates = (%v,%v), want (51.4120,-0.3010)", got.Latitude, got.Longitude)
	}
	if got.LocationName != "Bushy Park" {
		t.Errorf("location name = %q, want %q (from cache)", got.LocationName, "Bushy Park")
	}
	if got.Country != "London" {
		t.Errorf("country = %q, want %q (city mirrored, matching enricher parity)", got.Country, "London")
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

	got := ResolveLocationSummary(context.Background(), slog.Default(), act)
	if got == nil {
		t.Fatal("expected a LocationSummary, got nil")
	}
	if got.LocationName != "Anasa Fernwood" {
		t.Errorf("location name = %q, want hint label", got.LocationName)
	}
	if got.Latitude != 52.99 || got.Longitude != -0.78 {
		t.Errorf("coordinates = (%v,%v), want hint coords", got.Latitude, got.Longitude)
	}
}

// A real GPS track takes precedence over any pinned hint.
func TestResolveLocationSummary_RecordGPSBeatsHint(t *testing.T) {
	locationCacheMutex.Lock()
	locationCache["10.0000,20.0000"] = "Real Place|Real City"
	locationCacheMutex.Unlock()

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

	got := ResolveLocationSummary(context.Background(), slog.Default(), act)
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
