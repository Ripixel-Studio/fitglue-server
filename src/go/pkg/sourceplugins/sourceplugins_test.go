package sourceplugins

import (
	"context"
	"testing"

	userpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func TestForSource(t *testing.T) {
	for _, src := range []string{"SOURCE_STRAVA", "SOURCE_HEVY", "SOURCE_FITBIT", "SOURCE_INTERVALS"} {
		if _, ok := ForSource(src); !ok {
			t.Errorf("expected provider registered for %s", src)
		}
	}
	if _, ok := ForSource("SOURCE_UNKNOWN"); ok {
		t.Error("expected no provider for unknown source")
	}
}

// TestProviders_NotConfigured verifies each provider returns an error when the
// relevant integration credentials are missing, without making any network call.
func TestProviders_NotConfigured(t *testing.T) {
	ctx := context.Background()
	empty := &userpb.UserIntegrations{}

	for _, src := range []string{"SOURCE_STRAVA", "SOURCE_HEVY", "SOURCE_FITBIT", "SOURCE_INTERVALS"} {
		p, ok := ForSource(src)
		if !ok {
			t.Fatalf("provider %s not registered", src)
		}
		t.Run(src+"/ListActivities", func(t *testing.T) {
			if _, _, err := p.ListActivities(ctx, empty, ""); err == nil {
				t.Errorf("%s: expected error when not configured", src)
			}
		})
		t.Run(src+"/FetchActivity", func(t *testing.T) {
			if _, err := p.FetchActivity(ctx, empty, "user-1", "act-1"); err == nil {
				t.Errorf("%s: expected error when not configured", src)
			}
		})
	}
}
