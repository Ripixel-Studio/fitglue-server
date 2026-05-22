package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fitglue/server/src/go/internal/infra"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

// ---- Mock: UserServiceClient (admin flavour) ----
// All methods use ...grpc.CallOption to correctly implement the gRPC client interface.

type adminMockUserClient struct {
	listResp   *userpb.ListUsersResponse
	listErr    error
	getResp    *pbuser.UserProfile
	getErr     error
	updateResp *pbuser.UserProfile
	updateErr  error
	deleteErr  error
}

func (m *adminMockUserClient) CreateUser(_ context.Context, _ *userpb.CreateUserRequest, _ ...grpc.CallOption) (*pbuser.UserProfile, error) {
	return &pbuser.UserProfile{}, nil
}
func (m *adminMockUserClient) GetProfile(_ context.Context, _ *userpb.GetProfileRequest, _ ...grpc.CallOption) (*pbuser.UserProfile, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.getResp != nil {
		return m.getResp, nil
	}
	return &pbuser.UserProfile{}, nil
}
func (m *adminMockUserClient) ListUsers(_ context.Context, _ *userpb.ListUsersRequest, _ ...grpc.CallOption) (*userpb.ListUsersResponse, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if m.listResp != nil {
		return m.listResp, nil
	}
	return &userpb.ListUsersResponse{}, nil
}
func (m *adminMockUserClient) UpdateProfile(_ context.Context, _ *userpb.UpdateProfileRequest, _ ...grpc.CallOption) (*pbuser.UserProfile, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	if m.updateResp != nil {
		return m.updateResp, nil
	}
	return &pbuser.UserProfile{}, nil
}
func (m *adminMockUserClient) GetIntegration(_ context.Context, _ *userpb.GetIntegrationRequest, _ ...grpc.CallOption) (*userpb.GetIntegrationResponse, error) {
	return &userpb.GetIntegrationResponse{}, nil
}
func (m *adminMockUserClient) SetIntegration(_ context.Context, _ *userpb.SetIntegrationRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) DeleteIntegration(_ context.Context, _ *userpb.DeleteIntegrationRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) ListIntegrations(_ context.Context, _ *userpb.ListIntegrationsRequest, _ ...grpc.CallOption) (*pbuser.UserIntegrations, error) {
	return &pbuser.UserIntegrations{}, nil
}
func (m *adminMockUserClient) GetNotificationPrefs(_ context.Context, _ *userpb.GetNotificationPrefsRequest, _ ...grpc.CallOption) (*pbuser.NotificationPreferences, error) {
	return &pbuser.NotificationPreferences{}, nil
}
func (m *adminMockUserClient) UpdateNotificationPrefs(_ context.Context, _ *userpb.UpdateNotificationPrefsRequest, _ ...grpc.CallOption) (*pbuser.NotificationPreferences, error) {
	return &pbuser.NotificationPreferences{}, nil
}
func (m *adminMockUserClient) ListCounters(_ context.Context, _ *userpb.ListCountersRequest, _ ...grpc.CallOption) (*userpb.ListCountersResponse, error) {
	return &userpb.ListCountersResponse{}, nil
}
func (m *adminMockUserClient) UpdateCounter(_ context.Context, _ *userpb.UpdateCounterRequest, _ ...grpc.CallOption) (*pbuser.Counter, error) {
	return &pbuser.Counter{}, nil
}
func (m *adminMockUserClient) GetBoosterData(_ context.Context, _ *userpb.GetBoosterDataRequest, _ ...grpc.CallOption) (*userpb.GetBoosterDataResponse, error) {
	return &userpb.GetBoosterDataResponse{}, nil
}
func (m *adminMockUserClient) SetBoosterData(_ context.Context, _ *userpb.SetBoosterDataRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) DeleteBoosterData(_ context.Context, _ *userpb.DeleteBoosterDataRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) DeleteUser(_ context.Context, _ *userpb.DeleteUserRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) SendVerificationEmail(_ context.Context, _ *userpb.SendVerificationEmailRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) SendPasswordResetEmail(_ context.Context, _ *userpb.SendPasswordResetEmailRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) SendEmailChangeVerification(_ context.Context, _ *userpb.SendEmailChangeVerificationRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) GenerateRegistrationSummary(_ context.Context, _ *userpb.GenerateRegistrationSummaryRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) ResolveUserByIntegration(_ context.Context, _ *userpb.ResolveUserByIntegrationRequest, _ ...grpc.CallOption) (*userpb.ResolveUserByIntegrationResponse, error) {
	return &userpb.ResolveUserByIntegrationResponse{}, nil
}
func (m *adminMockUserClient) SendWelcomeEmail(_ context.Context, _ *userpb.SendWelcomeEmailRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) DeleteCounter(_ context.Context, _ *userpb.DeleteCounterRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) ListPersonalRecords(_ context.Context, _ *userpb.ListPersonalRecordsRequest, _ ...grpc.CallOption) (*userpb.ListPersonalRecordsResponse, error) {
	return &userpb.ListPersonalRecordsResponse{}, nil
}
func (m *adminMockUserClient) SetPersonalRecord(_ context.Context, _ *userpb.SetPersonalRecordRequest, _ ...grpc.CallOption) (*pbuser.PersonalRecord, error) {
	return &pbuser.PersonalRecord{}, nil
}
func (m *adminMockUserClient) DeletePersonalRecord(_ context.Context, _ *userpb.DeletePersonalRecordRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) ListPluginDefaults(_ context.Context, _ *userpb.ListPluginDefaultsRequest, _ ...grpc.CallOption) (*userpb.ListPluginDefaultsResponse, error) {
	return &userpb.ListPluginDefaultsResponse{}, nil
}
func (m *adminMockUserClient) SetPluginDefaults(_ context.Context, _ *userpb.SetPluginDefaultsRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) DeletePluginDefaults(_ context.Context, _ *userpb.DeletePluginDefaultsRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminMockUserClient) SetFCMToken(_ context.Context, _ *userpb.SetFCMTokenRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// ---- Mock: PipelineServiceClient (nop) ----

type adminNopPipelineClient struct{}

func (m *adminNopPipelineClient) ListPipelines(_ context.Context, _ *pipelinepb.ListPipelinesRequest, _ ...grpc.CallOption) (*pipelinepb.ListPipelinesResponse, error) {
	return &pipelinepb.ListPipelinesResponse{}, nil
}
func (m *adminNopPipelineClient) GetPipeline(_ context.Context, _ *pipelinepb.GetPipelineRequest, _ ...grpc.CallOption) (*pbpipeline.PipelineConfig, error) {
	return nil, nil
}
func (m *adminNopPipelineClient) CreatePipeline(_ context.Context, _ *pipelinepb.CreatePipelineRequest, _ ...grpc.CallOption) (*pbpipeline.PipelineConfig, error) {
	return nil, nil
}
func (m *adminNopPipelineClient) UpdatePipeline(_ context.Context, _ *pipelinepb.UpdatePipelineRequest, _ ...grpc.CallOption) (*pbpipeline.PipelineConfig, error) {
	return nil, nil
}
func (m *adminNopPipelineClient) DeletePipeline(_ context.Context, _ *pipelinepb.DeletePipelineRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminNopPipelineClient) SubmitInput(_ context.Context, _ *pipelinepb.SubmitInputRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminNopPipelineClient) ListPendingInputs(_ context.Context, _ *pipelinepb.ListPendingInputsRequest, _ ...grpc.CallOption) (*pipelinepb.ListPendingInputsResponse, error) {
	return &pipelinepb.ListPendingInputsResponse{}, nil
}
func (m *adminNopPipelineClient) ResolvePendingInput(_ context.Context, _ *pipelinepb.ResolvePendingInputRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminNopPipelineClient) CancelPipeline(_ context.Context, _ *pipelinepb.CancelPipelineRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminNopPipelineClient) CancelPipelineRun(_ context.Context, _ *pipelinepb.CancelPipelineRunRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminNopPipelineClient) RepostActivity(_ context.Context, _ *pipelinepb.RepostActivityRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminNopPipelineClient) GetPipelineRun(_ context.Context, _ *pipelinepb.GetPipelineRunRequest, _ ...grpc.CallOption) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}
func (m *adminNopPipelineClient) ListPipelineRuns(_ context.Context, _ *pipelinepb.ListPipelineRunsRequest, _ ...grpc.CallOption) (*pipelinepb.ListPipelineRunsResponse, error) {
	return &pipelinepb.ListPipelineRunsResponse{}, nil
}
func (m *adminNopPipelineClient) AdminListPipelineRuns(_ context.Context, _ *pipelinepb.AdminListPipelineRunsRequest, _ ...grpc.CallOption) (*pipelinepb.AdminListPipelineRunsResponse, error) {
	return &pipelinepb.AdminListPipelineRunsResponse{}, nil
}
func (m *adminNopPipelineClient) ListSourceActivities(_ context.Context, _ *pipelinepb.ListSourceActivitiesRequest, _ ...grpc.CallOption) (*pipelinepb.ListSourceActivitiesResponse, error) {
	return &pipelinepb.ListSourceActivitiesResponse{}, nil
}
func (m *adminNopPipelineClient) BackfillActivities(_ context.Context, _ *pipelinepb.BackfillActivitiesRequest, _ ...grpc.CallOption) (*pipelinepb.BackfillActivitiesResponse, error) {
	return &pipelinepb.BackfillActivitiesResponse{}, nil
}

