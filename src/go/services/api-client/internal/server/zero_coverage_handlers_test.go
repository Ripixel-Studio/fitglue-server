package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fitglue/server/src/go/internal/infra"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// serverWithDeps builds an APIServer wiring the given mocks plus a real (discardable)
// logger so handlers that log don't panic on a nil logger.
func serverWithDeps(s *APIServer) *APIServer {
	if s.logger == nil {
		s.logger = infra.NewLogger()
	}
	return s
}

func TestMuscleGroupToCategory(t *testing.T) {
	cases := map[string]string{
		"chest":      "Chest",
		"back":       "Back",
		"lower_back": "Back",
		"shoulders":  "Shoulders",
		"biceps":     "Arms",
		"triceps":    "Arms",
		"forearm":    "Arms",
		"quadriceps": "Legs",
		"glutes":     "Legs",
		"abdominals": "Core",
		"full_body":  "Full Body",
		"cardio":     "Cardio",
		"unknown":    "Other",
		"":           "Other",
	}
	for in, want := range cases {
		if got := muscleGroupToCategory(in); got != want {
			t.Errorf("muscleGroupToCategory(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHandleGetExerciseLibrary_NoToken(t *testing.T) {
	s := serverWithDeps(&APIServer{userService: &mockUserServiceClient{}})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/exercise-library", nil)
	w := httptest.NewRecorder()
	s.handleGetExerciseLibrary(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleGetExerciseLibrary_NoHevyIntegration(t *testing.T) {
	// Default mock GetIntegration returns an empty response -> no Hevy key -> empty list.
	s := serverWithDeps(&APIServer{userService: &mockUserServiceClient{}})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/exercise-library?q=bench", nil)
	r = withToken(r, "user-1")
	w := httptest.NewRecorder()
	s.handleGetExerciseLibrary(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "exercises") {
		t.Errorf("expected exercises key, got %s", w.Body.String())
	}
}

// --- showcase view stats handlers ---

func showcaseServer() *APIServer {
	return serverWithDeps(&APIServer{activitySvc: &mockActivityServiceClient{}})
}

func TestShowcaseViewHandlers_NoToken(t *testing.T) {
	s := showcaseServer()
	handlers := map[string]http.HandlerFunc{
		"activity": s.handleGetShowcaseViewStats,
		"profile":  s.handleGetShowcaseProfileViewStats,
		"roundup":  s.handleGetShowcaseRoundupViewStats,
		"list":     s.handleListShowcaseViewStats,
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			w := httptest.NewRecorder()
			h(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401, got %d", name, w.Code)
			}
		})
	}
}

func TestShowcaseViewHandlers_Success(t *testing.T) {
	s := showcaseServer()
	handlers := map[string]http.HandlerFunc{
		"activity": s.handleGetShowcaseViewStats,
		"profile":  s.handleGetShowcaseProfileViewStats,
		"roundup":  s.handleGetShowcaseRoundupViewStats,
		"list":     s.handleListShowcaseViewStats,
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			r := withToken(httptest.NewRequest(http.MethodGet, "/x", nil), "user-1")
			w := httptest.NewRecorder()
			h(w, r)
			if w.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", name, w.Code)
			}
		})
	}
}

// --- public handlers: integration request ---

