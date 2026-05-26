// nolint:proto-json
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/apikey"
	"github.com/fitglue/server/src/go/pkg/integrations/intervals"

	infraps "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"firebase.google.com/go/v4/auth"
	"github.com/go-chi/chi/v5"
)

// getUserToken extracts the verified Firebase token from the request context
func getUserToken(r *http.Request) *auth.Token {
	token, ok := r.Context().Value(userContextKey).(*auth.Token)
	if !ok {
		return nil
	}
	return token
}

func (s *APIServer) registerUserRoutes(r chi.Router) {
	r.Get("/users/me", s.handleGetProfile)
	r.Put("/users/me", s.handleUpdateProfile)
	r.Delete("/users/me", s.handleDeleteSelf)

	r.Get("/users/me/integrations", s.handleListIntegrations)
	r.Get("/users/me/integrations/{provider}", s.handleGetIntegration)
	r.Put("/users/me/integrations/{provider}", s.handleSetIntegration)
	r.Delete("/users/me/integrations/{provider}", s.handleDeleteIntegration)

	r.Get("/users/me/notification-prefs", s.handleGetNotificationPrefs)
	r.Put("/users/me/notification-prefs", s.handleUpdateNotificationPrefs)

	r.Get("/users/me/counters", s.handleListCounters)
	r.Put("/users/me/counters/{name}", s.handleUpdateCounter)

	r.Get("/users/me/booster-data", s.handleGetBoosterData)
	r.Put("/users/me/booster-data/{boosterId}", s.handleSetBoosterData)
	r.Delete("/users/me/booster-data/{boosterId}", s.handleDeleteBoosterData)

	// Connection actions (trigger sync, resync, etc)
	r.Post("/users/me/connections/{provider}/actions", s.handleConnectionActions)

	// Email auth
	r.Post("/users/me/auth-email/send-verification", s.handleSendVerificationEmail)
	r.Post("/users/me/auth-email/send-email-change", s.handleSendEmailChangeVerification)

	// Personal Records
	r.Get("/users/me/personal-records", s.handleListPersonalRecords)
	r.Put("/users/me/personal-records/{recordType}", s.handleSetPersonalRecord)
	r.Delete("/users/me/personal-records/{recordType}", s.handleDeletePersonalRecord)

	// Plugin Defaults
	r.Get("/users/me/plugin-defaults", s.handleListPluginDefaults)
	r.Put("/users/me/plugin-defaults/{pluginId}", s.handleSetPluginDefaults)
	r.Delete("/users/me/plugin-defaults/{pluginId}", s.handleDeletePluginDefaults)

	// Delete counter
	r.Delete("/users/me/counters/{name}", s.handleDeleteCounter)

	// FCM Token (push notifications)
	r.Post("/users/me/fcm-token", s.handleSetFCMToken)

	// Mobile sync (trigger data sync)
	r.Post("/users/me/mobile/sync", s.handleMobileSync)
}

func (s *APIServer) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &userpb.GetProfileRequest{
		UserId: token.UID,
	}

	profile, err := s.userService.GetProfile(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, profile)
}

func (s *APIServer) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var profileReq pbuser.UserProfile
	if err := decodeProto(r, &profileReq); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	var reqBody userpb.UpdateProfileRequest
	reqBody.Profile = &profileReq
	reqBody.UserId = token.UID

	result, err := s.userService.UpdateProfile(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, result)
}

