package activity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newLoggerOnlyService() *Service {
	return NewService(nil, nil, nil, "bucket", "showcase-bucket", infra.NewLogger())
}

func TestParseFitFile_Validation(t *testing.T) {
	s := newLoggerOnlyService()
	ctx := context.Background()

	t.Run("missing user id", func(t *testing.T) {
		_, err := s.ParseFitFile(ctx, &pbsvc.ParseFitFileRequest{FitFileContent: []byte("x")})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("missing content", func(t *testing.T) {
		_, err := s.ParseFitFile(ctx, &pbsvc.ParseFitFileRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("invalid fit bytes", func(t *testing.T) {
		_, err := s.ParseFitFile(ctx, &pbsvc.ParseFitFileRequest{UserId: "u1", FitFileContent: []byte("not a fit file")})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for unparseable FIT, got %v", err)
		}
	})
}

func TestHandleExportTrigger_BadRequest(t *testing.T) {
	s := newLoggerOnlyService()

	t.Run("invalid json", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/export", strings.NewReader("{not json"))
		w := httptest.NewRecorder()
		s.HandleExportTrigger(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/export", strings.NewReader(`{"user_id":"u1"}`))
		w := httptest.NewRecorder()
		s.HandleExportTrigger(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing job_id, got %d", w.Code)
		}
	})
}

func TestNewFirestoreStore(t *testing.T) {
	// Construction does not touch the network; the client is only used lazily.
	if got := NewFirestoreStore(nil); got == nil {
		t.Error("expected non-nil FirestoreStore")
	}
}
