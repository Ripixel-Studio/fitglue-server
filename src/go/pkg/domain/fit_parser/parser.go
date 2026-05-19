package fit_parser

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FIT message order: FileId -> DeviceInfo -> Records -> Lap -> Session -> Activity
// Records come BEFORE Lap/Session summaries, so we need to collect everything first
// and then organize into the proper hierarchy.

// ParseFitFile parses a FIT file and returns a StandardizedActivity.
// If the file contains multiple sessions, they are merged into a single session.
func ParseFitFile(data []byte) (*pbactivity.StandardizedActivity, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty FIT data")
	}

	fitDec := decoder.New(bytes.NewReader(data))

	// Collect all data first, then organize
	var allRecords []*pbactivity.Record
	var lapInfos []lapInfo
	var sessionInfos []sessionInfo
	var setInfos []setInfo
	var workoutSteps []workoutStepInfo
	var workoutName string

	var activityType pbactivity.ActivityType
	var activityName string
	var startTime time.Time

	for fitDec.Next() {
		fitData, err := fitDec.Decode()
		if err != nil {
			return nil, fmt.Errorf("failed to decode FIT file: %w", err)
		}

		for _, msg := range fitData.Messages {
			switch msg.Num {
			case typedef.MesgNumFileId:
				fileId := mesgdef.NewFileId(&msg)
				if startTime.IsZero() && !fileId.TimeCreated.IsZero() {
					startTime = fileId.TimeCreated.UTC()
				}

			case typedef.MesgNumRecord:
				record := parseRecord(&msg)
				if record != nil {
					allRecords = append(allRecords, record)
					if startTime.IsZero() && record.Timestamp != nil {
						startTime = record.Timestamp.AsTime()
					}
				}

			case typedef.MesgNumLap:
				lapMsg := mesgdef.NewLap(&msg)
				li := lapInfo{
					startTime:        lapMsg.StartTime.UTC(),
					totalElapsedTime: float64(lapMsg.TotalElapsedTime) / 1000,
				}
				if lapMsg.TotalDistance != 0xFFFFFFFF {
					li.totalDistance = float64(lapMsg.TotalDistance) / 100
				}

				// Extract workout step index for auto-detecting lap groups
				if lapMsg.WktStepIndex != typedef.MessageIndexInvalid {
					idx := int32(lapMsg.WktStepIndex)
					li.wktStepIndex = &idx
				}

				// Extract lap trigger for understanding lap boundaries
				if lapMsg.LapTrigger != typedef.LapTriggerInvalid {
					trigger := lapMsg.LapTrigger.String()
					li.lapTrigger = &trigger
				}

				// Extract intensity for interval identification
				if lapMsg.Intensity != typedef.IntensityInvalid {
					intensity := lapMsg.Intensity.String()
					li.intensity = &intensity
				}

				// Extract average heart rate for interval stats
				if lapMsg.AvgHeartRate != 0xFF {
					avgHR := int32(lapMsg.AvgHeartRate)
					li.avgHeartRate = &avgHR
				}

				lapInfos = append(lapInfos, li)

			case typedef.MesgNumSession:
				sessionMsg := mesgdef.NewSession(&msg)
				si := sessionInfo{
					startTime:        sessionMsg.StartTime.UTC(),
					totalElapsedTime: float64(sessionMsg.TotalElapsedTime) / 1000,
					sport:            sessionMsg.Sport,
					subSport:         sessionMsg.SubSport,
					sportProfileName: sessionMsg.SportProfileName,
				}
				if sessionMsg.TotalDistance != 0xFFFFFFFF {
					si.totalDistance = float64(sessionMsg.TotalDistance) / 100
				}
				if sessionMsg.AvgHeartRate != 0xFF {
					v := int32(sessionMsg.AvgHeartRate)
					si.avgHeartRate = &v
				}
				if sessionMsg.MaxHeartRate != 0xFF {
					v := int32(sessionMsg.MaxHeartRate)
					si.maxHeartRate = &v
				}
				if sessionMsg.MinHeartRate != 0xFF {
					v := int32(sessionMsg.MinHeartRate)
					si.minHeartRate = &v
				}
				sessionInfos = append(sessionInfos, si)

				// Set activity type from first session
				if activityType == pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
					activityType = mapFitSportToActivityType(sessionMsg.Sport, sessionMsg.SubSport)
				}
				if activityName == "" && sessionMsg.SportProfileName != "" {
					activityName = sessionMsg.SportProfileName
				}

			case typedef.MesgNumSet:
				setMsg := mesgdef.NewSet(&msg)
				// Skip rest sets (only process active exercise sets)
				if setMsg.SetType == typedef.SetTypeRest {
					continue
				}
				si := setInfo{
					startTime:    setMsg.StartTime.UTC(),
					duration:     setMsg.DurationScaled(),
					reps:         int32(setMsg.Repetitions),
					weightKg:     setMsg.WeightScaled(),
					wktStepIndex: int32(setMsg.WktStepIndex),
				}
				// Extract exercise name from category
				if len(setMsg.Category) > 0 {
					si.exerciseName = formatExerciseCategory(setMsg.Category[0])
				}
				setInfos = append(setInfos, si)

			case typedef.MesgNumWorkout:
				wktMsg := mesgdef.NewWorkout(&msg)
				if wktMsg.WktName != "" {
					workoutName = wktMsg.WktName
				}

			case typedef.MesgNumWorkoutStep:
				stepMsg := mesgdef.NewWorkoutStep(&msg)
				wsi := workoutStepInfo{
					durationType: stepMsg.DurationType.String(),
				}
				if stepMsg.Intensity != typedef.IntensityInvalid {
					wsi.intensity = stepMsg.Intensity.String()
				}
				// Duration value depends on type
				if stepMsg.DurationValue != 0xFFFFFFFF {
					wsi.durationValue = stepMsg.DurationValue
				}
				// Target type and range
				if stepMsg.TargetType != typedef.WktStepTargetInvalid {
					wsi.targetType = stepMsg.TargetType.String()
				}
				if stepMsg.CustomTargetValueLow != 0xFFFFFFFF {
					wsi.targetLow = stepMsg.CustomTargetValueLow
				}
				if stepMsg.CustomTargetValueHigh != 0xFFFFFFFF {
					wsi.targetHigh = stepMsg.CustomTargetValueHigh
				}
				workoutSteps = append(workoutSteps, wsi)
			}
		}
	}

	// Build sessions from collected data
	sessions := buildSessions(allRecords, lapInfos, sessionInfos, setInfos)

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found in FIT file")
	}

	// Merge multiple sessions into one
	var mergedSession *pbactivity.Session
	if len(sessions) > 1 {
		mergedSession = MergeSessions(sessions)
	} else {
		mergedSession = sessions[0]
	}

	// Generate activity name if not set
	if activityName == "" {
		activityName = generateActivityName(activityType, startTime)
	}

	activity := &pbactivity.StandardizedActivity{
		Source:      pbactivity.ActivitySource_SOURCE_FILE_UPLOAD,
		StartTime:   timestamppb.New(startTime),
		Name:        activityName,
		Type:        activityType,
		Sessions:    []*pbactivity.Session{mergedSession},
		TimeMarkers: generateExerciseTimeMarkers(setInfos),
	}

	// Populate workout definition if present
	if workoutName != "" || len(workoutSteps) > 0 {
		wkDefn := &pbactivity.WorkoutDefinition{}
		if workoutName != "" {
			wkDefn.Name = workoutName
		}
		for _, wsi := range workoutSteps {
			wkDefn.Steps = append(wkDefn.Steps, &pbactivity.WorkoutStep{
				Intensity:     wsi.intensity,
				DurationType:  wsi.durationType,
				DurationValue: wsi.durationValue,
				TargetType:    wsi.targetType,
				TargetLow:     wsi.targetLow,
				TargetHigh:    wsi.targetHigh,
			})
		}
		activity.Workout = wkDefn
	}

	return activity, nil
}

