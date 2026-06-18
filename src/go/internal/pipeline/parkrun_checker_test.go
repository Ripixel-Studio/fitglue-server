package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
)

func TestNewParkrunChecker(t *testing.T) {
	c := NewParkrunChecker(&mocks.MockDatabase{}, nil, infra.NewLogger())
	if c == nil {
		t.Fatal("expected checker")
	}
}

func TestParkrunChecker_HandleCheck_NoPending(t *testing.T) {
	c := NewParkrunChecker(&mocks.MockDatabase{}, nil, infra.NewLogger())
	w := httptest.NewRecorder()
	c.HandleCheck(w, httptest.NewRequest(http.MethodPost, "/pubsub/parkrun-check", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("unexpected body: %v", body)
	}
}
