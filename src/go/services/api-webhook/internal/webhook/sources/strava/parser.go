package strava

import (
	"time"

	stravaapi "github.com/fitglue/server/src/go/pkg/api/strava"
	stravamap "github.com/fitglue/server/src/go/pkg/domain/sourcemap/strava"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// The mapping lives in pkg/domain/sourcemap/strava so the historical-import source
// plugin produces the same activities as the webhook. These wrappers keep this
// package's call sites and tests unchanged.

func mapToStandardizedActivity(rawJSON []byte, userID string, streams *stravaapi.StreamSet) (*activitypb.StandardizedActivity, error) {
	return stravamap.MapToStandardizedActivity(rawJSON, userID, streams)
}

func buildRecordsFromStreams(streams *stravaapi.StreamSet, startTime time.Time) []*activitypb.Record {
	return stravamap.BuildRecordsFromStreams(streams, startTime)
}

func computeSessionHR(streams *stravaapi.StreamSet) (avg int32, max int32, ok bool) {
	return stravamap.ComputeSessionHR(streams)
}

func mapActivityType(t stravaapi.ActivityType) activitypb.ActivityType {
	return stravamap.MapActivityType(t)
}
