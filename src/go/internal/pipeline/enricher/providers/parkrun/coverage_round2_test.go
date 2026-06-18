package parkrun

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// runActivityAt builds a RUN activity with the given start time and GPS start point.
func runActivityAt(start time.Time, lat, long float64) *pbactivity.StandardizedActivity {
	return &pbactivity.StandardizedActivity{
		Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime: timestamppb.New(start),
		Sessions: []*pbactivity.Session{
			{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{{PositionLat: lat, PositionLong: long}}}}},
		},
	}
}

func TestParkrun_Enrich_SkipBranches(t *testing.T) {
	// Saturday 09:00 UTC (a valid parkrun day/time) used where the skip reason
	// is location-based rather than time-based.
	sat := time.Date(2025, 12, 20, 9, 0, 0, 0, time.UTC)

	t.Run("NoParkrunWithin5km", func(t *testing.T) {
		p := NewParkrunProviderWithService(createMockLocationsService())
		// Middle of the ocean — no location nearby.
		act := runActivityAt(sat, 0.0, -30.0)
		res, err := p.Enrich(context.Background(), discardLogger(), act, nil, map[string]string{}, false)
		if err != nil {
			t.Fatalf("Enrich error: %v", err)
		}
		if res.Metadata["reason"] != "no_parkrun_within_5km" {
			t.Errorf("expected no_parkrun_within_5km, got %v", res.Metadata)
		}
	})

	t.Run("NotNearParkrun", func(t *testing.T) {
		p := NewParkrunProviderWithService(createMockLocationsService())
		// ~2km from Bushy Park (51.4106,-0.3421): within 5km but >1500m.
		act := runActivityAt(sat, 51.4106, -0.3700)
		res, err := p.Enrich(context.Background(), discardLogger(), act, nil, map[string]string{}, false)
		if err != nil {
			t.Fatalf("Enrich error: %v", err)
		}
		if res.Metadata["reason"] != "not_near_parkrun" {
			t.Errorf("expected not_near_parkrun, got %v", res.Metadata)
		}
	})

	t.Run("NotParkrunDay", func(t *testing.T) {
		p := NewParkrunProviderWithService(createMockLocationsService())
		// Friday at Bushy Park.
		fri := time.Date(2025, 12, 19, 9, 0, 0, 0, time.UTC)
		act := runActivityAt(fri, 51.4106, -0.3421)
		res, err := p.Enrich(context.Background(), discardLogger(), act, nil, map[string]string{}, false)
		if err != nil {
			t.Fatalf("Enrich error: %v", err)
		}
		if res.Metadata["reason"] != "not_parkrun_day" {
			t.Errorf("expected not_parkrun_day, got %v", res.Metadata)
		}
	})

	t.Run("OutsideTimeWindow", func(t *testing.T) {
		p := NewParkrunProviderWithService(createMockLocationsService())
		// Saturday but 14:00 local — outside the 08:45-09:15 window.
		satAfternoon := time.Date(2025, 12, 20, 14, 0, 0, 0, time.UTC)
		act := runActivityAt(satAfternoon, 51.4106, -0.3421)
		res, err := p.Enrich(context.Background(), discardLogger(), act, nil, map[string]string{}, false)
		if err != nil {
			t.Fatalf("Enrich error: %v", err)
		}
		if res.Metadata["reason"] != "outside_time_window" {
			t.Errorf("expected outside_time_window, got %v", res.Metadata)
		}
	})

	t.Run("ZeroStartTime", func(t *testing.T) {
		p := NewParkrunProviderWithService(createMockLocationsService())
		act := &pbactivity.StandardizedActivity{
			Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			StartTime: timestamppb.New(time.Time{}),
			Sessions: []*pbactivity.Session{
				{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{{PositionLat: 51.4106, PositionLong: -0.3421}}}}},
			},
		}
		_, err := p.Enrich(context.Background(), discardLogger(), act, nil, map[string]string{}, false)
		if err == nil {
			t.Error("expected error for zero start time")
		}
	})
}

func TestParkrun_SetService(t *testing.T) {
	p := NewParkrunProvider()
	svc := &bootstrap.Service{}
	p.SetService(svc)
	if p.service != svc {
		t.Error("SetService did not set service")
	}
}