type lapInfo struct {
	startTime        time.Time
	totalElapsedTime float64
	totalDistance    float64
	wktStepIndex     *int32  // Workout step index - laps with same index belong together
	lapTrigger       *string // What triggered this lap (manual, distance, time, etc.)
	intensity        *string // Interval intensity: "warmup", "active", "recovery", "cooldown"
	avgHeartRate     *int32  // Average heart rate for the lap
}

type workoutStepInfo struct {
	intensity     string
	durationType  string
	durationValue uint32
	targetType    string
	targetLow     uint32
	targetHigh    uint32
}

type sessionInfo struct {
	startTime        time.Time
	totalElapsedTime float64
	totalDistance    float64
	sport            typedef.Sport
	subSport         typedef.SubSport
	sportProfileName string
	avgHeartRate     *int32
	maxHeartRate     *int32
	minHeartRate     *int32
}

type setInfo struct {
	startTime    time.Time
	duration     float64 // seconds
	reps         int32
	weightKg     float64
	exerciseName string
	wktStepIndex int32
}

// buildSessions organizes records into laps and sessions based on timestamps
func buildSessions(records []*pbactivity.Record, lapInfos []lapInfo, sessionInfos []sessionInfo, setInfos []setInfo) []*pbactivity.Session {
	if len(records) == 0 {
		return nil
	}

	// If no session info, create a synthetic session with all records
	if len(sessionInfos) == 0 {
		var duration float64
		if len(records) > 1 {
			first := records[0].Timestamp.AsTime()
			last := records[len(records)-1].Timestamp.AsTime()
			duration = last.Sub(first).Seconds()
		}

		lap := &pbactivity.Lap{
			StartTime:        records[0].Timestamp,
			TotalElapsedTime: duration,
			Records:          records,
		}

		return []*pbactivity.Session{{
			StartTime:        records[0].Timestamp,
			TotalElapsedTime: duration,
			Laps:             []*pbactivity.Lap{lap},
		}}
	}

	// If no lap info, create one lap per session containing all records
	if len(lapInfos) == 0 {
		session := &sessionInfos[0]
		lap := &pbactivity.Lap{
			StartTime:        timestamppb.New(session.startTime),
			TotalElapsedTime: session.totalElapsedTime,
			TotalDistance:    session.totalDistance,
			Records:          records,
		}

		return []*pbactivity.Session{{
			StartTime:        timestamppb.New(session.startTime),
			TotalElapsedTime: session.totalElapsedTime,
			TotalDistance:    session.totalDistance,
			Laps:             []*pbactivity.Lap{lap},
		}}
	}

	// Merge consecutive laps with the same workout step index
	// This handles cases where Garmin records multiple sub-laps per interval (e.g., 1km auto-laps within a Hyrox station)
	mergedLapInfos := mergeLapInfosByWorkoutStep(lapInfos)

	// Assign records to laps based on timestamps
	laps := make([]*pbactivity.Lap, len(mergedLapInfos))
	for i, li := range mergedLapInfos {
		laps[i] = &pbactivity.Lap{
			StartTime:        timestamppb.New(li.startTime),
			TotalElapsedTime: li.totalElapsedTime,
			TotalDistance:    li.totalDistance,
			Records:          []*pbactivity.Record{},
		}
		// Preserve interval intensity on the proto Lap
		if li.intensity != nil {
			laps[i].Intensity = *li.intensity
		}
	}

	// Assign each record to the appropriate lap (using merged lap infos)
	for _, record := range records {
		rt := record.Timestamp.AsTime()
		assigned := false

		for i := len(mergedLapInfos) - 1; i >= 0; i-- {
			lapStart := mergedLapInfos[i].startTime
			lapEnd := lapStart.Add(time.Duration(mergedLapInfos[i].totalElapsedTime) * time.Second)

			// Record belongs to this lap if it falls within its time range
			if (rt.Equal(lapStart) || rt.After(lapStart)) && (rt.Before(lapEnd) || rt.Equal(lapEnd)) {
				laps[i].Records = append(laps[i].Records, record)
				assigned = true
				break
			}
		}

		// If not assigned (edge case), put in first lap that starts before this record
		if !assigned {
			for i := len(mergedLapInfos) - 1; i >= 0; i-- {
				if !rt.Before(mergedLapInfos[i].startTime) {
					laps[i].Records = append(laps[i].Records, record)
					assigned = true
					break
				}
			}
			// Last resort: first lap
			if !assigned && len(laps) > 0 {
				laps[0].Records = append(laps[0].Records, record)
			}
		}
	}

	// Build sessions with laps
	sessions := make([]*pbactivity.Session, len(sessionInfos))
	for i, si := range sessionInfos {
		sessions[i] = &pbactivity.Session{
			StartTime:        timestamppb.New(si.startTime),
			TotalElapsedTime: si.totalElapsedTime,
			TotalDistance:    si.totalDistance,
			Laps:             []*pbactivity.Lap{},
			AvgHeartRate:     si.avgHeartRate,
			MaxHeartRate:     si.maxHeartRate,
			MinHeartRate:     si.minHeartRate,
		}
	}

	// Assign laps to sessions based on lap start time
	for _, lap := range laps {
		lapStart := lap.StartTime.AsTime()
		assigned := false

		for i := len(sessionInfos) - 1; i >= 0; i-- {
			sessionStart := sessionInfos[i].startTime
			sessionEnd := sessionStart.Add(time.Duration(sessionInfos[i].totalElapsedTime) * time.Second)

			if (lapStart.Equal(sessionStart) || lapStart.After(sessionStart)) &&
				(lapStart.Before(sessionEnd) || lapStart.Equal(sessionEnd)) {
				sessions[i].Laps = append(sessions[i].Laps, lap)
				assigned = true
				break
			}
		}

		// Fallback: assign to last session that starts before this lap
		if !assigned {
			for i := len(sessionInfos) - 1; i >= 0; i-- {
				if !lapStart.Before(sessionInfos[i].startTime) {
					sessions[i].Laps = append(sessions[i].Laps, lap)
					assigned = true
					break
				}
			}
			if !assigned && len(sessions) > 0 {
				sessions[0].Laps = append(sessions[0].Laps, lap)
			}
		}
	}

	// Convert setInfos to StrengthSets and assign to sessions
	for _, si := range setInfos {
		strengthSet := &pbactivity.StrengthSet{
			ExerciseName:    si.exerciseName,
			Reps:            si.reps,
			DurationSeconds: int32(si.duration),
			StartTime:       timestamppb.New(si.startTime),
		}
		// Only set weight if it's a valid number (NaN from FIT means no weight)
		if si.weightKg == si.weightKg { // NaN != NaN check
			strengthSet.WeightKg = si.weightKg
		}

		// Assign to session based on timestamp
		assigned := false
		for i := len(sessions) - 1; i >= 0; i-- {
			sessionStart := sessions[i].StartTime.AsTime()
			sessionEnd := sessionStart.Add(time.Duration(sessions[i].TotalElapsedTime) * time.Second)

			if (si.startTime.Equal(sessionStart) || si.startTime.After(sessionStart)) &&
				(si.startTime.Before(sessionEnd) || si.startTime.Equal(sessionEnd)) {
				sessions[i].StrengthSets = append(sessions[i].StrengthSets, strengthSet)
				assigned = true
				break
			}
		}
		// Fallback: assign to first session
		if !assigned && len(sessions) > 0 {
			sessions[0].StrengthSets = append(sessions[0].StrengthSets, strengthSet)
		}
	}

	return sessions
}

