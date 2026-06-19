package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	gatewaypb "github.com/fitglue/server/src/go/pkg/types/pb/gateway"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

func (s *APIServer) handleGetPipelineRun(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	runID := chi.URLParam(r, "runId")
	if userID == "" || runID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or run id"))
		return
	}
	res, err := s.pipelineSvc.GetPipelineRun(r.Context(), &pipelinepb.GetPipelineRunRequest{
		UserId: userID,
		RunId:  runID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, res)
}

func (s *APIServer) handleRepostActivity(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	activityID := chi.URLParam(r, "activityId")
	if userID == "" || activityID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or activity id"))
		return
	}

	var req struct {
		Mode        string `json:"mode"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	_, err := s.pipelineSvc.RepostActivity(r.Context(), &pipelinepb.RepostActivityRequest{
		UserId:      userID,
		ActivityId:  activityID,
		Mode:        req.Mode,
		Destination: req.Destination,
	})
	s.auditAction(r.Context(), r, "repost_activity", userID, map[string]string{
		"activityId":  activityID,
		"mode":        req.Mode,
		"destination": req.Destination,
	}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleCancelPipelineRun(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	runID := chi.URLParam(r, "runId")
	if userID == "" || runID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or run id"))
		return
	}
	_, err := s.pipelineSvc.CancelPipelineRun(r.Context(), &pipelinepb.CancelPipelineRunRequest{
		UserId: userID,
		RunId:  runID,
	})
	s.auditAction(r.Context(), r, "cancel_pipeline_run", userID, map[string]string{"runId": runID}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleListPendingInputs(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}
	res, err := s.pipelineSvc.ListPendingInputs(r.Context(), &pipelinepb.ListPendingInputsRequest{UserId: userID})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, &gatewaypb.ListPendingInputsAdminResponse{Inputs: pendingInputSummaries(res.GetInputs())})
}

func (s *APIServer) handleResolvePendingInput(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	inputID := chi.URLParam(r, "inputId")
	if userID == "" || inputID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or input id"))
		return
	}
	_, err := s.pipelineSvc.ResolvePendingInput(r.Context(), &pipelinepb.ResolvePendingInputRequest{
		UserId:         userID,
		PendingInputId: inputID,
	})
	s.auditAction(r.Context(), r, "resolve_pending_input", userID, map[string]string{"inputId": inputID}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
