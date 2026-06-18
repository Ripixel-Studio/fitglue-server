// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// parkrunDB wraps mocks.MockDatabase to override ListPendingInputsByEnricher,
// which has no override hook on the base mock. Everything else delegates to the
// embedded MockDatabase (GetUserFunc / UpdatePendingInputFunc etc.).
type parkrunDB struct {
	*mocks.MockDatabase
	listFn func(ctx context.Context, enricherID string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error)
}

func (p *parkrunDB) ListPendingInputsByEnricher(ctx context.Context, enricherID string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
	if p.listFn != nil {
		return p.listFn(ctx, enricherID, status)
	}
	return nil, nil
}

func doCheck(t *testing.T, c *ParkrunChecker) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	c.HandleCheck(w, httptest.NewRequest(http.MethodPost, "/pubsub/parkrun-check", nil))
	var body map[string]interface{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &body)
	}
	return w.Code, body
}

func TestParkrunChecker_HandleCheck_ListError(t *testing.T) {
	db := &parkrunDB{
		MockDatabase: &mocks.MockDatabase{},
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return nil, errors.New("firestore down")
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger())
	code, _ := doCheck(t, c)
	if code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on list error, got %d", code)
	}
}

func TestParkrunChecker_ProcessInput_ExpiredByDeadline(t *testing.T) {
	updated := false
	base := &mocks.MockDatabase{
		UpdatePendingInputFunc: func(ctx context.Context, userID, id string, data map[string]interface{}) error {
			updated = true
			return nil
		},
	}
	past := timestamppb.New(time.Now().Add(-time.Hour))
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId:   "i1",
				UserId:       "u1",
				AutoDeadline: past,
			}}, nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger())
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["expired"].(float64) != 1 {
		t.Errorf("expected 1 expired, got %v", body["expired"])
	}
	if !updated {
		t.Error("expected UpdatePendingInput to mark input completed")
	}
}

func TestParkrunChecker_ProcessInput_DeadlineUpdateErrorStillExpires(t *testing.T) {
	base := &mocks.MockDatabase{
		UpdatePendingInputFunc: func(ctx context.Context, userID, id string, data map[string]interface{}) error {
			return errors.New("write failed")
		},
	}
	past := timestamppb.New(time.Now().Add(-time.Hour))
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{ActivityId: "i1", UserId: "u1", AutoDeadline: past}}, nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger())
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["expired"].(float64) != 1 {
		t.Errorf("expected expired even when update errors, got %v", body["expired"])
	}
}

func TestParkrunChecker_ProcessInput_GetUserError(t *testing.T) {
	base := &mocks.MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return nil, errors.New("user lookup failed")
		},
	}
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{ActivityId: "i1", UserId: "u1"}}, nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger())
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["skipped"].(float64) != 1 {
		t.Errorf("expected 1 skipped on get-user error, got %v", body["skipped"])
	}
}

func TestParkrunChecker_ProcessInput_NoIntegrationExpires(t *testing.T) {
	expired := false
	base := &mocks.MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			// Integrations nil → no parkrun → expire immediately.
			return &user.Record{}, nil
		},
		UpdatePendingInputFunc: func(ctx context.Context, userID, id string, data map[string]interface{}) error {
			expired = true
			return nil
		},
	}
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{ActivityId: "i1", UserId: "u1"}}, nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger())
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["expired"].(float64) != 1 {
		t.Errorf("expected 1 expired on missing integration, got %v", body["expired"])
	}
	if !expired {
		t.Error("expected pending input to be expired")
	}
}

func parkrunUser() *user.Record {
	return &user.Record{
		Integrations: &pbuser.UserIntegrations{
			Parkrun: &pbuser.ParkrunIntegration{
				Enabled:    true,
				AthleteId:  "12345",
				CountryUrl: "parkrun.org.uk",
			},
		},
	}
}

func TestParkrunChecker_ProcessInput_MissingMetadataSkips(t *testing.T) {
	base := &mocks.MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) { return parkrunUser(), nil },
	}
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId:       "i1",
				UserId:           "u1",
				ProviderMetadata: map[string]string{}, // no slug/date
			}}, nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger())
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["skipped"].(float64) != 1 {
		t.Errorf("expected 1 skipped on missing metadata, got %v", body["skipped"])
	}
}

func TestParkrunChecker_ProcessInput_BadDateSkips(t *testing.T) {
	base := &mocks.MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) { return parkrunUser(), nil },
	}
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId: "i1",
				UserId:     "u1",
				ProviderMetadata: map[string]string{
					"parkrun_event_slug": "bushy",
					"expected_date":      "not-a-date",
				},
			}}, nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger())
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["skipped"].(float64) != 1 {
		t.Errorf("expected 1 skipped on bad date, got %v", body["skipped"])
	}
}
