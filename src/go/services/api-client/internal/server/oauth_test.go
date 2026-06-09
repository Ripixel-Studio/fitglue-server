package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

// -------- Extended mock for OAuth (SetIntegration) --------
// We extend mockUserServiceClient by embedding it. Since SetIntegration
// is already implemented on mockUserServiceClient, we override via struct field.

type oauthUserClient struct {
	mockUserServiceClient
	setIntegrationErr error
}

func (m *oauthUserClient) SetIntegration(ctx context.Context, in *userpb.SetIntegrationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if m.setIntegrationErr != nil {
		return nil, m.setIntegrationErr
	}
	return &emptypb.Empty{}, nil
}

func withOAuthProvider(r *http.Request, provider string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", provider)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func firstKnownProvider() string {
	for _, p := range []string{"strava", "fitbit", "spotify", "wahoo", "polar", "oura"} {
		if GetOAuthConfig(p) != nil {
			return p
		}
	}
	return ""
}

// ---- handleOAuthConnect ----

func TestHandleOAuthConnect_MissingToken(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := withOAuthProvider(httptest.NewRequest(http.MethodPost, "/", nil), "strava")
	w := httptest.NewRecorder()
	s.handleOAuthConnect(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleOAuthConnect_UnsupportedProvider(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := withToken(
		withOAuthProvider(httptest.NewRequest(http.MethodPost, "/", nil), "not_real_provider"),
		"uid-123",
	)
	w := httptest.NewRecorder()
	s.handleOAuthConnect(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleOAuthConnect_ValidProvider(t *testing.T) {
	provider := firstKnownProvider()
	if provider == "" {
		t.Skip("no oauth providers configured")
	}
	t.Setenv("API_URL", "http://localhost:8080")
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := withToken(
		withOAuthProvider(httptest.NewRequest(http.MethodPost, "/", nil), provider),
		"uid-123",
	)
	w := httptest.NewRecorder()
	s.handleOAuthConnect(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		assert.Contains(t, resp, "url")
	}
}

// ---- handleOAuthCallback ----

func TestHandleOAuthCallback_UnsupportedProvider(t *testing.T) {
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := withOAuthProvider(httptest.NewRequest(http.MethodGet, "/", nil), "not_real")
	w := httptest.NewRecorder()
	s.handleOAuthCallback(w, r)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "error=unsupported_provider")
}

func TestHandleOAuthCallback_ErrorDescription(t *testing.T) {
	provider := firstKnownProvider()
	if provider == "" {
		t.Skip("no oauth providers configured")
	}
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := withOAuthProvider(
		httptest.NewRequest(http.MethodGet, "/?error_description=access_denied", nil),
		provider,
	)
	w := httptest.NewRecorder()
	s.handleOAuthCallback(w, r)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "error=")
}

func TestHandleOAuthCallback_MissingCodeOrState(t *testing.T) {
	provider := firstKnownProvider()
	if provider == "" {
		t.Skip("no oauth providers configured")
	}
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := withOAuthProvider(httptest.NewRequest(http.MethodGet, "/", nil), provider)
	w := httptest.NewRecorder()
	s.handleOAuthCallback(w, r)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "error=missing_code_or_state")
}

func TestHandleOAuthCallback_InvalidState_NoDot(t *testing.T) {
	provider := firstKnownProvider()
	if provider == "" {
		t.Skip("no oauth providers configured")
	}
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})
	r := withOAuthProvider(
		httptest.NewRequest(http.MethodGet, "/?code=abc&state=no_dot_in_state", nil),
		provider,
	)
	w := httptest.NewRecorder()
	s.handleOAuthCallback(w, r)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "error=invalid_state")
}

func TestHandleOAuthCallback_InvalidStateSignature(t *testing.T) {
	provider := firstKnownProvider()
	if provider == "" {
		t.Skip("no oauth providers configured")
	}
	s := buildTestServer(&mockUserServiceClient{}, &mockPublisher{})

	// Build a state with wrong HMAC secret
	payload, _ := json.Marshal(map[string]string{"uid": "u1", "ts": "1234"})
	stateB64 := base64.URLEncoding.EncodeToString(payload)
	h := hmac.New(sha256.New, []byte("wrong_secret"))
	h.Write(payload)
	badSig := hex.EncodeToString(h.Sum(nil))
	stateArg := stateB64 + "." + badSig

	r := withOAuthProvider(
		httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?code=abc&state=%s", stateArg), nil),
		provider,
	)
	w := httptest.NewRecorder()
	s.handleOAuthCallback(w, r)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "error=invalid_state_signature")
}
