package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pipelinemodelpb "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	usermodelpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
)

// ---- Billing mock ----

type adminNopBillingClient struct {
	startErr error
}

func (m *adminNopBillingClient) GetSubscription(_ context.Context, _ *billingpb.GetSubscriptionRequest, _ ...grpc.CallOption) (*usermodelpb.SubscriptionState, error) {
	return &usermodelpb.SubscriptionState{StripeCustomerId: "cus_123"}, nil
}
func (m *adminNopBillingClient) CreateCheckoutSession(_ context.Context, _ *billingpb.CreateCheckoutSessionRequest, _ ...grpc.CallOption) (*billingpb.CreateCheckoutSessionResponse, error) {
	return &billingpb.CreateCheckoutSessionResponse{}, nil
}
func (m *adminNopBillingClient) HandleWebhookEvent(_ context.Context, _ *billingpb.HandleWebhookEventRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (m *adminNopBillingClient) GetTierStatus(_ context.Context, _ *billingpb.GetTierStatusRequest, _ ...grpc.CallOption) (*billingpb.GetTierStatusResponse, error) {
	return &billingpb.GetTierStatusResponse{EffectiveTier: usermodelpb.UserTier_USER_TIER_ATHLETE, IsTrial: true}, nil
}
func (m *adminNopBillingClient) StartTrial(_ context.Context, _ *billingpb.StartTrialRequest, _ ...grpc.CallOption) (*usermodelpb.SubscriptionState, error) {
	return &usermodelpb.SubscriptionState{}, m.startErr
}
func (m *adminNopBillingClient) CancelSubscription(_ context.Context, _ *billingpb.CancelSubscriptionRequest, _ ...grpc.CallOption) (*usermodelpb.SubscriptionState, error) {
	return &usermodelpb.SubscriptionState{}, nil
}
func (m *adminNopBillingClient) CreateBillingPortalSession(_ context.Context, _ *billingpb.CreateBillingPortalSessionRequest, _ ...grpc.CallOption) (*billingpb.CreateBillingPortalSessionResponse, error) {
	return &billingpb.CreateBillingPortalSessionResponse{Url: "https://portal.example/x"}, nil
}

func newAdminTestServerWithBilling(b billingpb.BillingServiceClient) *APIServer {
	s := newAdminTestServer(&adminMockUserClient{getResp: &usermodelpb.UserProfile{UserId: "u1"}})
	s.billingSvc = b
	return s
}

// withAdminActor attaches an authenticated admin token to the request context.
func withAdminActor(r *http.Request, uid, email string) *http.Request {
	token := &auth.Token{UID: uid, Claims: map[string]interface{}{"email": email}}
	return r.WithContext(context.WithValue(r.Context(), userContextKey, token))
}

// withTwoParams attaches two chi URL params to a single request context.
func withTwoParams(r *http.Request, k1, v1, k2, v2 string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(k1, v1)
	rctx.URLParams.Add(k2, v2)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// ---- RBAC / actor helpers ----

func TestIsSelf(t *testing.T) {
	s := newAdminTestServer(&adminMockUserClient{})
	r := withAdminActor(httptest.NewRequest(http.MethodGet, "/", nil), "admin1", "a@b.com")
	assert.True(t, s.isSelf(r, "admin1"))
	assert.False(t, s.isSelf(r, "someone-else"))
	assert.False(t, s.isSelf(httptest.NewRequest(http.MethodGet, "/", nil), "admin1"))
}

func TestAdminActor(t *testing.T) {
	r := withAdminActor(httptest.NewRequest(http.MethodGet, "/", nil), "u9", "x@y.com")
	uid, email := adminActor(r)
	assert.Equal(t, "u9", uid)
	assert.Equal(t, "x@y.com", email)

	uid2, email2 := adminActor(httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Empty(t, uid2)
	assert.Empty(t, email2)
}

// ---- UpdateUser: extended fields + self-protection ----

func TestUpdateUser_AppliesTierAndAdmin(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &usermodelpb.UserProfile{UserId: "u1"}})
	body := bytes.NewBufferString(`{"tier":"USER_TIER_ATHLETE","isAdmin":true,"displayName":"Ada"}`)
	req := withAdminChiParam(httptest.NewRequest(http.MethodPut, "/", body), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateUser_InvalidTier(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &usermodelpb.UserProfile{UserId: "u1"}})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"tier":"BOGUS"}`)), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateUser_BlocksSelfDemotion(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &usermodelpb.UserProfile{UserId: "admin1"}})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"isAdmin":false}`)), "id", "admin1")
	req = withAdminActor(req, "admin1", "a@b.com")
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "own admin role")
}

