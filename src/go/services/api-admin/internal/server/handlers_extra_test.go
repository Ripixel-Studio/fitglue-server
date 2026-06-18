package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fitglue/server/src/go/internal/infra"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

// adminErrPipelineClient embeds the nop client and overrides the methods used by
// the admin handlers so their error branches can be exercised.
type adminErrPipelineClient struct {
	adminNopPipelineClient
	listPipelinesResp *pipelinepb.ListPipelinesResponse
	listPipelinesErr  error
	deletePipelineErr error
	listPendingResp   *pipelinepb.ListPendingInputsResponse
	listPendingErr    error
	resolvePendingErr error
	adminListRunsErr  error
	adminListRunsResp *pipelinepb.AdminListPipelineRunsResponse
}

func (m *adminErrPipelineClient) ListPipelines(_ context.Context, _ *pipelinepb.ListPipelinesRequest, _ ...grpc.CallOption) (*pipelinepb.ListPipelinesResponse, error) {
	if m.listPipelinesErr != nil {
		return nil, m.listPipelinesErr
	}
	if m.listPipelinesResp != nil {
		return m.listPipelinesResp, nil
	}
	return &pipelinepb.ListPipelinesResponse{}, nil
}

func (m *adminErrPipelineClient) DeletePipeline(_ context.Context, _ *pipelinepb.DeletePipelineRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.deletePipelineErr != nil {
		return nil, m.deletePipelineErr
	}
	return &emptypb.Empty{}, nil
}

func (m *adminErrPipelineClient) ListPendingInputs(_ context.Context, _ *pipelinepb.ListPendingInputsRequest, _ ...grpc.CallOption) (*pipelinepb.ListPendingInputsResponse, error) {
	if m.listPendingErr != nil {
		return nil, m.listPendingErr
	}
	if m.listPendingResp != nil {
		return m.listPendingResp, nil
	}
	return &pipelinepb.ListPendingInputsResponse{}, nil
}

func (m *adminErrPipelineClient) ResolvePendingInput(_ context.Context, _ *pipelinepb.ResolvePendingInputRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.resolvePendingErr != nil {
		return nil, m.resolvePendingErr
	}
	return &emptypb.Empty{}, nil
}

func (m *adminErrPipelineClient) AdminListPipelineRuns(_ context.Context, _ *pipelinepb.AdminListPipelineRunsRequest, _ ...grpc.CallOption) (*pipelinepb.AdminListPipelineRunsResponse, error) {
	if m.adminListRunsErr != nil {
		return nil, m.adminListRunsErr
	}
	if m.adminListRunsResp != nil {
		return m.adminListRunsResp, nil
	}
	return &pipelinepb.AdminListPipelineRunsResponse{}, nil
}

// newAdminTestServerWithPipeline builds a server with a custom pipeline client.
func newAdminTestServerWithPipeline(userClient *adminMockUserClient, pipelineClient pipelinepb.PipelineServiceClient) *APIServer {
	return &APIServer{
		logger:      infra.NewLogger(),
		userService: userClient,
		pipelineSvc: pipelineClient,
	}
}

// ---- Missing-id branches ----