// ---- Helpers ----

func newAdminTestServer(userClient *adminMockUserClient) *APIServer {
	return &APIServer{
		logger:      infra.NewLogger(),
		userService: userClient,
		pipelineSvc: &adminNopPipelineClient{},
	}
}

func withAdminChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// ---- Errors / WriteError / WriteJSON ----

func TestAdminWriteError_GRPCCodes(t *testing.T) {
	tests := []struct {
		code     codes.Code
		httpCode int
		errCode  string
	}{
		{codes.NotFound, http.StatusNotFound, "NOT_FOUND"},
		{codes.InvalidArgument, http.StatusBadRequest, "INVALID_ARGUMENT"},
		{codes.PermissionDenied, http.StatusForbidden, "PERMISSION_DENIED"},
		{codes.Unauthenticated, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{codes.AlreadyExists, http.StatusConflict, "ALREADY_EXISTS"},
		{codes.Unimplemented, http.StatusNotImplemented, "NOT_IMPLEMENTED"},
		{codes.Unavailable, http.StatusServiceUnavailable, "UNAVAILABLE"},
		{codes.FailedPrecondition, http.StatusPreconditionFailed, "FAILED_PRECONDITION"},
		{codes.ResourceExhausted, http.StatusTooManyRequests, "RESOURCE_EXHAUSTED"},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout, "DEADLINE_EXCEEDED"},
		{codes.Internal, http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tc := range tests {
		t.Run(tc.errCode, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteError(w, status.Error(tc.code, "msg"))
			assert.Equal(t, tc.httpCode, w.Code)
			var apiErr APIError
			require.NoError(t, json.NewDecoder(w.Body).Decode(&apiErr))
			assert.Equal(t, tc.errCode, apiErr.Code)
		})
	}
}

func TestAdminWriteError_CustomError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, &CustomError{HTTPCode: http.StatusTeapot, Msg: "teapot"})
	assert.Equal(t, http.StatusTeapot, w.Code)
	var apiErr APIError
	require.NoError(t, json.NewDecoder(w.Body).Decode(&apiErr))
	assert.Equal(t, "CLIENT_ERROR", apiErr.Code)
}

func TestAdminWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	require.NoError(t, WriteJSON(w, map[string]string{"x": "y"}))
	assert.Contains(t, w.Body.String(), "y")
}

// ---- Middleware ----

func TestAdminJSONResponseMiddleware(t *testing.T) {
	handler := JSONResponseMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestAdminLoggerMiddleware(t *testing.T) {
	called := false
	handler := LoggerMiddleware(infra.NewLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	assert.True(t, called)
}

func TestAdminAuthMiddleware_MissingHeader(t *testing.T) {
	handler := AuthMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---- Handler tests ----

func TestAdminHandleListUsers_DefaultLimit(t *testing.T) {
	client := &adminMockUserClient{
		listResp: &userpb.ListUsersResponse{
			Users: []*pbuser.UserProfile{{UserId: "u1"}, {UserId: "u2"}},
		},
	}
	svc := newAdminTestServer(client)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	w := httptest.NewRecorder()
	svc.handleListUsers(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAdminHandleListUsers_CustomLimit(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?limit=10&page_token=tok", nil)
	w := httptest.NewRecorder()
	svc.handleListUsers(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAdminHandleListUsers_Error(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{listErr: status.Error(codes.Internal, "db error")})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	w := httptest.NewRecorder()
	svc.handleListUsers(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAdminHandleGetUser_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &pbuser.UserProfile{UserId: "u1"}})
	req := withAdminChiParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleGetUser(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "u1")
}

func TestAdminHandleGetUser_NotFound(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getErr: status.Error(codes.NotFound, "not found")})
	req := withAdminChiParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleGetUser(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandleDeleteUser_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withAdminChiParam(httptest.NewRequest(http.MethodDelete, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleDeleteUser(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestAdminHandleDeleteUser_Error(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{deleteErr: status.Error(codes.Internal, "fail")})
	req := withAdminChiParam(httptest.NewRequest(http.MethodDelete, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleDeleteUser(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAdminHandleUpdateUser_BadBody(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withAdminChiParam(
		httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString("not json")),
		"id", "u1",
	)
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminHandleUpdateUser_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &pbuser.UserProfile{UserId: "u1"}})
	req := withAdminChiParam(
		httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"accessEnabled":true}`)),
		"id", "u1",
	)
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAdminHandleListAllPipelines_MissingUserID(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/pipelines", nil)
	w := httptest.NewRecorder()
	svc.handleListAllPipelines(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminHandleListAllPipelines_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/pipelines?user_id=u1", nil)
	w := httptest.NewRecorder()
	svc.handleListAllPipelines(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
