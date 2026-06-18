// nolint:proto-json
package pipeline

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func cancelTestService() (*Service, *MockPipelineStore) {
	store := NewMockStore()
	return NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil), store
}

func TestCancelPipeline_Validation(t *testing.T) {
	svc, _ := cancelTestService()
	if _, err := svc.CancelPipeline(context.Background(), &pbsvc.CancelPipelineRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCancelPipeline_NotFound(t *testing.T) {
	svc, _ := cancelTestService()
	_, err := svc.CancelPipeline(context.Background(), &pbsvc.CancelPipelineRequest{UserId: "u1", PendingInputId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestCancelPipeline_WrongState(t *testing.T) {
	svc, store := cancelTestService()
	store.PendingInputs[store.key("u1", "pi1")] = &pipeline.PendingInput{
		ActivityId: "pi1",
		Status:     pipeline.PendingInput_STATUS_COMPLETED,
	}
	_, err := svc.CancelPipeline(context.Background(), &pbsvc.CancelPipelineRequest{UserId: "u1", PendingInputId: "pi1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", err)
	}
}

func TestCancelPipeline_Success(t *testing.T) {
	svc, store := cancelTestService()
	store.PendingInputs[store.key("u1", "pi1")] = &pipeline.PendingInput{
		ActivityId: "pi1",
		Status:     pipeline.PendingInput_STATUS_WAITING,
	}
	if _, err := svc.CancelPipeline(context.Background(), &pbsvc.CancelPipelineRequest{UserId: "u1", PendingInputId: "pi1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := store.PendingInputs[store.key("u1", "pi1")]; got.Status != pipeline.PendingInput_STATUS_COMPLETED {
		t.Errorf("expected pending input completed, got %v", got.Status)
	}
}

func TestCancelPipelineRun_Validation(t *testing.T) {
	svc, _ := cancelTestService()
	if _, err := svc.CancelPipelineRun(context.Background(), &pbsvc.CancelPipelineRunRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestCancelPipelineRun_NotFound(t *testing.T) {
	svc, _ := cancelTestService()
	_, err := svc.CancelPipelineRun(context.Background(), &pbsvc.CancelPipelineRunRequest{UserId: "u1", RunId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestCancelPipelineRun_WrongState(t *testing.T) {
	svc, store := cancelTestService()
	store.Runs[store.key("u1", "r1")] = &pipeline.PipelineRun{
		Id:     "r1",
		Status: pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED,
	}
	_, err := svc.CancelPipelineRun(context.Background(), &pbsvc.CancelPipelineRunRequest{UserId: "u1", RunId: "r1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", err)
	}
}

func TestCancelPipelineRun_Success(t *testing.T) {
	svc, store := cancelTestService()
	store.Runs[store.key("u1", "r1")] = &pipeline.PipelineRun{
		Id:     "r1",
		Status: pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING,
	}
	// The mock's UpdatePipelineRun is a no-op, so assert the call path succeeds
	// rather than the persisted status (the real store applies the update map).
	if _, err := svc.CancelPipelineRun(context.Background(), &pbsvc.CancelPipelineRunRequest{UserId: "u1", RunId: "r1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = store
}

func TestAdminListPipelineRuns(t *testing.T) {
	svc, store := cancelTestService()
	store.Runs[store.key("u1", "r1")] = &pipeline.PipelineRun{Id: "r1"}
	resp, err := svc.AdminListPipelineRuns(context.Background(), &pbsvc.AdminListPipelineRunsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
}

func TestListSourceActivities_Validation(t *testing.T) {
	svc, _ := cancelTestService()
	if _, err := svc.ListSourceActivities(context.Background(), &pbsvc.ListSourceActivitiesRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
	// Valid args but nil user service -> Unimplemented.
	_, err := svc.ListSourceActivities(context.Background(), &pbsvc.ListSourceActivitiesRequest{UserId: "u1", Source: "SOURCE_STRAVA"})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("expected Unimplemented, got %v", err)
	}
}

func TestBackfillActivities_Validation(t *testing.T) {
	svc, _ := cancelTestService()
	if _, err := svc.BackfillActivities(context.Background(), &pbsvc.BackfillActivitiesRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
	_, err := svc.BackfillActivities(context.Background(), &pbsvc.BackfillActivitiesRequest{UserId: "u1", Source: "SOURCE_STRAVA", SourceActivityIds: []string{"a1"}})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("expected Unimplemented, got %v", err)
	}
}
