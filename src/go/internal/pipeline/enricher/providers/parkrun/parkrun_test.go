package parkrun

import (
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"log/slog"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// createMockLocationsService creates a service with test locations pre-loaded.
func createMockLocationsService() *ParkrunLocationsService {
	svc := NewParkrunLocationsService()

	// Add test locations (mimicking the old hardcoded ones)
	testLocations := []ParkrunLocation{
		{Name: "Bushy Park Parkrun", EventSlug: "bushypark", CountryURL: "www.parkrun.org.uk", Latitude: 51.4106, Longitude: -0.3421},
		{Name: "Newark Parkrun", EventSlug: "newark", CountryURL: "www.parkrun.org.uk", Latitude: 53.0764, Longitude: -0.8088},
		{Name: "Cardiff Parkrun", EventSlug: "cardiff", CountryURL: "www.parkrun.org.uk", Latitude: 51.4845, Longitude: -3.1647},
		{Name: "Woodhouse Moor Parkrun", EventSlug: "woodhousemoor", CountryURL: "www.parkrun.org.uk", Latitude: 53.8130, Longitude: -1.5612},
		{Name: "Albert Parkrun, Melbourne", EventSlug: "albertpark", CountryURL: "www.parkrun.com.au", Latitude: -37.8427, Longitude: 144.9654},
		{Name: "Delta Park Parkrun", EventSlug: "deltapark", CountryURL: "www.parkrun.co.za", Latitude: -26.0931, Longitude: 28.0039},
	}

	svc.locations = testLocations
	svc.lastFetch = time.Now()

	// Build grid index
	svc.grid = make(map[string][]ParkrunLocation)
	for _, loc := range testLocations {
		key := gridKey(loc.Latitude, loc.Longitude)
		svc.grid[key] = append(svc.grid[key], loc)
	}

	return svc
}

func TestParkrunProvider_Enrich(t *testing.T) {
	// Use mock location service
	mockSvc := createMockLocationsService()
	provider := NewParkrunProviderWithService(mockSvc)

	// Helper to create activity with location
	createActivity := func(timeStr string, lat, long float64) *pbactivity.StandardizedActivity {
		tParsed, _ := time.Parse(time.RFC3339, timeStr)
		return &pbactivity.StandardizedActivity{
			Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			StartTime: timestamppb.New(tParsed),
			Sessions: []*pbactivity.Session{
				{
					Laps: []*pbactivity.Lap{
						{
							Records: []*pbactivity.Record{
								{
									PositionLat:  lat,
									PositionLong: long,
								},
							},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name      string
		time      string // RFC3339
		lat, long float64
		inputs    map[string]string
		wantMatch bool
		wantName  string
		wantTags  []string
	}{
		{
			name:      "Saturday Morning at Bushy Park (Perfect Match)",
			time:      "2025-12-20T09:00:00Z", // UTC check (09:00 UTC is 09:00 GMT)
			lat:       51.4106,
			long:      -0.3421,
			wantMatch: true,
			wantName:  "Bushy Park Parkrun",
			wantTags:  []string{"Parkrun"},
		},
		{
			name:      "Saturday Morning at Bushy Park (Slightly Away - 100m)",
			time:      "2025-12-20T09:00:00Z",
			lat:       51.4115, // Approx 100m North
			long:      -0.3421,
			wantMatch: true,
			wantName:  "Bushy Park Parkrun",
			wantTags:  []string{"Parkrun"},
		},
		{
			name:      "Saturday Morning at Bushy Park (Too Far - 2.5km)",
			time:      "2025-12-20T09:00:00Z",
			lat:       51.4306, // Approx 2.2km North (0.02 deg)
			long:      -0.3421,
			wantMatch: false,
		},
		{
			name:      "Saturday Afternoon (Not Parkrun)",
			time:      "2025-12-20T14:00:00Z",
			lat:       51.4106,
			long:      -0.3421,
			wantMatch: false,
		},
		{
			name:      "Tuesday Morning (Not Parkrun)",
			time:      "2025-12-23T09:00:00Z",
			lat:       51.4106,
			long:      -0.3421,
			wantMatch: false,
		},
		{
			name: "Christmas Day (Special Event)",
			time: "2025-12-25T09:00:00Z", // Thursday, but Xmas
			lat:  51.4106,
			long: -0.3421,
			inputs: map[string]string{
				"enable_titling": "true",
			},
			wantMatch: true,
			wantName:  "Bushy Park Parkrun - Christmas Day Edition",
		},
		{
			name: "Australian Parkrun (Timezone check - Albert Park)",
			// Albert Park Melbourne: AEDT = UTC+11 in December (southern hemisphere summer)
			// Parkrun at 09:00 AEDT = 22:00 UTC the previous day
			// 2025-12-19T22:00:00Z → 2025-12-20T09:00:00+11:00 (Saturday)
			time:      "2025-12-19T22:00:00Z",
			lat:       -37.8427,
			long:      144.9654,
			wantMatch: true,
			wantName:  "Albert Parkrun, Melbourne",
		},
		{
			name:      "Saturday Morning at Bushy Park (BST - Summer, 09:00 local = 08:00 UTC)",
			time:      "2025-06-21T08:00:00Z", // 09:00 BST (UTC+1)
			lat:       51.4106,
			long:      -0.3421,
			wantMatch: true,
			wantName:  "Bushy Park Parkrun",
			wantTags:  []string{"Parkrun"},
		},
		{
			name:      "Saturday Morning at Bushy Park (BST early start - 07:56 UTC = 08:56 BST)",
			time:      "2025-06-21T07:56:00Z", // 08:56 BST, the reported bug case
			lat:       51.4106,
			long:      -0.3421,
			wantMatch: true,
			wantName:  "Bushy Park Parkrun",
			wantTags:  []string{"Parkrun"},
		},
		{
			name: "Custom Tags",
			time: "2025-12-20T09:00:00Z",
			lat:  51.4106,
			long: -0.3421,
			inputs: map[string]string{
				"tags": "Parkrun,Race,5k",
			},
			wantMatch: true,
			wantName:  "Bushy Park Parkrun",
			wantTags:  []string{"Parkrun", "Race", "5k"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity := createActivity(tt.time, tt.lat, tt.long)

			inputs := tt.inputs
			if inputs == nil {
				inputs = make(map[string]string)
			}

			res, err := provider.Enrich(context.Background(), slog.Default(), activity, nil, inputs, false)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !tt.wantMatch {
				if res == nil {
					t.Fatalf("Expected result with skip metadata, got nil")
				}
				if res.Metadata == nil || res.Metadata["status"] != "skipped" {
					t.Errorf("Expected status='skipped' in metadata, got %v", res.Metadata)
				}
				return
			}

			if res == nil {
				t.Fatal("Expected matching result, got nil")
			}

			if res.Name != tt.wantName {
				t.Errorf("Expected Name %q, got %q", tt.wantName, res.Name)
			}

			if len(tt.wantTags) > 0 {
				if len(res.Tags) != len(tt.wantTags) {
					t.Errorf("Expected %d tags, got %v", len(tt.wantTags), res.Tags)
				}
			}
		})
	}
}

func TestTitlePatterns(t *testing.T) {
	tests := []struct {
		name           string
		normalPattern  string
		specialPattern string
		location       string
		time           time.Time
		specialDay     string
		want           string
	}{
		{
			name:          "Default location only",
			normalPattern: "{location}",
			location:      "Newark Parkrun",
			time:          time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC),
			want:          "Newark Parkrun",
		},
		{
			name:          "Location with date",
			normalPattern: "{location} - {date}",
			location:      "Newark Parkrun",
			time:          time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC),
			want:          "Newark Parkrun - 20 Dec 2025",
		},
		{
			name:           "Special day uses special pattern",
			normalPattern:  "{location}",
			specialPattern: "{location} - {special} Edition",
			location:       "Newark Parkrun",
			time:           time.Date(2025, 12, 25, 9, 0, 0, 0, time.UTC),
			specialDay:     "Christmas Day",
			want:           "Newark Parkrun - Christmas Day Edition",
		},
		{
			name:           "New Year's Day",
			normalPattern:  "{location}",
			specialPattern: "{location} 🎉 {special}",
			location:       "Newark Parkrun",
			time:           time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
			specialDay:     "New Year's Day",
			want:           "Newark Parkrun 🎉 New Year's Day",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyTitlePattern(tt.normalPattern, tt.specialPattern, tt.location, tt.time, tt.specialDay)
			if got != tt.want {
				t.Errorf("applyTitlePattern() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParkrunLocationsService_FindNearest(t *testing.T) {
	svc := createMockLocationsService()

	tests := []struct {
		name      string
		lat, lng  float64
		threshold float64
		wantName  string
		wantFound bool
	}{
		{
			name:      "Exact match Bushy Park",
			lat:       51.4106,
			lng:       -0.3421,
			threshold: 200,
			wantName:  "Bushy Park Parkrun",
			wantFound: true,
		},
		{
			name:      "100m away still matches",
			lat:       51.4115,
			lng:       -0.3421,
			threshold: 200,
			wantName:  "Bushy Park Parkrun",
			wantFound: true,
		},
		{
			name:      "1km away does not match",
			lat:       51.4206,
			lng:       -0.3421,
			threshold: 200,
			wantFound: false,
		},
		{
			name:      "Southern hemisphere (Melbourne)",
			lat:       -37.8427,
			lng:       144.9654,
			threshold: 200,
			wantName:  "Albert Parkrun, Melbourne",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.FindNearest(tt.lat, tt.lng, tt.threshold)
			if tt.wantFound {
				if got == nil {
					t.Errorf("Expected to find location, got nil")
				} else if got.Name != tt.wantName {
					t.Errorf("Expected %q, got %q", tt.wantName, got.Name)
				}
			} else {
				if got != nil {
					t.Errorf("Expected nil, got %s", got.Name)
				}
			}
		})
	}
}

func TestSpecialDayDetection(t *testing.T) {
	tests := []struct {
		time time.Time
		want string
	}{
		{time.Date(2025, 12, 25, 9, 0, 0, 0, time.UTC), "Christmas Day"},
		{time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), "New Year's Day"},
		{time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC), ""},
		{time.Date(2025, 6, 15, 9, 0, 0, 0, time.UTC), ""},
	}

	for _, tt := range tests {
		t.Run(tt.time.Format("2006-01-02"), func(t *testing.T) {
			got := getSpecialDay(tt.time)
			if got != tt.want {
				t.Errorf("getSpecialDay(%v) = %q, want %q", tt.time, got, tt.want)
			}
		})
	}
}

func TestParseEventsJSON_FiltersJuniorEvents(t *testing.T) {
	// Simulate events.json with both regular (seriesid=1) and junior (seriesid=2) events
	eventsJSON := `{
		"events": {
			"type": "FeatureCollection",
			"features": [
				{
					"id": 1,
					"type": "Feature",
					"geometry": {"type": "Point", "coordinates": [-0.8088, 53.0764]},
					"properties": {
						"eventname": "newark",
						"EventLongName": "Newark parkrun",
						"EventShortName": "Newark",
						"countrycode": 97,
						"seriesid": 1
					}
				},
				{
					"id": 2,
					"type": "Feature",
					"geometry": {"type": "Point", "coordinates": [-0.8085, 53.0762]},
					"properties": {
						"eventname": "newark-juniors",
						"EventLongName": "Newark junior parkrun",
						"EventShortName": "Newark juniors",
						"countrycode": 97,
						"seriesid": 2
					}
				},
				{
					"id": 3,
					"type": "Feature",
					"geometry": {"type": "Point", "coordinates": [-0.3421, 51.4106]},
					"properties": {
						"eventname": "bushy",
						"EventLongName": "Bushy parkrun",
						"EventShortName": "Bushy Park",
						"countrycode": 97,
						"seriesid": 1
					}
				}
			]
		}
	}`

	locations, err := parseEventsJSON([]byte(eventsJSON))
	if err != nil {
		t.Fatalf("parseEventsJSON returned error: %v", err)
	}

	// Should only have 2 locations (the junior event should be filtered out)
	if len(locations) != 2 {
		t.Fatalf("Expected 2 locations, got %d", len(locations))
	}

	// Verify the correct events were retained
	for _, loc := range locations {
		if loc.EventSlug == "newark-juniors" {
			t.Errorf("Junior event 'newark-juniors' should have been filtered out")
		}
	}

	// Verify newark and bushy are present
	slugs := map[string]bool{}
	for _, loc := range locations {
		slugs[loc.EventSlug] = true
	}
	if !slugs["newark"] {
		t.Error("Expected 'newark' to be in locations")
	}
	if !slugs["bushy"] {
		t.Error("Expected 'bushy' to be in locations")
	}
}
