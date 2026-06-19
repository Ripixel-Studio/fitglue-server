package server

import (
	"net/http"
	"strconv"

	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

func (s *APIServer) handleAdminPipelineRuns(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	source := r.URL.Query().Get("source")
	// The generated OpenAPI client sends camelCase query params; accept the
	// snake_case proto field names too for compatibility.
	userID := queryParam(r, "userId", "user_id")
	pageToken := queryParam(r, "pageToken", "page_token")

	limit := int32(50)
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = int32(parsed)
		}
	}

	res, err := s.pipelineSvc.AdminListPipelineRuns(r.Context(), &pipelinepb.AdminListPipelineRunsRequest{
		Status:    status,
		Source:    source,
		UserId:    userID,
		Limit:     limit,
		PageToken: pageToken,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}
