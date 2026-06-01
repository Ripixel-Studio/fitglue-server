// Package distanceeffort provides sliding-window fastest-segment calculations
// shared between the personal_records and best_efforts enrichers.
package distanceeffort

import (
	"math"
	"sort"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// DistanceTimePoint is a cumulative distance/time sample from the activity data stream.
type DistanceTimePoint struct {
	CumulativeDistanceM float64
	ElapsedTimeSec      float64
}

// FindFastestSegment scans through an activity's record-level, lap-level, or session-level data
// to find the minimum elapsed time for a contiguous segment covering targetDistanceM.
// Returns 0 if the activity doesn't cover the target distance.
//
// Fidelity levels (tried in order):
//  1. Record-level: Uses 1Hz speed data to build cumulative distance, then sliding window
//  2. Lap-level: Uses lap total_distance/total_elapsed_time with sliding window
//  3. Proportional fallback: Assumes even pacing across the entire activity
func FindFastestSegment(activity *pbactivity.StandardizedActivity, targetDistanceM float64) float64 {
	if time := findFastestFromRecords(activity, targetDistanceM); time > 0 {
		return time
	}
	if time := findFastestFromLaps(activity, targetDistanceM); time > 0 {
		return time
	}
	return findFastestProportional(activity, targetDistanceM)
}

func findFastestFromRecords(activity *pbactivity.StandardizedActivity, targetDistanceM float64) float64 {
	points := buildDistanceTimePoints(activity)
	if len(points) < 2 {
		return 0
	}
	if points[len(points)-1].CumulativeDistanceM < targetDistanceM {
		return 0
	}
	return slidingWindowMinTime(points, targetDistanceM)
}

// buildDistanceTimePoints collects all records across sessions/laps and builds
// cumulative distance/time points. Prefers native GPS cumulative distance from
// Record.Distance when available (highest fidelity, matches Strava).
func buildDistanceTimePoints(activity *pbactivity.StandardizedActivity) []DistanceTimePoint {
	if points := buildFromNativeDistance(activity); len(points) >= 2 {
		return points
	}
	return buildFromSpeedDerived(activity)
}

// buildFromNativeDistance uses Record.Distance (cumulative meters from start) when available.
func buildFromNativeDistance(activity *pbactivity.StandardizedActivity) []DistanceTimePoint {
	var points []DistanceTimePoint
	var hasDistanceData bool

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Distance > 0 {
					hasDistanceData = true
					break
				}
			}
			if hasDistanceData {
				break
			}
		}
		if hasDistanceData {
			break
		}
	}

	if !hasDistanceData {
		return nil
	}

	var baseTimestamp int64
	var firstTimestampSet bool

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				ts := record.Timestamp.GetSeconds()
				if !firstTimestampSet {
					baseTimestamp = ts
					firstTimestampSet = true
					points = append(points, DistanceTimePoint{0, 0})
				}

				elapsed := float64(ts - baseTimestamp)
				if elapsed < 0 {
					continue
				}

				if record.Distance > 0 {
					points = append(points, DistanceTimePoint{
						CumulativeDistanceM: record.Distance,
						ElapsedTimeSec:      elapsed,
					})
				}
			}
		}
	}

	return points
}

// buildFromSpeedDerived reconstructs cumulative distance from speed × Δtime.
func buildFromSpeedDerived(activity *pbactivity.StandardizedActivity) []DistanceTimePoint {
	var points []DistanceTimePoint

	var cumulativeDistance float64
	var cumulativeTime float64
	var hasSpeedData bool

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			for _, record := range lap.Records {
				if record.Speed > 0 {
					hasSpeedData = true
				}
			}
		}
	}

	if !hasSpeedData {
		return nil
	}

	points = append(points, DistanceTimePoint{0, 0})

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			var prevTimestamp int64
			for i, record := range lap.Records {
				ts := record.Timestamp.GetSeconds()
				if i == 0 && prevTimestamp == 0 && len(points) == 1 {
					prevTimestamp = ts
					continue
				}

				if prevTimestamp == 0 {
					prevTimestamp = ts
					continue
				}

				dt := float64(ts - prevTimestamp)
				if dt <= 0 {
					prevTimestamp = ts
					continue
				}

				speed := record.Speed
				if speed < 0 {
					speed = 0
				}
				cumulativeDistance += speed * dt
				cumulativeTime += dt

				points = append(points, DistanceTimePoint{
					CumulativeDistanceM: cumulativeDistance,
					ElapsedTimeSec:      cumulativeTime,
				})

				prevTimestamp = ts
			}
		}
	}

	return points
}

