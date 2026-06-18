// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockUserSvcClient implements userpb.UserServiceClient by embedding the
// interface (nil) and overriding only GetIntegration, which is the single
// method the pipeline service calls.
type mockUserSvcClient struct {
	userpb.UserServiceClient
	resp *userpb.GetIntegrationResponse
	err  error
}

func (m *mockUserSvcClient) GetIntegration(ctx context.Context, in *userpb.GetIntegrationRequest, opts ...grpc.CallOption) (*userpb.GetIntegrationResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

// --- GetPipeline gaps ---

func TestGetPipeline_NotFoundAndStoreError(t *testing.T) {
	ctx := context.Background()

	t.Run("NotFound", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		_, err := svc.GetPipeline(ctx, &pbsvc.GetPipelineRequest{UserId: "u1", PipelineId: "missing"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), getPipelineErr: errors.New("boom")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		_, err := svc.GetPipeline(ctx, &pbsvc.GetPipelineRequest{UserId: "u1", PipelineId: "p1"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})
}

// --- SubmitInput additional branches ---

func TestSubmitInput_Coverage(t *testing.T) {
	ctx := context.Background()

	t.Run("NotWaitingFailedPrecondition", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{
			ActivityId: "i1",
			Status:     pipeline.PendingInput_STATUS_COMPLETED,
		}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		_, err := svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{UserId: "u1", PendingInputId: "i1"})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})

	t.Run("MissingLinkedActivityID", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{
			ActivityId:         "i1",
			Status:             pipeline.PendingInput_STATUS_WAITING,
			OriginalPayloadUri: "gs://b/p.json",
			InputData:          nil,
		}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		_, err := svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{
			UserId:         "u1",
			PendingInputId: "i1",
			InputData:      map[string]string{"a": "b"},
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal for missing linked activity, got %v", err)
		}
	})

	t.Run("PayloadURIFallbackFromRun", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{
			ActivityId:       "i1",
			Status:           pipeline.PendingInput_STATUS_WAITING,
			LinkedActivityId: "act1",
		}
		// Run for linked activity provides the fallback OriginalPayloadUri.
		store.Runs["u1_run1"] = &pipeline.PipelineRun{
			Id:                 "run1",
			ActivityId:         "act1",
			OriginalPayloadUri: "gs://b/fallback.json",
		}
		payloadBytes, _ := json.Marshal(map[string]interface{}{"foo": "bar"})
		blob := &MockBlobStore{Blobs: map[string][]byte{"gs://b/fallback.json": payloadBytes}}
		pub := &MockPublisher{}
		svc := NewService(store, pub, blob, mockLogger{}, nil)
		_, err := svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{
			UserId:         "u1",
			PendingInputId: "i1",
			InputData:      map[string]string{"a": "b"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pub.PublishedEvents) != 1 {
			t.Fatalf("expected resume event published")
		}
		// FindPipelineRunByPendingInputId is nil here so pipelineExecutionId is not set.
		var published map[string]interface{}
		json.Unmarshal(pub.PublishedEvents[0].Data(), &published)
		if published["activityId"] != "act1" {
			t.Errorf("expected activityId act1, got %v", published["activityId"])
		}
	})

	t.Run("PayloadURIFallbackNoRunFails", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{
			ActivityId:       "i1",
			Status:           pipeline.PendingInput_STATUS_WAITING,
			LinkedActivityId: "act1",
		}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		_, err := svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{
			UserId:         "u1",
			PendingInputId: "i1",
			InputData:      map[string]string{"a": "b"},
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal when no fallback run, got %v", err)
		}
	})

	t.Run("GCSFetchError", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{
			ActivityId:         "i1",
			Status:             pipeline.PendingInput_STATUS_WAITING,
			LinkedActivityId:   "act1",
			OriginalPayloadUri: "gs://b/missing.json",
		}
		// Empty blob store → Get returns error.
		blob := &MockBlobStore{Blobs: map[string][]byte{}}
		svc := NewService(store, &MockPublisher{}, blob, mockLogger{}, nil)
		_, err := svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{
			UserId:         "u1",
			PendingInputId: "i1",
			InputData:      map[string]string{"a": "b"},
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal on GCS fetch error, got %v", err)
		}
	})

	t.Run("ReusesExecutionIDFromRun", func(t *testing.T) {
		store := NewMockStore()
		pendingID := "i1"
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{
			ActivityId:         "i1",
			Status:             pipeline.PendingInput_STATUS_WAITING,
			LinkedActivityId:   "act1",
			OriginalPayloadUri: "gs://b/p.json",
		}
		// Run linked to the pending input id → exec id reused.
		store.Runs["u1_run1"] = &pipeline.PipelineRun{
			Id:             "exec-reuse",
			PendingInputId: &pendingID,
		}
		payloadBytes, _ := json.Marshal(map[string]interface{}{"foo": "bar"})
		blob := &MockBlobStore{Blobs: map[string][]byte{"gs://b/p.json": payloadBytes}}
		pub := &MockPublisher{}
		svc := NewService(store, pub, blob, mockLogger{}, nil)
		_, err := svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{
			UserId:         "u1",
			PendingInputId: "i1",
			InputData:      map[string]string{"a": "b"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var published map[string]interface{}
		json.Unmarshal(pub.PublishedEvents[0].Data(), &published)
		if published["pipelineExecutionId"] != "exec-reuse" {
			t.Errorf("expected reused pipelineExecutionId, got %v", published["pipelineExecutionId"])
		}
	})
}

// --- ListPipelineRuns with since/until ---

func TestListPipelineRuns_WithSinceUntil(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()
	store.Runs["u1_r1"] = &pipeline.PipelineRun{Id: "r1", PipelineId: "p1"}
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)

	now := time.Now()
	resp, err := svc.ListPipelineRuns(ctx, &pbsvc.ListPipelineRunsRequest{
		UserId:     "u1",
		PipelineId: "p1",
		Limit:      10,
		Since:      timestamppb.New(now.Add(-time.Hour)),
		Until:      timestamppb.New(now),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Errorf("expected 1 run, got %d", len(resp.Runs))
	}
}

// --- AdminListPipelineRuns error path ---

func TestAdminListPipelineRuns_StoreError(t *testing.T) {
	es := &adminErrStore{MockPipelineStore: NewMockStore()}
	svc := NewService(es, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	_, err := svc.AdminListPipelineRuns(context.Background(), &pbsvc.AdminListPipelineRunsRequest{Limit: 5})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", err)
	}
}

type adminErrStore struct {
	*MockPipelineStore
}

func (a *adminErrStore) AdminListPipelineRuns(ctx context.Context, st, source, userID string, limit int32) ([]*pipeline.PipelineRun, error) {
	return nil, errors.New("admin boom")
}

// --- Cancel branches not covered elsewhere ---

func TestCancelPipeline_WithLinkedRun(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()
	pendingID := "pi1"
	store.PendingInputs["u1_pi1"] = &pipeline.PendingInput{
		ActivityId: "pi1",
		Status:     pipeline.PendingInput_STATUS_WAITING,
	}
	// A run linked to this pending input → CancelPipeline marks it CANCELLED.
	store.Runs["u1_run1"] = &pipeline.PipelineRun{Id: "run1", PendingInputId: &pendingID}
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	if _, err := svc.CancelPipeline(ctx, &pbsvc.CancelPipelineRequest{UserId: "u1", PendingInputId: "pi1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancelPipelineRun_CleansLinkedPendingInput(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()
	pendingID := "pi1"
	store.Runs["u1_r1"] = &pipeline.PipelineRun{
		Id:             "r1",
		Status:         pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING,
		PendingInputId: &pendingID,
	}
	store.PendingInputs["u1_pi1"] = &pipeline.PendingInput{
		ActivityId: "pi1",
		Status:     pipeline.PendingInput_STATUS_WAITING,
	}
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	if _, err := svc.CancelPipelineRun(ctx, &pbsvc.CancelPipelineRunRequest{UserId: "u1", RunId: "r1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := store.PendingInputs["u1_pi1"]; got.Status != pipeline.PendingInput_STATUS_COMPLETED {
		t.Errorf("expected linked pending input completed, got %v", got.Status)
	}
}

// --- ListSourceActivities deeper branches ---

func emptyIntegResp() *userpb.GetIntegrationResponse {
	return &userpb.GetIntegrationResponse{Integrations: &pbuser.UserIntegrations{}}
}

func TestListSourceActivities_Coverage(t *testing.T) {
	ctx := context.Background()

	t.Run("UnsupportedSource", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, &mockUserSvcClient{})
		_, err := svc.ListSourceActivities(ctx, &pbsvc.ListSourceActivitiesRequest{
			UserId: "u1",
			Source: "SOURCE_PARKRUN", // not in sourceplugins registry
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument for unsupported source, got %v", err)
		}
	})

	t.Run("GetIntegrationError", func(t *testing.T) {
		uc := &mockUserSvcClient{err: errors.New("integ down")}
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, uc)
		_, err := svc.ListSourceActivities(ctx, &pbsvc.ListSourceActivitiesRequest{
			UserId: "u1",
			Source: "SOURCE_STRAVA",
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal on integration error, got %v", err)
		}
	})

	t.Run("ProviderListError", func(t *testing.T) {
		// Empty integration → provider.ListActivities returns an error without network I/O.
		uc := &mockUserSvcClient{resp: emptyIntegResp()}
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, uc)
		_, err := svc.ListSourceActivities(ctx, &pbsvc.ListSourceActivitiesRequest{
			UserId: "u1",
			Source: "SOURCE_STRAVA",
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal on provider list error, got %v", err)
		}
	})
}

// --- BackfillActivities deeper branches ---

func TestBackfillActivities_Coverage(t *testing.T) {
	ctx := context.Background()

	t.Run("UnsupportedSource", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, &mockUserSvcClient{})
		_, err := svc.BackfillActivities(ctx, &pbsvc.BackfillActivitiesRequest{
			UserId:            "u1",
			Source:            "SOURCE_PARKRUN",
			SourceActivityIds: []string{"a1"},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("GetIntegrationError", func(t *testing.T) {
		uc := &mockUserSvcClient{err: errors.New("integ down")}
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, uc)
		_, err := svc.BackfillActivities(ctx, &pbsvc.BackfillActivitiesRequest{
			UserId:            "u1",
			Source:            "SOURCE_STRAVA",
			SourceActivityIds: []string{"a1"},
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("FetchActivityErrorContinuesWithZeroQueued", func(t *testing.T) {
		// Empty integration → FetchActivity errors for each id → loop continues, queued=0.
		uc := &mockUserSvcClient{resp: emptyIntegResp()}
		pub := &MockPublisher{}
		svc := NewService(NewMockStore(), pub, &MockBlobStore{}, mockLogger{}, uc)
		resp, err := svc.BackfillActivities(ctx, &pbsvc.BackfillActivitiesRequest{
			UserId:            "u1",
			Source:            "SOURCE_STRAVA",
			PipelineId:        "p1",
			SourceActivityIds: []string{"a1", "a2"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.QueuedCount != 0 {
			t.Errorf("expected 0 queued on fetch errors, got %d", resp.QueuedCount)
		}
		if len(pub.PublishedEvents) != 0 {
			t.Errorf("expected no events published, got %d", len(pub.PublishedEvents))
		}
	})
}
