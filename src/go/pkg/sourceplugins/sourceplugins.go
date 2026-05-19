// nolint:proto-json
package sourceplugins

import (
	"context"
	"time"

	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// SourceActivity is a lightweight summary of an activity returned by ListActivities.
type SourceActivity struct {
	ID        string
	Title     string
	Type      string
	StartTime time.Time
}

// Provider can list historical activities and fetch individual ones for a given source.
type Provider interface {
	ListActivities(ctx context.Context, integ *userpb.UserIntegrations, pageToken string) ([]*SourceActivity, string, error)
	FetchActivity(ctx context.Context, integ *userpb.UserIntegrations, userID string, sourceActivityID string) (*pbevents.ActivityPayload, error)
}

var registry = map[string]Provider{}

func register(source string, p Provider) {
	registry[source] = p
}

// ForSource returns the Provider for the given source enum string (e.g. "SOURCE_HEVY").
// Returns (nil, false) if the source does not support historical sync.
func ForSource(source string) (Provider, bool) {
	p, ok := registry[source]
	return p, ok
}