func TestAdminHandleGetUser_MissingID(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := httptest.NewRequest(http.MethodGet, "/", nil) // no chi param set
	w := httptest.NewRecorder()
	svc.handleGetUser(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminHandleUpdateUser_MissingID(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := httptest.NewRequest(http.MethodPut, "/", nil)
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminHandleDeleteUser_MissingID(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	w := httptest.NewRecorder()
	svc.handleDeleteUser(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- UpdateUser error branches ----

func TestAdminHandleUpdateUser_GetProfileError(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getErr: status.Error(codes.NotFound, "no user")})
	req := withAdminChiParam(
		httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"accessEnabled":true}`)),
		"id", "u1",
	)
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandleUpdateUser_UpdateProfileError(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{updateErr: status.Error(codes.Internal, "write failed")})
	req := withAdminChiParam(
		httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"accessEnabled":false}`)),
		"id", "u1",
	)
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---- ListAllPipelines error branch ----

func TestAdminHandleListAllPipelines_PipelineError(t *testing.T) {
	svc := newAdminTestServerWithPipeline(
		&adminMockUserClient{},
		&adminErrPipelineClient{listPipelinesErr: status.Error(codes.Unavailable, "down")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/pipelines?user_id=u1", nil)
	w := httptest.NewRecorder()
	svc.handleListAllPipelines(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ---- deleteUserPipelines / deleteUserPendingInputs branches ----

func TestDeleteUserPipelines_DeletesEach(t *testing.T) {
	pc := &adminErrPipelineClient{
		listPipelinesResp: &pipelinepb.ListPipelinesResponse{
			Pipelines: []*pbpipeline.PipelineConfig{{Id: "p1"}, {Id: "p2"}},
		},
	}
	svc := newAdminTestServerWithPipeline(&adminMockUserClient{}, pc)
	if err := svc.deleteUserPipelines(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteUserPipelines_ListError(t *testing.T) {
	pc := &adminErrPipelineClient{listPipelinesErr: status.Error(codes.Internal, "boom")}
	svc := newAdminTestServerWithPipeline(&adminMockUserClient{}, pc)
	if err := svc.deleteUserPipelines(context.Background(), "u1"); err == nil {
		t.Error("expected error from ListPipelines")
	}
}

func TestDeleteUserPipelines_DeleteError(t *testing.T) {
	pc := &adminErrPipelineClient{
		listPipelinesResp: &pipelinepb.ListPipelinesResponse{Pipelines: []*pbpipeline.PipelineConfig{{Id: "p1"}}},
		deletePipelineErr: status.Error(codes.Internal, "delete failed"),
	}
	svc := newAdminTestServerWithPipeline(&adminMockUserClient{}, pc)
	if err := svc.deleteUserPipelines(context.Background(), "u1"); err == nil {
		t.Error("expected error from DeletePipeline")
	}
}

func TestDeleteUserPendingInputs_ResolvesEach(t *testing.T) {
	pc := &adminErrPipelineClient{
		listPendingResp: &pipelinepb.ListPendingInputsResponse{
			Inputs: []*pbpipeline.PendingInput{{ActivityId: "a1"}, {ActivityId: "a2"}},
		},
	}
	svc := newAdminTestServerWithPipeline(&adminMockUserClient{}, pc)
	if err := svc.deleteUserPendingInputs(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteUserPendingInputs_ListError(t *testing.T) {
	pc := &adminErrPipelineClient{listPendingErr: status.Error(codes.Internal, "boom")}
	svc := newAdminTestServerWithPipeline(&adminMockUserClient{}, pc)
	if err := svc.deleteUserPendingInputs(context.Background(), "u1"); err == nil {
		t.Error("expected error from ListPendingInputs")
	}
}

func TestDeleteUserPendingInputs_ResolveError(t *testing.T) {
	pc := &adminErrPipelineClient{
		listPendingResp:   &pipelinepb.ListPendingInputsResponse{Inputs: []*pbpipeline.PendingInput{{ActivityId: "a1"}}},
		resolvePendingErr: status.Error(codes.Internal, "resolve failed"),
	}
	svc := newAdminTestServerWithPipeline(&adminMockUserClient{}, pc)
	if err := svc.deleteUserPendingInputs(context.Background(), "u1"); err == nil {
		t.Error("expected error from ResolvePendingInput")
	}
}

// ---- pipeline-runs error branch ----

func TestHandleAdminPipelineRuns_Error(t *testing.T) {
	svc := newAdminTestServerWithPipeline(
		&adminMockUserClient{},
		&adminErrPipelineClient{adminListRunsErr: status.Error(codes.Internal, "fail")},
	)
	r := httptest.NewRequest(http.MethodGet, "/admin/pipeline-runs", nil)
	w := httptest.NewRecorder()
	svc.handleAdminPipelineRuns(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---- deleteUserIntegrations / deleteUserPendingInputs against dead Firestore ----

func TestDeleteUserIntegrations_FirestoreError(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	svc.firestoreClient = deadFirestore(t)
	defer svc.firestoreClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// The integrations delete should fail against an unreachable emulator.
	if err := svc.deleteUserIntegrations(ctx, "u1"); err == nil {
		t.Error("expected error deleting integrations from unreachable Firestore")
	}
}

// ---- Full router wiring: health + ServeHTTP ----

func TestServeHTTP_HealthRoute(t *testing.T) {
	// setupRoutes wires the health endpoint (registered before AdminMiddleware), so
	// it is reachable with nil auth/firestore clients. This exercises setupRoutes +
	// ServeHTTP without needing a live backend.
	svc := newAdminTestServer(&adminMockUserClient{})
	svc.router = chi.NewRouter()
	svc.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/health", nil)
	w := httptest.NewRecorder()
	svc.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}
