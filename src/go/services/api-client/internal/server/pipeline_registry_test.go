package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	registrypb "github.com/fitglue/server/src/go/pkg/types/pb/services/registry"
)

// =============================================================
// Mock PipelineServiceClient
// =============================================================

type mockPipelineServiceClient struct {
	listPipelines    func(ctx context.Context, in *pipelinepb.ListPipelinesRequest, opts ...grpc.CallOption) (*pipelinepb.ListPipelinesResponse, error)
	getPipeline      func(ctx context.Context, in *pipelinepb.GetPipelineRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineConfig, error)
	createPipeline   func(ctx context.Context, in *pipelinepb.CreatePipelineRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineConfig, error)
	updatePipeline   func(ctx context.Context, in *pipelinepb.UpdatePipelineRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineConfig, error)
	deletePipeline   func(ctx context.Context, in *pipelinepb.DeletePipelineRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	listPipelineRuns func(ctx context.Context, in *pipelinepb.ListPipelineRunsRequest, opts ...grpc.CallOption) (*pipelinepb.ListPipelineRunsResponse, error)
	getPipelineRun   func(ctx context.Context, in *pipelinepb.GetPipelineRunRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineRun, error)
	submitInput      func(ctx context.Context, in *pipelinepb.SubmitInputRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	repostActivity   func(ctx context.Context, in *pipelinepb.RepostActivityRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

func (m *mockPipelineServiceClient) ListPipelines(ctx context.Context, in *pipelinepb.ListPipelinesRequest, opts ...grpc.CallOption) (*pipelinepb.ListPipelinesResponse, error) {
	if m.listPipelines != nil {
		return m.listPipelines(ctx, in, opts...)
	}
	return &pipelinepb.ListPipelinesResponse{}, nil
}
func (m *mockPipelineServiceClient) GetPipeline(ctx context.Context, in *pipelinepb.GetPipelineRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineConfig, error) {
	if m.getPipeline != nil {
		return m.getPipeline(ctx, in, opts...)
	}
	return &pbpipeline.PipelineConfig{}, nil
}
func (m *mockPipelineServiceClient) CreatePipeline(ctx context.Context, in *pipelinepb.CreatePipelineRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineConfig, error) {
	if m.createPipeline != nil {
		return m.createPipeline(ctx, in, opts...)
	}
	return &pbpipeline.PipelineConfig{}, nil
}
func (m *mockPipelineServiceClient) UpdatePipeline(ctx context.Context, in *pipelinepb.UpdatePipelineRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineConfig, error) {
	if m.updatePipeline != nil {
		return m.updatePipeline(ctx, in, opts...)
	}
	return &pbpipeline.PipelineConfig{}, nil
}
func (m *mockPipelineServiceClient) DeletePipeline(ctx context.Context, in *pipelinepb.DeletePipelineRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.deletePipeline != nil {
		return m.deletePipeline(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockPipelineServiceClient) SubmitInput(ctx context.Context, in *pipelinepb.SubmitInputRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.submitInput != nil {
		return m.submitInput(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockPipelineServiceClient) ListPendingInputs(ctx context.Context, in *pipelinepb.ListPendingInputsRequest, opts ...grpc.CallOption) (*pipelinepb.ListPendingInputsResponse, error) {
	return &pipelinepb.ListPendingInputsResponse{}, nil
}
func (m *mockPipelineServiceClient) ResolvePendingInput(ctx context.Context, in *pipelinepb.ResolvePendingInputRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockPipelineServiceClient) CancelPipeline(ctx context.Context, in *pipelinepb.CancelPipelineRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockPipelineServiceClient) RepostActivity(ctx context.Context, in *pipelinepb.RepostActivityRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.repostActivity != nil {
		return m.repostActivity(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockPipelineServiceClient) GetPipelineRun(ctx context.Context, in *pipelinepb.GetPipelineRunRequest, opts ...grpc.CallOption) (*pbpipeline.PipelineRun, error) {
	if m.getPipelineRun != nil {
		return m.getPipelineRun(ctx, in, opts...)
	}
	return &pbpipeline.PipelineRun{}, nil
}
func (m *mockPipelineServiceClient) ListPipelineRuns(ctx context.Context, in *pipelinepb.ListPipelineRunsRequest, opts ...grpc.CallOption) (*pipelinepb.ListPipelineRunsResponse, error) {
	if m.listPipelineRuns != nil {
		return m.listPipelineRuns(ctx, in, opts...)
	}
	return &pipelinepb.ListPipelineRunsResponse{}, nil
}
func (m *mockPipelineServiceClient) AdminListPipelineRuns(ctx context.Context, in *pipelinepb.AdminListPipelineRunsRequest, opts ...grpc.CallOption) (*pipelinepb.AdminListPipelineRunsResponse, error) {
	return &pipelinepb.AdminListPipelineRunsResponse{}, nil
}
func (m *mockPipelineServiceClient) ListSourceActivities(ctx context.Context, in *pipelinepb.ListSourceActivitiesRequest, opts ...grpc.CallOption) (*pipelinepb.ListSourceActivitiesResponse, error) {
	return &pipelinepb.ListSourceActivitiesResponse{}, nil
}
func (m *mockPipelineServiceClient) BackfillActivities(ctx context.Context, in *pipelinepb.BackfillActivitiesRequest, opts ...grpc.CallOption) (*pipelinepb.BackfillActivitiesResponse, error) {
	return &pipelinepb.BackfillActivitiesResponse{}, nil
}

// =============================================================
// Mock RegistryServiceClient
// =============================================================

type mockRegistryServiceClient struct {
	listPlugins   func(ctx context.Context, in *registrypb.ListPluginsRequest, opts ...grpc.CallOption) (*registrypb.ListPluginsResponse, error)
	getPlugin     func(ctx context.Context, in *registrypb.GetPluginRequest, opts ...grpc.CallOption) (*pbplugin.PluginManifest, error)
	getPluginIcon func(ctx context.Context, in *registrypb.GetPluginIconRequest, opts ...grpc.CallOption) (*registrypb.GetPluginIconResponse, error)
}

func (m *mockRegistryServiceClient) ListPlugins(ctx context.Context, in *registrypb.ListPluginsRequest, opts ...grpc.CallOption) (*registrypb.ListPluginsResponse, error) {
	if m.listPlugins != nil {
		return m.listPlugins(ctx, in, opts...)
	}
	return &registrypb.ListPluginsResponse{}, nil
}
func (m *mockRegistryServiceClient) GetPlugin(ctx context.Context, in *registrypb.GetPluginRequest, opts ...grpc.CallOption) (*pbplugin.PluginManifest, error) {
	if m.getPlugin != nil {
		return m.getPlugin(ctx, in, opts...)
	}
	return &pbplugin.PluginManifest{}, nil
}
func (m *mockRegistryServiceClient) ListCategories(ctx context.Context, in *registrypb.ListCategoriesRequest, opts ...grpc.CallOption) (*registrypb.ListCategoriesResponse, error) {
	return &registrypb.ListCategoriesResponse{}, nil
}
func (m *mockRegistryServiceClient) GetPluginIcon(ctx context.Context, in *registrypb.GetPluginIconRequest, opts ...grpc.CallOption) (*registrypb.GetPluginIconResponse, error) {
	if m.getPluginIcon != nil {
		return m.getPluginIcon(ctx, in, opts...)
	}
	return &registrypb.GetPluginIconResponse{ContentType: "image/png", IconData: []byte{0x89, 0x50}}, nil
}
func (m *mockRegistryServiceClient) ListSources(ctx context.Context, in *registrypb.ListSourcesRequest, opts ...grpc.CallOption) (*registrypb.ListSourcesResponse, error) {
	return &registrypb.ListSourcesResponse{}, nil
}
func (m *mockRegistryServiceClient) ListDestinations(ctx context.Context, in *registrypb.ListDestinationsRequest, opts ...grpc.CallOption) (*registrypb.ListDestinationsResponse, error) {
	return &registrypb.ListDestinationsResponse{}, nil
}
func (m *mockRegistryServiceClient) GetPluginRegistry(ctx context.Context, in *registrypb.GetPluginRegistryRequest, opts ...grpc.CallOption) (*pbplugin.PluginRegistryResponse, error) {
	return &pbplugin.PluginRegistryResponse{}, nil
}

// =============================================================
// Pipeline Handler Tests
// =============================================================

func buildPipelineServer(pSvc pipelinepb.PipelineServiceClient) *APIServer {
	return &APIServer{pipelineSvc: pSvc}
}

func TestHandleListPipelines_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListPipelines(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListPipelines_NoToken(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines", nil)
	w := httptest.NewRecorder()
	s.handleListPipelines(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleListPipelines_ServiceError(t *testing.T) {
	svc := &mockPipelineServiceClient{
		listPipelines: func(_ context.Context, _ *pipelinepb.ListPipelinesRequest, _ ...grpc.CallOption) (*pipelinepb.ListPipelinesResponse, error) {
			return nil, status.Error(codes.Internal, "db error")
		},
	}
	s := buildPipelineServer(svc)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListPipelines(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetPipeline_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines/pipe1", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetPipeline(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetPipeline_NoToken(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines/pipe1", nil)
	w := httptest.NewRecorder()
	s.handleGetPipeline(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleCreatePipeline_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	body, _ := json.Marshal(map[string]string{"name": "My Pipeline"})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/pipelines", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleCreatePipeline(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestHandleCreatePipeline_InvalidJSON(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/pipelines", strings.NewReader("not json"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleCreatePipeline(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreatePipeline_NoToken(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	body, _ := json.Marshal(map[string]string{"name": "Test"})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/pipelines", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleCreatePipeline(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleUpdatePipeline_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	body, _ := json.Marshal(map[string]string{"name": "Updated"})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/pipelines/pipe1", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdatePipeline(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdatePipeline_InvalidJSON(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/pipelines/pipe1", strings.NewReader("bad"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdatePipeline(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeletePipeline_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodDelete, "/api/v2/users/me/pipelines/pipe1", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleDeletePipeline(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleDeletePipeline_NoToken(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodDelete, "/api/v2/users/me/pipelines/pipe1", nil)
	w := httptest.NewRecorder()
	s.handleDeletePipeline(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleListPipelineRuns_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines/pipe1/runs?limit=10", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListPipelineRuns(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListPipelineRuns_NoToken(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines/pipe1/runs", nil)
	w := httptest.NewRecorder()
	s.handleListPipelineRuns(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleGetPipelineRun_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines/pipe1/runs/run1", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetPipelineRun(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetPipelineRun_NoToken(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pipelines/pipe1/runs/run1", nil)
	w := httptest.NewRecorder()
	s.handleGetPipelineRun(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleSubmitInput_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	body, _ := json.Marshal(map[string]string{"value": "abc"})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/pending-inputs/inp1/submit", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleSubmitInput(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleSubmitInput_InvalidJSON(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/pending-inputs/inp1/submit", strings.NewReader("bad"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleSubmitInput(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSubmitInput_NoToken(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	body, _ := json.Marshal(map[string]string{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/pending-inputs/inp1/submit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSubmitInput(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleRepostActivity_Success(t *testing.T) {
	s := buildPipelineServer(&mockPipelineServiceClient{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/activities/act1/repost", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleRepostActivity(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleRepostActivity_ServiceError(t *testing.T) {
	svc := &mockPipelineServiceClient{
		repostActivity: func(_ context.Context, _ *pipelinepb.RepostActivityRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, status.Error(codes.NotFound, "activity not found")
		},
	}
	s := buildPipelineServer(svc)
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/activities/act1/repost", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleRepostActivity(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// =============================================================
// Registry Handler Tests
// =============================================================

func buildRegistryServer(regSvc registrypb.RegistryServiceClient) *APIServer {
	return &APIServer{registrySvc: regSvc}
}

func TestHandleListPlugins_Success(t *testing.T) {
	s := buildRegistryServer(&mockRegistryServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/plugins?category=enricher", nil)
	w := httptest.NewRecorder()
	s.handleListPlugins(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListPlugins_ServiceError(t *testing.T) {
	reg := &mockRegistryServiceClient{
		listPlugins: func(_ context.Context, _ *registrypb.ListPluginsRequest, _ ...grpc.CallOption) (*registrypb.ListPluginsResponse, error) {
			return nil, status.Error(codes.Internal, "load failed")
		},
	}
	s := buildRegistryServer(reg)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/plugins", nil)
	w := httptest.NewRecorder()
	s.handleListPlugins(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetPlugin_Success(t *testing.T) {
	s := buildRegistryServer(&mockRegistryServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/plugins/strava", nil)
	w := httptest.NewRecorder()
	s.handleGetPlugin(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetPlugin_NotFound(t *testing.T) {
	reg := &mockRegistryServiceClient{
		getPlugin: func(_ context.Context, _ *registrypb.GetPluginRequest, _ ...grpc.CallOption) (*pbplugin.PluginManifest, error) {
			return nil, status.Error(codes.NotFound, "plugin not found")
		},
	}
	s := buildRegistryServer(reg)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/plugins/unknown", nil)
	w := httptest.NewRecorder()
	s.handleGetPlugin(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetPluginIcon_Success(t *testing.T) {
	s := buildRegistryServer(&mockRegistryServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/plugins/strava/icon", nil)
	w := httptest.NewRecorder()
	s.handleGetPluginIcon(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected image/png content type, got %q", w.Header().Get("Content-Type"))
	}
}

func TestHandleGetPluginIcon_Error(t *testing.T) {
	reg := &mockRegistryServiceClient{
		getPluginIcon: func(_ context.Context, _ *registrypb.GetPluginIconRequest, _ ...grpc.CallOption) (*registrypb.GetPluginIconResponse, error) {
			return nil, status.Error(codes.NotFound, "icon not found")
		},
	}
	s := buildRegistryServer(reg)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/plugins/unknown/icon", nil)
	w := httptest.NewRecorder()
	s.handleGetPluginIcon(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleListCategories_Success(t *testing.T) {
	s := buildRegistryServer(&mockRegistryServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/categories", nil)
	w := httptest.NewRecorder()
	s.handleListCategories(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListSources_Success(t *testing.T) {
	s := buildRegistryServer(&mockRegistryServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/registry/sources", nil)
	w := httptest.NewRecorder()
	s.handleListSources(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
