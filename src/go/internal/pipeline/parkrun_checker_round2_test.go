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
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/encoding/protojson"
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
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	code, _ := doCheck(t, c)
	if code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on list error, got %d", code)
	}
}

// A past-deadline input is turned into a manual-entry prompt, NOT closed: it stays
// WAITING (so the user can still submit), auto_populated flips to false, and
// provider_metadata.parkrun_results_state becomes EXPIRED. Exactly one PENDING_INPUT
// notification is published.
func TestParkrunChecker_ProcessInput_ExpiredByDeadline(t *testing.T) {
	var gotUpdate map[string]interface{}
	base := &mocks.MockDatabase{
		UpdatePendingInputFunc: func(ctx context.Context, userID, id string, data map[string]interface{}) error {
			gotUpdate = data
			return nil
		},
	}
	past := timestamppb.New(time.Now().Add(-time.Hour))
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId:       "i1",
				UserId:           "u1",
				LinkedActivityId: "act-1",
				AutoDeadline:     past,
			}}, nil
		},
	}
	var notifications []*pbnotification.NotificationRequest
	pub := &mocks.MockPublisher{
		PublishJSONFunc: func(ctx context.Context, topic string, data []byte) error {
			var req pbnotification.NotificationRequest
			if err := protojson.Unmarshal(data, &req); err != nil {
				t.Fatalf("notification not valid NotificationRequest JSON: %v", err)
			}
			notifications = append(notifications, &req)
			return nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), pub)
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["expired"].(float64) != 1 {
		t.Errorf("expected 1 expired, got %v", body["expired"])
	}
	if gotUpdate == nil {
		t.Fatal("expected UpdatePendingInput to be called")
	}
	if got := gotUpdate["status"]; got != int32(pbpipeline.PendingInput_STATUS_WAITING) {
		t.Errorf("expected status WAITING, got %v", got)
	}
	if got := gotUpdate["auto_populated"]; got != false {
		t.Errorf("expected auto_populated=false, got %v", got)
	}
	meta, ok := gotUpdate["provider_metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected provider_metadata nested map, got %T", gotUpdate["provider_metadata"])
	}
	if meta["parkrun_results_state"] != "EXPIRED" {
		t.Errorf("expected parkrun_results_state=EXPIRED, got %v", meta["parkrun_results_state"])
	}

	if len(notifications) != 1 {
		t.Fatalf("expected exactly 1 notification, got %d", len(notifications))
	}
	n := notifications[0]
	if n.Type != pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT {
		t.Errorf("expected PENDING_INPUT notification, got %v", n.Type)
	}
	if n.UserId != "u1" {
		t.Errorf("expected notification for u1, got %q", n.UserId)
	}
	if n.Data["activity_id"] != "act-1" {
		t.Errorf("expected activity_id=act-1 (linked activity) for deep-link, got %q", n.Data["activity_id"])
	}
	if n.Data["pending_input_id"] != "i1" {
		t.Errorf("expected pending_input_id=i1 for deep-link, got %q", n.Data["pending_input_id"])
	}
}

// Once an input is EXPIRED it stays WAITING for manual submit but must leave the
// auto-poll set: a later checker run skips it entirely — no second notification,
// no re-expire, no fetch.
func TestParkrunChecker_ProcessInput_AlreadyExpiredSkips(t *testing.T) {
	base := &mocks.MockDatabase{
		UpdatePendingInputFunc: func(ctx context.Context, userID, id string, data map[string]interface{}) error {
			t.Error("EXPIRED input must not be updated again")
			return nil
		},
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			t.Error("EXPIRED input must not trigger a user lookup / fetch")
			return parkrunUser(), nil
		},
	}
	past := timestamppb.New(time.Now().Add(-time.Hour))
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(ctx context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId:   "i1",
				UserId:       "u1",
				AutoDeadline: past, // past deadline, but already EXPIRED so it's skipped before that check
				ProviderMetadata: map[string]string{
					"parkrun_results_state": "EXPIRED",
				},
			}}, nil
		},
	}
	pub := &mocks.MockPublisher{
		PublishJSONFunc: func(ctx context.Context, topic string, data []byte) error {
			t.Error("EXPIRED input must not publish a second notification")
			return nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), pub)
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["skipped"].(float64) != 1 {
		t.Errorf("expected 1 skipped for already-EXPIRED input, got %v", body["skipped"])
	}
	if body["expired"].(float64) != 0 {
		t.Errorf("expected 0 expired for already-EXPIRED input, got %v", body["expired"])
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
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
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
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
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
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
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
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
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
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["skipped"].(float64) != 1 {
		t.Errorf("expected 1 skipped on bad date, got %v", body["skipped"])
	}
}
