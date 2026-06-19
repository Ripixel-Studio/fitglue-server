package server

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/encoding/protojson"

	gatewaypb "github.com/fitglue/server/src/go/pkg/types/pb/gateway"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

func (s *APIServer) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	pipelineID := chi.URLParam(r, "pipelineId")
	if userID == "" || pipelineID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or pipeline id"))
		return
	}
	res, err := s.pipelineSvc.GetPipeline(r.Context(), &pipelinepb.GetPipelineRequest{
		UserId:     userID,
		PipelineId: pipelineID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, res)
}

func (s *APIServer) handleUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	pipelineID := chi.URLParam(r, "pipelineId")
	if userID == "" || pipelineID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or pipeline id"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "failed to read request body"))
		return
	}
	var req gatewaypb.UpdatePipelineAdminRequest
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, &req); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid pipeline config: "+err.Error()))
		return
	}
	if req.GetPipeline() == nil {
		WriteError(w, statusError(http.StatusBadRequest, "pipeline config is required"))
		return
	}

	res, err := s.pipelineSvc.UpdatePipeline(r.Context(), &pipelinepb.UpdatePipelineRequest{
		UserId:     userID,
		PipelineId: pipelineID,
		Pipeline:   req.GetPipeline(),
	})
	s.auditAction(r.Context(), r, "update_pipeline", userID, map[string]string{"pipelineId": pipelineID}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, res)
}

func (s *APIServer) handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	pipelineID := chi.URLParam(r, "pipelineId")
	if userID == "" || pipelineID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id or pipeline id"))
		return
	}
	_, err := s.pipelineSvc.DeletePipeline(r.Context(), &pipelinepb.DeletePipelineRequest{
		UserId:     userID,
		PipelineId: pipelineID,
	})
	s.auditAction(r.Context(), r, "delete_pipeline", userID, map[string]string{"pipelineId": pipelineID}, err)
	if err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