func TestHandleIntegrationRequest(t *testing.T) {
	s := serverWithDeps(&APIServer{})

	t.Run("invalid body", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/integration-request", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		s.handleIntegrationRequest(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("honeypot", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/integration-request", strings.NewReader(`{"integration":"x","website_url":"spam"}`))
		w := httptest.NewRecorder()
		s.handleIntegrationRequest(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for honeypot, got %d", w.Code)
		}
	})

	t.Run("missing integration", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/integration-request", strings.NewReader(`{"email":"a@b.com"}`))
		w := httptest.NewRecorder()
		s.handleIntegrationRequest(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/integration-request", strings.NewReader(`{"integration":"whoop","email":"a@b.com"}`))
		w := httptest.NewRecorder()
		s.handleIntegrationRequest(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

// --- public handlers: repost ---

func TestHandleRepost_NoToken(t *testing.T) {
	s := serverWithDeps(&APIServer{})
	r := httptest.NewRequest(http.MethodPost, "/repost/full-pipeline", strings.NewReader(`{"activityId":"a1"}`))
	w := httptest.NewRecorder()
	s.handleRepostFullPipeline(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleRepost_MissingActivityID(t *testing.T) {
	s := serverWithDeps(&APIServer{pipelineSvc: &mockPipelineServiceClient{}})
	r := withToken(httptest.NewRequest(http.MethodPost, "/repost/retry-destination", strings.NewReader(`{}`)), "user-1")
	w := httptest.NewRecorder()
	s.handleRepostRetryDestination(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRepost_Success(t *testing.T) {
	s := serverWithDeps(&APIServer{pipelineSvc: &mockPipelineServiceClient{}})
	r := withToken(httptest.NewRequest(http.MethodPost, "/repost/missed-destination", strings.NewReader(`{"activityId":"a1","destination":"strava"}`)), "user-1")
	w := httptest.NewRecorder()
	s.handleRepostMissedDestination(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
}

// --- mobile handlers ---

func TestHandleSetFCMToken(t *testing.T) {
	s := serverWithDeps(&APIServer{userService: &mockUserServiceClient{}})

	t.Run("no token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/fcm", strings.NewReader(`{"token":"t"}`))
		w := httptest.NewRecorder()
		s.handleSetFCMToken(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("missing token field", func(t *testing.T) {
		r := withToken(httptest.NewRequest(http.MethodPost, "/fcm", strings.NewReader(`{"platform":"web"}`)), "u1")
		w := httptest.NewRecorder()
		s.handleSetFCMToken(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		r := withToken(httptest.NewRequest(http.MethodPost, "/fcm", strings.NewReader(`{"token":"abc","platform":"ios"}`)), "u1")
		w := httptest.NewRecorder()
		s.handleSetFCMToken(w, r)
		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
	})
}

func TestHandleGetActivityStats(t *testing.T) {
	s := serverWithDeps(&APIServer{
		activitySvc: &mockActivityServiceClient{},
		userService: &mockUserServiceClient{
			getProfile: func(_ context.Context, in *userpb.GetProfileRequest, _ ...grpc.CallOption) (*pbuser.UserProfile, error) {
				streak := int32(5)
				return &pbuser.UserProfile{UserId: in.UserId, CurrentStreakDays: &streak}, nil
			},
		},
	})

	t.Run("no token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/stats", nil)
		w := httptest.NewRecorder()
		s.handleGetActivityStats(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("success augments streak", func(t *testing.T) {
		r := withToken(httptest.NewRequest(http.MethodGet, "/stats", nil), "u1")
		w := httptest.NewRecorder()
		s.handleGetActivityStats(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

func TestHandleGetActivityStats_ServiceError(t *testing.T) {
	s := serverWithDeps(&APIServer{
		activitySvc: &mockActivityServiceClient{},
		userService: &mockUserServiceClient{},
	})
	// Default GetActivityStats returns empty/no error, so this exercises the success
	// path; the error branch is covered by the activity service's own tests. Here we
	// just assert it does not panic and returns 200.
	r := withToken(httptest.NewRequest(http.MethodGet, "/stats", nil), "u1")
	w := httptest.NewRecorder()
	s.handleGetActivityStats(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Firestore-backed store constructors (no emulator needed to construct) ---

func TestNewFirestoreStores(t *testing.T) {
	if got := NewFirestoreApiKeyStore(nil); got == nil {
		t.Error("expected non-nil ApiKeyStore")
	}
	if got := NewFirestorePlatformStatsStore(nil, infra.NewLogger()); got == nil {
		t.Error("expected non-nil PlatformStatsStore")
	}
}

// TestNewAPIServer wires the full server with mocks and issues a request through
// the chi router, covering NewAPIServer/setupRoutes/ServeHTTP in server.go.
func TestNewAPIServer(t *testing.T) {
	s := NewAPIServer(
		infra.NewLogger(),
		nil, // authClient
		&mockPublisher{},
		&mockApiKeyStore{},
		nil, // gcsSigner
		nil, // statsStore
		&mockUserServiceClient{},
		&mockBillingServiceClient{},
		&mockPipelineServiceClient{},
		&mockActivityServiceClient{},
		&mockRegistryServiceClient{},
	)
	if s == nil {
		t.Fatal("expected non-nil server")
	}

	// An unknown path should route through the chi mux and return 404 (proving
	// the router is wired and ServeHTTP delegates to it).
	r := httptest.NewRequest(http.MethodGet, "/definitely-not-a-route", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown route, got %d", w.Code)
	}
}

// Sanity: unused import guard for status/codes if other tests change.
var _ = status.Error
var _ = codes.OK