func (s *APIServer) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)

	req := &userpb.ListIntegrationsRequest{
		UserId: token.UID,
	}

	// Wait, ListIntegrations is in userpb, let me double check the proto definitions.
	// user.proto: "rpc ListIntegrations(ListIntegrationsRequest) returns (ListIntegrationsResponse);"
	res, err := s.userService.ListIntegrations(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetIntegration(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	provider := chi.URLParam(r, "provider")

	req := &userpb.GetIntegrationRequest{
		UserId:   token.UID,
		Provider: provider,
	}

	res, err := s.userService.GetIntegration(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleSetIntegration(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	provider := chi.URLParam(r, "provider")

	// Parse the request body into a generic struct for the integration data
	var bodyMap map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&bodyMap); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	// Provider-specific pre-processing before storing.
	if provider == "intervals" {
		apiKey, _ := bodyMap["apiKey"].(string)
		athleteID, _ := bodyMap["athleteId"].(string)
		if apiKey == "" || athleteID == "" {
			WriteError(w, statusError(http.StatusBadRequest, "API key and athlete ID are both required"))
			return
		}
		if err := intervals.VerifyCredentials(r.Context(), apiKey, athleteID); err != nil {
			WriteError(w, statusError(http.StatusBadRequest, fmt.Sprintf("could not verify Intervals.icu credentials: %s", err.Error())))
			return
		}
		// Rebuild bodyMap with the snake_case field names the Firestore converter expects.
		bodyMap = map[string]interface{}{
			"api_key":    apiKey,
			"athlete_id": athleteID,
		}
	}

	// Enrich integration data with standard metadata
	bodyMap["enabled"] = true
	bodyMap["consent_given"] = true
	bodyMap["connected_at"] = time.Now().UTC().Format(time.RFC3339)

	integrationData, err := structpb.NewStruct(bodyMap)
	if err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "failed to parse integration data"))
		return
	}

	req := &userpb.SetIntegrationRequest{
		UserId:          token.UID,
		Provider:        provider,
		IntegrationData: integrationData,
	}

	_, err = s.userService.SetIntegration(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	// For API_KEY providers, generate an ingress API key for webhooks
	if isApiKeyProvider(provider) && s.apiKeyStore != nil {
		rawKey, keyHash, err := apikey.GenerateRandomIngressKey()
		if err != nil {
			s.logger.Error(r.Context(), "failed to generate ingress key", "error", err, "provider", provider)
			WriteError(w, statusError(http.StatusInternalServerError, "failed to generate ingress key"))
			return
		}

		label := provider + "-webhook"
		scopes := []string{"webhook:" + provider}
		now := time.Now()

		if err := s.apiKeyStore.CreateIngressKey(r.Context(), keyHash, token.UID, label, scopes, now); err != nil {
			s.logger.Error(r.Context(), "failed to store ingress key", "error", err, "provider", provider)
			WriteError(w, statusError(http.StatusInternalServerError, "failed to store ingress key"))
			return
		}

		WriteJSON(w, map[string]string{
			"ingressApiKey":   rawKey,
			"ingressKeyLabel": label,
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// isApiKeyProvider returns true for providers that use API key auth and require
// an ingress key to receive webhooks.
func isApiKeyProvider(provider string) bool {
	switch provider {
	case "hevy":
		return true
	default:
		return false
	}
}

func (s *APIServer) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	provider := chi.URLParam(r, "provider")

	req := &userpb.DeleteIntegrationRequest{
		UserId:   token.UID,
		Provider: provider,
	}

	res, err := s.userService.DeleteIntegration(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	req := &userpb.GetNotificationPrefsRequest{
		UserId: token.UID,
	}
	res, err := s.userService.GetNotificationPrefs(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, res)
}

func (s *APIServer) handleUpdateNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)

	// Read current prefs
	current, err := s.userService.GetNotificationPrefs(r.Context(), &userpb.GetNotificationPrefsRequest{UserId: token.UID})
	if err != nil {
		// If not found, start from defaults (all true)
		st, _ := status.FromError(err)
		if st.Code() != codes.NotFound {
			WriteError(w, err)
			return
		}
		current = &pbuser.NotificationPreferences{
			NotifyPendingInput:     true,
			NotifyPipelineSuccess:  true,
			NotifyPipelineFailure:  true,
			NotifyConnectionAction: true,
		}
	}

	// Decode partial body into a map to identify which fields were explicitly sent
	var partial map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&partial); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	// Merge: only overwrite fields present in the partial update
	merged := current
	if v, ok := partial["notifyPendingInput"]; ok {
		if b, ok := v.(bool); ok {
			merged.NotifyPendingInput = b
		}
	}
	if v, ok := partial["notifyPipelineSuccess"]; ok {
		if b, ok := v.(bool); ok {
			merged.NotifyPipelineSuccess = b
		}
	}
	if v, ok := partial["notifyPipelineFailure"]; ok {
		if b, ok := v.(bool); ok {
			merged.NotifyPipelineFailure = b
		}
	}
	if v, ok := partial["notifyConnectionAction"]; ok {
		if b, ok := v.(bool); ok {
			merged.NotifyConnectionAction = b
		}
	}

	var req userpb.UpdateNotificationPrefsRequest
	req.UserId = token.UID
	req.Prefs = &pbuser.NotificationPreferences{
		NotifyPendingInput:     merged.NotifyPendingInput,
		NotifyPipelineSuccess:  merged.NotifyPipelineSuccess,
		NotifyPipelineFailure:  merged.NotifyPipelineFailure,
		NotifyConnectionAction: merged.NotifyConnectionAction,
	}

	res, err := s.userService.UpdateNotificationPrefs(r.Context(), &req)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, res)
}

func (s *APIServer) handleListCounters(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	req := &userpb.ListCountersRequest{
		UserId: token.UID,
	}
	res, err := s.userService.ListCounters(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, res)
}

func (s *APIServer) handleUpdateCounter(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	name := chi.URLParam(r, "name")

	var req userpb.UpdateCounterRequest
	if err := decodeProto(r, &req); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	req.UserId = token.UID
	req.CounterId = name

	res, err := s.userService.UpdateCounter(r.Context(), &req)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, res)
}

