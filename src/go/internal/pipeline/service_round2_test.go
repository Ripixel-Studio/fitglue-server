// nolint:proto-json
package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- ResolvePendingInput branches ---

func TestResolvePendingInput_Coverage(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		if _, err := svc.ResolvePendingInput(ctx, &pbsvc.ResolvePendingInputRequest{}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		_, err := svc.ResolvePendingInput(ctx, &pbsvc.ResolvePendingInputRequest{UserId: "u1", PendingInputId: "missing"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs[store.key("u1", "pi1")] = &pipeline.PendingInput{
			ActivityId: "pi1",
			Status:     pipeline.PendingInput_STATUS_WAITING,
		}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		if _, err := svc.ResolvePendingInput(ctx, &pbsvc.ResolvePendingInputRequest{UserId: "u1", PendingInputId: "pi1"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := store.PendingInputs[store.key("u1", "pi1")]; got.Status != pipeline.PendingInput_STATUS_COMPLETED {
			t.Errorf("expected completed, got %v", got.Status)
		}
	})

	t.Run("UpdateError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), updatePendingErr: errors.New("write boom")}
		es.PendingInputs[es.key("u1", "pi1")] = &pipeline.PendingInput{
			ActivityId: "pi1",
			Status:     pipeline.PendingInput_STATUS_WAITING,
		}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
		_, err := svc.ResolvePendingInput(ctx, &pbsvc.ResolvePendingInputRequest{UserId: "u1", PendingInputId: "pi1"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal on update error, got %v", err)
		}
	})
}

// --- CancelPipeline store-error branches ---

// cancelGetErrStore returns an error from GetPendingInput.
type cancelGetErrStore struct {
	*MockPipelineStore
}

func (c *cancelGetErrStore) GetPendingInput(ctx context.Context, userID, inputID string) (*pipeline.PendingInput, error) {
	return nil, errors.New("get pending boom")
}

func TestCancelPipeline_GetPendingError(t *testing.T) {
	svc := NewService(&cancelGetErrStore{MockPipelineStore: NewMockStore()}, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	_, err := svc.CancelPipeline(context.Background(), &pbsvc.CancelPipelineRequest{UserId: "u1", PendingInputId: "pi1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", err)
	}
}

// cancelFindErrStore has a WAITING pending input but errors on
// FindPipelineRunByPendingInputId.
type cancelFindErrStore struct {
	*MockPipelineStore
}

func (c *cancelFindErrStore) FindPipelineRunByPendingInputId(ctx context.Context, userID, pendingInputID string) (*pipeline.PipelineRun, error) {
	return nil, errors.New("find run boom")
}

func TestCancelPipeline_FindRunError(t *testing.T) {
	store := NewMockStore()
	store.PendingInputs[store.key("u1", "pi1")] = &pipeline.PendingInput{
		ActivityId: "pi1",
		Status:     pipeline.PendingInput_STATUS_WAITING,
	}
	svc := NewService(&cancelFindErrStore{MockPipelineStore: store}, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	_, err := svc.CancelPipeline(context.Background(), &pbsvc.CancelPipelineRequest{UserId: "u1", PendingInputId: "pi1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", err)
	}
}

func TestCancelPipeline_UpdatePendingError(t *testing.T) {
	es := &ErrorStore{MockPipelineStore: NewMockStore(), updatePendingErr: errors.New("update boom")}
	es.PendingInputs[es.key("u1", "pi1")] = &pipeline.PendingInput{
		ActivityId: "pi1",
		Status:     pipeline.PendingInput_STATUS_WAITING,
	}
	svc := NewService(es, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	_, err := svc.CancelPipeline(context.Background(), &pbsvc.CancelPipelineRequest{UserId: "u1", PendingInputId: "pi1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal on update pending error, got %v", err)
	}
}

// --- CancelPipelineRun store-error branch ---

func TestCancelPipelineRun_GetError(t *testing.T) {
	es := &ErrorStore{MockPipelineStore: NewMockStore(), getPipelineRunErr: errors.New("get run boom")}
	svc := NewService(es, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	_, err := svc.CancelPipelineRun(context.Background(), &pbsvc.CancelPipelineRunRequest{UserId: "u1", RunId: "r1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", err)
	}
}
