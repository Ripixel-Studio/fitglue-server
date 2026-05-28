// nolint:proto-json
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	pipelinem "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *APIServer) registerPipelineRoutes(r chi.Router) {
	r.Get("/users/me/pipelines", s.handleListPipelines)
	r.Get("/users/me/pipelines/{id}", s.handleGetPipeline)
	r.Post("/users/me/pipelines", s.handleCreatePipeline)
	r.Put("/users/me/pipelines/{id}", s.handleUpdatePipeline)
	r.Delete("/users/me/pipelines/{id}", s.handleDeletePipeline)

	r.Get("/users/me/pipelines/{id}/runs", s.handleListPipelineRuns)
	r.Get("/users/me/pipelines/{id}/runs/{runId}", s.handleGetPipelineRun)

	r.Get("/users/me/pipelines/{id}/source-activities", s.handleListSourceActivities)
	r.Post("/users/me/pipelines/{id}/backfill", s.handleBackfillActivities)

	r.Get("/users/me/connections/{provider}/activities", s.handleListConnectionActivities)
	r.Post("/users/me/connections/{provider}/backfill", s.handleBackfillConnectionActivities)

	r.Get("/users/me/pipeline-runs/{runId}/payload", s.handleGetPipelineRunPayload)

	r.Post("/users/me/pending-inputs/{inputId}/submit", s.handleSubmitInput)
	r.Post("/users/me/pending-inputs/{inputId}/cancel", s.handleCancelPipeline)
	r.Post("/users/me/pipeline-runs/{runId}/cancel", s.handleCancelPipelineRun)
	r.Post("/users/me/activities/{id}/repost", s.handleRepostActivity)
}

