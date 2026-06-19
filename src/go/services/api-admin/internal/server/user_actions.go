package server

import (
	"encoding/json"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/reflect/protoreflect"

	usermodelpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

func (s *APIServer) handleSendPasswordReset(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}

	profile, err := s.userService.GetProfile(r.Context(), &userpb.GetProfileRequest{UserId: userID})
	if err != nil {
		s.auditAction(r.Context(), r, "send_password_reset", userID, nil, err)
		WriteError(w, err)
		return
	}
	if profile.GetEmail() == "" {
		WriteError(w, statusError(http.StatusBadRequest, "user has no email address"))
		return
	}

	_, err = s.userService.SendPasswordResetEmail(r.Context(), &userpb.SendPasswordResetEmailRequest{
		Email: profile.GetEmail(),
	})
	s.auditAction(r.Context(), r, "send_password_reset", userID, map[string]string{"email": profile.GetEmail()}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleSendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}
	_, err := s.userService.SendVerificationEmail(r.Context(), &userpb.SendVerificationEmailRequest{UserId: userID})
	s.auditAction(r.Context(), r, "send_verification_email", userID, nil, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleSetIntegrationEnabled(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	provider := chi.URLParam(r, "provider")
	if userID == "" || provider == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or provider"))
		return
	}
	if !validIntegrationProvider(provider) {
		WriteError(w, statusError(http.StatusBadRequest, "unknown integration provider: "+provider))
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	// Flip only the `enabled` flag on the provider's integration sub-document,
	// leaving tokens untouched. The user document stores integrations under the
	// `integrations.<provider>` map field.
	_, err := s.firestoreClient.Collection("users").Doc(userID).Update(r.Context(), []firestore.Update{
		{Path: "integrations." + provider + ".enabled", Value: req.Enabled},
	})
	s.auditAction(r.Context(), r, "set_integration_enabled", userID, map[string]string{
		"provider": provider,
		"enabled":  boolString(req.Enabled),
	}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	provider := chi.URLParam(r, "provider")
	if userID == "" || provider == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or provider"))
		return
	}

	_, err := s.userService.DeleteIntegration(r.Context(), &userpb.DeleteIntegrationRequest{
		UserId:   userID,
		Provider: provider,
	})
	s.auditAction(r.Context(), r, "delete_integration", userID, map[string]string{"provider": provider}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validIntegrationProvider reports whether the provider is a known field on the
// UserIntegrations message (guards against arbitrary Firestore field writes).
func validIntegrationProvider(provider string) bool {
	fields := (&usermodelpb.UserIntegrations{}).ProtoReflect().Descriptor().Fields()
	return fields.ByName(protoreflect.Name(provider)) != nil
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