// mergeLapInfosByWorkoutStep merges consecutive laps that share the same workout step index.
// This handles Garmin's behavior of recording multiple sub-laps per workout interval
// (e.g., 1km auto-laps within a Hyrox station that should be a single interval).
// Also merges zero-duration laps with adjacent laps (these are transition markers).
func mergeLapInfosByWorkoutStep(lapInfos []lapInfo) []lapInfo {
	if len(lapInfos) == 0 {
		return lapInfos
	}

	// Check if any laps have workout step data
	hasStepData := false
	for _, li := range lapInfos {
		if li.wktStepIndex != nil {
			hasStepData = true
			break
		}
	}
	if !hasStepData {
		// No workout step data - still merge zero-duration laps
		return mergeZeroDurationLaps(lapInfos)
	}

	var result []lapInfo
	var currentGroup []lapInfo
	var currentStepIndex *int32

	for _, li := range lapInfos {
		// Very short laps (< 2 seconds) are transition markers - always merge with current group
		isTransitionLap := li.totalElapsedTime < 2.0

		// Check if this lap continues the current group (same workout step OR transition lap)
		sameStep := currentStepIndex != nil && li.wktStepIndex != nil && *li.wktStepIndex == *currentStepIndex
		if sameStep || (isTransitionLap && len(currentGroup) > 0) {
			// Same step index or transition lap - add to current group
			currentGroup = append(currentGroup, li)
		} else {
			// Different step index - finalize current group
			if len(currentGroup) > 0 {
				result = append(result, mergeConsecutiveLapInfos(currentGroup))
			}
			// Start new group
			currentGroup = []lapInfo{li}
			currentStepIndex = li.wktStepIndex
		}
	}

	// Don't forget the last group
	if len(currentGroup) > 0 {
		result = append(result, mergeConsecutiveLapInfos(currentGroup))
	}

	return result
}

