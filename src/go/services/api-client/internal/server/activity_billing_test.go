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

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
)

// =============================================================
// Mock ActivityServiceClient
// =============================================================

type mockActivityServiceClient struct {
	listActivities            func(ctx context.Context, in *activitypb.ListActivitiesRequest, opts ...grpc.CallOption) (*activitypb.ListActivitiesResponse, error)
	getActivity               func(ctx context.Context, in *activitypb.GetActivityRequest, opts ...grpc.CallOption) (*pbactivity.StandardizedActivity, error)
	deleteActivity            func(ctx context.Context, in *activitypb.DeleteActivityRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	listShowcases             func(ctx context.Context, in *activitypb.ListShowcasesRequest, opts ...grpc.CallOption) (*activitypb.ListShowcasesResponse, error)
	getShowcase               func(ctx context.Context, in *activitypb.GetShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error)
	createShowcase            func(ctx context.Context, in *activitypb.CreateShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error)
	updateShowcase            func(ctx context.Context, in *activitypb.UpdateShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error)
	deleteShowcase            func(ctx context.Context, in *activitypb.DeleteShowcaseRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	exportData                func(ctx context.Context, in *activitypb.ExportDataRequest, opts ...grpc.CallOption) (*activitypb.ExportDataResponse, error)
	getShowcasePreferences    func(ctx context.Context, in *activitypb.GetShowcasePreferencesRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error)
	updateShowcasePreferences func(ctx context.Context, in *activitypb.UpdateShowcasePreferencesRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error)
	generateShowcaseImages    func(ctx context.Context, in *activitypb.GenerateShowcaseImagesRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

func (m *mockActivityServiceClient) GetActivity(ctx context.Context, in *activitypb.GetActivityRequest, opts ...grpc.CallOption) (*pbactivity.StandardizedActivity, error) {
	if m.getActivity != nil {
		return m.getActivity(ctx, in, opts...)
	}
	return &pbactivity.StandardizedActivity{}, nil
}
func (m *mockActivityServiceClient) ListActivities(ctx context.Context, in *activitypb.ListActivitiesRequest, opts ...grpc.CallOption) (*activitypb.ListActivitiesResponse, error) {
	if m.listActivities != nil {
		return m.listActivities(ctx, in, opts...)
	}
	return &activitypb.ListActivitiesResponse{}, nil
}
func (m *mockActivityServiceClient) DeleteActivity(ctx context.Context, in *activitypb.DeleteActivityRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.deleteActivity != nil {
		return m.deleteActivity(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockActivityServiceClient) GetShowcase(ctx context.Context, in *activitypb.GetShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	if m.getShowcase != nil {
		return m.getShowcase(ctx, in, opts...)
	}
	return &pbactivity.ShowcasedActivity{}, nil
}
func (m *mockActivityServiceClient) ListShowcases(ctx context.Context, in *activitypb.ListShowcasesRequest, opts ...grpc.CallOption) (*activitypb.ListShowcasesResponse, error) {
	if m.listShowcases != nil {
		return m.listShowcases(ctx, in, opts...)
	}
	return &activitypb.ListShowcasesResponse{}, nil
}
func (m *mockActivityServiceClient) CreateShowcase(ctx context.Context, in *activitypb.CreateShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	if m.createShowcase != nil {
		return m.createShowcase(ctx, in, opts...)
	}
	return &pbactivity.ShowcasedActivity{}, nil
}
func (m *mockActivityServiceClient) UpdateShowcase(ctx context.Context, in *activitypb.UpdateShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	if m.updateShowcase != nil {
		return m.updateShowcase(ctx, in, opts...)
	}
	return &pbactivity.ShowcasedActivity{}, nil
}
func (m *mockActivityServiceClient) DeleteShowcase(ctx context.Context, in *activitypb.DeleteShowcaseRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.deleteShowcase != nil {
		return m.deleteShowcase(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockActivityServiceClient) ExportData(ctx context.Context, in *activitypb.ExportDataRequest, opts ...grpc.CallOption) (*activitypb.ExportDataResponse, error) {
	if m.exportData != nil {
		return m.exportData(ctx, in, opts...)
	}
	return &activitypb.ExportDataResponse{}, nil
}
func (m *mockActivityServiceClient) GetExportJob(ctx context.Context, in *activitypb.GetExportJobRequest, opts ...grpc.CallOption) (*activitypb.ExportJob, error) {
	return &activitypb.ExportJob{}, nil
}
func (m *mockActivityServiceClient) ExportPipelineRun(ctx context.Context, in *activitypb.ExportPipelineRunRequest, opts ...grpc.CallOption) (*activitypb.ExportPipelineRunResponse, error) {
	return &activitypb.ExportPipelineRunResponse{}, nil
}
func (m *mockActivityServiceClient) ParseFitFile(ctx context.Context, in *activitypb.ParseFitFileRequest, opts ...grpc.CallOption) (*pbactivity.StandardizedActivity, error) {
	return &pbactivity.StandardizedActivity{}, nil
}
func (m *mockActivityServiceClient) GetShowcasePreferences(ctx context.Context, in *activitypb.GetShowcasePreferencesRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error) {
	if m.getShowcasePreferences != nil {
		return m.getShowcasePreferences(ctx, in, opts...)
	}
	return &pbactivity.ShowcaseProfile{}, nil
}
func (m *mockActivityServiceClient) UpdateShowcasePreferences(ctx context.Context, in *activitypb.UpdateShowcasePreferencesRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error) {
	if m.updateShowcasePreferences != nil {
		return m.updateShowcasePreferences(ctx, in, opts...)
	}
	return &pbactivity.ShowcaseProfile{}, nil
}
func (m *mockActivityServiceClient) GenerateShowcaseImages(ctx context.Context, in *activitypb.GenerateShowcaseImagesRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.generateShowcaseImages != nil {
		return m.generateShowcaseImages(ctx, in, opts...)
	}
	return &emptypb.Empty{}, nil
}
func (m *mockActivityServiceClient) GetPublicShowcase(ctx context.Context, in *activitypb.GetPublicShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	return &pbactivity.ShowcasedActivity{}, nil
}
func (m *mockActivityServiceClient) GetShowcaseSettings(ctx context.Context, in *activitypb.GetShowcaseSettingsRequest, opts ...grpc.CallOption) (*activitypb.GetShowcaseSettingsResponse, error) {
	return &activitypb.GetShowcaseSettingsResponse{}, nil
}
func (m *mockActivityServiceClient) UpdateShowcaseSettings(ctx context.Context, in *activitypb.UpdateShowcaseSettingsRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error) {
	return &pbactivity.ShowcaseProfile{}, nil
}
func (m *mockActivityServiceClient) UpdateShowcaseSlug(ctx context.Context, in *activitypb.UpdateShowcaseSlugRequest, opts ...grpc.CallOption) (*activitypb.UpdateShowcaseSlugResponse, error) {
	return &activitypb.UpdateShowcaseSlugResponse{}, nil
}
func (m *mockActivityServiceClient) AddShowcaseEntry(ctx context.Context, in *activitypb.AddShowcaseEntryRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockActivityServiceClient) RemoveShowcaseEntry(ctx context.Context, in *activitypb.RemoveShowcaseEntryRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockActivityServiceClient) GetShowcaseProfilePictureUploadUrl(ctx context.Context, in *activitypb.GetShowcaseProfilePictureUploadUrlRequest, opts ...grpc.CallOption) (*activitypb.GetShowcaseProfilePictureUploadUrlResponse, error) {
	return &activitypb.GetShowcaseProfilePictureUploadUrlResponse{}, nil
}
func (m *mockActivityServiceClient) GetActivityPhotoUploadUrl(ctx context.Context, in *activitypb.GetActivityPhotoUploadUrlRequest, opts ...grpc.CallOption) (*activitypb.GetActivityPhotoUploadUrlResponse, error) {
	return &activitypb.GetActivityPhotoUploadUrlResponse{}, nil
}
func (m *mockActivityServiceClient) GetPublicShowcaseProfile(ctx context.Context, in *activitypb.GetPublicShowcaseProfileRequest, opts ...grpc.CallOption) (*activitypb.GetPublicShowcaseProfileResponse, error) {
	return &activitypb.GetPublicShowcaseProfileResponse{}, nil
}
func (m *mockActivityServiceClient) GetActivityStats(ctx context.Context, in *activitypb.GetActivityStatsRequest, opts ...grpc.CallOption) (*activitypb.GetActivityStatsResponse, error) {
	return &activitypb.GetActivityStatsResponse{}, nil
}

func (m *mockActivityServiceClient) GetPublicRoundup(ctx context.Context, in *activitypb.GetPublicRoundupRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseRoundup, error) {
	return &pbactivity.ShowcaseRoundup{}, nil
}
func (m *mockActivityServiceClient) GetRecentPublicRoundups(ctx context.Context, in *activitypb.GetRecentPublicRoundupsRequest, opts ...grpc.CallOption) (*activitypb.GetRecentPublicRoundupsResponse, error) {
	return &activitypb.GetRecentPublicRoundupsResponse{}, nil
}
func (m *mockActivityServiceClient) UpdateRoundupSettings(ctx context.Context, in *activitypb.UpdateRoundupSettingsRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error) {
	return &pbactivity.ShowcaseProfile{}, nil
}

func (m *mockActivityServiceClient) RecomputeRoundup(ctx context.Context, in *activitypb.RecomputeRoundupRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseRoundup, error) {
	return &pbactivity.ShowcaseRoundup{}, nil
}

func (m *mockActivityServiceClient) RecordShowcaseView(ctx context.Context, in *activitypb.RecordShowcaseViewRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockActivityServiceClient) GetShowcaseViewStats(ctx context.Context, in *activitypb.GetShowcaseViewStatsRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseViewStats, error) {
	return &pbactivity.ShowcaseViewStats{}, nil
}
func (m *mockActivityServiceClient) ListShowcaseViewStats(ctx context.Context, in *activitypb.ListShowcaseViewStatsRequest, opts ...grpc.CallOption) (*activitypb.ListShowcaseViewStatsResponse, error) {
	return &activitypb.ListShowcaseViewStatsResponse{}, nil
}

// =============================================================
// Mock BillingServiceClient
// =============================================================

type mockBillingServiceClient struct {
	getSubscription       func(ctx context.Context, in *billingpb.GetSubscriptionRequest, opts ...grpc.CallOption) (*pbuser.SubscriptionState, error)
	createCheckoutSession func(ctx context.Context, in *billingpb.CreateCheckoutSessionRequest, opts ...grpc.CallOption) (*billingpb.CreateCheckoutSessionResponse, error)
	cancelSubscription    func(ctx context.Context, in *billingpb.CancelSubscriptionRequest, opts ...grpc.CallOption) (*pbuser.SubscriptionState, error)
	getTierStatus         func(ctx context.Context, in *billingpb.GetTierStatusRequest, opts ...grpc.CallOption) (*billingpb.GetTierStatusResponse, error)
}

func (m *mockBillingServiceClient) GetSubscription(ctx context.Context, in *billingpb.GetSubscriptionRequest, opts ...grpc.CallOption) (*pbuser.SubscriptionState, error) {
	if m.getSubscription != nil {
		return m.getSubscription(ctx, in, opts...)
	}
	return &pbuser.SubscriptionState{}, nil
}
func (m *mockBillingServiceClient) CreateCheckoutSession(ctx context.Context, in *billingpb.CreateCheckoutSessionRequest, opts ...grpc.CallOption) (*billingpb.CreateCheckoutSessionResponse, error) {
	if m.createCheckoutSession != nil {
		return m.createCheckoutSession(ctx, in, opts...)
	}
	return &billingpb.CreateCheckoutSessionResponse{SessionUrl: "https://checkout.stripe.com"}, nil
}
func (m *mockBillingServiceClient) HandleWebhookEvent(ctx context.Context, in *billingpb.HandleWebhookEventRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *mockBillingServiceClient) GetTierStatus(ctx context.Context, in *billingpb.GetTierStatusRequest, opts ...grpc.CallOption) (*billingpb.GetTierStatusResponse, error) {
	if m.getTierStatus != nil {
		return m.getTierStatus(ctx, in, opts...)
	}
	return &billingpb.GetTierStatusResponse{}, nil
}
func (m *mockBillingServiceClient) StartTrial(ctx context.Context, in *billingpb.StartTrialRequest, opts ...grpc.CallOption) (*pbuser.SubscriptionState, error) {
	return &pbuser.SubscriptionState{}, nil
}
func (m *mockBillingServiceClient) CancelSubscription(ctx context.Context, in *billingpb.CancelSubscriptionRequest, opts ...grpc.CallOption) (*pbuser.SubscriptionState, error) {
	if m.cancelSubscription != nil {
		return m.cancelSubscription(ctx, in, opts...)
	}
	return &pbuser.SubscriptionState{}, nil
}
func (m *mockBillingServiceClient) CreateBillingPortalSession(ctx context.Context, in *billingpb.CreateBillingPortalSessionRequest, opts ...grpc.CallOption) (*billingpb.CreateBillingPortalSessionResponse, error) {
	return &billingpb.CreateBillingPortalSessionResponse{Url: "https://billing.stripe.com/p/session/test"}, nil
}

// =============================================================
// Activity Handler Tests
// =============================================================

func buildActivityServer(actSvc activitypb.ActivityServiceClient) *APIServer {
	return &APIServer{
		activitySvc: actSvc,
	}
}

func TestHandleListActivities_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListActivities(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListActivities_NoToken(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities", nil)
	w := httptest.NewRecorder()
	s.handleListActivities(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleListActivities_WithLimit(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities?limit=10&page_token=abc", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListActivities(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListActivities_ServiceError(t *testing.T) {
	svc := &mockActivityServiceClient{
		listActivities: func(_ context.Context, _ *activitypb.ListActivitiesRequest, _ ...grpc.CallOption) (*activitypb.ListActivitiesResponse, error) {
			return nil, status.Error(codes.Internal, "db error")
		},
	}
	s := buildActivityServer(svc)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListActivities(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetActivity_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities/act123", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetActivity(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetActivity_NoToken(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities/act123", nil)
	w := httptest.NewRecorder()
	s.handleGetActivity(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleDeleteActivity_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodDelete, "/api/v2/users/me/activities/act123", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleDeleteActivity(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleDeleteActivity_NoToken(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodDelete, "/api/v2/users/me/activities/act123", nil)
	w := httptest.NewRecorder()
	s.handleDeleteActivity(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleListShowcases_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/showcases", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListShowcases(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetShowcase_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/showcases/sc1", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetShowcase(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleCreateShowcase_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	body, _ := json.Marshal(map[string]string{"title": "My Showcase"})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/showcases", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleCreateShowcase(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestHandleCreateShowcase_NoToken(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	body, _ := json.Marshal(map[string]string{"title": "My Showcase"})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/showcases", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleCreateShowcase(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleCreateShowcase_InvalidJSON(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/showcases", strings.NewReader("not json"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleCreateShowcase(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleUpdateShowcase_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	body, _ := json.Marshal(map[string]string{"title": "Updated"})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/showcases/sc1", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdateShowcase(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateShowcase_InvalidJSON(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/showcases/sc1", strings.NewReader("bad"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdateShowcase(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeleteShowcase_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodDelete, "/api/v2/users/me/showcases/sc1", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleDeleteShowcase(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestHandleExportData_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/export", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleExportData(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetShowcasePreferences_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/showcase-management/preferences", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetShowcasePreferences(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateShowcasePreferences_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	body, _ := json.Marshal(map[string]bool{"showGps": true})
	r := httptest.NewRequest(http.MethodPut, "/api/v2/users/me/showcase-management/preferences", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleUpdateShowcasePreferences(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGenerateShowcaseImages_Success(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/users/me/showcases/sc1/generate", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGenerateShowcaseImages(w, r)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

// =============================================================
// Billing Handler Tests
// =============================================================

func buildBillingServer(billSvc billingpb.BillingServiceClient) *APIServer {
	return &APIServer{
		billingService: billSvc,
	}
}

func TestHandleGetSubscription_Success(t *testing.T) {
	s := buildBillingServer(&mockBillingServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/billing/subscription", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetSubscription(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetSubscription_NoToken(t *testing.T) {
	s := buildBillingServer(&mockBillingServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/billing/subscription", nil)
	w := httptest.NewRecorder()
	s.handleGetSubscription(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleCreateCheckoutSession_Success(t *testing.T) {
	s := buildBillingServer(&mockBillingServiceClient{})
	body, _ := json.Marshal(map[string]string{"priceId": "price_123"})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/billing/checkout", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleCreateCheckoutSession(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleCreateCheckoutSession_InvalidJSON(t *testing.T) {
	s := buildBillingServer(&mockBillingServiceClient{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/billing/checkout", strings.NewReader("not json"))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleCreateCheckoutSession(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCancelSubscription_Success(t *testing.T) {
	s := buildBillingServer(&mockBillingServiceClient{})
	body, _ := json.Marshal(map[string]string{})
	r := httptest.NewRequest(http.MethodPost, "/api/v2/billing/cancel", bytes.NewReader(body))
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleCancelSubscription(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetTierStatus_Success(t *testing.T) {
	s := buildBillingServer(&mockBillingServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/billing/tier", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetTierStatus(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