func TestUpdateUser_BlocksSelfAccessRevoke(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &usermodelpb.UserProfile{UserId: "admin1"}})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"accessEnabled":false}`)), "id", "admin1")
	req = withAdminActor(req, "admin1", "a@b.com")
	w := httptest.NewRecorder()
	svc.handleUpdateUser(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---- User actions ----

func TestSendPasswordReset_NoEmail(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &usermodelpb.UserProfile{UserId: "u1"}})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPost, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleSendPasswordReset(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSendPasswordReset_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{getResp: &usermodelpb.UserProfile{UserId: "u1", Email: "u@x.com"}})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPost, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleSendPasswordReset(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSendVerificationEmail_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPost, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleSendVerificationEmail(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestSetIntegrationEnabled_InvalidProvider(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withTwoParams(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"enabled":true}`)),
		"id", "u1", "provider", "notreal",
	)
	w := httptest.NewRecorder()
	svc.handleSetIntegrationEnabled(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteIntegration_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withTwoParams(httptest.NewRequest(http.MethodDelete, "/", nil), "id", "u1", "provider", "strava")
	w := httptest.NewRecorder()
	svc.handleDeleteIntegration(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestValidIntegrationProvider(t *testing.T) {
	assert.True(t, validIntegrationProvider("strava"))
	assert.True(t, validIntegrationProvider("hevy"))
	assert.False(t, validIntegrationProvider("definitely-not-a-provider"))
}

// ---- Pipeline ops ----

func TestPipelineOps_MissingIDs(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	cases := []func(http.ResponseWriter, *http.Request){
		svc.handleGetPipelineRun, svc.handleRepostActivity,
		svc.handleCancelPipelineRun, svc.handleResolvePendingInput,
	}
	for _, h := range cases {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodPost, "/", nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	}
}

func TestRepostActivity_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withTwoParams(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"mode":"retry-destination","destination":"STRAVA"}`)),
		"id", "u1", "activityId", "a1",
	)
	w := httptest.NewRecorder()
	svc.handleRepostActivity(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestListPendingInputs_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withAdminChiParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleListPendingInputs(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetPipelineRun_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withTwoParams(httptest.NewRequest(http.MethodGet, "/", nil), "id", "u1", "runId", "r1")
	w := httptest.NewRecorder()
	svc.handleGetPipelineRun(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCancelPipelineRun_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withTwoParams(httptest.NewRequest(http.MethodPost, "/", nil), "id", "u1", "runId", "r1")
	w := httptest.NewRecorder()
	svc.handleCancelPipelineRun(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestResolvePendingInput_Success(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withTwoParams(httptest.NewRequest(http.MethodPost, "/", nil), "id", "u1", "inputId", "i1")
	w := httptest.NewRecorder()
	svc.handleResolvePendingInput(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestRepostActivity_BadBody(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	req := withTwoParams(httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not json")), "id", "u1", "activityId", "a1")
	w := httptest.NewRecorder()
	svc.handleRepostActivity(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSetIntegrationEnabled_BadBody(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{})
	// Valid provider but malformed JSON body — exercises the decode-error branch
	// before any Firestore write.
	req := withTwoParams(httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not json")), "id", "u1", "provider", "strava")
	w := httptest.NewRecorder()
	svc.handleSetIntegrationEnabled(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCancelSubscription_Success(t *testing.T) {
	svc := newAdminTestServerWithBilling(&adminNopBillingClient{})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPost, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleCancelSubscription(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestBoolString(t *testing.T) {
	assert.Equal(t, "true", boolString(true))
	assert.Equal(t, "false", boolString(false))
}

func TestPipelineSummaries(t *testing.T) {
	cfgs := []*pipelinemodelpb.PipelineConfig{
		{Id: "p1", Name: "My Pipe", Sources: []string{"SOURCE_STRAVA"}, Disabled: false},
		{Id: "p2", Name: "Off Pipe", Source: "SOURCE_HEVY", Disabled: true},
	}
	out := pipelineSummaries(cfgs)
	assert.Len(t, out, 2)
	assert.Equal(t, "p1", out[0].GetId())
	assert.Equal(t, "SOURCE_STRAVA", out[0].GetSource()) // falls back to sources[0]
	assert.True(t, out[0].GetEnabled())
	assert.False(t, out[1].GetEnabled()) // disabled → not enabled
}

func TestPendingInputSummaries(t *testing.T) {
	inputs := []*pipelinemodelpb.PendingInput{
		{ActivityId: "a1", EnricherProviderId: "weather", Status: pipelinemodelpb.PendingInput_STATUS_WAITING},
	}
	out := pendingInputSummaries(inputs)
	assert.Len(t, out, 1)
	assert.Equal(t, "a1", out[0].GetActivityId())
	assert.Equal(t, "weather", out[0].GetEnricherProviderId())
}

// ---- Billing ----

func TestGetUserBilling_Success(t *testing.T) {
	svc := newAdminTestServerWithBilling(&adminNopBillingClient{})
	req := withAdminChiParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleGetUserBilling(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "cus_123")
}

func TestStartTrial_Success(t *testing.T) {
	svc := newAdminTestServerWithBilling(&adminNopBillingClient{})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPost, "/", nil), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleStartTrial(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestCreateBillingPortal_Success(t *testing.T) {
	svc := newAdminTestServerWithBilling(&adminNopBillingClient{})
	req := withAdminChiParam(httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"returnUrl":"https://x"}`)), "id", "u1")
	w := httptest.NewRecorder()
	svc.handleCreateBillingPortal(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "portal.example")
}

