package server

import (
	"net/http"

	pbactivitym "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"github.com/go-chi/chi/v5"
)

// handleGetShowcaseViewStats returns view metrics for one of the caller's
// showcased activities. Ownership is enforced by the activity service.
func (s *APIServer) handleGetShowcaseViewStats(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	res, err := s.activitySvc.GetShowcaseViewStats(r.Context(), &activitypb.GetShowcaseViewStatsRequest{
		UserId:   token.UID,
		Target:   pbactivitym.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_ACTIVITY,
		TargetId: chi.URLParam(r, "id"),
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

// handleGetShowcaseProfileViewStats returns view metrics for the caller's profile page.
func (s *APIServer) handleGetShowcaseProfileViewStats(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	res, err := s.activitySvc.GetShowcaseViewStats(r.Context(), &activitypb.GetShowcaseViewStatsRequest{
		UserId: token.UID,
		Target: pbactivitym.ShowcaseViewTarget_SHOWCASE_VIEW_TARGET_PROFILE,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

// handleListShowcaseViewStats returns aggregate + per-showcase view metrics for the caller.
func (s *APIServer) handleListShowcaseViewStats(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	res, err := s.activitySvc.ListShowcaseViewStats(r.Context(), &activitypb.ListShowcaseViewStatsRequest{
		UserId: token.UID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}
