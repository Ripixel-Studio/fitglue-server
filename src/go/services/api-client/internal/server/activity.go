package server

import (
	"net/http"
	"strconv"

	pbactivitym "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"github.com/go-chi/chi/v5"
)

func (s *APIServer) registerActivityRoutes(r chi.Router) {
	r.Get("/users/me/activities", s.handleListActivities)
	r.Get("/users/me/activities/{id}", s.handleGetActivity)
	r.Delete("/users/me/activities/{id}", s.handleDeleteActivity)
	r.Get("/users/me/activities/stats", s.handleGetActivityStats)

	r.Get("/users/me/showcases", s.handleListShowcases)
	r.Get("/users/me/showcases/{id}", s.handleGetShowcase)
	r.Post("/users/me/showcases", s.handleCreateShowcase)
	r.Put("/users/me/showcases/{id}", s.handleUpdateShowcase)
	r.Delete("/users/me/showcases/{id}", s.handleDeleteShowcase)

	r.Get("/users/me/showcase-management/preferences", s.handleGetShowcasePreferences)
	r.Put("/users/me/showcase-management/preferences", s.handleUpdateShowcasePreferences)
	r.Post("/users/me/showcases/{id}/generate", s.handleGenerateShowcaseImages)

	r.Post("/users/me/export", s.handleExportData)

	r.Post("/users/me/parse-fit", s.handleParseFitFile)

	// Showcase Management
	r.Get("/users/me/showcase-management/profile", s.handleGetShowcaseSettings)
	r.Put("/users/me/showcase-management/profile", s.handleUpdateShowcaseSettings)
	r.Put("/users/me/showcase-management/profile/slug", s.handleUpdateShowcaseSlug)
	r.Post("/users/me/showcase-management/profile/entries/{showcaseId}", s.handleAddShowcaseEntry)
	r.Delete("/users/me/showcase-management/profile/entries/{showcaseId}", s.handleRemoveShowcaseEntry)
	r.Post("/users/me/showcase-management/profile/picture", s.handleGetShowcaseProfilePictureUploadUrl)
	r.Post("/users/me/activity-photos/upload-url", s.handleGetActivityPhotoUploadUrl)
}

func (s *APIServer) handleListActivities(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	limitStr := r.URL.Query().Get("limit")
	pageToken := r.URL.Query().Get("page_token")
	var limit int32 = 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = int32(l)
		}
	}

	req := &activitypb.ListActivitiesRequest{
		UserId:    token.UID,
		Limit:     limit,
		PageToken: pageToken,
	}

	res, err := s.activitySvc.ListActivities(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.GetActivityRequest{
		UserId:     token.UID,
		ActivityId: chi.URLParam(r, "id"),
	}

	res, err := s.activitySvc.GetActivity(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleDeleteActivity(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.DeleteActivityRequest{
		UserId:     token.UID,
		ActivityId: chi.URLParam(r, "id"),
	}

	_, err := s.activitySvc.DeleteActivity(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleListShowcases(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.ListShowcasesRequest{
		UserId: token.UID,
	}

	res, err := s.activitySvc.ListShowcases(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetShowcase(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.GetShowcaseRequest{
		UserId:     token.UID,
		ShowcaseId: chi.URLParam(r, "id"),
	}

	res, err := s.activitySvc.GetShowcase(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleCreateShowcase(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var showcase pbactivitym.ShowcasedActivity
	if err := decodeProto(r, &showcase); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	var reqBody activitypb.CreateShowcaseRequest
	reqBody.Showcase = &showcase
	reqBody.UserId = token.UID

	res, err := s.activitySvc.CreateShowcase(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	WriteJSON(w, res)
}

func (s *APIServer) handleUpdateShowcase(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var showcase pbactivitym.ShowcasedActivity
	if err := decodeProto(r, &showcase); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	var reqBody activitypb.UpdateShowcaseRequest
	reqBody.Showcase = &showcase
	reqBody.UserId = token.UID
	reqBody.ShowcaseId = chi.URLParam(r, "id")

	res, err := s.activitySvc.UpdateShowcase(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleDeleteShowcase(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.DeleteShowcaseRequest{
		UserId:     token.UID,
		ShowcaseId: chi.URLParam(r, "id"),
	}

	_, err := s.activitySvc.DeleteShowcase(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleExportData(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.ExportDataRequest{
		UserId: token.UID,
	}

	res, err := s.activitySvc.ExportData(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetShowcasePreferences(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.GetShowcasePreferencesRequest{
		UserId: token.UID,
	}

	res, err := s.activitySvc.GetShowcasePreferences(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleUpdateShowcasePreferences(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var prefs pbactivitym.ShowcaseProfile
	if err := decodeProto(r, &prefs); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	var reqBody activitypb.UpdateShowcasePreferencesRequest
	reqBody.Preferences = &prefs
	reqBody.UserId = token.UID

	res, err := s.activitySvc.UpdateShowcasePreferences(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGenerateShowcaseImages(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &activitypb.GenerateShowcaseImagesRequest{
		UserId:     token.UID,
		ShowcaseId: chi.URLParam(r, "id"),
	}

	res, err := s.activitySvc.GenerateShowcaseImages(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	WriteJSON(w, res)
}

func (s *APIServer) handleParseFitFile(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody activitypb.ParseFitFileRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	reqBody.UserId = token.UID

	res, err := s.activitySvc.ParseFitFile(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

// =============================================================
// Showcase Settings Management
// =============================================================

func (s *APIServer) handleGetShowcaseSettings(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	res, err := s.activitySvc.GetShowcaseSettings(r.Context(), &activitypb.GetShowcaseSettingsRequest{
		UserId: token.UID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleUpdateShowcaseSettings(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var settings pbactivitym.ShowcaseProfile
	if err := decodeProto(r, &settings); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	var reqBody activitypb.UpdateShowcaseSettingsRequest
	reqBody.Settings = &settings
	reqBody.UserId = token.UID

	res, err := s.activitySvc.UpdateShowcaseSettings(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleUpdateShowcaseSlug(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody activitypb.UpdateShowcaseSlugRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	reqBody.UserId = token.UID

	res, err := s.activitySvc.UpdateShowcaseSlug(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleAddShowcaseEntry(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	_, err := s.activitySvc.AddShowcaseEntry(r.Context(), &activitypb.AddShowcaseEntryRequest{
		UserId:     token.UID,
		ShowcaseId: chi.URLParam(r, "showcaseId"),
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleRemoveShowcaseEntry(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	_, err := s.activitySvc.RemoveShowcaseEntry(r.Context(), &activitypb.RemoveShowcaseEntryRequest{
		UserId:     token.UID,
		ShowcaseId: chi.URLParam(r, "showcaseId"),
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleGetShowcaseProfilePictureUploadUrl(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody activitypb.GetShowcaseProfilePictureUploadUrlRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	reqBody.UserId = token.UID

	res, err := s.activitySvc.GetShowcaseProfilePictureUploadUrl(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetActivityPhotoUploadUrl(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody activitypb.GetActivityPhotoUploadUrlRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	reqBody.UserId = token.UID

	res, err := s.activitySvc.GetActivityPhotoUploadUrl(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}
