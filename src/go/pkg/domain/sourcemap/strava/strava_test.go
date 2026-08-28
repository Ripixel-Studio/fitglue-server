package strava

import (
	"testing"

	stravaapi "github.com/fitglue/server/src/go/pkg/api/strava"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func TestMapActivityType_SportTypes(t *testing.T) {
	cases := map[string]activitypb.ActivityType{
		"Run":                           activitypb.ActivityType_ACTIVITY_TYPE_RUN,
		"HighIntensityIntervalTraining": activitypb.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING,
		"Pilates":                       activitypb.ActivityType_ACTIVITY_TYPE_PILATES,
		"TrailRun":                      activitypb.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
		"GravelRide":                    activitypb.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE,
		"NotAThing":                     activitypb.ActivityType_ACTIVITY_TYPE_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := MapActivityType(stravaapi.ActivityType(in)); got != want {
			t.Errorf("MapActivityType(%q) = %v, want %v", in, got, want)
		}
	}
}
