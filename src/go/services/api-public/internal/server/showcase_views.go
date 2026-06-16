package server

import (
	"net"
	"net/http"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/showcaseview"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"github.com/go-chi/chi/v5"
)

// handleRecordShowcaseActivityView records a view for a single showcased activity.
func (s *APIServer) handleRecordShowcaseActivityView(w http.ResponseWriter, r *http.Request) {
	s.recordView(w, r, showcaseview.ActivityKey(chi.URLParam(r, "id")))
}

// handleRecordShowcaseProfileView records a view for an athlete profile page.
func (s *APIServer) handleRecordShowcaseProfileView(w http.ResponseWriter, r *http.Request) {
	s.recordView(w, r, showcaseview.ProfileKey(chi.URLParam(r, "slug")))
}

// handleRecordShowcaseRoundupView records a view for a roundup page.
func (s *APIServer) handleRecordShowcaseRoundupView(w http.ResponseWriter, r *http.Request) {
	s.recordView(w, r, showcaseview.RoundupKey(chi.URLParam(r, "slug"), chi.URLParam(r, "periodKey")))
}

// recordView applies bot filtering, derives the privacy-friendly visitor hash,
// and forwards a de-duplicated view to the activity service. It always responds
// 204 — counts are never leaked on the public surface, and a tracking failure
// must never break the page.
func (s *APIServer) recordView(w http.ResponseWriter, r *http.Request, targetKey string) {
	// Drop crawlers / social-card unfurlers before they reach the counter.
	if showcaseview.IsBot(r.UserAgent()) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	visitorHash := showcaseview.VisitorHash(s.viewSalt, clientIP(r), r.UserAgent(), targetKey, time.Now())

	if _, err := s.activitySvc.RecordShowcaseView(r.Context(), &activitypb.RecordShowcaseViewRequest{
		TargetKey:   targetKey,
		VisitorHash: visitorHash,
	}); err != nil {
		// Log but never fail the beacon — the page must not depend on tracking.
		s.logger.Warn(r.Context(), "failed to record showcase view", "error", err, "target_key", targetKey)
	}

	w.WriteHeader(http.StatusNoContent)
}

// clientIP returns the caller's IP without the port. middleware.RealIP has
// already normalised X-Forwarded-For / X-Real-IP into RemoteAddr.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
