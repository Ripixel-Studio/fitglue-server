package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

// mockApiKeyStore implements ApiKeyStore for testing
type mockApiKeyStore struct {
	createFunc func(ctx context.Context, keyHash, userID, label string, scopes []string, createdAt time.Time) error
}

func (m *mockApiKeyStore) CreateIngressKey(ctx context.Context, keyHash, userID, label string, scopes []string, createdAt time.Time) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, keyHash, userID, label, scopes, createdAt)
	}
	return nil
}

// =============================================================
// Mock UserServiceClient
// =============================================================

type mockUserServiceClient struct {
	getProfile              func(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error)
	updateProfile           func(ctx context.Context, in *userpb.UpdateProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error)
	getIntegration          func(ctx context.Context, in *userpb.GetIntegrationRequest, opts ...grpc.CallOption) (*userpb.GetIntegrationResponse, error)
	setIntegration          func(ctx context.Context, in *userpb.SetIntegrationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	deleteIntegration       func(ctx context.Context, in *userpb.DeleteIntegrationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	listIntegrations        func(ctx context.Context, in *userpb.ListIntegrationsRequest, opts ...grpc.CallOption) (*pbuser.UserIntegrations, error)
	getNotificationPrefs    func(ctx context.Context, in *userpb.GetNotificationPrefsRequest, opts ...grpc.CallOption) (*pbuser.NotificationPreferences, error)
	updateNotificationPrefs func(ctx context.Context, in *userpb.UpdateNotificationPrefsRequest, opts ...grpc.CallOption) (*pbuser.NotificationPreferences, error)
	listCounters            func(ctx context.Context, in *userpb.ListCountersRequest, opts ...grpc.CallOption) (*userpb.ListCountersResponse, error)
	updateCounter           func(ctx context.Context, in *userpb.UpdateCounterRequest, opts ...grpc.CallOption) (*pbuser.Counter, error)
}

func (m *mockUserServiceClient) CreateUser(ctx context.Context, in *userpb.CreateUserRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
	return &pbuser.UserProfile{}, nil
}
func (m *mockUserServiceClient) GetProfile(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
	if m.getProfile != nil {
		return m.getProfile(ctx, in, opts...)
	}
	return &pbuser.UserProfile{UserId: in.UserId}, nil
}
func (m *mockUserServiceClient) ListUsers(ctx context.Context, in *userpb.ListUsersRequest, opts ...grpc.CallOption) (*userpb.ListUsersResponse, error) {
	return &userpb.ListUsersResponse{}, nil
}
func (m *mockUserServiceClient) UpdateProfile(ctx context.Context, in *userpb.UpdateProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
	if m.updateProfile != nil {
		return m.updateProfile(ctx, in, opts...)
	}
	return &pbuser.UserProfile{UserId: in.UserId}, nil
}
func (m *mockUserServiceClient) GetIntegration(ctx context.Context, in *userpb.GetIntegrationRequest, opts ...grpc.CallOption) (*userpb.GetIntegrationResponse, error) {
	if m.getIntegration != nil {
		return m.getIntegration(ctx, in, opts...)
	}
	return &userpb.GetIntegrationResponse{}, nil
}
func (m *mockUserServiceClient) SetIntegration(ctx context.Context, in *userpb.SetIntegrationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.setIntegration != nil {
		return m.setIntegration(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) DeleteIntegration(ctx context.Context, in *userpb.DeleteIntegrationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.deleteIntegration != nil {
		return m.deleteIntegration(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) ListIntegrations(ctx context.Context, in *userpb.ListIntegrationsRequest, opts ...grpc.CallOption) (*pbuser.UserIntegrations, error) {
	if m.listIntegrations != nil {
		return m.listIntegrations(ctx, in, opts...)
	}
	return &pbuser.UserIntegrations{}, nil
}
func (m *mockUserServiceClient) GetNotificationPrefs(ctx context.Context, in *userpb.GetNotificationPrefsRequest, opts ...grpc.CallOption) (*pbuser.NotificationPreferences, error) {
	if m.getNotificationPrefs != nil {
		return m.getNotificationPrefs(ctx, in, opts...)
	}
	return &pbuser.NotificationPreferences{}, nil
}
func (m *mockUserServiceClient) UpdateNotificationPrefs(ctx context.Context, in *userpb.UpdateNotificationPrefsRequest, opts ...grpc.CallOption) (*pbuser.NotificationPreferences, error) {
	if m.updateNotificationPrefs != nil {
		return m.updateNotificationPrefs(ctx, in, opts...)
	}
	return &pbuser.NotificationPreferences{}, nil
}
func (m *mockUserServiceClient) ListCounters(ctx context.Context, in *userpb.ListCountersRequest, opts ...grpc.CallOption) (*userpb.ListCountersResponse, error) {
	if m.listCounters != nil {
		return m.listCounters(ctx, in, opts...)
	}
	return &userpb.ListCountersResponse{}, nil
}
func (m *mockUserServiceClient) UpdateCounter(ctx context.Context, in *userpb.UpdateCounterRequest, opts ...grpc.CallOption) (*pbuser.Counter, error) {
	if m.updateCounter != nil {
		return m.updateCounter(ctx, in, opts...)
	}
	return &pbuser.Counter{}, nil
}
func (m *mockUserServiceClient) GetBoosterData(ctx context.Context, in *userpb.GetBoosterDataRequest, opts ...grpc.CallOption) (*userpb.GetBoosterDataResponse, error) {
	return &userpb.GetBoosterDataResponse{}, nil
}
func (m *mockUserServiceClient) SetBoosterData(ctx context.Context, in *userpb.SetBoosterDataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) DeleteBoosterData(ctx context.Context, in *userpb.DeleteBoosterDataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) DeleteUser(ctx context.Context, in *userpb.DeleteUserRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) SendVerificationEmail(ctx context.Context, in *userpb.SendVerificationEmailRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) SendPasswordResetEmail(ctx context.Context, in *userpb.SendPasswordResetEmailRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) SendEmailChangeVerification(ctx context.Context, in *userpb.SendEmailChangeVerificationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) GenerateRegistrationSummary(ctx context.Context, in *userpb.GenerateRegistrationSummaryRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) ResolveUserByIntegration(ctx context.Context, in *userpb.ResolveUserByIntegrationRequest, opts ...grpc.CallOption) (*userpb.ResolveUserByIntegrationResponse, error) {
	return &userpb.ResolveUserByIntegrationResponse{}, nil
}
func (m *mockUserServiceClient) SendWelcomeEmail(ctx context.Context, in *userpb.SendWelcomeEmailRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) ListPersonalRecords(ctx context.Context, in *userpb.ListPersonalRecordsRequest, opts ...grpc.CallOption) (*userpb.ListPersonalRecordsResponse, error) {
	return &userpb.ListPersonalRecordsResponse{}, nil
}
func (m *mockUserServiceClient) SetPersonalRecord(ctx context.Context, in *userpb.SetPersonalRecordRequest, opts ...grpc.CallOption) (*pbuser.PersonalRecord, error) {
	return &pbuser.PersonalRecord{}, nil
}
func (m *mockUserServiceClient) DeletePersonalRecord(ctx context.Context, in *userpb.DeletePersonalRecordRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) ListPluginDefaults(ctx context.Context, in *userpb.ListPluginDefaultsRequest, opts ...grpc.CallOption) (*userpb.ListPluginDefaultsResponse, error) {
	return &userpb.ListPluginDefaultsResponse{}, nil
}
func (m *mockUserServiceClient) SetPluginDefaults(ctx context.Context, in *userpb.SetPluginDefaultsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) DeletePluginDefaults(ctx context.Context, in *userpb.DeletePluginDefaultsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) DeleteCounter(ctx context.Context, in *userpb.DeleteCounterRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockUserServiceClient) SetFCMToken(ctx context.Context, in *userpb.SetFCMTokenRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// =============================================================
// Mock Publisher
// =============================================================

type mockPublisher struct {
	publishFunc func(ctx context.Context, topicID string, e event.Event) (string, error)
}

func (m *mockPublisher) PublishCloudEvent(ctx context.Context, topicID string, e event.Event) (string, error) {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, topicID, e)
	}
	return "msg-id", nil
}

func (m *mockPublisher) PublishJSON(_ context.Context, _ string, _ []byte) error { return nil }

// =============================================================
// Test helpers
// =============================================================

// buildServer creates an APIServer with mock dependencies — no Firebase auth client needed.
// This returns the raw server struct directly for direct handler invocation.
func buildTestServer(userSvc userpb.UserServiceClient, pub Publisher) *APIServer {
	return &APIServer{
		router:      nil, // Not needed for direct handler tests
		userService: userSvc,
		publisher:   pub,
	}
}

// withToken injects a fake Firebase token into the request context. This
// bypasses Firebase verification so we can test handlers directly.
func withToken(r *http.Request, uid string) *http.Request {
	token := &auth.Token{UID: uid}
	ctx := context.WithValue(r.Context(), userContextKey, token)
	return r.WithContext(ctx)
}

// =============================================================
// WriteJSON / WriteError
// =============================================================

func TestWriteJSON_Proto(t *testing.T) {
	w := httptest.NewRecorder()
	profile := &pbuser.UserProfile{UserId: "user1"}
	if err := WriteJSON(w, profile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "user1") {
		t.Error("expected user1 in response body")
	}
}

func TestWriteJSON_NoProto(t *testing.T) {
	w := httptest.NewRecorder()
	if err := WriteJSON(w, map[string]string{"key": "value"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(w.Body.String(), "value") {
		t.Error("expected value in response body")
	}
}

func TestWriteError_GRPCNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, status.Error(codes.NotFound, "not found"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestWriteError_GRPCPermissionDenied(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, status.Error(codes.PermissionDenied, "forbidden"))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestWriteError_CustomError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, &CustomError{HTTPCode: http.StatusBadRequest, Msg: "bad input"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWriteError_InternalError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, status.Error(codes.Internal, "internal"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// =============================================================
// handleGetProfile
// =============================================================

func TestHandleGetProfile_Success(t *testing.T) {
	svc := &mockUserServiceClient{
		getProfile: func(_ context.Context, in *userpb.GetProfileRequest, _ ...grpc.CallOption) (*pbuser.UserProfile, error) {
			return &pbuser.UserProfile{UserId: in.UserId, DisplayName: "Alice"}, nil
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me", nil)
	r = withToken(r, "user-alice")
	w := httptest.NewRecorder()
	s.handleGetProfile(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Alice") {
		t.Error("expected Alice in response body")
	}
}

func TestHandleGetProfile_NoToken(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me", nil)
	// note: no token injected
	w := httptest.NewRecorder()
	s.handleGetProfile(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleGetProfile_ServiceError(t *testing.T) {
	svc := &mockUserServiceClient{
		getProfile: func(_ context.Context, _ *userpb.GetProfileRequest, _ ...grpc.CallOption) (*pbuser.UserProfile, error) {
			return nil, status.Error(codes.NotFound, "user not found")
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me", nil)
	r = withToken(r, "user-unknown")
	w := httptest.NewRecorder()
	s.handleGetProfile(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// =============================================================
// handleUpdateProfile
// =============================================================

func TestHandleUpdateProfile_Success(t *testing.T) {
	svc := &mockUserServiceClient{
		updateProfile: func(_ context.Context, in *userpb.UpdateProfileRequest, _ ...grpc.CallOption) (*pbuser.UserProfile, error) {
			return &pbuser.UserProfile{UserId: in.UserId, DisplayName: "Bob"}, nil
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	body, _ := json.Marshal(map[string]string{"displayName": "Bob"})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me", bytes.NewReader(body))
	r = withToken(r, "user-bob")
	w := httptest.NewRecorder()
	s.handleUpdateProfile(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateProfile_NoToken(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	body, _ := json.Marshal(map[string]string{"displayName": "Bob"})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleUpdateProfile(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleUpdateProfile_InvalidJSON(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me", strings.NewReader("not json"))
	r = withToken(r, "user-bob")
	w := httptest.NewRecorder()
	s.handleUpdateProfile(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// =============================================================
// handleListIntegrations
// =============================================================

func TestHandleListIntegrations_Success(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/integrations", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListIntegrations(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// =============================================================
// handleGetIntegration
// =============================================================

func TestHandleGetIntegration_Success(t *testing.T) {
	svc := &mockUserServiceClient{
		getIntegration: func(_ context.Context, in *userpb.GetIntegrationRequest, _ ...grpc.CallOption) (*userpb.GetIntegrationResponse, error) {
			return &userpb.GetIntegrationResponse{}, nil
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/integrations/strava", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	// manually set URL params since we're bypassing chi
	s.handleGetIntegration(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetIntegration_ServiceError(t *testing.T) {
	svc := &mockUserServiceClient{
		getIntegration: func(_ context.Context, _ *userpb.GetIntegrationRequest, _ ...grpc.CallOption) (*userpb.GetIntegrationResponse, error) {
			return nil, status.Error(codes.NotFound, "integration not found")
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/integrations/strava", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetIntegration(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// =============================================================
// handleSetIntegration
// =============================================================

func TestHandleSetIntegration_Success(t *testing.T) {
	var captured *userpb.SetIntegrationRequest
	svc := &mockUserServiceClient{
		setIntegration: func(_ context.Context, in *userpb.SetIntegrationRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			captured = in
			return &emptypb.Empty{}, nil
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	body := strings.NewReader(`{"access_token":"abc123","refresh_token":"xyz"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/integrations/strava", body)
	r = withToken(r, "user1")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "strava")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	s.handleSetIntegration(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	// Verify enrichment fields were added
	if captured == nil {
		t.Fatal("SetIntegration was not called")
	}
	data := captured.IntegrationData.AsMap()
	if data["enabled"] != true {
		t.Error("expected enabled=true in integration data")
	}
	if data["consent_given"] != true {
		t.Error("expected consent_given=true in integration data")
	}
	if _, ok := data["connected_at"]; !ok {
		t.Error("expected connected_at in integration data")
	}
}

func TestHandleSetIntegration_ApiKeyProvider(t *testing.T) {
	var createdKeyHash, createdUserID, createdLabel string
	svc := &mockUserServiceClient{}
	mockStore := &mockApiKeyStore{
		createFunc: func(_ context.Context, keyHash, userID, label string, _ []string, _ time.Time) error {
			createdKeyHash = keyHash
			createdUserID = userID
			createdLabel = label
			return nil
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	s.apiKeyStore = mockStore

	body := strings.NewReader(`{"apiKey":"test-key-123"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/integrations/hevy", body)
	r = withToken(r, "user1")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "hevy")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	s.handleSetIntegration(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify response contains ingress key
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["ingressApiKey"] == "" {
		t.Error("expected non-empty ingressApiKey in response")
	}
	if resp["ingressKeyLabel"] != "hevy-webhook" {
		t.Errorf("expected ingressKeyLabel=hevy-webhook, got %s", resp["ingressKeyLabel"])
	}
	// Verify store was called
	if createdKeyHash == "" {
		t.Error("expected CreateIngressKey to be called with non-empty hash")
	}
	if createdUserID != "user1" {
		t.Errorf("expected userID=user1, got %s", createdUserID)
	}
	if createdLabel != "hevy-webhook" {
		t.Errorf("expected label=hevy-webhook, got %s", createdLabel)
	}
}

// =============================================================
// handleDeleteIntegration
// =============================================================

func TestHandleDeleteIntegration_Success(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodDelete, "/api/v2/users/me/integrations/strava", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleDeleteIntegration(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleDeleteIntegration_ServiceError(t *testing.T) {
	svc := &mockUserServiceClient{
		deleteIntegration: func(_ context.Context, _ *userpb.DeleteIntegrationRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, status.Error(codes.Internal, "internal error")
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	r := httptest.NewRequest(http.MethodDelete, "/api/v2/users/me/integrations/strava", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleDeleteIntegration(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// =============================================================
// handleGetNotificationPrefs
// =============================================================

func TestHandleGetNotificationPrefs_Success(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/notification-prefs", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetNotificationPrefs(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// =============================================================
// handleUpdateNotificationPrefs
// =============================================================

func TestHandleUpdateNotificationPrefs_Success(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	body, _ := json.Marshal(map[string]bool{"activitySynced": true})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/notification-prefs", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdateNotificationPrefs(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateNotificationPrefs_InvalidJSON(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/notification-prefs", strings.NewReader("not json"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdateNotificationPrefs(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// =============================================================
// handleListCounters
// =============================================================

func TestHandleListCounters_Success(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/counters", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListCounters(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListCounters_ServiceError(t *testing.T) {
	svc := &mockUserServiceClient{
		listCounters: func(_ context.Context, _ *userpb.ListCountersRequest, _ ...grpc.CallOption) (*userpb.ListCountersResponse, error) {
			return nil, status.Error(codes.Internal, "db error")
		},
	}
	s := buildTestServer(svc, &mockPublisher{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/counters", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListCounters(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// =============================================================
// handleUpdateCounter
// =============================================================

func TestHandleUpdateCounter_Success(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	body, _ := json.Marshal(map[string]interface{}{"name": "streak", "value": 5})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/counters/streak", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdateCounter(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateCounter_InvalidJSON(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/counters/streak", strings.NewReader("not json"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdateCounter(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// =============================================================
// WriteError edge cases
// =============================================================

func TestWriteError_AllGRPCCodes(t *testing.T) {
	cases := []struct {
		code     codes.Code
		expected int
	}{
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.Unauthenticated, http.StatusUnauthorized},
		{codes.AlreadyExists, http.StatusConflict},
		{codes.Unimplemented, http.StatusNotImplemented},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.FailedPrecondition, http.StatusPreconditionFailed},
		{codes.ResourceExhausted, http.StatusTooManyRequests},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		WriteError(w, status.Error(c.code, "test"))
		if w.Code != c.expected {
			t.Errorf("gRPC code %v: expected HTTP %d, got %d", c.code, c.expected, w.Code)
		}
	}
}

func TestCustomError_Error(t *testing.T) {
	e := &CustomError{HTTPCode: 400, Msg: "bad"}
	if e.Error() != "bad" {
		t.Errorf("expected 'bad', got %q", e.Error())
	}
}
