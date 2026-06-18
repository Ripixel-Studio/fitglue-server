package sourceplugins

import (
	"context"
	"testing"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// configuredIntegrations returns a UserIntegrations with credentials set for all
// providers, so calls get past the "not configured" guard into the client/request
// building code.
func configuredIntegrations() *pbuser.UserIntegrations {
	return &pbuser.UserIntegrations{
		Strava:    &pbuser.StravaIntegration{AccessToken: "tok"},
		Hevy:      &pbuser.HevyIntegration{ApiKey: "key"},
		Fitbit:    &pbuser.FitbitIntegration{AccessToken: "tok"},
		Intervals: &pbuser.IntervalsIntegration{ApiKey: "key", AthleteId: "i123"},
	}
}

// cancelled returns an already-cancelled context so the underlying HTTP calls
// fail fast without reaching the real provider APIs.
func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestProviders_ConfiguredButCancelled(t *testing.T) {
	integ := configuredIntegrations()
	ctx := cancelled()

	for _, src := range []string{"SOURCE_STRAVA", "SOURCE_HEVY", "SOURCE_FITBIT", "SOURCE_INTERVALS"} {
		p, ok := ForSource(src)
		if !ok {
			t.Fatalf("%s not registered", src)
		}
		// These get past credential validation and fail on the cancelled HTTP call.
		if _, _, err := p.ListActivities(ctx, integ, ""); err == nil {
			t.Errorf("%s ListActivities: expected error", src)
		}
		if _, err := p.FetchActivity(ctx, integ, "user-1", "12345"); err == nil {
			t.Errorf("%s FetchActivity: expected error", src)
		}
	}
}

func TestStravaFetchActivity_InvalidID(t *testing.T) {
	p, _ := ForSource("SOURCE_STRAVA")
	// Non-numeric id is rejected before any HTTP call.
	if _, err := p.FetchActivity(context.Background(), configuredIntegrations(), "user-1", "not-a-number"); err == nil {
		t.Error("expected invalid-id error")
	}
}

func TestProviders_ListActivities_WithPageToken(t *testing.T) {
	integ := configuredIntegrations()
	ctx := cancelled()
	for _, src := range []string{"SOURCE_STRAVA", "SOURCE_HEVY", "SOURCE_FITBIT", "SOURCE_INTERVALS"} {
		p, _ := ForSource(src)
		// Exercise the pageToken parsing branch.
		_, _, _ = p.ListActivities(ctx, integ, "2")
	}
}
