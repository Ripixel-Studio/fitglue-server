package strava_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/pkg/integrations/strava"
)

func stravaAllServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
}

// TestAllStravaMethods calls ALL remaining strava client methods.
func TestAllStravaMethods(t *testing.T) {
	srv := stravaAllServer()
	defer srv.Close()

	c, err := strava.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx := context.Background()

	testCases := []struct {
		name string
		fn   func() (*http.Response, error)
	}{
		// Activities
		{"GetActivityById", func() (*http.Response, error) {
			p := &strava.GetActivityByIdParams{}
			return c.GetActivityById(ctx, 123456, p)
		}},
		{"GetCommentsByActivityId", func() (*http.Response, error) {
			p := &strava.GetCommentsByActivityIdParams{}
			return c.GetCommentsByActivityId(ctx, 123456, p)
		}},
		{"GetKudoersByActivityId", func() (*http.Response, error) {
			p := &strava.GetKudoersByActivityIdParams{}
			return c.GetKudoersByActivityId(ctx, 123456, p)
		}},
		{"GetLapsByActivityId", func() (*http.Response, error) {
			return c.GetLapsByActivityId(ctx, 123456)
		}},
		{"GetActivityStreams", func() (*http.Response, error) {
			p := &strava.GetActivityStreamsParams{Keys: []strava.GetActivityStreamsParamsKeys{"time"}, KeyByType: true}
			return c.GetActivityStreams(ctx, 123456, p)
		}},
		{"GetZonesByActivityId", func() (*http.Response, error) {
			return c.GetZonesByActivityId(ctx, 123456)
		}},
		// Athlete
		{"GetLoggedInAthlete", func() (*http.Response, error) {
			return c.GetLoggedInAthlete(ctx)
		}},
		{"GetLoggedInAthleteActivities", func() (*http.Response, error) {
			p := &strava.GetLoggedInAthleteActivitiesParams{}
			return c.GetLoggedInAthleteActivities(ctx, p)
		}},
		{"GetLoggedInAthleteClubs", func() (*http.Response, error) {
			p := &strava.GetLoggedInAthleteClubsParams{}
			return c.GetLoggedInAthleteClubs(ctx, p)
		}},
		{"GetLoggedInAthleteZones", func() (*http.Response, error) {
			return c.GetLoggedInAthleteZones(ctx)
		}},
		{"GetStats", func() (*http.Response, error) {
			return c.GetStats(ctx, 789)
		}},
		{"GetRoutesByAthleteId", func() (*http.Response, error) {
			p := &strava.GetRoutesByAthleteIdParams{}
			return c.GetRoutesByAthleteId(ctx, 789, p)
		}},
		// Clubs
		{"GetClubById", func() (*http.Response, error) {
			return c.GetClubById(ctx, 456)
		}},
		// Gear
		{"GetGearById", func() (*http.Response, error) {
			return c.GetGearById(ctx, "g12345")
		}},
		// Routes
		{"GetRouteById", func() (*http.Response, error) {
			return c.GetRouteById(ctx, 789)
		}},
		{"GetRouteAsGPX", func() (*http.Response, error) {
			return c.GetRouteAsGPX(ctx, 789)
		}},
		{"GetRouteAsTCX", func() (*http.Response, error) {
			return c.GetRouteAsTCX(ctx, 789)
		}},
		{"GetRouteStreams", func() (*http.Response, error) {
			return c.GetRouteStreams(ctx, 789)
		}},
		// Segments
		{"GetEffortsBySegmentId", func() (*http.Response, error) {
			p := &strava.GetEffortsBySegmentIdParams{SegmentId: 111}
			return c.GetEffortsBySegmentId(ctx, p)
		}},
		{"GetSegmentEffortById", func() (*http.Response, error) {
			return c.GetSegmentEffortById(ctx, 222)
		}},
		{"GetSegmentEffortStreams", func() (*http.Response, error) {
			p := &strava.GetSegmentEffortStreamsParams{
				Keys:      []strava.GetSegmentEffortStreamsParamsKeys{"time"},
				KeyByType: true,
			}
			return c.GetSegmentEffortStreams(ctx, 222, p)
		}},
		{"GetLoggedInAthleteStarredSegments", func() (*http.Response, error) {
			p := &strava.GetLoggedInAthleteStarredSegmentsParams{}
			return c.GetLoggedInAthleteStarredSegments(ctx, p)
		}},
		{"GetSegmentById", func() (*http.Response, error) {
			return c.GetSegmentById(ctx, 111)
		}},
		{"GetSegmentStreams", func() (*http.Response, error) {
			p := &strava.GetSegmentStreamsParams{
				Keys:      []strava.GetSegmentStreamsParamsKeys{"latlng"},
				KeyByType: true,
			}
			return c.GetSegmentStreams(ctx, 111, p)
		}},
		// Uploads
		{"GetUploadById", func() (*http.Response, error) {
			return c.GetUploadById(ctx, 99)
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.fn()
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
				return
			}
			if resp == nil {
				t.Errorf("%s: expected non-nil response", tc.name)
			}
		})
	}
}

func TestStravaIntegrationsNewClientWithResponses(t *testing.T) {
	srv := stravaAllServer()
	defer srv.Close()

	c, err := strava.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("NewClientWithResponses failed: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}