func TestNewParkrunLocationsServiceWithClient_AndLocationCount(t *testing.T) {
	svc := NewParkrunLocationsServiceWithClient(&http.Client{})
	if svc == nil {
		t.Fatal("expected service")
	}
	if svc.LocationCount() != 0 {
		t.Errorf("fresh service should have 0 locations, got %d", svc.LocationCount())
	}
}

const validEventsJSON = `{"events":{"type":"FeatureCollection","features":[
	{"id":1,"properties":{"eventname":"newark","EventShortName":"Newark","countrycode":97,"seriesid":1},"geometry":{"type":"Point","coordinates":[-0.81,53.07]}},
	{"id":2,"properties":{"eventname":"junior","EventShortName":"Junior","countrycode":97,"seriesid":2},"geometry":{"type":"Point","coordinates":[-0.81,53.07]}}
]}}`

func TestRefreshFromSource(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(validEventsJSON))
		}))
		defer ts.Close()

		svc := NewParkrunLocationsServiceWithClient(ts.Client())
		// Point the service at the test server by overriding via a custom transport
		// that rewrites the host. Simpler: use a RoundTripper that serves our body.
		svc.client = &http.Client{Transport: rewriteTransport{ts.URL}}

		if err := svc.RefreshFromSource(context.Background()); err != nil {
			t.Fatalf("RefreshFromSource error: %v", err)
		}
		// Junior (seriesid 2) filtered out -> exactly 1 location.
		if got := svc.LocationCount(); got != 1 {
			t.Errorf("LocationCount = %d, want 1", got)
		}
		// EnsureLoaded should now be a no-op (not stale).
		if err := svc.EnsureLoaded(context.Background()); err != nil {
			t.Errorf("EnsureLoaded after refresh: %v", err)
		}
		// FindNearest should find the loaded Newark location.
		near := svc.FindNearest(53.07, -0.81, 1000)
		if near == nil || near.EventSlug != "newark" {
			t.Errorf("FindNearest did not return newark, got %+v", near)
		}
	})

	t.Run("BadStatus", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()
		svc := NewParkrunLocationsServiceWithClient(&http.Client{Transport: rewriteTransport{ts.URL}})
		if err := svc.RefreshFromSource(context.Background()); err == nil {
			t.Error("expected error on non-200 status")
		}
	})

	t.Run("NetworkError", func(t *testing.T) {
		svc := NewParkrunLocationsServiceWithClient(&http.Client{Transport: errTransport{}})
		if err := svc.RefreshFromSource(context.Background()); err == nil {
			t.Error("expected error on transport failure")
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer ts.Close()
		svc := NewParkrunLocationsServiceWithClient(&http.Client{Transport: rewriteTransport{ts.URL}})
		if err := svc.RefreshFromSource(context.Background()); err == nil {
			t.Error("expected error parsing invalid JSON")
		}
	})
}