// mergeTransitionLaps merges very short laps (< 2 seconds) with their preceding lap.
// Used when there's no workout step data but we still want to clean up transition markers.
func mergeZeroDurationLaps(lapInfos []lapInfo) []lapInfo {
	if len(lapInfos) == 0 {
		return lapInfos
	}

	var result []lapInfo
	for i, li := range lapInfos {
		isTransition := li.totalElapsedTime < 2.0
		if isTransition && len(result) > 0 {
			// Merge transition lap with previous
			result[len(result)-1] = mergeConsecutiveLapInfos([]lapInfo{result[len(result)-1], li})
		} else if isTransition && i < len(lapInfos)-1 {
			// Transition at start - will be merged when we add the next lap
			// Skip for now, handle in next iteration
			continue
		} else {
			result = append(result, li)
		}
	}

	return result
}

// mergeConsecutiveLapInfos merges a group of consecutive lap infos into a single lap info.
// Uses the start time of the first lap, and sums the elapsed time and distance.
func mergeConsecutiveLapInfos(group []lapInfo) lapInfo {
	if len(group) == 0 {
		return lapInfo{}
	}
	if len(group) == 1 {
		return group[0]
	}

	merged := lapInfo{
		startTime:        group[0].startTime,
		totalElapsedTime: 0,
		totalDistance:    0,
		wktStepIndex:     group[0].wktStepIndex,
		lapTrigger:       group[0].lapTrigger,
		intensity:        group[0].intensity,
		avgHeartRate:     group[0].avgHeartRate,
	}

	for _, li := range group {
		merged.totalElapsedTime += li.totalElapsedTime
		merged.totalDistance += li.totalDistance
	}

	return merged
}