func (s *APIServer) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.ListPipelinesRequest{
		UserId: token.UID,
	}

	res, err := s.pipelineSvc.ListPipelines(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.GetPipelineRequest{
		UserId:     token.UID,
		PipelineId: chi.URLParam(r, "id"),
	}

	res, err := s.pipelineSvc.GetPipeline(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleCreatePipeline(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var pipeline pipelinem.PipelineConfig
	if err := decodeProto(r, &pipeline); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	reqBody := pipelinepb.CreatePipelineRequest{
		UserId:   token.UID,
		Pipeline: &pipeline,
	}

	res, err := s.pipelineSvc.CreatePipeline(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	WriteJSON(w, res)
}

func (s *APIServer) handleUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var pipeline pipelinem.PipelineConfig
	if err := decodeProto(r, &pipeline); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}
	var reqBody pipelinepb.UpdatePipelineRequest
	reqBody.Pipeline = &pipeline
	reqBody.UserId = token.UID
	reqBody.PipelineId = chi.URLParam(r, "id")

	res, err := s.pipelineSvc.UpdatePipeline(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.DeletePipelineRequest{
		UserId:     token.UID,
		PipelineId: chi.URLParam(r, "id"),
	}

	_, err := s.pipelineSvc.DeletePipeline(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleListPipelineRuns(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	pipelineID := chi.URLParam(r, "id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	pageToken := r.URL.Query().Get("page_token")

	req := &pipelinepb.ListPipelineRunsRequest{
		UserId:     token.UID,
		PipelineId: pipelineID,
		Limit:      int32(limit),
		PageToken:  pageToken,
	}

	res, err := s.pipelineSvc.ListPipelineRuns(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetPipelineRun(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.GetPipelineRunRequest{
		UserId: token.UID,
		RunId:  chi.URLParam(r, "runId"),
	}

	res, err := s.pipelineSvc.GetPipelineRun(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleSubmitInput(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var reqBody pipelinepb.SubmitInputRequest
	if err := decodeProto(r, &reqBody); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	reqBody.UserId = token.UID
	reqBody.PendingInputId = chi.URLParam(r, "inputId")

	_, err := s.pipelineSvc.SubmitInput(r.Context(), &reqBody)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleListSourceActivities(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.ListSourceActivitiesRequest{
		UserId:     token.UID,
		PipelineId: chi.URLParam(r, "id"),
		Source:     r.URL.Query().Get("source"),
		PageToken:  r.URL.Query().Get("page_token"),
	}

	res, err := s.pipelineSvc.ListSourceActivities(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

type backfillRequestBody struct {
	Source            string   `json:"source"`
	SourceActivityIds []string `json:"sourceActivityIds"`
}

func (s *APIServer) handleBackfillActivities(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	var body backfillRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	req := &pipelinepb.BackfillActivitiesRequest{
		UserId:            token.UID,
		PipelineId:        chi.URLParam(r, "id"),
		Source:            body.Source,
		SourceActivityIds: body.SourceActivityIds,
	}

	res, err := s.pipelineSvc.BackfillActivities(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleCancelPipeline(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.CancelPipelineRequest{
		UserId:         token.UID,
		PendingInputId: chi.URLParam(r, "inputId"),
	}

	_, err := s.pipelineSvc.CancelPipeline(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleCancelPipelineRun(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.CancelPipelineRunRequest{
		UserId: token.UID,
		RunId:  chi.URLParam(r, "runId"),
	}

	_, err := s.pipelineSvc.CancelPipelineRun(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleRepostActivity(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	req := &pipelinepb.RepostActivityRequest{
		UserId:     token.UID,
		ActivityId: chi.URLParam(r, "id"),
	}

	_, err := s.pipelineSvc.RepostActivity(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetPipelineRunPayload returns a short-lived signed GCS download URL for the
// original or enriched payload JSON stored for a pipeline run.
// Query param: which=original (default) | enriched
func (s *APIServer) handleGetPipelineRunPayload(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	if s.gcsSigner == nil {
		WriteError(w, statusError(http.StatusNotImplemented, "storage signing not configured"))
		return
	}

	runID := chi.URLParam(r, "runId")
	which := r.URL.Query().Get("which")

	run, err := s.pipelineSvc.GetPipelineRun(r.Context(), &pipelinepb.GetPipelineRunRequest{
		UserId: token.UID,
		RunId:  runID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	var gcsURI string
	if which == "enriched" {
		gcsURI = run.EnrichedEventUri
	} else {
		gcsURI = run.OriginalPayloadUri
	}

	if gcsURI == "" {
		WriteError(w, statusError(http.StatusNotFound, "payload not available for this run"))
		return
	}

	// Parse gs://bucket/path
	withoutScheme := strings.TrimPrefix(gcsURI, "gs://")
	parts := strings.SplitN(withoutScheme, "/", 2)
	if len(parts) != 2 {
		WriteError(w, statusError(http.StatusInternalServerError, "invalid GCS URI"))
		return
	}
	bucket, object := parts[0], parts[1]

	const signedURLExpiry = 15 * time.Minute
	downloadURL, err := s.gcsSigner.SignedURL(r.Context(), bucket, object, "", 0, signedURLExpiry)
	if err != nil {
		WriteError(w, statusError(http.StatusInternalServerError, "failed to sign URL"))
		return
	}

	expiresAt := time.Now().Add(signedURLExpiry)
	WriteJSON(w, map[string]interface{}{
		"download_url": downloadURL,
		"content_type": "application/json",
		"expires_at":   timestamppb.New(expiresAt),
	})
}

// providerToSource maps a connection provider ID (e.g. "strava") to the source
// enum string expected by the pipeline service (e.g. "SOURCE_STRAVA").
var providerToSource = map[string]string{
	"strava":    "SOURCE_STRAVA",
	"hevy":      "SOURCE_HEVY",
	"fitbit":    "SOURCE_FITBIT",
	"intervals": "SOURCE_INTERVALS",
}

func (s *APIServer) handleListConnectionActivities(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	provider := chi.URLParam(r, "provider")
	source, ok := providerToSource[provider]
	if !ok {
		WriteError(w, statusError(http.StatusBadRequest, "provider does not support historical activity import"))
		return
	}

	req := &pipelinepb.ListSourceActivitiesRequest{
		UserId:    token.UID,
		Source:    source,
		PageToken: r.URL.Query().Get("pageToken"),
	}

	res, err := s.pipelineSvc.ListSourceActivities(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleBackfillConnectionActivities(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	provider := chi.URLParam(r, "provider")
	source, ok := providerToSource[provider]
	if !ok {
		WriteError(w, statusError(http.StatusBadRequest, "provider does not support historical activity import"))
		return
	}

	var body backfillRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	req := &pipelinepb.BackfillActivitiesRequest{
		UserId:            token.UID,
		Source:            source,
		SourceActivityIds: body.SourceActivityIds,
	}

	res, err := s.pipelineSvc.BackfillActivities(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}