func (s *APIServer) handleConnectionActions(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	provider := chi.URLParam(r, "provider")
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	if req.Action == "sync" {
		_, err := s.userService.GetIntegration(r.Context(), &userpb.GetIntegrationRequest{
			UserId:   token.UID,
			Provider: provider,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				WriteError(w, statusError(http.StatusNotFound, "integration not found"))
			} else {
				WriteError(w, err)
			}
			return
		}

		evt, err := infraps.NewCloudEvent("api-client", "fitglue.connection.action", map[string]string{
			"action":   "sync_provider",
			"provider": provider,
			"user_id":  token.UID,
		})
		if err != nil {
			WriteError(w, statusError(http.StatusInternalServerError, "failed to create sync event"))
			return
		}

		// Use the correct topic or whatever fallback is required.
		_, err = s.publisher.PublishCloudEvent(r.Context(), "fitglue-activities-commands", evt)
		if err != nil {
			WriteError(w, statusError(http.StatusInternalServerError, "failed to trigger sync"))
			return
		}

		w.WriteHeader(http.StatusAccepted)
		return
	} else if req.Action == "clear" {
		_, err := s.userService.DeleteIntegration(r.Context(), &userpb.DeleteIntegrationRequest{
			UserId:   token.UID,
			Provider: provider,
		})
		if err != nil {
			WriteError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	WriteError(w, statusError(http.StatusBadRequest, "unknown action"))
}

// =============================================================
// Booster Data
// =============================================================

func (s *APIServer) handleGetBoosterData(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &userpb.GetBoosterDataRequest{
		UserId: token.UID,
	}

	res, err := s.userService.GetBoosterData(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleSetBoosterData(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody userpb.SetBoosterDataRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	reqBody.UserId = token.UID
	reqBody.BoosterId = chi.URLParam(r, "boosterId")

	_, err := s.userService.SetBoosterData(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleDeleteBoosterData(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &userpb.DeleteBoosterDataRequest{
		UserId:    token.UID,
		BoosterId: chi.URLParam(r, "boosterId"),
	}

	_, err := s.userService.DeleteBoosterData(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================
// Delete Self
// =============================================================

func (s *APIServer) handleDeleteSelf(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &userpb.DeleteUserRequest{
		UserId: token.UID,
	}

	_, err := s.userService.DeleteUser(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================
// Auth Email
// =============================================================

func (s *APIServer) handleSendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &userpb.SendVerificationEmailRequest{
		UserId: token.UID,
	}

	_, err := s.userService.SendVerificationEmail(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleSendEmailChangeVerification(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody struct {
		NewEmail string `json:"newEmail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	req := &userpb.SendEmailChangeVerificationRequest{
		UserId:   token.UID,
		NewEmail: reqBody.NewEmail,
	}

	_, err := s.userService.SendEmailChangeVerification(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleSendPasswordResetEmail(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	req := &userpb.SendPasswordResetEmailRequest{
		Email: reqBody.Email,
	}

	_, err := s.userService.SendPasswordResetEmail(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================
// Personal Records
// =============================================================

func (s *APIServer) handleListPersonalRecords(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	res, err := s.userService.ListPersonalRecords(r.Context(), &userpb.ListPersonalRecordsRequest{
		UserId: token.UID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleSetPersonalRecord(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody userpb.SetPersonalRecordRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	reqBody.UserId = token.UID
	reqBody.RecordType = chi.URLParam(r, "recordType")

	res, err := s.userService.SetPersonalRecord(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleDeletePersonalRecord(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	_, err := s.userService.DeletePersonalRecord(r.Context(), &userpb.DeletePersonalRecordRequest{
		UserId:     token.UID,
		RecordType: chi.URLParam(r, "recordType"),
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================
// Plugin Defaults
// =============================================================

func (s *APIServer) handleListPluginDefaults(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	res, err := s.userService.ListPluginDefaults(r.Context(), &userpb.ListPluginDefaultsRequest{
		UserId: token.UID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleSetPluginDefaults(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody userpb.SetPluginDefaultsRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	reqBody.UserId = token.UID
	reqBody.PluginId = chi.URLParam(r, "pluginId")

	_, err := s.userService.SetPluginDefaults(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleDeletePluginDefaults(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	_, err := s.userService.DeletePluginDefaults(r.Context(), &userpb.DeletePluginDefaultsRequest{
		UserId:   token.UID,
		PluginId: chi.URLParam(r, "pluginId"),
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================
// Delete Counter
// =============================================================

func (s *APIServer) handleDeleteCounter(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	_, err := s.userService.DeleteCounter(r.Context(), &userpb.DeleteCounterRequest{
		UserId:    token.UID,
		CounterId: chi.URLParam(r, "name"),
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// statusError is a helper for manually generating an error satisfying gRPC status layout
func statusError(code int, msg string) error {
	// Simple wrapper for non-gRPC errors to use WriteError
	return &CustomError{HTTPCode: code, Msg: msg}
}

// CustomError helps map generic HTTP errors into our WriteError handler
type CustomError struct {
	HTTPCode int
	Msg      string
}

func (e *CustomError) Error() string {
	return e.Msg
}
