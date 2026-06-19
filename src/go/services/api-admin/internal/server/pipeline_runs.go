package server

import (
	"net/http"
	"strconv"

	"github.com/fitglue/server/src/go/pkg/types/formatters"
	gatewaypb "github.com/fitglue/server/src/go/pkg/types/pb/gateway"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

func (s *APIServer) handleAdminPipelineRuns(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	source := r.URL.Query().Get("source")
	userID := queryParam(r, "userId", "user_id")
	pageToken := queryParam(r, "pageToken", "page_token")

	limit := int32(50)
	if l := queryParam(r, "limit"); l != "" {
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

	runs := res.GetRuns()
	// Stats are computed over the returned page. by_status is keyed by the human
	// label (via the shared formatter) so the console can read it directly.
	stats := &gatewaypb.AdminPipelineRunStats{
		Total:    int32(len(runs)),
		ByStatus: map[string]int32{},
		BySource: map[string]int32{},
	}
	for _, run := range runs {
		stats.ByStatus[formatters.FormatPipelineRunStatus(run.GetStatus())]++
		if src := run.GetSource(); src != "" {
			stats.BySource[src]++
		}
	}

	WriteJSON(w, &gatewaypb.ListPipelineRunsAdminResponse{
		Runs:          runs,
		NextPageToken: res.GetNextPageToken(),
		HasMore:       int32(len(runs)) == limit,
		Stats:         stats,
	})
}
