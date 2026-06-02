package strava

import (
	"encoding/json"
	"fmt"

	stravaapi "github.com/fitglue/server/src/go/pkg/integrations/strava"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func mapToStandardizedActivity(rawJSON []byte, userID string) (*activitypb.StandardizedActivity, error) {
	var activity stravaapi.DetailedActivity
	if err := json.Unmarshal(rawJSON, &activity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal strava activity: %w", err)
	}

	act := &activitypb.StandardizedActivity{
		Source: activitypb.ActivitySource_SOURCE_STRAVA,
		UserId: userID,
	}

	if activity.Id != nil {
		act.ExternalId = fmt.Sprintf("%d", *activity.Id)
	}
	if activity.Name != nil {
		act.Name = *activity.Name
	}
	if activity.Description != nil {
		act.Description = *activity.Description
	}

	sportType := activity.SportType
	if sportType == nil {
		sportType = activity.Type
	}
	if sportType != nil {
		act.Type = mapActivityType(*sportType)
	}

	session := &activitypb.Session{}

	if activity.StartDate != nil {
		act.StartTime = timestamppb.New(*activity.StartDate)
		session.StartTime = timestamppb.New(*activity.StartDate)
	}
	if activity.ElapsedTime != nil {
		session.TotalElapsedTime = float64(*activity.ElapsedTime)
	}
	if activity.Distance != nil {
		session.TotalDistance = float64(*activity.Distance)
	}
	if activity.Calories != nil {
		cal := float64(*activity.Calories)
		session.TotalCalories = &cal
	}

	if activity.Laps != nil {
		for _, l := range *activity.Laps {
			lap := &activitypb.Lap{}
			if l.StartDate != nil {
				lap.StartTime = timestamppb.New(*l.StartDate)
			}
			if l.ElapsedTime != nil {
				lap.TotalElapsedTime = float64(*l.ElapsedTime)
			}
			if l.Distance != nil {
				lap.TotalDistance = float64(*l.Distance)
			}
			session.Laps = append(session.Laps, lap)
		}
	}

	act.Sessions = []*activitypb.Session{session}
	return act, nil
}

var stravaTypeMap = map[stravaapi.ActivityType]activitypb.ActivityType{
	stravaapi.ActivityTypeAlpineSki:       activitypb.ActivityType_ACTIVITY_TYPE_ALPINE_SKI,
	stravaapi.ActivityTypeBackcountrySki:  activitypb.ActivityType_ACTIVITY_TYPE_BACKCOUNTRY_SKI,
	stravaapi.ActivityTypeCanoeing:        activitypb.ActivityType_ACTIVITY_TYPE_CANOEING,
	stravaapi.ActivityTypeCrossfit:        activitypb.ActivityType_ACTIVITY_TYPE_CROSSFIT,
	stravaapi.ActivityTypeEBikeRide:       activitypb.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE,
	stravaapi.ActivityTypeElliptical:      activitypb.ActivityType_ACTIVITY_TYPE_ELLIPTICAL,
	stravaapi.ActivityTypeGolf:            activitypb.ActivityType_ACTIVITY_TYPE_GOLF,
	stravaapi.ActivityTypeHandcycle:       activitypb.ActivityType_ACTIVITY_TYPE_HANDCYCLE,
	stravaapi.ActivityTypeHike:            activitypb.ActivityType_ACTIVITY_TYPE_HIKE,
	stravaapi.ActivityTypeIceSkate:        activitypb.ActivityType_ACTIVITY_TYPE_ICE_SKATE,
	stravaapi.ActivityTypeInlineSkate:     activitypb.ActivityType_ACTIVITY_TYPE_INLINE_SKATE,
	stravaapi.ActivityTypeKayaking:        activitypb.ActivityType_ACTIVITY_TYPE_KAYAKING,
	stravaapi.ActivityTypeKitesurf:        activitypb.ActivityType_ACTIVITY_TYPE_KITESURF,
	stravaapi.ActivityTypeNordicSki:       activitypb.ActivityType_ACTIVITY_TYPE_NORDIC_SKI,
	stravaapi.ActivityTypeRide:            activitypb.ActivityType_ACTIVITY_TYPE_RIDE,
	stravaapi.ActivityTypeRockClimbing:    activitypb.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING,
	stravaapi.ActivityTypeRollerSki:       activitypb.ActivityType_ACTIVITY_TYPE_ROLLER_SKI,
	stravaapi.ActivityTypeRowing:          activitypb.ActivityType_ACTIVITY_TYPE_ROWING,
	stravaapi.ActivityTypeRun:             activitypb.ActivityType_ACTIVITY_TYPE_RUN,
	stravaapi.ActivityTypeSail:            activitypb.ActivityType_ACTIVITY_TYPE_SAIL,
	stravaapi.ActivityTypeSkateboard:      activitypb.ActivityType_ACTIVITY_TYPE_SKATEBOARD,
	stravaapi.ActivityTypeSnowboard:       activitypb.ActivityType_ACTIVITY_TYPE_SNOWBOARD,
	stravaapi.ActivityTypeSnowshoe:        activitypb.ActivityType_ACTIVITY_TYPE_SNOWSHOE,
	stravaapi.ActivityTypeSoccer:          activitypb.ActivityType_ACTIVITY_TYPE_SOCCER,
	stravaapi.ActivityTypeStairStepper:    activitypb.ActivityType_ACTIVITY_TYPE_STAIR_STEPPER,
	stravaapi.ActivityTypeStandUpPaddling: activitypb.ActivityType_ACTIVITY_TYPE_STAND_UP_PADDLING,
	stravaapi.ActivityTypeSurfing:         activitypb.ActivityType_ACTIVITY_TYPE_SURFING,
	stravaapi.ActivityTypeSwim:            activitypb.ActivityType_ACTIVITY_TYPE_SWIM,
	stravaapi.ActivityTypeVelomobile:      activitypb.ActivityType_ACTIVITY_TYPE_VELOMOBILE,
	stravaapi.ActivityTypeVirtualRide:     activitypb.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE,
	stravaapi.ActivityTypeVirtualRun:      activitypb.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN,
	stravaapi.ActivityTypeWalk:            activitypb.ActivityType_ACTIVITY_TYPE_WALK,
	stravaapi.ActivityTypeWeightTraining:  activitypb.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
	stravaapi.ActivityTypeWheelchair:      activitypb.ActivityType_ACTIVITY_TYPE_WHEELCHAIR,
	stravaapi.ActivityTypeWindsurf:        activitypb.ActivityType_ACTIVITY_TYPE_WINDSURF,
	stravaapi.ActivityTypeWorkout:         activitypb.ActivityType_ACTIVITY_TYPE_WORKOUT,
	stravaapi.ActivityTypeYoga:            activitypb.ActivityType_ACTIVITY_TYPE_YOGA,
}

func mapActivityType(t stravaapi.ActivityType) activitypb.ActivityType {
	if mapped, ok := stravaTypeMap[t]; ok {
		return mapped
	}
	return activitypb.ActivityType_ACTIVITY_TYPE_UNSPECIFIED
}
