package activity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// failingPublisher returns an error from PublishJSON to exercise enqueue-error paths.
type failingPublisher struct{ cloudEventsPublisher }

func (p *failingPublisher) PublishJSON(_ context.Context, _ string, _ []byte) error {
	return errors.New("publish failed")
}

func TestExportData_PublishError(t *testing.T) {
	var failed bool
	store := &MockActivityStore{
		UpdateExportJobFunc: func(_ context.Context, _, _ string, fields map[string]interface{}) error {
			if s, _ := fields["status"].(string); s == ExportStatusFailed {
				failed = true
			}
			return nil
		},
	}
	svc := NewService(store, &MockBlobStore{}, &failingPublisher{}, "test-bucket", "test-showcase-bucket", infra.NewLogger())
	_, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal on publish failure, got %v", err)
	}
	if !failed {
		t.Error("expected job marked FAILED after publish failure")
	}
}

// ---------------- HandleExportTrigger / runExportJob ----------------

func TestHandleExportTrigger_EnvelopeAndBadRequest(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})

	// Missing fields → bad request.
	bad, _ := json.Marshal(exportTriggerMessage{})
	req := httptest.NewRequest(http.MethodPost, "/pubsub/data-export", strings.NewReader(string(bad)))
	rec := httptest.NewRecorder()
	svc.HandleExportTrigger(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty message, got %d", rec.Code)
	}

	// Pub/Sub push envelope wrapping the trigger payload.
	inner, _ := json.Marshal(exportTriggerMessage{UserID: "u1", JobID: "j1"})
	envelope, _ := json.Marshal(struct {
		Message struct {
			Data []byte `json:"data"`
		} `json:"message"`
	}{Message: struct {
		Data []byte `json:"data"`
	}{Data: inner}})
	req2 := httptest.NewRequest(http.MethodPost, "/pubsub/data-export", strings.NewReader(string(envelope)))
	rec2 := httptest.NewRecorder()
	svc.HandleExportTrigger(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 for valid envelope, got %d", rec2.Code)
	}
}

