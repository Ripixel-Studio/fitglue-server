package strava

import (
	"encoding/json"
	"fmt"
	"time"

	stravaapi "github.com/fitglue/server/src/go/pkg/api/strava"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func mapToStandardizedActivity(rawJSON []byte, userID string, streams *stravaapi.StreamSet) (*activitypb.StandardizedActivity, error) {
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

	var startTime time.Time
	if activity.StartDate != nil {
		startTime = *activity.StartDate
		act.StartTime = timestamppb.New(startTime)
		session.StartTime = timestamppb.New(startTime)
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

	// Build per-second records from streams — enables HR, GPS, speed, cadence, power enrichers
	if records := buildRecordsFromStreams(streams, startTime); len(records) > 0 {
		avgHR, maxHR, hasHR := computeSessionHR(streams)
		if hasHR {
			session.AvgHeartRate = &avgHR
			session.MaxHeartRate = &maxHR
		}
		// Single telemetry lap containing all stream records
		session.Laps = []*activitypb.Lap{
			{
				StartTime:                session.StartTime,
				TotalElapsedTime:         session.TotalElapsedTime,
				TotalDistance:            session.TotalDistance,
				Records:                  records,
				IsTelemetryContainerOnly: true,
			},
		}
	} else if activity.Laps != nil {
		// Fall back to summary laps when no stream data is available
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

func buildRecordsFromStreams(streams *stravaapi.StreamSet, startTime time.Time) []*activitypb.Record {
	if streams == nil || streams.Time == nil || streams.Time.Data == nil {
		return nil
	}
	times := *streams.Time.Data
	n := len(times)
	if n == 0 {
		return nil
	}

	records := make([]*activitypb.Record, n)
	for i := 0; i < n; i++ {
		ts := startTime.Add(time.Duration(times[i]) * time.Second)
		r := &activitypb.Record{
			Timestamp: timestamppb.New(ts),
		}
		if streams.Heartrate != nil && streams.Heartrate.Data != nil && i < len(*streams.Heartrate.Data) {
			r.HeartRate = int32((*streams.Heartrate.Data)[i])
		}
		if streams.VelocitySmooth != nil && streams.VelocitySmooth.Data != nil && i < len(*streams.VelocitySmooth.Data) {
			r.Speed = float64((*streams.VelocitySmooth.Data)[i])
		}
		if streams.Altitude != nil && streams.Altitude.Data != nil && i < len(*streams.Altitude.Data) {
			r.Altitude = float64((*streams.Altitude.Data)[i])
		}
		if streams.Latlng != nil && streams.Latlng.Data != nil && i < len(*streams.Latlng.Data) {
			pos := (*streams.Latlng.Data)[i]
			if len(pos) >= 2 {
				r.PositionLat = float64(pos[0])
				r.PositionLong = float64(pos[1])
			}
		}
		if streams.Cadence != nil && streams.Cadence.Data != nil && i < len(*streams.Cadence.Data) {
			r.Cadence = int32((*streams.Cadence.Data)[i])
		}
		if streams.Watts != nil && streams.Watts.Data != nil && i < len(*streams.Watts.Data) {
			r.Power = int32((*streams.Watts.Data)[i])
		}
		if streams.Distance != nil && streams.Distance.Data != nil && i < len(*streams.Distance.Data) {
			r.Distance = float64((*streams.Distance.Data)[i])
		}
		if streams.Temp != nil && streams.Temp.Data != nil && i < len(*streams.Temp.Data) {
			temp := int32((*streams.Temp.Data)[i])
			r.Temperature = &temp
		}
		records[i] = r
	}
	return records
}

// computeSessionHR derives avg and max heart rate from the heartrate stream.
func computeSessionHR(streams *stravaapi.StreamSet) (avg int32, max int32, ok bool) {
	if streams == nil || streams.Heartrate == nil || streams.Heartrate.Data == nil {
		return 0, 0, false
	}
	data := *streams.Heartrate.Data
	if len(data) == 0 {
		return 0, 0, false
	}
	sum, maxHR := 0, 0
	for _, hr := range data {
		sum += hr
		if hr > maxHR {
			maxHR = hr
		}
	}
	return int32(sum / len(data)), int32(maxHR), true
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