// parseRecord extracts record data from a FIT message
func parseRecord(msg *proto.Message) *pbactivity.Record {
	recordMsg := mesgdef.NewRecord(msg)

	ts := recordMsg.Timestamp
	if ts.IsZero() {
		return nil
	}

	record := &pbactivity.Record{
		Timestamp: timestamppb.New(ts.UTC()),
	}

	// Heart rate
	if recordMsg.HeartRate != 0xFF { // 0xFF is invalid
		record.HeartRate = int32(recordMsg.HeartRate)
	}

	// Power
	if recordMsg.Power != 0xFFFF { // 0xFFFF is invalid
		record.Power = int32(recordMsg.Power)
	}

	// Cadence
	if recordMsg.Cadence != 0xFF {
		record.Cadence = int32(recordMsg.Cadence)
	}

	// Speed (FIT uses mm/s, we want m/s)
	// Fallback to EnhancedSpeed if Speed is 0xFFFF (invalid)
	if recordMsg.Speed != 0xFFFF {
		record.Speed = float64(recordMsg.Speed) / 1000
	} else if recordMsg.EnhancedSpeed != 0xFFFFFFFF {
		record.Speed = float64(recordMsg.EnhancedSpeed) / 1000
	}

	// Altitude (FIT uses 5 * (altitude + 500) scale)
	// Fallback to EnhancedAltitude if Altitude is 0xFFFF
	if recordMsg.Altitude != 0xFFFF {
		record.Altitude = (float64(recordMsg.Altitude) / 5) - 500
	} else if recordMsg.EnhancedAltitude != 0xFFFFFFFF {
		record.Altitude = (float64(recordMsg.EnhancedAltitude) / 5) - 500
	}

	// Position (FIT uses semicircles, convert to decimal degrees)
	if recordMsg.PositionLat != 0x7FFFFFFF && recordMsg.PositionLong != 0x7FFFFFFF {
		const semicircleConst = 11930464.7111 // 2^31 / 180
		record.PositionLat = float64(recordMsg.PositionLat) / semicircleConst
		record.PositionLong = float64(recordMsg.PositionLong) / semicircleConst
	}

	// Garmin Running Dynamics
	// Stance Time (Ground Contact Time) - FIT unit is 0.1ms
	if recordMsg.StanceTime != 0xFFFF {
		gct := int32(recordMsg.StanceTime / 10) // Convert to ms
		record.GroundContactTime = &gct
	}

	// Vertical Oscillation - FIT unit is 0.1mm
	if recordMsg.VerticalOscillation != 0xFFFF {
		vo := int32(recordMsg.VerticalOscillation / 10) // Convert to mm
		record.VerticalOscillation = &vo
	}

	// Vertical Ratio - FIT unit is 0.01%
	if recordMsg.VerticalRatio != 0xFFFF {
		vr := int32(recordMsg.VerticalRatio / 10) // Convert to 0.1% (or keep as int?)
		// Actually, let's store it such that it matches the user's "8.4%"
		// If FIT gives 840 for 8.4%, then recordMsg.VerticalRatio / 10 is 84.
		// Let's check the user screenshot. It says 8.4%.
		// If we store 84, we can format as 8.4 in the UI.
		record.VerticalRatio = &vr
	}

	// Step Length (Stride Length) - FIT unit is mm
	if recordMsg.StepLength != 0xFFFF {
		sl := float64(recordMsg.StepLength) / 1000 // Convert to meters
		record.StepLength = &sl
	}

	// Distance (cumulative from activity start)
	// FIT stores as uint32 with scale 100 (units: meters)
	// DistanceScaled() returns float64 in meters
	if recordMsg.Distance != 0xFFFFFFFF {
		record.Distance = recordMsg.DistanceScaled()
	}

	return record
}

