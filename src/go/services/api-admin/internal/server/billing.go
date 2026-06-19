package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	gatewaypb "github.com/fitglue/server/src/go/pkg/types/pb/gateway"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
)

// userBilling assembles a user's billing view (subscription + effective tier).
// Best-effort: a billing-service failure degrades to an empty view rather than
// failing the caller (used by both buildUserDetail and the billing endpoint).
func (s *APIServer) userBilling(ctx context.Context, userID string) *gatewaypb.AdminUserBilling {
	billing := &gatewaypb.AdminUserBilling{}
	if s.billingSvc == nil {
		return billing
	}
	if sub, err := s.billingSvc.GetSubscription(ctx, &billingpb.GetSubscriptionRequest{UserId: userID}); err == nil {
		billing.Subscription = sub
	} else {
		s.logger.Warn(ctx, "admin: get subscription failed", "userId", userID, "error", err)
	}
	if tier, err := s.billingSvc.GetTierStatus(ctx, &billingpb.GetTierStatusRequest{UserId: userID}); err == nil {
		billing.EffectiveTier = tier.GetEffectiveTier()
		billing.IsTrial = tier.GetIsTrial()
	}
	return billing
}

func (s *APIServer) handleGetUserBilling(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}
	WriteJSON(w, s.userBilling(r.Context(), userID))
}

func (s *APIServer) handleStartTrial(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}
	_, err := s.billingSvc.StartTrial(r.Context(), &billingpb.StartTrialRequest{UserId: userID})
	s.auditAction(r.Context(), r, "start_trial", userID, nil, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleCancelSubscription(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}
	_, err := s.billingSvc.CancelSubscription(r.Context(), &billingpb.CancelSubscriptionRequest{UserId: userID})
	s.auditAction(r.Context(), r, "cancel_subscription", userID, nil, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleCreateBillingPortal(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}
	var req struct {
		ReturnURL string `json:"returnUrl"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body is optional

	res, err := s.billingSvc.CreateBillingPortalSession(r.Context(), &billingpb.CreateBillingPortalSessionRequest{
		UserId:    userID,
		ReturnUrl: req.ReturnURL,
	})
	s.auditAction(r.Context(), r, "create_billing_portal", userID, nil, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, &gatewaypb.CreateBillingPortalAdminResponse{Url: res.GetUrl()})
}
