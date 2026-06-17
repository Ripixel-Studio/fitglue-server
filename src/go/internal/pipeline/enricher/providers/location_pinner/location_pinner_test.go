package location_pinner

import (
	"context"
	"log/slog"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// activityWithGPS builds a single-record activity with the given coordinates.
func activityWithGPS(name string, lat, lng float64) *pbactivity.StandardizedActivity {
	return &pbactivity.StandardizedActivity{
		Name: name,
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 1800,
				Laps: []*pbactivity.Lap{
					{Records: []*pbactivity.Record{{PositionLat: lat, PositionLong: lng}}},
				},
			},
		},
	}
}

// activityNoGPS builds a session-only activity with no distance and no GPS records
// (e.g. a Pilates class). TotalDistance deliberately 0 to prove distance isn't required.
func activityNoGPS(name string) *pbactivity.StandardizedActivity {
	return &pbactivity.StandardizedActivity{
		Name: name,
		Sessions: []*pbactivity.Session{
			{TotalElapsedTime: 1800, TotalDistance: 0},
		},
	}
}

func TestLocationPinner_Enrich(t *testing.T) {
	provider := NewLocationPinnerProvider()
	ctx := context.Background()

	const codeFitness = `{"Hyrox Class":"53.07|-0.81|Code Fitness, Newark","Pilates Class":"52.99|-0.78|Anasa Fernwood"}`

	tests := []struct {
		name       string
		activity   *pbactivity.StandardizedActivity
		config     map[string]string
		wantHint   bool
		wantLat    float64
		wantLng    float64
		wantLabel  string
		wantReason string // expected skip reason when wantHint is false
	}{
		{
			name:      "matches title, no distance class still works",
			activity:  activityNoGPS("Evening Pilates Class"),
			config:    map[string]string{"location_rules": codeFitness},
			wantHint:  true,
			wantLat:   52.99,
			wantLng:   -0.78,
			wantLabel: "Anasa Fernwood",
		},
		{
			name:      "case-insensitive substring match",
			activity:  activityNoGPS("HYROX class #4"),
			config:    map[string]string{"location_rules": codeFitness},
			wantHint:  true,
			wantLat:   53.07,
			wantLng:   -0.81,
			wantLabel: "Code Fitness, Newark",
		},
		{
			name:       "existing GPS track is not overridden",
			activity:   activityWithGPS("Hyrox Class", 51.5, -0.12),
			config:     map[string]string{"location_rules": codeFitness},
			wantReason: "gps_already_exists",
		},
		{
			name:      "force overrides existing GPS",
			activity:  activityWithGPS("Hyrox Class", 51.5, -0.12),
			config:    map[string]string{"location_rules": codeFitness, "force": "true"},
			wantHint:  true,
			wantLat:   53.07,
			wantLng:   -0.81,
			wantLabel: "Code Fitness, Newark",
		},
		{
			name:       "no matching rule skips",
			activity:   activityNoGPS("Random Workout"),
			config:     map[string]string{"location_rules": codeFitness},
			wantReason: "no_matching_rule",
		},
		{
			name:       "empty title skips",
			activity:   activityNoGPS(""),
			config:     map[string]string{"location_rules": codeFitness},
			wantReason: "no_activity_title",
		},
		{
			name:       "no rules configured skips",
			activity:   activityNoGPS("Hyrox Class"),
			config:     map[string]string{},
			wantReason: "no_rules_configured",
		},
		{
			name:       "invalid rules json skips",
			activity:   activityNoGPS("Hyrox Class"),
			config:     map[string]string{"location_rules": `{invalid}`},
			wantReason: "invalid_rules_json",
		},
		{
			name:       "no sessions skips",
			activity:   &pbactivity.StandardizedActivity{Name: "Hyrox Class"},
			config:     map[string]string{"location_rules": codeFitness},
			wantReason: "no_sessions",
		},
		{
			name:       "malformed coordinate value skips (no other rule matches)",
			activity:   activityNoGPS("Hyrox Class"),
			config:     map[string]string{"location_rules": `{"Hyrox Class":"not-a-coord"}`},
			wantReason: "no_matching_rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := provider.Enrich(ctx, slog.Default(), tt.activity, nil, tt.config, false)
			if err != nil {
				t.Fatalf("Enrich returned error: %v", err)
			}

			// The enricher must never write GPS into records.
			for _, s := range tt.activity.Sessions {
				for _, lap := range s.Laps {
					for _, rec := range lap.Records {
						if tt.wantHint && rec.PositionLat == tt.wantLat {
							t.Errorf("enricher wrote pinned coords into a record; it must only set HintLocation")
						}
					}
				}
			}

			if !tt.wantHint {
				if !res.Skipped {
					t.Fatalf("expected skipped result, got %+v", res)
				}
				if res.Metadata["reason"] != tt.wantReason {
					t.Errorf("expected skip reason %q, got %q", tt.wantReason, res.Metadata["reason"])
				}
				if res.HintLocation != nil {
					t.Errorf("expected no HintLocation on skip, got %+v", res.HintLocation)
				}
				return
			}

			if res.HintLocation == nil {
				t.Fatalf("expected HintLocation to be set, got nil (metadata: %v)", res.Metadata)
			}
			if res.HintLocation.Latitude != tt.wantLat || res.HintLocation.Longitude != tt.wantLng {
				t.Errorf("expected coords (%v,%v), got (%v,%v)", tt.wantLat, tt.wantLng, res.HintLocation.Latitude, res.HintLocation.Longitude)
			}
			if res.HintLocation.LocationName != tt.wantLabel {
				t.Errorf("expected label %q, got %q", tt.wantLabel, res.HintLocation.LocationName)
			}
			if res.Metadata["matched_pattern"] == "" {
				t.Error("expected matched_pattern in metadata")
			}
		})
	}
}