func TestHandleExportTrigger_Worker(t *testing.T) {
	var statuses []string
	store := &MockActivityStore{
		UpdateExportJobFunc: func(_ context.Context, _, _ string, fields map[string]interface{}) error {
			if s, ok := fields["status"].(string); ok {
				statuses = append(statuses, s)
			}
			return nil
		},
	}
	blob := &MockBlobStore{}
	svc := newTestSvc(store, blob)

	msg, _ := json.Marshal(exportTriggerMessage{UserID: "u1", JobID: "j1"})
	req := httptest.NewRequest(http.MethodPost, "/pubsub/data-export", strings.NewReader(string(msg)))
	rec := httptest.NewRecorder()
	svc.HandleExportTrigger(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	// Should transition through PROCESSING and READY.
	joined := strings.Join(statuses, ",")
	if !strings.Contains(joined, ExportStatusProcessing) || !strings.Contains(joined, ExportStatusReady) {
		t.Errorf("expected PROCESSING and READY transitions, got %v", statuses)
	}
}

func TestHandleExportTrigger_BuildFails(t *testing.T) {
	var failed bool
	store := &MockActivityStore{
		ListPipelineRunsFunc: func(_ context.Context, _ string, _ int32, _ string) ([]*pbpipeline.PipelineRun, string, error) {
			return nil, "", errors.New("list error")
		},
		UpdateExportJobFunc: func(_ context.Context, _, _ string, fields map[string]interface{}) error {
			if s, _ := fields["status"].(string); s == ExportStatusFailed {
				failed = true
			}
			return nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})

	msg, _ := json.Marshal(exportTriggerMessage{UserID: "u1", JobID: "j1"})
	req := httptest.NewRequest(http.MethodPost, "/pubsub/data-export", strings.NewReader(string(msg)))
	rec := httptest.NewRecorder()
	svc.HandleExportTrigger(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (job carries error), got %d", rec.Code)
	}
	if !failed {
		t.Error("expected job to be marked FAILED when archive build fails")
	}
}

// ---------------- HandleRoundupTrigger / generateRoundup ----------------

func TestHandleRoundupTrigger_BadPeriodType(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	msg, _ := json.Marshal(roundupTriggerMessage{PeriodType: "decade"})
	req := httptest.NewRequest(http.MethodPost, "/pubsub/roundup", strings.NewReader(string(msg)))
	rec := httptest.NewRecorder()
	svc.HandleRoundupTrigger(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown period type, got %d", rec.Code)
	}
}

func TestHandleRoundupTrigger_ListUsersError(t *testing.T) {
	store := &MockActivityStore{}
	svc := newTestSvc(store, &MockBlobStore{})
	msg, _ := json.Marshal(roundupTriggerMessage{PeriodType: "week"})
	req := httptest.NewRequest(http.MethodPost, "/pubsub/roundup", strings.NewReader(string(msg)))
	rec := httptest.NewRecorder()
	// ListAllShowcaseUserIDs returns nil,nil by default → no users → 200 OK.
	svc.HandleRoundupTrigger(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with no users, got %d", rec.Code)
	}
}

func TestGenerateRoundup_FullPath(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)

	store := &MockActivityStore{
		GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{
				UserId:          "u1",
				Slug:            "athlete",
				DisplayName:     "Athlete",
				RoundupSettings: &pbactivity.RoundupSettings{EnabledWeekly: true},
			}, nil
		},
		ListShowcaseEntriesInRangeFunc: func(_ context.Context, _ string, _, _ time.Time) ([]*pbactivity.ShowcaseProfileEntry, error) {
			cal := int32(500)
			return []*pbactivity.ShowcaseProfileEntry{
				{
					ShowcaseId:      "s1",
					Title:           "Long Run",
					ActivityType:    pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
					DurationSeconds: 5400,
					DistanceMeters:  12000,
					CaloriesKcal:    &cal,
					StartTime:       timestamppb.New(start.AddDate(0, 0, 1)),
					HrZoneMinutes:   []int32{0, 10, 20, 10, 5, 5},
				},
			}, nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	roundup, err := svc.generateRoundup(ctx, "u1", pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roundup == nil {
		t.Fatal("expected a roundup to be generated")
	}
	if roundup.TotalActivities != 1 || roundup.Slug != "athlete" {
		t.Errorf("unexpected roundup aggregates: activities=%d slug=%q", roundup.TotalActivities, roundup.Slug)
	}
	if len(roundup.CalloutActivities) == 0 {
		t.Error("expected at least a BIGGEST SESSION callout")
	}
}

func TestGenerateRoundup_SettingsDisabled(t *testing.T) {
	ctx := context.Background()
	store := &MockActivityStore{
		GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
			return &pbactivity.ShowcaseProfile{
				UserId:          "u1",
				Slug:            "athlete",
				RoundupSettings: &pbactivity.RoundupSettings{EnabledWeekly: false},
			}, nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	roundup, err := svc.generateRoundup(ctx, "u1", pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, time.Now(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roundup != nil {
		t.Errorf("expected nil roundup when weekly disabled, got %+v", roundup)
	}
}

// ---------------- Export error branches ----------------

func TestGetExportJob_StoreError(t *testing.T) {
	store := &MockActivityStore{
		GetExportJobFunc: func(_ context.Context, _, _ string) (*ExportJobRecord, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.GetExportJob(context.Background(), &pbsvc.GetExportJobRequest{UserId: "u1", JobId: "j1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestExportPipelineRun_WriteError(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{Id: "r1"}, nil
		},
	}
	blob := &MockBlobStore{
		WriteFunc: func(_ context.Context, _, _ string, _ []byte) error { return errors.New("write failed") },
	}
	svc := newTestSvc(store, blob)
	_, err := svc.ExportPipelineRun(context.Background(), &pbsvc.ExportPipelineRunRequest{UserId: "u1", RunId: "r1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal on write error, got %v", err)
	}
}

func TestExportPipelineRun_SignError(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{Id: "r1"}, nil
		},
	}
	blob := &MockBlobStore{
		SignedURLFunc: func(_ context.Context, _, _, _ string, _ int64, _ time.Duration) (string, error) {
			return "", errors.New("sign failed")
		},
	}
	svc := newTestSvc(store, blob)
	_, err := svc.ExportPipelineRun(context.Background(), &pbsvc.ExportPipelineRunRequest{UserId: "u1", RunId: "r1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal on sign error, got %v", err)
	}
}

func TestHandleRoundupTrigger_MonthWithUsers(t *testing.T) {
	store := &MockActivityStore{
		ListAllShowcaseUserIDsFunc: func(_ context.Context) ([]string, error) {
			return []string{"u1", "u2"}, nil
		},
		// Users have no roundup settings → generateRoundup returns nil,nil (no-op).
	}
	svc := newTestSvc(store, &MockBlobStore{})
	msg, _ := json.Marshal(roundupTriggerMessage{PeriodType: "month"})
	req := httptest.NewRequest(http.MethodPost, "/pubsub/roundup", strings.NewReader(string(msg)))
	rec := httptest.NewRecorder()
	svc.HandleRoundupTrigger(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// ---------------- UpdateShowcaseSettings patch path (buildShowcasePatch) ----------------

func TestUpdateShowcaseSettings_PatchPath(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-showcase-update-fields", "bio,displayName"))

	var patchKeys map[string]interface{}
	store := &MockActivityStore{
		PatchShowcaseProfileFunc: func(_ context.Context, _ string, fields map[string]interface{}) (*pbactivity.ShowcaseProfile, error) {
			patchKeys = fields
			return &pbactivity.ShowcaseProfile{UserId: "u1"}, nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
		UserId:   "u1",
		Settings: &pbactivity.ShowcaseProfile{Bio: "new bio", DisplayName: "New Name"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := patchKeys["bio"]; !ok {
		t.Errorf("expected bio in patch, got keys %v", patchKeys)
	}
	if _, ok := patchKeys["display_name"]; !ok {
		t.Errorf("expected display_name in patch, got keys %v", patchKeys)
	}
}

func TestUpdateShowcaseSettings_PatchStoreError(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-showcase-update-fields", "bio"))
	store := &MockActivityStore{
		PatchShowcaseProfileFunc: func(_ context.Context, _ string, _ map[string]interface{}) (*pbactivity.ShowcaseProfile, error) {
			return nil, errors.New("patch failed")
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
		UserId:   "u1",
		Settings: &pbactivity.ShowcaseProfile{Bio: "x"},
	})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}
