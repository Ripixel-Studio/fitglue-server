package server

import (
	"encoding/json"
	"net/http"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

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

// handleMobileSync triggers a data synchronization for the mobile app.
// For now this is a no-op placeholder that returns success.
func (s *APIServer) handleMobileSync(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	// Mobile sync is currently a placeholder — the sync pipeline is triggered
	// by webhook events, not client requests. This endpoint exists to satisfy
	// the mobile client's API contract.
	WriteJSON(w, map[string]interface{}{
		"status":  "ok",
		"message": "Sync triggered",
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