// MergeSessions merges multiple sessions into a single session.
// This is useful for FIT files that contain multiple sessions from device auto-pause.
func MergeSessions(sessions []*pbactivity.Session) *pbactivity.Session {
	if len(sessions) == 0 {
		return nil
	}
	if len(sessions) == 1 {
		return sessions[0]
	}

	merged := &pbactivity.Session{
		StartTime:        sessions[0].StartTime,
		TotalElapsedTime: 0,
		TotalDistance:    0,
		Laps:             make([]*pbactivity.Lap, 0),
		StrengthSets:     make([]*pbactivity.StrengthSet, 0),
	}

	for _, session := range sessions {
		merged.TotalElapsedTime += session.TotalElapsedTime
		merged.TotalDistance += session.TotalDistance
		merged.Laps = append(merged.Laps, session.Laps...)
		merged.StrengthSets = append(merged.StrengthSets, session.StrengthSets...)
	}

	return merged
}

// mapFitSportToActivityType converts FIT SDK sport types to our ActivityType enum
func mapFitSportToActivityType(sport typedef.Sport, subSport typedef.SubSport) pbactivity.ActivityType {
	switch sport {
	case typedef.SportRunning:
		switch subSport {
		case typedef.SubSportTrail:
			return pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN
		case typedef.SubSportVirtualActivity:
			return pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN
		default:
			return pbactivity.ActivityType_ACTIVITY_TYPE_RUN
		}

	case typedef.SportCycling:
		switch subSport {
		case typedef.SubSportVirtualActivity:
			return pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE
		case typedef.SubSportMountain:
			return pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE
		case typedef.SubSportGravelCycling:
			return pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE
		case typedef.SubSportEBikeMountain:
			return pbactivity.ActivityType_ACTIVITY_TYPE_EMOUNTAIN_BIKE_RIDE
		case typedef.SubSportEBikeFitness:
			return pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE
		default:
			return pbactivity.ActivityType_ACTIVITY_TYPE_RIDE
		}

	case typedef.SportSwimming:
		return pbactivity.ActivityType_ACTIVITY_TYPE_SWIM

	case typedef.SportWalking:
		return pbactivity.ActivityType_ACTIVITY_TYPE_WALK

	case typedef.SportHiking:
		return pbactivity.ActivityType_ACTIVITY_TYPE_HIKE

	case typedef.SportTraining:
		switch subSport {
		case typedef.SubSportStrengthTraining:
			return pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING
		case typedef.SubSportYoga:
			return pbactivity.ActivityType_ACTIVITY_TYPE_YOGA
		case typedef.SubSportHiit:
			return pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING
		case typedef.SubSportPilates:
			return pbactivity.ActivityType_ACTIVITY_TYPE_PILATES
		case typedef.SubSportElliptical:
			return pbactivity.ActivityType_ACTIVITY_TYPE_ELLIPTICAL
		case typedef.SubSportStairClimbing:
			return pbactivity.ActivityType_ACTIVITY_TYPE_STAIR_STEPPER
		default:
			return pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT
		}

	case typedef.SportRowing:
		return pbactivity.ActivityType_ACTIVITY_TYPE_ROWING

	case typedef.SportAlpineSkiing:
		return pbactivity.ActivityType_ACTIVITY_TYPE_ALPINE_SKI

	case typedef.SportCrossCountrySkiing:
		return pbactivity.ActivityType_ACTIVITY_TYPE_NORDIC_SKI

	case typedef.SportSnowboarding:
		return pbactivity.ActivityType_ACTIVITY_TYPE_SNOWBOARD

	case typedef.SportSoccer:
		return pbactivity.ActivityType_ACTIVITY_TYPE_SOCCER

	case typedef.SportTennis:
		return pbactivity.ActivityType_ACTIVITY_TYPE_TENNIS

	case typedef.SportGolf:
		return pbactivity.ActivityType_ACTIVITY_TYPE_GOLF

	case typedef.SportPaddling:
		return pbactivity.ActivityType_ACTIVITY_TYPE_KAYAKING

	case typedef.SportStandUpPaddleboarding:
		return pbactivity.ActivityType_ACTIVITY_TYPE_STAND_UP_PADDLING

	case typedef.SportSurfing:
		return pbactivity.ActivityType_ACTIVITY_TYPE_SURFING

	case typedef.SportSailing:
		return pbactivity.ActivityType_ACTIVITY_TYPE_SAIL

	case typedef.SportIceSkating:
		return pbactivity.ActivityType_ACTIVITY_TYPE_ICE_SKATE

	case typedef.SportInlineSkating:
		return pbactivity.ActivityType_ACTIVITY_TYPE_INLINE_SKATE

	case typedef.SportRockClimbing:
		return pbactivity.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING

	default:
		return pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT
	}
}