func TestLocationPinner_SkipsWhenHintAlreadySet(t *testing.T) {
	provider := NewLocationPinnerProvider()
	act := activityNoGPS("Hyrox Class")
	act.HintLocation = &pbactivity.LocationSummary{Latitude: 1, Longitude: 1, LocationName: "Existing"}

	res, err := provider.Enrich(context.Background(), slog.Default(), act, nil,
		map[string]string{"location_rules": `{"Hyrox Class":"53.07|-0.81|Code Fitness"}`}, false)
	if err != nil {
		t.Fatalf("Enrich returned error: %v", err)
	}
	if !res.Skipped || res.Metadata["reason"] != "hint_location_already_set" {
		t.Errorf("expected skip with reason hint_location_already_set, got %+v", res)
	}
}

func TestParseLocationValue(t *testing.T) {
	tests := []struct {
		in        string
		wantOK    bool
		wantLat   float64
		wantLng   float64
		wantLabel string
	}{
		{"53.07|-0.81|Code Fitness, Newark", true, 53.07, -0.81, "Code Fitness, Newark"},
		{"53.07|-0.81", true, 53.07, -0.81, ""},
		{" 53.07 | -0.81 | Gym ", true, 53.07, -0.81, "Gym"},
		{"label|with|pipes embedded", false, 0, 0, ""}, // first two parts not numeric
		{"53.07", false, 0, 0, ""},
		{"not|coords", false, 0, 0, ""},
		{"0|0|Null Island", false, 0, 0, ""},
		{"91|0|Too far north", false, 0, 0, ""},
		{"", false, 0, 0, ""},
	}
	for _, tt := range tests {
		got, ok := parseLocationValue(tt.in)
		if ok != tt.wantOK {
			t.Errorf("parseLocationValue(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			continue
		}
		if ok && (got.lat != tt.wantLat || got.lng != tt.wantLng || got.label != tt.wantLabel) {
			t.Errorf("parseLocationValue(%q) = %+v, want lat=%v lng=%v label=%q", tt.in, got, tt.wantLat, tt.wantLng, tt.wantLabel)
		}
	}
}