func TestParseEventsJSON_EdgeCases(t *testing.T) {
	t.Run("InvalidJSON", func(t *testing.T) {
		if _, err := parseEventsJSON([]byte("{")); err == nil {
			t.Error("expected error on malformed JSON")
		}
	})

	t.Run("DirectFeatureCollectionFallback", func(t *testing.T) {
		// No "events" wrapper -> fallback to direct FeatureCollection.
		direct := `{"type":"FeatureCollection","features":[
			{"id":1,"properties":{"eventname":"bushy","EventShortName":"Bushy","countrycode":97,"seriesid":1},"geometry":{"type":"Point","coordinates":[-0.33,51.41]}}
		]}`
		locs, err := parseEventsJSON([]byte(direct))
		if err != nil {
			t.Fatalf("parseEventsJSON error: %v", err)
		}
		if len(locs) != 1 || locs[0].EventSlug != "bushy" {
			t.Errorf("expected bushy via direct fallback, got %+v", locs)
		}
	})

	t.Run("SkipsNonPointAndMissingCoords", func(t *testing.T) {
		j := `{"events":{"type":"FeatureCollection","features":[
			{"id":1,"properties":{"eventname":"poly","EventShortName":"Poly","countrycode":97,"seriesid":1},"geometry":{"type":"Polygon","coordinates":[1,2]}},
			{"id":2,"properties":{"eventname":"nocoord","EventShortName":"NoCoord","countrycode":97,"seriesid":1},"geometry":{"type":"Point","coordinates":[1]}}
		]}}`
		locs, err := parseEventsJSON([]byte(j))
		if err != nil {
			t.Fatalf("parseEventsJSON error: %v", err)
		}
		if len(locs) != 0 {
			t.Errorf("expected 0 valid locations, got %d", len(locs))
		}
	})

	t.Run("FallsBackToLongName", func(t *testing.T) {
		// Empty EventShortName -> name falls back to EventLongName.
		j := `{"events":{"type":"FeatureCollection","features":[
			{"id":1,"properties":{"eventname":"x","EventShortName":"","EventLongName":"Full Long Name parkrun","countrycode":3,"seriesid":1},"geometry":{"type":"Point","coordinates":[13.4,52.5]}}
		]}}`
		locs, err := parseEventsJSON([]byte(j))
		if err != nil {
			t.Fatalf("parseEventsJSON error: %v", err)
		}
		if len(locs) != 1 || locs[0].Name != "Full Long Name parkrun" {
			t.Errorf("expected long-name fallback, got %+v", locs)
		}
		if locs[0].CountryURL != "www.parkrun.de" {
			t.Errorf("expected DE country URL, got %q", locs[0].CountryURL)
		}
	})
}

func TestCountryCodeToURL_MoreCodes(t *testing.T) {
	cases := map[int]string{
		97:   "www.parkrun.org.uk",
		65:   "www.parkrun.com.au",
		14:   "www.parkrun.ie",
		64:   "www.parkrun.ca",
		98:   "www.parkrun.us",
		59:   "www.parkrun.jp",
		9999: "www.parkrun.com", // unknown -> default
	}
	for code, want := range cases {
		if got := countryCodeToURL(code); got != want {
			t.Errorf("countryCodeToURL(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestResolveTimezone_MoreBranches(t *testing.T) {
	cases := []struct {
		country string
		lng     float64
		want    string
	}{
		{"www.parkrun.de", 13, "Europe/Berlin"},
		{"www.parkrun.co.za", 28, "Africa/Johannesburg"},
		{"www.parkrun.co.nz", 174, "Pacific/Auckland"},
		{"www.parkrun.com.au", 133, "Australia/Darwin"},
		{"www.parkrun.com.au", 139, "Australia/Adelaide"},
		{"www.parkrun.ca", -55, "America/Halifax"},
		{"www.parkrun.ca", -120, "America/Vancouver"},
		{"www.parkrun.us", -85, "America/Chicago"},
		{"www.parkrun.us", -105, "America/Denver"},
		{"www.parkrun.sg", 103, "Asia/Singapore"},
		{"www.parkrun.my", 101, "Asia/Kuala_Lumpur"},
		{"www.parkrun.dk", 12, "Europe/Copenhagen"},
		{"www.parkrun.fi", 25, "Europe/Helsinki"},
		{"www.parkrun.fr", 2, "Europe/Paris"},
		{"www.parkrun.it", 12, "Europe/Rome"},
		{"www.parkrun.nl", 5, "Europe/Amsterdam"},
		{"www.parkrun.no", 10, "Europe/Oslo"},
		{"www.parkrun.pl", 21, "Europe/Warsaw"},
		{"www.parkrun.se", 18, "Europe/Stockholm"},
	}
	for _, c := range cases {
		if got := resolveTimezone(ParkrunLocation{CountryURL: c.country, Longitude: c.lng}); got != c.want {
			t.Errorf("resolveTimezone(%s,%v) = %q, want %q", c.country, c.lng, got, c.want)
		}
	}
}

// rewriteTransport redirects every request to the given base URL (test server),
// so RefreshFromSource hits our handler instead of the real events.json host.
type rewriteTransport struct{ base string }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := http.NewRequest(req.Method, rt.base, req.Body)
	if err != nil {
		return nil, err
	}
	u.Header = req.Header
	return http.DefaultTransport.RoundTrip(u.WithContext(req.Context()))
}

// errTransport always fails, simulating a network error.
type errTransport struct{}

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, http.ErrUseLastResponse // any non-nil error
}