// generateActivityName creates a default activity name based on type and time
func generateActivityName(activityType pbactivity.ActivityType, startTime time.Time) string {
	hour := startTime.Hour()
	var timeOfDay string
	switch {
	case hour < 12:
		timeOfDay = "Morning"
	case hour < 17:
		timeOfDay = "Afternoon"
	case hour < 21:
		timeOfDay = "Evening"
	default:
		timeOfDay = "Night"
	}

	var activityName string
	switch activityType {
	case pbactivity.ActivityType_ACTIVITY_TYPE_RUN, pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN, pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN:
		activityName = "Run"
	case pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE, pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE, pbactivity.ActivityType_ACTIVITY_TYPE_EMOUNTAIN_BIKE_RIDE, pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE:
		activityName = "Ride"
	case pbactivity.ActivityType_ACTIVITY_TYPE_SWIM:
		activityName = "Swim"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WALK:
		activityName = "Walk"
	case pbactivity.ActivityType_ACTIVITY_TYPE_HIKE:
		activityName = "Hike"
	case pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING:
		activityName = "Workout"
	case pbactivity.ActivityType_ACTIVITY_TYPE_YOGA:
		activityName = "Yoga"
	default:
		activityName = "Activity"
	}

	return fmt.Sprintf("%s %s", timeOfDay, activityName)
}

// generateExerciseTimeMarkers creates TimeMarker objects from FIT Set messages.
// Each Set message represents a distinct exercise/move and gets its own marker.
// The reconciler will later overwrite these generic category labels with proper
// exercise names from sources like Hevy.
func generateExerciseTimeMarkers(sets []setInfo) []*pbactivity.TimeMarker {
	if len(sets) == 0 {
		return nil
	}

	var markers []*pbactivity.TimeMarker

	for _, s := range sets {
		name := s.exerciseName
		if name == "" {
			name = "Exercise"
		}

		markers = append(markers, &pbactivity.TimeMarker{
			Timestamp:  timestamppb.New(s.startTime),
			Label:      name,
			MarkerType: "exercise_start",
		})
	}

	return markers
}

// formatExerciseCategory converts a FIT ExerciseCategory enum to a readable exercise name.
// e.g. ExerciseCategoryBenchPress.String() = "bench_press" → "Bench Press"
func formatExerciseCategory(cat typedef.ExerciseCategory) string {
	s := cat.String()
	if s == "" || s == "invalid" {
		return "Exercise"
	}
	// Convert snake_case to Title Case
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
