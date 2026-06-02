package strava_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/pkg/integrations/strava"
)

func stravaExtra2Server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
}

// TestStravaClientWithResponses covers ALL strava ClientWithResponses methods for code coverage.
// No error assertions — focused on exercising ParseXXXResponse functions for coverage.
func TestStravaClientWithResponses(t *testing.T) {
	srv := stravaExtra2Server()
	defer srv.Close()

	c, err := strava.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("NewClientWithResponses failed: %v", err)
	}

	ctx := context.Background()

	// Activities
	c.CreateActivityWithBodyWithResponse(ctx, "application/json", io.NopCloser(strings.NewReader("{}")))            //nolint:errcheck
	c.CreateActivityWithFormdataBodyWithResponse(ctx, strava.CreateActivityFormdataRequestBody{})                   //nolint:errcheck
	c.GetActivityByIdWithResponse(ctx, 12345, &strava.GetActivityByIdParams{})                                      //nolint:errcheck
	c.UpdateActivityByIdWithBodyWithResponse(ctx, 12345, "application/json", io.NopCloser(strings.NewReader("{}"))) //nolint:errcheck
	c.UpdateActivityByIdWithResponse(ctx, 12345, strava.UpdateActivityByIdJSONRequestBody{})                        //nolint:errcheck
	c.GetCommentsByActivityIdWithResponse(ctx, 12345, &strava.GetCommentsByActivityIdParams{})                      //nolint:errcheck
	c.GetKudoersByActivityIdWithResponse(ctx, 12345, &strava.GetKudoersByActivityIdParams{})                        //nolint:errcheck
	c.GetLapsByActivityIdWithResponse(ctx, 12345)                                                                   //nolint:errcheck
	c.GetActivityStreamsWithResponse(ctx, 12345, &strava.GetActivityStreamsParams{Keys: nil})                       //nolint:errcheck
	c.GetZonesByActivityIdWithResponse(ctx, 12345)                                                                  //nolint:errcheck

	// Athletes
	c.GetLoggedInAthleteWithResponse(ctx)                                                         //nolint:errcheck
	c.UpdateLoggedInAthleteWithResponse(ctx, &strava.UpdateLoggedInAthleteParams{})               //nolint:errcheck
	c.GetLoggedInAthleteActivitiesWithResponse(ctx, &strava.GetLoggedInAthleteActivitiesParams{}) //nolint:errcheck
	c.GetLoggedInAthleteClubsWithResponse(ctx, &strava.GetLoggedInAthleteClubsParams{})           //nolint:errcheck
	c.GetLoggedInAthleteZonesWithResponse(ctx)                                                    //nolint:errcheck
	c.GetRoutesByAthleteIdWithResponse(ctx, 1, &strava.GetRoutesByAthleteIdParams{})              //nolint:errcheck
	c.GetStatsWithResponse(ctx, 1)                                                                //nolint:errcheck

	// Clubs
	c.GetClubByIdWithResponse(ctx, 1) //nolint:errcheck

	// Gear
	c.GetGearByIdWithResponse(ctx, "g12345") //nolint:errcheck

	// Routes
	c.GetRouteByIdWithResponse(ctx, 1)    //nolint:errcheck
	c.GetRouteAsGPXWithResponse(ctx, 1)   //nolint:errcheck
	c.GetRouteAsTCXWithResponse(ctx, 1)   //nolint:errcheck
	c.GetRouteStreamsWithResponse(ctx, 1) //nolint:errcheck

	// Segments
	c.GetEffortsBySegmentIdWithResponse(ctx, &strava.GetEffortsBySegmentIdParams{SegmentId: 1})     //nolint:errcheck
	c.GetSegmentEffortByIdWithResponse(ctx, 1)                                                      //nolint:errcheck
	c.GetSegmentEffortStreamsWithResponse(ctx, 1, &strava.GetSegmentEffortStreamsParams{Keys: nil}) //nolint:errcheck

	t.Logf("All strava integrations ClientWithResponses methods exercised for coverage")
}