// ---- Aggregation helpers ----

func TestIntegrationSummaries(t *testing.T) {
	past := timestamppb.New(time.Now().Add(-time.Hour))
	ui := &usermodelpb.UserIntegrations{
		Strava: &usermodelpb.StravaIntegration{Enabled: true, ExpiresAt: past},
		Hevy:   &usermodelpb.HevyIntegration{Enabled: true},
	}
	summaries := integrationSummaries(ui)
	assert.Len(t, summaries, 2)

	health := map[string]string{}
	enabled := map[string]bool{}
	for _, s := range summaries {
		health[s.GetProvider()] = s.GetTokenHealth()
		enabled[s.GetProvider()] = s.GetEnabled()
	}
	assert.True(t, enabled["strava"])
	assert.Equal(t, "expired", health["strava"])
	assert.Equal(t, "n/a", health["hevy"]) // no expiring token
}

func TestIntegrationSummaries_Nil(t *testing.T) {
	assert.Empty(t, integrationSummaries(nil))
}

func TestTokenHealth(t *testing.T) {
	assert.Equal(t, "n/a", tokenHealth(nil))
	assert.Equal(t, "expired", tokenHealth(timestamppb.New(time.Now().Add(-time.Minute))))
	assert.Equal(t, "valid", tokenHealth(timestamppb.New(time.Now().Add(time.Hour))))
}

// ---- Audit helpers ----

func TestAuditEntryFromDoc(t *testing.T) {
	entry := auditEntryFromDoc("id1", map[string]interface{}{
		"actor_uid":      "admin1",
		"actor_email":    "a@b.com",
		"action":         "update_user",
		"target_user_id": "u1",
		"result":         "ok",
		"params":         map[string]interface{}{"tier": "USER_TIER_ATHLETE"},
		"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
	})
	assert.Equal(t, "id1", entry.GetId())
	assert.Equal(t, "admin1", entry.GetActorUid())
	assert.Equal(t, "update_user", entry.GetAction())
	assert.Equal(t, "USER_TIER_ATHLETE", entry.GetParams()["tier"])
	assert.NotNil(t, entry.GetTimestamp())
}

func TestAuditAction_NilFirestoreIsNoop(t *testing.T) {
	svc := newAdminTestServer(&adminMockUserClient{}) // firestoreClient is nil
	assert.NotPanics(t, func() {
		svc.auditAction(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), "x", "u1", nil, nil)
	})
}
