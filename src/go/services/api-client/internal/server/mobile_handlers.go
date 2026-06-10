package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	activitymodelpb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// handleGetWebAuthToken mints a Firebase custom token for the authenticated user so
// the mobile WebView can call signInWithCustomToken() on the web app without a
// separate login. Custom tokens are valid for 1 hour; the web Firebase SDK manages
// its own refresh-token session after the first sign-in.
func (s *APIServer) handleGetWebAuthToken(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	customToken, err := s.authClient.CustomToken(r.Context(), token.UID)
	if err != nil {
		WriteError(w, statusError(http.StatusInternalServerError, "failed to create web auth token"))
		return
	}

	WriteJSON(w, map[string]string{"customToken": customToken})
}

// handleSetFCMToken registers or updates a device's FCM token for push notifications
func (s *APIServer) handleSetFCMToken(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var body struct {
		Token    string `json:"token"`
		Platform string `json:"platform"` // "web", "android", "ios"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	if body.Token == "" {
		WriteError(w, statusError(http.StatusBadRequest, "token is required"))
		return
	}

	_, err := s.userService.SetFCMToken(r.Context(), &userpb.SetFCMTokenRequest{
		UserId:   token.UID,
		Token:    body.Token,
		Platform: body.Platform,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type mobileSyncActivity struct {
	ExternalID       string             `json:"externalId"`
	ActivityName     string             `json:"activityName"`
	StartTime        string             `json:"startTime"`
	EndTime          string             `json:"endTime"`
	Duration         float64            `json:"duration"`
	Calories         *float64           `json:"calories"`
	Distance         *float64           `json:"distance"`
	HeartRateSamples []mobileHRSample   `json:"heartRateSamples"`
	Route            []mobileRoutePoint `json:"route"`
	Source           string             `json:"source"`
}

type mobileHRSample struct {
	Timestamp string `json:"timestamp"`
	BPM       int32  `json:"bpm"`
}

type mobileRoutePoint struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Altitude  *float64 `json:"altitude"`
	Timestamp string   `json:"timestamp"`
}

// handleMobileSync receives health data from the iOS/Android app and publishes it to the
// activity pipeline. Each activity becomes a separate Pub/Sub message on topic-raw-activity,
// exactly as webhook sources do. Auth is already verified by the Firebase middleware.
func (s *APIServer) handleMobileSync(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var body struct {
		Activities []mobileSyncActivity `json:"activities"`
		Device     struct {
			Platform string `json:"platform"`
		} `json:"device"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	if len(body.Activities) == 0 {
		WriteJSON(w, map[string]interface{}{"status": "ok", "processedCount": 0, "skippedCount": 0})
		return
	}

	source := activitymodelpb.ActivitySource_SOURCE_HEALTH_CONNECT
	if body.Device.Platform == "ios" {
		source = activitymodelpb.ActivitySource_SOURCE_APPLE_HEALTH
	}

	processedCount := 0
	skippedCount := 0

	for _, act := range body.Activities {
		startTime, err := time.Parse(time.RFC3339, act.StartTime)
		if err != nil {
			s.logger.Warn(r.Context(), "Skipping mobile activity: invalid start time", "start_time", act.StartTime, "error", err)
			skippedCount++
			continue
		}

		// Build HR records
		var records []*activitymodelpb.Record
		var hrSum int32
		for _, sample := range act.HeartRateSamples {
			t, parseErr := time.Parse(time.RFC3339, sample.Timestamp)
			if parseErr != nil {
				continue
			}
			records = append(records, &activitymodelpb.Record{
				Timestamp: timestamppb.New(t),
				HeartRate: sample.BPM,
			})
			hrSum += sample.BPM
		}

		// Append GPS points to records
		for _, pt := range act.Route {
			t, parseErr := time.Parse(time.RFC3339, pt.Timestamp)
			if parseErr != nil {
				continue
			}
			rec := &activitymodelpb.Record{
				Timestamp:    timestamppb.New(t),
				PositionLat:  pt.Latitude,
				PositionLong: pt.Longitude,
			}
			if pt.Altitude != nil {
				rec.Altitude = *pt.Altitude
			}
			records = append(records, rec)
		}

		lap := &activitymodelpb.Lap{
			StartTime:        timestamppb.New(startTime),
			TotalElapsedTime: act.Duration,
			Records:          records,
		}
		if act.Distance != nil {
			lap.TotalDistance = *act.Distance
		}

		session := &activitymodelpb.Session{
			StartTime:        timestamppb.New(startTime),
			TotalElapsedTime: act.Duration,
			Laps:             []*activitymodelpb.Lap{lap},
		}
		if act.Distance != nil {
			session.TotalDistance = *act.Distance
		}
		if act.Calories != nil {
			session.TotalCalories = act.Calories
		}
		if len(act.HeartRateSamples) > 0 {
			avg := int32(int(hrSum) / len(act.HeartRateSamples))
			session.AvgHeartRate = &avg
		}

		externalID := act.ExternalID
		if externalID == "" {
			externalID = fmt.Sprintf("mobile-%s-%s", act.Source, act.StartTime)
		}

		sa := &activitymodelpb.StandardizedActivity{
			Source:     source,
			ExternalId: externalID,
			UserId:     token.UID,
			StartTime:  timestamppb.New(startTime),
			Name:       act.ActivityName,
			Sessions:   []*activitymodelpb.Session{session},
		}

		execID := uuid.NewString()
		payload := &pbevents.ActivityPayload{
			Source:               source,
			UserId:               token.UID,
			StandardizedActivity: sa,
			PipelineExecutionId:  &execID,
		}

		ce, ceErr := infrapubsub.NewCloudEvent(
			"/integrations/mobile/sync",
			"com.fitglue.activity.created",
			payload,
		)
		if ceErr != nil {
			s.logger.Error(r.Context(), "Failed to create cloud event for mobile activity", "error", ceErr, "external_id", externalID)
			skippedCount++
			continue
		}

		if _, pubErr := s.publisher.PublishCloudEvent(r.Context(), "topic-raw-activity", ce); pubErr != nil {
			s.logger.Error(r.Context(), "Failed to publish mobile activity", "error", pubErr, "external_id", externalID)
			skippedCount++
			continue
		}

		processedCount++
	}

	WriteJSON(w, map[string]interface{}{
		"status":         "ok",
		"processedCount": processedCount,
		"skippedCount":   skippedCount,
	})
}

// handleGetActivityStats returns aggregate activity statistics for the dashboard.
// It merges counts from the activity service with streak data from the user profile.
func (s *APIServer) handleGetActivityStats(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	res, err := s.activitySvc.GetActivityStats(r.Context(), &activitypb.GetActivityStatsRequest{
		UserId: token.UID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	// Augment with streak data from user profile (best-effort; failures don't block the response).
	profile, profileErr := s.userService.GetProfile(r.Context(), &userpb.GetProfileRequest{
		UserId: token.UID,
	})
	if profileErr == nil && profile != nil {
		if profile.CurrentStreakDays != nil {
			res.CurrentStreakDays = *profile.CurrentStreakDays
		}
		if profile.LongestStreakDays != nil {
			res.LongestStreakDays = *profile.LongestStreakDays
		}
	}

	WriteJSON(w, res)
}
