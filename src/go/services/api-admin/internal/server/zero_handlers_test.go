package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/go-chi/chi/v5"
)

// withDeleteParams attaches both the user id and data type to a single chi route
// context (calling withAdminChiParam twice would replace the context and lose the
// first param).
func withDeleteParams(r *http.Request, id, dataType string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	rctx.URLParams.Add("dataType", dataType)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestHandleAdminPipelineRuns(t *testing.T) {
	s := newAdminTestServer(&adminMockUserClient{})

	t.Run("success", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/admin/pipeline-runs", nil)
		w := httptest.NewRecorder()
		s.handleAdminPipelineRuns(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("with filters and limit", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/admin/pipeline-runs?status=FAILED&source=strava&user_id=u1&limit=10&page_token=abc", nil)
		w := httptest.NewRecorder()
		s.handleAdminPipelineRuns(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("invalid limit falls back to default", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/admin/pipeline-runs?limit=99999", nil)
		w := httptest.NewRecorder()
		s.handleAdminPipelineRuns(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

func TestHandleDeleteUserData(t *testing.T) {
	s := newAdminTestServer(&adminMockUserClient{})

	t.Run("missing params", func(t *testing.T) {
		r := withAdminChiParam(httptest.NewRequest(http.MethodDelete, "/x", nil), "id", "")
		w := httptest.NewRecorder()
		s.handleDeleteUserData(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("unknown data type", func(t *testing.T) {
		r := withDeleteParams(httptest.NewRequest(http.MethodDelete, "/x", nil), "u1", "bogus")
		w := httptest.NewRecorder()
		s.handleDeleteUserData(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("pipelines via pipeline service", func(t *testing.T) {
		r := withDeleteParams(httptest.NewRequest(http.MethodDelete, "/x", nil), "u1", "pipelines")
		w := httptest.NewRecorder()
		s.handleDeleteUserData(w, r)
		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
	})

	t.Run("pending-inputs via pipeline service", func(t *testing.T) {
		r := withDeleteParams(httptest.NewRequest(http.MethodDelete, "/x", nil), "u1", "pending-inputs")
		w := httptest.NewRecorder()
		s.handleDeleteUserData(w, r)
		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
	})
}

// deadFirestore returns a client pointed at a closed emulator address so that
// any query fails fast — enough to exercise the error branches of Firestore-backed
// handlers without a live emulator.
func deadFirestore(t *testing.T) *firestore.Client {
	t.Helper()
	t.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:1")
	c, err := firestore.NewClient(context.Background(), "test-project")
	if err != nil {
		t.Skipf("could not create firestore client: %v", err)
	}
	return c
}

func TestHandleGetStats_FirestoreError(t *testing.T) {
	s := newAdminTestServer(&adminMockUserClient{})
	s.firestoreClient = deadFirestore(t)
	defer s.firestoreClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/admin/stats", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	s.handleGetStats(w, r)
	// The users iterator fails to connect, so the handler writes an error.
	if w.Code == http.StatusOK {
		t.Errorf("expected an error status from unreachable Firestore, got %d", w.Code)
	}
}

func TestDeleteUserActivities_FirestoreError(t *testing.T) {
	s := newAdminTestServer(&adminMockUserClient{})
	s.firestoreClient = deadFirestore(t)
	defer s.firestoreClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.deleteUserActivities(ctx, "u1"); err == nil {
		t.Error("expected error from unreachable Firestore")
	}
}

var _ = infra.NewLogger