// slidingWindowMinTime uses a two-pointer technique on cumulative distance/time points
// to find the minimum elapsed time for a contiguous segment covering targetDistanceM.
func slidingWindowMinTime(points []DistanceTimePoint, targetDistanceM float64) float64 {
	minTime := math.MaxFloat64
	left := 0

	for right := 1; right < len(points); right++ {
		for left < right-1 {
			windowDist := points[right].CumulativeDistanceM - points[left+1].CumulativeDistanceM
			if windowDist >= targetDistanceM {
				left++
			} else {
				break
			}
		}

		windowDist := points[right].CumulativeDistanceM - points[left].CumulativeDistanceM
		if windowDist >= targetDistanceM {
			exactStartDist := points[right].CumulativeDistanceM - targetDistanceM
			startTime := interpolateTime(points, exactStartDist)
			elapsed := points[right].ElapsedTimeSec - startTime
			if elapsed > 0 && elapsed < minTime {
				minTime = elapsed
			}
		}
	}

	if minTime == math.MaxFloat64 {
		return 0
	}
	return minTime
}

// interpolateTime finds the elapsed time at a given cumulative distance
// by interpolating between surrounding data points.
func interpolateTime(points []DistanceTimePoint, targetDist float64) float64 {
	idx := sort.Search(len(points), func(i int) bool {
		return points[i].CumulativeDistanceM > targetDist
	})

	if idx == 0 {
		return points[0].ElapsedTimeSec
	}
	if idx >= len(points) {
		return points[len(points)-1].ElapsedTimeSec
	}

	p1 := points[idx-1]
	p2 := points[idx]
	distRange := p2.CumulativeDistanceM - p1.CumulativeDistanceM
	if distRange <= 0 {
		return p1.ElapsedTimeSec
	}

	fraction := (targetDist - p1.CumulativeDistanceM) / distRange
	return p1.ElapsedTimeSec + fraction*(p2.ElapsedTimeSec-p1.ElapsedTimeSec)
}

// findFastestFromLaps uses lap-level distance/time data with a sliding window approach.
func findFastestFromLaps(activity *pbactivity.StandardizedActivity, targetDistanceM float64) float64 {
	var points []DistanceTimePoint
	var cumulativeDistance float64
	var cumulativeTime float64
	var hasLapData bool

	points = append(points, DistanceTimePoint{0, 0})

	for _, session := range activity.Sessions {
		for _, lap := range session.Laps {
			if lap.TotalDistance > 0 && lap.TotalElapsedTime > 0 {
				hasLapData = true
				cumulativeDistance += lap.TotalDistance
				cumulativeTime += lap.TotalElapsedTime
				points = append(points, DistanceTimePoint{
					CumulativeDistanceM: cumulativeDistance,
					ElapsedTimeSec:      cumulativeTime,
				})
			}
		}
	}

	if !hasLapData || len(points) <= 2 {
		return 0
	}

	if cumulativeDistance < targetDistanceM {
		return 0
	}

	return slidingWindowMinTime(points, targetDistanceM)
}

// findFastestProportional estimates time via proportional extrapolation (assumes even pacing).
func findFastestProportional(activity *pbactivity.StandardizedActivity, targetDistanceM float64) float64 {
	var totalDistanceM float64
	var totalDurationSec float64

	for _, session := range activity.Sessions {
		totalDistanceM += session.TotalDistance
		totalDurationSec += session.TotalElapsedTime
	}

	if totalDistanceM < targetDistanceM || totalDurationSec <= 0 {
		return 0
	}

	return (targetDistanceM / totalDistanceM) * totalDurationSec
}
